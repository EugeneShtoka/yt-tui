package config

// This file holds the concurrency-safe accessors used to cross goroutine
// boundaries. A live *Config is shared between the TUI's Update loop (which may
// overwrite fields wholesale on a mid-session profile import) and background
// readers — the single-binary enrichment loop and the download worker. All
// three touch the embedded DaemonConfig fields, so every cross-goroutine write
// goes through Mutate and every cross-goroutine read through DaemonSnapshot;
// mu (an RWMutex) establishes the happens-before that keeps them race-free.

// Mutate runs fn while holding the write lock, so a wholesale field rewrite
// (e.g. applying an imported config profile) is atomic with respect to the
// background readers that take a DaemonSnapshot. fn must not call back into
// any method that re-acquires mu (Save, SaveAsync, DaemonSnapshot) or it will
// deadlock — do the field writes directly on the *Config passed alongside.
func (c *Config) Mutate(fn func()) {
	c.mu.Lock()
	defer c.mu.Unlock()
	fn()
}

// DaemonSnapshot returns a consistent value copy of the daemon-side settings,
// taken under the read lock. Background readers (enrichment loop, download
// worker) call this once and read fields off the copy, so a concurrent Mutate
// can never tear a slice header or interleave a half-applied profile. The copy
// is shallow: its SubtitleLangs / SponsorBlockCats slice headers are read
// atomically here, and Mutate only ever *replaces* those slices (never mutates
// their elements), so a held snapshot keeps pointing at its own backing array.
func (c *Config) DaemonSnapshot() DaemonConfig {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.DaemonConfig
}
