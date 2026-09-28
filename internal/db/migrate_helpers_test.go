package db

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
