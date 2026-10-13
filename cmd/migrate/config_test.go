package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManagedMigrationConfigurationIsBoundedAndStrict(t *testing.T) {
	path := filepath.Join(t.TempDir(), "migration.json")
	valid := `{"MIGRATION_DATABASE_URL":"postgres://fixture.invalid/test","ROLE":"fixture_sources"}`
	for _, body := range []string{`{}`, `{"MIGRATION_DATABASE_URL":3,"ROLE":"x"}`, valid + `{}`, strings.Replace(valid, `"ROLE"`, `"unknown"`, 1), strings.Repeat(" ", 65537) + valid, valid + strings.Repeat(" ", 65537) + `{}`} {
		if err := os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := readMigrationConfig(path); err == nil {
			t.Fatal("invalid managed migration configuration accepted")
		}
	}
	if err := os.WriteFile(path, []byte(valid), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := readMigrationConfig(path)
	if err != nil || c.Role != "fixture_sources" || c.DatabaseURL != "postgres://fixture.invalid/test" {
		t.Fatal("valid configuration rejected")
	}
}
