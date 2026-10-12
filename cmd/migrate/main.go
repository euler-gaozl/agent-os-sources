package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/euler-gaozl/agent-os-sources/sources"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"time"
)

func main() {
	role := flag.String("role", "agentos_sources", "existing application database role")
	flag.Parse()
	dsn := os.Getenv("MIGRATION_DATABASE_URL")
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
func fail() {
	fmt.Fprintln(os.Stderr, "sources migration failed; verify migration connection, existing role, permissions and lock availability")
	os.Exit(1)
}
