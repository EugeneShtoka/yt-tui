package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
)

// migrationsFS holds the ordered, embedded schema migrations. Each file is
// migrations/NNNN_description.sql; the NNNN prefix is the target schema version.
//
//go:embed migrations/*.sql
var migrationsFS embed.FS

// migration is one parsed, embedded migration file.
type migration struct {
	version int    // target user_version, parsed from the NNNN filename prefix
	name    string // file name, for error context
	sql     string // the DDL body
}

// loadMigrations parses every embedded migration file and returns them ordered
// by version. It enforces a gap-free sequence starting at 1 (version N is the
// N-th file), so a database's user_version equals the number of migrations
// applied and adding a migration out of order fails loudly at startup rather
// than silently skipping.
func loadMigrations() ([]migration, error) {
	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return nil, fmt.Errorf("loadMigrations: %w", err)
	}
	var ms []migration
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		prefix, _, ok := strings.Cut(e.Name(), "_")
		if !ok {
			return nil, fmt.Errorf("loadMigrations: bad migration name %q (want NNNN_description.sql)", e.Name())
		}
		v, err := strconv.Atoi(prefix)
		if err != nil {
			return nil, fmt.Errorf("loadMigrations: bad version prefix in %q: %w", e.Name(), err)
		}
		body, err := migrationsFS.ReadFile("migrations/" + e.Name())
		if err != nil {
			return nil, fmt.Errorf("loadMigrations: read %q: %w", e.Name(), err)
		}
		ms = append(ms, migration{version: v, name: e.Name(), sql: string(body)})
	}
	sort.Slice(ms, func(i, j int) bool { return ms[i].version < ms[j].version })
	for i, m := range ms {
		if m.version != i+1 {
			return nil, fmt.Errorf("loadMigrations: migrations must be a gap-free sequence from 1; got version %d at position %d (%s)", m.version, i+1, m.name)
		}
	}
	return ms, nil
}

// latestSchemaVersion is the highest embedded migration version — the schema
// version a fully-migrated database ends up at.
func latestSchemaVersion() (int, error) {
	ms, err := loadMigrations()
	if err != nil {
		return 0, err
	}
	if len(ms) == 0 {
		return 0, nil
	}
	return ms[len(ms)-1].version, nil
}

// migrate applies every embedded migration whose version exceeds the database's
// current PRAGMA user_version, each in its own transaction that also stamps the
// new user_version. Applying and stamping together means a crash mid-migration
// rolls back cleanly and the migration re-runs from the same version on the next
// start — never leaving the schema half-applied.
func (d *DB) migrate() error {
	ctx := context.Background()
	ms, err := loadMigrations()
	if err != nil {
		return err
	}
	current, err := d.SchemaVersion(ctx)
	if err != nil {
		return err
	}
	for _, m := range ms {
		if m.version <= current {
			continue
		}
		if err := d.applyMigration(ctx, m); err != nil {
			return err
		}
	}
	return nil
}

// applyMigration runs one migration's DDL and stamps user_version to its version
// atomically. The user_version value is interpolated (PRAGMA can't be
// parameterized) from a trusted int parsed off the embedded filename.
func (d *DB) applyMigration(ctx context.Context, m migration) error {
	return d.withTx(ctx, "migrate "+m.name, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, m.sql); err != nil {
			return fmt.Errorf("migrate %s: %w", m.name, err)
		}
		if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", m.version)); err != nil {
			return fmt.Errorf("migrate %s stamp: %w", m.name, err)
		}
		return nil
	})
}

// SchemaVersion reads the schema revision stamped in the SQLite user_version
// pragma (0 on a fresh database, before any migration has run).
func (d *DB) SchemaVersion(ctx context.Context) (int, error) {
	var v int
	if err := d.sql.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&v); err != nil {
		return 0, fmt.Errorf("SchemaVersion: %w", err)
	}
	return v, nil
}
