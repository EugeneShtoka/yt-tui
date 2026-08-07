package videotable

// ColumnKeys returns the stable key of each column, in order. It is the identity
// used by per-panel column configuration (Phase 22): keys are order-independent
// and rename-safe, exactly like the panel-name refs in config.TabKeys.
func ColumnKeys[T any](cols []ColumnDef[T]) []string {
	keys := make([]string, len(cols))
	for i := range cols {
		keys[i] = cols[i].Col.Key()
	}
	return keys
}

// SelectColumns returns the subset of all whose keys appear in wantKeys,
// reordered to match wantKeys' order. It powers per-panel configurable columns:
//
//   - An empty wantKeys (the default) returns all columns unchanged, so a panel
//     with no configured selection keeps its full, natural-order column set.
//   - A present list both filters (only listed keys survive) and reorders (list
//     order wins).
//   - Keys in wantKeys not present in all are skipped (validated + warned about
//     at startup; see app.ValidateColumns).
//   - A selection that resolves to no columns at all falls back to the full set
//     rather than an empty table.
func SelectColumns[T any](all []ColumnDef[T], wantKeys []string) []ColumnDef[T] {
	if len(wantKeys) == 0 {
		return all
	}
	byKey := make(map[string]ColumnDef[T], len(all))
	for _, c := range all {
		byKey[c.Col.Key()] = c
	}
	out := make([]ColumnDef[T], 0, len(wantKeys))
	for _, k := range wantKeys {
		if c, ok := byKey[k]; ok {
			out = append(out, c)
		}
	}
	if len(out) == 0 {
		return all
	}
	return out
}
