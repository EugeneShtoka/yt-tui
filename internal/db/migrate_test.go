package db

import (
	"context"
	"testing"
)

// TestLoadMigrationsSequence locks in the gap-free-from-1 invariant: every
// embedded migration's version equals its position, so a database's
// user_version is exactly the count of migrations applied.
func TestLoadMigrationsSequence(t *testing.T) {
	ms, err := loadMigrations()
	if err != nil {
		t.Fatalf("loadMigrations: %v", err)
	}
	if len(ms) == 0 {
		t.Fatal("loadMigrations: no migrations embedded")
	}
	for i, m := range ms {
		if m.version != i+1 {
			t.Errorf("migration %q: version %d, want %d", m.name, m.version, i+1)
		}
	}
}

// TestMigrateIsIdempotent verifies re-running migrate() on an already-migrated
// database is a no-op: every migration is skipped (version <= user_version) and
// the version is unchanged. A second migrate() would fail loudly if it re-ran
// the baseline (plain CREATE against existing tables).
func TestMigrateIsIdempotent(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	before, err := db.SchemaVersion(ctx)
	if err != nil {
		t.Fatalf("SchemaVersion: %v", err)
	}
	if err = db.migrate(); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	after, err := db.SchemaVersion(ctx)
	if err != nil {
		t.Fatalf("SchemaVersion: %v", err)
	}
	if before != after {
		t.Errorf("user_version changed on re-migrate: %d -> %d", before, after)
	}
	want, err := latestSchemaVersion()
	if err != nil {
		t.Fatalf("latestSchemaVersion: %v", err)
	}
	if after != want {
		t.Errorf("user_version = %d, want %d", after, want)
	}
}

// TestBaselineCreatesCoreTables spot-checks that applying migrations from a
// fresh database produced the expected schema (a table the app relies on).
func TestBaselineCreatesCoreTables(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	for _, tbl := range []string{"videos", "collections", "collection_videos", "subscribed_channels", "meta"} {
		var name string
		err := db.sql.QueryRowContext(ctx,
			`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, tbl).Scan(&name)
		if err != nil {
			t.Errorf("table %q not found after migrate: %v", tbl, err)
		}
	}
}
