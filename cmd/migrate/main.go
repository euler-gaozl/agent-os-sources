package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/euler-gaozl/agent-os-sources/sources"
	"github.com/jackc/pgx/v5/pgxpool"
	"io"
	"os"
	"time"
)

func main() {
	role := flag.String("role", "agentos_sources", "existing application database role")
	configPath := flag.String("config", "", "protected migration configuration for managed process recovery")
	flag.Parse()
	dsn := os.Getenv("MIGRATION_DATABASE_URL")
	if *configPath != "" {
		explicitRole := false
		flag.Visit(func(f *flag.Flag) { explicitRole = explicitRole || f.Name == "role" })
		if explicitRole || dsn != "" {
			fail()
		}
		config, err := readMigrationConfig(*configPath)
		if err != nil {
			fail()
		}
		dsn, *role = config.DatabaseURL, config.Role
	}
	config, err := pgxpool.ParseConfig(dsn)
	if dsn == "" || err != nil {
		fail()
	}
	config.MaxConns = 2
	config.ConnConfig.ConnectTimeout = 5 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	db, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		fail()
	}
	defer db.Close()
	if err = sources.Migrate(ctx, db, *role); err != nil {
		fail()
	}
	fmt.Println("sources migration committed")
}

type migrationConfig struct {
	DatabaseURL string `json:"MIGRATION_DATABASE_URL"`
	Role        string `json:"ROLE"`
}

func readMigrationConfig(path string) (migrationConfig, error) {
	var out migrationConfig
	f, err := os.Open(path)
	if err != nil {
		return out, fmt.Errorf("migration configuration unavailable")
	}
	defer f.Close()
	body, err := io.ReadAll(io.LimitReader(f, 65537))
	if err != nil || len(body) > 65536 {
		return out, fmt.Errorf("invalid migration configuration")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&out); err != nil || decoder.Decode(new(any)) != io.EOF || out.DatabaseURL == "" || out.Role == "" {
		return migrationConfig{}, fmt.Errorf("invalid migration configuration")
	}
	return out, nil
}
func fail() {
	fmt.Fprintln(os.Stderr, "sources migration failed; verify migration connection, existing role, permissions and lock availability")
	os.Exit(1)
}
