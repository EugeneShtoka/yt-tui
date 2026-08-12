package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/adrg/xdg"
)

// withXDGDirs points the XDG base homes at three distinct temp dirs for the
// duration of a test, restoring the originals afterwards. Returns the base
// homes so the test can assert against them. Not safe for t.Parallel (it
// mutates package-global xdg state).
func withXDGDirs(t *testing.T) (configHome, dataHome, stateHome string) {
	t.Helper()
	root := t.TempDir()
	configHome = filepath.Join(root, "config")
	dataHome = filepath.Join(root, "data")
	stateHome = filepath.Join(root, "state")

	origC, origD, origS := xdg.ConfigHome, xdg.DataHome, xdg.StateHome
	xdg.ConfigHome, xdg.DataHome, xdg.StateHome = configHome, dataHome, stateHome
	t.Cleanup(func() {
		xdg.ConfigHome, xdg.DataHome, xdg.StateHome = origC, origD, origS
	})
	return configHome, dataHome, stateHome
}

func TestResolveAppDirsSeparatesXDGBases(t *testing.T) {
	configHome, dataHome, stateHome := withXDGDirs(t)

	dirs, err := resolveAppDirs()
	if err != nil {
		t.Fatalf("resolveAppDirs: %v", err)
	}

	cases := []struct {
		name, got, wantBase string
	}{
		{"Config", dirs.Config, configHome},
		{"Data", dirs.Data, dataHome},
		{"State", dirs.State, stateHome},
	}
	for _, c := range cases {
		want := filepath.Join(c.wantBase, appName)
		if c.got != want {
			t.Errorf("%s dir = %q, want %q", c.name, c.got, want)
		}
		if info, statErr := os.Stat(c.got); statErr != nil || !info.IsDir() {
			t.Errorf("%s dir %q was not created: err=%v", c.name, c.got, statErr)
		}
	}
}

func TestMigrateLegacyFilesMovesDBAndLog(t *testing.T) {
	dirs := mkDirs(t)
	// Legacy layout: everything under the config dir, including WAL/SHM sidecars.
	writeFile(t, filepath.Join(dirs.Config, dbFileName), "db")
	writeFile(t, filepath.Join(dirs.Config, dbFileName+"-wal"), "wal")
	writeFile(t, filepath.Join(dirs.Config, dbFileName+"-shm"), "shm")
	writeFile(t, filepath.Join(dirs.Config, logFileName), "log")

	migrateLegacyFiles(dirs)

	// DB + sidecars land in the data dir; log lands in the state dir.
	assertFile(t, filepath.Join(dirs.Data, dbFileName), "db")
	assertFile(t, filepath.Join(dirs.Data, dbFileName+"-wal"), "wal")
	assertFile(t, filepath.Join(dirs.Data, dbFileName+"-shm"), "shm")
	assertFile(t, filepath.Join(dirs.State, logFileName), "log")

	// The originals are gone from the config dir.
	for _, gone := range []string{dbFileName, dbFileName + "-wal", dbFileName + "-shm", logFileName} {
		if _, err := os.Stat(filepath.Join(dirs.Config, gone)); !os.IsNotExist(err) {
			t.Errorf("legacy %s still present in config dir (err=%v)", gone, err)
		}
	}
}

func TestMigrateLegacyFilesSkipsWhenDstExists(t *testing.T) {
	dirs := mkDirs(t)
	writeFile(t, filepath.Join(dirs.Config, dbFileName), "legacy")
	writeFile(t, filepath.Join(dirs.Data, dbFileName), "current") // already migrated

	migrateLegacyFiles(dirs)

	// The in-use DB must not be clobbered by a stale legacy copy...
	assertFile(t, filepath.Join(dirs.Data, dbFileName), "current")
	// ...and the legacy copy is left untouched (not moved onto the newer file).
	assertFile(t, filepath.Join(dirs.Config, dbFileName), "legacy")
}

func TestMigrateLegacyFilesNoopWhenSrcAbsent(t *testing.T) {
	dirs := mkDirs(t)

	migrateLegacyFiles(dirs) // nothing to move — must not error or fabricate files

	if _, err := os.Stat(filepath.Join(dirs.Data, dbFileName)); !os.IsNotExist(err) {
		t.Errorf("db fabricated in data dir with no source (err=%v)", err)
	}
	if _, err := os.Stat(filepath.Join(dirs.State, logFileName)); !os.IsNotExist(err) {
		t.Errorf("log fabricated in state dir with no source (err=%v)", err)
	}
}

func TestLoadUsesXDGDirs(t *testing.T) {
	configHome, dataHome, stateHome := withXDGDirs(t)
	// Pin DownloadDir into the temp tree so Load doesn't touch the real $HOME.
	dl := filepath.Join(t.TempDir(), "downloads")
	writeFile(t, filepath.Join(configHome, appName, "config.toml"),
		"download_dir = "+`"`+dl+`"`)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	defer cfg.Close()

	if want := filepath.Join(configHome, appName); cfg.ConfigDir != want {
		t.Errorf("ConfigDir = %q, want %q", cfg.ConfigDir, want)
	}
	if want := filepath.Join(dataHome, appName); cfg.DataDir != want {
		t.Errorf("DataDir = %q, want %q", cfg.DataDir, want)
	}
	if want := filepath.Join(stateHome, appName); cfg.StateDir != want {
		t.Errorf("StateDir = %q, want %q", cfg.StateDir, want)
	}
	if want := filepath.Join(stateHome, appName, logFileName); cfg.LogPath() != want {
		t.Errorf("LogPath() = %q, want %q", cfg.LogPath(), want)
	}
	// config.toml lives in the config dir, not the data/state dirs.
	if _, statErr := os.Stat(filepath.Join(configHome, appName, "config.toml")); statErr != nil {
		t.Errorf("config.toml not written to config dir: %v", statErr)
	}
}

func TestLoadFromOverridesConfigPath(t *testing.T) {
	configHome, dataHome, _ := withXDGDirs(t)
	// A custom config living entirely outside the XDG config home.
	custom := filepath.Join(t.TempDir(), "profiles", "work.toml")
	dl := filepath.Join(t.TempDir(), "downloads")
	writeFile(t, custom, "download_dir = "+`"`+dl+`"`)

	cfg, err := LoadFrom(custom)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	defer cfg.Close()

	if cfg.ConfigFile != custom {
		t.Errorf("ConfigFile = %q, want %q", cfg.ConfigFile, custom)
	}
	if want := filepath.Dir(custom); cfg.ConfigDir != want {
		t.Errorf("ConfigDir = %q, want %q", cfg.ConfigDir, want)
	}
	// With no data_dir override, the DB still lives in the XDG data home.
	if want := filepath.Join(dataHome, appName); cfg.DataDir != want {
		t.Errorf("DataDir = %q, want %q", cfg.DataDir, want)
	}
	// The override is honored on save: nothing is written to the XDG config home.
	if _, statErr := os.Stat(filepath.Join(configHome, appName, "config.toml")); !os.IsNotExist(statErr) {
		t.Errorf("config.toml leaked into XDG config home (err=%v)", statErr)
	}
	if _, statErr := os.Stat(custom); statErr != nil {
		t.Errorf("config not written to override path: %v", statErr)
	}
}

func TestLoadFromEnvVarOverridesConfigPath(t *testing.T) {
	withXDGDirs(t)
	custom := filepath.Join(t.TempDir(), "env.toml")
	dl := filepath.Join(t.TempDir(), "downloads")
	writeFile(t, custom, "download_dir = "+`"`+dl+`"`)
	t.Setenv(envConfigPath, custom)

	// Empty arg falls back to $YT_TUI_CONFIG.
	cfg, err := LoadFrom("")
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	defer cfg.Close()

	if cfg.ConfigFile != custom {
		t.Errorf("ConfigFile = %q, want %q (from %s)", cfg.ConfigFile, custom, envConfigPath)
	}
}

func TestLoadFromArgBeatsEnvVar(t *testing.T) {
	withXDGDirs(t)
	dl := filepath.Join(t.TempDir(), "downloads")
	argPath := filepath.Join(t.TempDir(), "arg.toml")
	envPathVal := filepath.Join(t.TempDir(), "env.toml")
	writeFile(t, argPath, "download_dir = "+`"`+dl+`"`)
	writeFile(t, envPathVal, "download_dir = "+`"`+dl+`"`)
	t.Setenv(envConfigPath, envPathVal)

	cfg, err := LoadFrom(argPath)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	defer cfg.Close()

	if cfg.ConfigFile != argPath {
		t.Errorf("ConfigFile = %q, want the --config arg %q", cfg.ConfigFile, argPath)
	}
}

func TestLoadDataDirOverrideRelocatesStores(t *testing.T) {
	_, dataHome, _ := withXDGDirs(t)
	dataOverride := filepath.Join(t.TempDir(), "mydata")
	dl := filepath.Join(t.TempDir(), "downloads")
	writeFile(t, filepath.Join(xdg.ConfigHome, appName, "config.toml"),
		"data_dir = "+`"`+dataOverride+`"`+"\ndownload_dir = "+`"`+dl+`"`)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	defer cfg.Close()

	if cfg.DataDir != dataOverride {
		t.Errorf("DataDir = %q, want override %q", cfg.DataDir, dataOverride)
	}
	if info, statErr := os.Stat(dataOverride); statErr != nil || !info.IsDir() {
		t.Errorf("override data dir %q not created: err=%v", dataOverride, statErr)
	}
	// DataDir-derived stores follow the override, not the XDG data home.
	if want := filepath.Join(dataOverride, "thumbnails"); cfg.ThumbnailsPath() != want {
		t.Errorf("ThumbnailsPath() = %q, want %q", cfg.ThumbnailsPath(), want)
	}
	if want := filepath.Join(dataOverride, "profiles"); cfg.ProfilesPath() != want {
		t.Errorf("ProfilesPath() = %q, want %q", cfg.ProfilesPath(), want)
	}
	if xdgData := filepath.Join(dataHome, appName); cfg.DataDir == xdgData {
		t.Errorf("DataDir should not be the XDG default %q", xdgData)
	}
}

// --- helpers ---

func mkDirs(t *testing.T) appDirs {
	t.Helper()
	root := t.TempDir()
	dirs := appDirs{
		Config: filepath.Join(root, "config"),
		Data:   filepath.Join(root, "data"),
		State:  filepath.Join(root, "state"),
	}
	for _, d := range []string{dirs.Config, dirs.Data, dirs.State} {
		if err := os.MkdirAll(d, 0750); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}
	return dirs
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		t.Fatalf("mkdir for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func assertFile(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(got) != want {
		t.Errorf("%s = %q, want %q", path, got, want)
	}
}
