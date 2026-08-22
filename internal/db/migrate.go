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
	// Databases predating this runner carry a hand-stamped user_version (2) that
	// has no relation to the migration sequence, so every migration up to it
	// would be skipped and their drifted schema frozen forever. Their schema is
	// baseline-equivalent, so treat them as version 1 and let the reconciling
	// migrations (0002 onward) run.
	if current > 0 {
		legacy, err := d.hasLegacySchema(ctx)
		if err != nil {
			return err
		}
		if legacy {
			current = 1
		}
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

// legacySchemaMarkers are schema artifacts that only a pre-runner database can
// carry: each one is removed by 0002_reconcile_legacy_schema, so the check below
// goes false once reconciled and can never misfire on a database this runner
// built itself.
var legacySchemaMarkers = []struct{ table, column string }{
	{"feed_cache", "position"},
	{"local_videos", "last_position_ms"},
}

// hasLegacySchema reports whether the database was created before the migration
// runner existed, and therefore carries a hand-stamped user_version that must
// not be trusted as a migration count.
func (d *DB) hasLegacySchema(ctx context.Context) (bool, error) {
	// blocked_names was dropped when channel blocking went ID-only; 0001 never
	// creates it.
	var n int
	if err := d.sql.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='blocked_names'`,
	).Scan(&n); err != nil {
		return false, fmt.Errorf("hasLegacySchema blocked_names: %w", err)
	}
	if n > 0 {
		return true, nil
	}
	for _, m := range legacySchemaMarkers {
		has, err := d.hasColumn(ctx, m.table, m.column)
		if err != nil {
			return false, err
		}
		if has {
			return true, nil
		}
	}
	return false, nil
}

// hasColumn reports whether table declares a column of the given name. A table
// that does not exist simply has no columns, so a missing table reads as false.
func (d *DB) hasColumn(ctx context.Context, table, column string) (bool, error) {
	var n int
	if err := d.sql.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?`, table, column,
	).Scan(&n); err != nil {
		return false, fmt.Errorf("hasColumn %s.%s: %w", table, column, err)
	}
	return n > 0, nil
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
