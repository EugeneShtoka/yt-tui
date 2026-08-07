// Command yt-tui is the keyboard-driven terminal UI for YouTube. It talks to an
// in-process backend by default, or a remote yt-tuid daemon when --connect (or
// a configured daemon address) is set.
package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/EugeneShtoka/yt-tui/internal/api"
	"github.com/EugeneShtoka/yt-tui/internal/backend/thumbs"
	"github.com/EugeneShtoka/yt-tui/internal/backend/transcache"
	"github.com/EugeneShtoka/yt-tui/internal/buildinfo"
	"github.com/EugeneShtoka/yt-tui/internal/config"
	"github.com/EugeneShtoka/yt-tui/internal/db"
	"github.com/EugeneShtoka/yt-tui/internal/debug"
	"github.com/EugeneShtoka/yt-tui/internal/device/player"
	"github.com/EugeneShtoka/yt-tui/internal/downloader"
	"github.com/EugeneShtoka/yt-tui/internal/prewarm"
	"github.com/EugeneShtoka/yt-tui/internal/theme"
	"github.com/EugeneShtoka/yt-tui/internal/tui/app"
	"github.com/EugeneShtoka/yt-tui/internal/tui/render"
	"github.com/EugeneShtoka/yt-tui/internal/tui/styles"
	"github.com/EugeneShtoka/yt-tui/internal/youtube"
	"github.com/charmbracelet/x/term"
)

func main() {
	// The hidden tracker subcommand runs headless (no TUI) and outlives the
	// parent process, saving resume position while a video keeps playing after
	// the user quits. Dispatch here, before any TUI setup.
	runFn := run
	if len(os.Args) > 1 && os.Args[1] == trackSubcommand {
		runFn = func() error { return runPositionTracker(os.Args[2:]) }
	}
	if err := runFn(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

const (
	// trackSubcommand is the hidden first-arg that runs the headless post-quit
	// position tracker instead of the TUI (see runPositionTracker).
	trackSubcommand = "__track-position"
	// trackInterval paces the tracker's MPRIS position polls.
	trackInterval = 5 * time.Second
	// trackerMaxLifetime caps how long a detached tracker can run, so a wedged
	// player process can't keep it alive indefinitely.
	trackerMaxLifetime = 12 * time.Hour
)

func run() error {
	debugFlag := flag.Bool("debug", false, "write debug log to $XDG_STATE_HOME/yt-tui/debug.log (~/.local/state/yt-tui/debug.log)")
	connectAddr := flag.String("connect", "", "dial a remote yt-tuid daemon (e.g. http://localhost:7373)")
	configPath := flag.String("config", "", "path to config file (overrides $YT_TUI_CONFIG and the default ~/.config/yt-tui/config.toml)")
	versionFlag := flag.Bool("version", false, "print version information and exit")
	flag.Parse()

	if *versionFlag {
		fmt.Println(buildinfo.String("yt-tui"))
		return nil
	}

	cfg, err := config.LoadFrom(*configPath)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	defer cfg.Close()

	if *debugFlag {
		if initDebugLog(cfg) {
			defer debug.Close()
		}
	}

	render.SetDurFmt(render.DurFmt(cfg.DurationFormat))
	render.SetDateFmt(render.DateFmt(cfg.DateFormat))

	// Collect every non-fatal startup problem — config-validation issues from
	// Load plus the runtime warnings that used to vanish to stderr (theme, TLS,
	// player, YouTube availability) — and surface them together in a dismissible
	// overlay rather than losing them behind the alt-screen.
	issues := append([]config.ConfigIssue(nil), cfg.Issues...)
	issues = append(issues, app.ValidateColumns(cfg.Panels, cfg.Columns)...)
	issues = append(issues, initTheme(cfg)...)

	backend, media, closeBackend, backendIssues, err := buildBackend(cfg, *connectAddr)
	if err != nil {
		return err
	}
	defer closeBackend()
	issues = append(issues, backendIssues...)

	pl, playerErr := player.New(cfg)
	if playerErr != nil {
		issues = append(issues, config.ConfigIssue{Severity: config.SeverityError, Message: "player: " + playerErr.Error()})
	} else {
		defer pl.Close()
	}

	if av, perr := backend.CheckAvailability(context.Background()); perr == nil {
		issues = append(issues, av...)
	} else {
		debug.Log("availability probe failed: %v", perr)
	}

	return runTUI(backend, media, cfg, pl, issues, *connectAddr, *configPath)
}

// runTUI runs the Bubble Tea program to completion, then hands a still-playing
// video off to the background position tracker.
func runTUI(backend api.Backend, media api.MediaProvider, cfg *config.Config, pl player.Backend, issues []config.ConfigIssue, connectAddr, configPath string) error {
	// Bubble Tea v2 pushes the Kitty keyboard protocol onto the terminal's
	// stack (key disambiguation) on capable terminals like WezTerm. If teardown
	// is skipped — a panic, a kill, or an incomplete restore on exit — the
	// terminal stays in that mode and the user's shell can no longer read arrow
	// keys (broken history navigation). Always pop the keyboard stack on the way
	// out; popping an already-clean stack is a no-op per the Kitty spec.
	defer restoreKeyboard()

	// App-lifetime context canceled at teardown so in-flight backend commands
	// aren't orphaned when the program exits (H-1). p.Run returns only on quit.
	appCtx, cancelApp := context.WithCancel(context.Background())
	defer cancelApp()

	maybeStartPrewarm(appCtx, backend, media, cfg, connectAddr)

	p := tea.NewProgram(app.New(appCtx, backend, media, cfg, pl, issues))
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("run: %w", err)
	}

	handoffToTracker(pl, cfg, connectAddr, configPath)
	return nil
}

// maybeStartPrewarm launches the eager thumbnail pre-warm in remote mode when
// the client keeps a local cache and local_prewarm is enabled. It walks the
// daemon's eligible set through the media seam on a bounded worker pool, filling
// the local cache so thumbnails render instantly and offline. Fire-and-forget
// and tied to ctx (canceled at quit): a best-effort, read-only pass with nothing
// to join — local-store writes are atomic file ops, safe to abandon on exit.
func maybeStartPrewarm(ctx context.Context, backend api.Backend, media api.MediaProvider, cfg *config.Config, connectAddr string) {
	remote := connectAddr != "" || cfg.DaemonAddr != ""
	if !remote || !cfg.LocalThumbnails || cfg.LocalPrewarm != "eager" {
		return
	}
	go prewarm.New(backend, media, cfg.LocalPrewarmConcurrency).Run(ctx)
}

// handoffToTracker hands a still-playing video to a detached position tracker
// after the TUI exits. The player process survives quitting (intentional), and
// the deferred pl.Close()/closeBackend() only stop *our* polling and release the
// DB handle — they don't kill the player. Remote mode is skipped: the player is
// client-side but the DB lives on the daemon, so a local tracker has nothing to
// write to.
func handoffToTracker(pl player.Backend, cfg *config.Config, connectAddr, configPath string) {
	remote := connectAddr != "" || cfg.DaemonAddr != ""
	if pl == nil || remote {
		return
	}
	if info, ok := pl.Active(); ok {
		spawnPositionTracker(info, configPath)
	}
}

// spawnPositionTracker re-execs this binary as a detached, headless position
// tracker (trackSubcommand) for the still-playing video, then returns without
// waiting so it outlives us. Best-effort: a spawn failure just means resume
// position stops updating at quit, as it did before.
func spawnPositionTracker(info player.ActivePlayback, configPath string) {
	exe, err := os.Executable()
	if err != nil {
		debug.Log("spawnPositionTracker: os.Executable: %v", err)
		return
	}
	args := []string{trackSubcommand}
	if configPath != "" {
		args = append(args, "-config", configPath)
	}
	args = append(args, info.VideoID, strconv.Itoa(info.PID))

	cmd := exec.Command(exe, args...)                    //nolint:gosec,noctx // exe is our own path; the tracker must NOT be tied to our context — it outlives us by design
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} // detach from our process group
	if null, oerr := os.Open(os.DevNull); oerr == nil {
		cmd.Stdin, cmd.Stdout, cmd.Stderr = null, null, null
		defer null.Close()
	}
	if serr := cmd.Start(); serr != nil {
		debug.Log("spawnPositionTracker: start: %v", serr)
		return
	}
	debug.Log("spawnPositionTracker: tracking %s (pid %d)", info.VideoID, info.PID)
	// Deliberately no Wait: the tracker must outlive this process.
}

// runPositionTracker is the hidden trackSubcommand entry point. It opens its own
// DB handle and polls the player's MPRIS position, saving it until the player
// exits (or a bounded lifetime elapses). It runs in a process detached from the
// TUI, so it keeps resume position current while a video plays on after quit.
func runPositionTracker(args []string) error {
	fs := flag.NewFlagSet(trackSubcommand, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	cfgPath := fs.String("config", "", "path to config file")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("track-position: %w", err)
	}
	rest := fs.Args()
	if len(rest) != 2 {
		return fmt.Errorf("track-position: usage: %s [-config path] <videoID> <pid>", trackSubcommand)
	}
	videoID := rest[0]
	pid, err := strconv.Atoi(rest[1])
	if err != nil {
		return fmt.Errorf("track-position: bad pid %q: %w", rest[1], err)
	}

	cfg, err := config.LoadFrom(*cfgPath)
	if err != nil {
		return fmt.Errorf("track-position: config: %w", err)
	}
	defer cfg.Close()

	database, err := db.New(cfg.DataDir, cfg.StripEmojis, cfg.RecommendedMaxAgeDays)
	if err != nil {
		return fmt.Errorf("track-position: db: %w", err)
	}
	defer func() { _ = database.Close() }()

	// Bound the lifetime and honor a shutdown signal so the detached process
	// can't linger forever.
	ctx, cancel := context.WithTimeout(context.Background(), trackerMaxLifetime)
	defer cancel()
	sigCtx, stop := signal.NotifyContext(ctx, syscall.SIGTERM, os.Interrupt)
	defer stop()

	if err := player.Track(sigCtx, pid, trackInterval, func(ms int64) {
		if serr := database.SaveVideoPosition(sigCtx, videoID, ms); serr != nil {
			debug.Log("track-position: save %s @ %dms: %v", videoID, ms, serr)
		}
	}); err != nil {
		return fmt.Errorf("track-position: %w", err)
	}
	return nil
}

// restoreKeyboard pops one entry off the terminal's Kitty keyboard-protocol
// stack, undoing the enhancement Bubble Tea requests at startup. It only writes
// when stdout is a real terminal, so redirected output stays clean.
func restoreKeyboard() {
	if term.IsTerminal(os.Stdout.Fd()) {
		// Best-effort escape write on teardown; a failed stdout write is nothing we
		// can act on here.
		_, _ = fmt.Fprint(os.Stdout, "\x1b[<u")
	}
}

// initDebugLog opens the debug log under the XDG state dir
// (~/.local/state/yt-tui/debug.log). It returns true when the log was opened
// (and thus needs closing by the caller). On failure it warns and returns false.
func initDebugLog(cfg *config.Config) bool {
	logPath := cfg.LogPath()
	if initErr := debug.Init(logPath); initErr != nil {
		fmt.Fprintf(os.Stderr, "debug log: %v\n", initErr)
		return false
	}
	fmt.Fprintf(os.Stderr, "debug log: %s\n", logPath)
	return true
}

// initTheme writes a sample theme.toml if missing, then loads the user's theme
// if configured, applying it via styles.Init. A load failure is non-fatal: the
// built-in defaults stand and the problem is returned as an issue so it surfaces
// in the startup overlay instead of being swallowed to stderr.
func initTheme(cfg *config.Config) []config.ConfigIssue {
	// Write a sample theme.toml to the config dir if it doesn't exist yet.
	_ = theme.WriteDefault(filepath.Join(cfg.ConfigDir, "theme.toml"))

	if cfg.Theme == "" {
		return nil
	}
	themeFile := cfg.Theme
	if !filepath.IsAbs(themeFile) {
		themeFile = filepath.Join(cfg.ConfigDir, themeFile)
	}
	t, loadErr := theme.Load(themeFile)
	if loadErr != nil {
		return []config.ConfigIssue{{
			Severity: config.SeverityError,
			Message:  "theme " + cfg.Theme + " failed to load (" + loadErr.Error() + "); using the default theme",
		}}
	}
	styles.Init(t)
	return nil
}

// buildBackend returns a remote backend when a daemon address is configured
// (via the --connect flag or config), otherwise an in-process backend. The
// returned cleanup func must be deferred by the caller; it closes the local
// database when one was opened and is a no-op for the remote backend. Non-fatal
// setup problems (unreadable TLS CA, a profile that couldn't be applied) come
// back as issues for the startup overlay.
func buildBackend(cfg *config.Config, connectAddr string) (api.Backend, api.MediaProvider, func(), []config.ConfigIssue, error) {
	addr := connectAddr
	if addr == "" {
		addr = cfg.DaemonAddr
	}

	if addr != "" {
		client, issues := buildHTTPClient(cfg)
		remote := api.NewRemote(addr, cfg.DaemonToken, client)
		// Apply a daemon-stored named profile over the local config before the
		// TUI reads keybindings/panels. A failure (not fatal) keeps the client
		// usable on the local config if the daemon or profile is absent.
		if err := app.LoadProfileOnConnect(context.Background(), remote, cfg, cfg.LoadProfile); err != nil {
			issues = append(issues, config.ConfigIssue{Severity: config.SeverityError, Message: "load profile " + cfg.LoadProfile + ": " + err.Error()})
		}
		// Media seam. The remote client keeps its own caches only when opted in
		// (local_thumbnails / local_transcripts). backendServes comes from a
		// capability probe: when the daemon serves thumbnails the client routes
		// through it (the daemon stays the single network boundary); when it doesn't,
		// the client fetches the CDN itself. A probe failure defaults to true — the
		// conservative choice that keeps egress on the daemon.
		localThumbs, localTranscripts, lissues := buildLocalCaches(cfg, cfg.LocalThumbnails, cfg.LocalTranscripts)
		issues = append(issues, lissues...)
		backendServes := true
		if caps, cerr := remote.Capabilities(context.Background()); cerr == nil {
			backendServes = caps.ThumbnailsEnabled
		} else {
			debug.Log("buildBackend: capabilities probe failed, assuming daemon serves thumbnails: %v", cerr)
		}
		media := api.NewMediaProvider(remote, localThumbs, localTranscripts, backendServes)
		return remote, media, func() {}, issues, nil
	}

	database, err := db.New(cfg.DataDir, cfg.StripEmojis, cfg.RecommendedMaxAgeDays)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("db: %w", err)
	}
	dl := downloader.New(cfg, database)
	ytClient := youtube.NewClient(cfg)
	inproc := api.NewInProc(database, ytClient, dl, cfg)
	// Single-binary mode: run the enrichment pass in-process (there is no daemon
	// doing it). It's resumable and side-effect-safe, so tying it to the process
	// lifetime is enough — it simply pauses when the TUI exits. Give it a
	// cancellable context and tear down in order on quit: cancel the background
	// pass and stop in-flight yt-dlp children *before* closing the DB, so a live
	// goroutine can't write to a closed SQLite handle (use-after-close race).
	ctx, cancel := context.WithCancel(context.Background())
	inproc.StartBackgroundEnrichment(ctx)
	// Single-binary mode is purely local: there is no remote server cache. The
	// client-local caches keep every thumbnail/transcript you view (in their own
	// directories, so the enrichment Retain sweeps can't evict them), on top of
	// whatever the in-process backend already has. backendServes follows whether
	// the backend's own thumbnail store came up — if it didn't, this machine
	// fetches the CDN directly.
	localThumbs, localTranscripts, lissues := buildLocalCaches(cfg, true, true)
	media := api.NewMediaProvider(inproc, localThumbs, localTranscripts, inproc.ThumbnailsEnabled())
	return inproc, media, func() {
		cancel()
		inproc.WaitEnrichment()
		dl.Stop()
		_ = database.Close()
	}, lissues, nil
}

// buildLocalCaches opens the client-local media caches. thumbsOn/transcriptsOn
// gate each: single-binary passes both true (it is purely local); remote follows
// config. Each store is bounded to its newest-N cap at startup. A store that
// can't be created is non-fatal — it comes back as a warning and a nil store
// (that cache simply off).
func buildLocalCaches(cfg *config.Config, thumbsOn, transcriptsOn bool) (*thumbs.Store, *transcache.Store, []config.ConfigIssue) {
	var (
		lt     *thumbs.Store
		tr     *transcache.Store
		issues []config.ConfigIssue
	)
	if thumbsOn {
		if store, err := thumbs.NewStore(cfg.LocalThumbnailsPath()); err != nil {
			issues = append(issues, config.ConfigIssue{Severity: config.SeverityWarning, Message: "local thumbnail cache disabled: " + err.Error()})
		} else {
			if _, rerr := store.RetainNewest(cfg.LocalThumbnailsMax); rerr != nil {
				debug.Log("buildLocalCaches: thumbnail retain-newest: %v", rerr)
			}
			lt = store
		}
	}
	if transcriptsOn {
		if store, err := transcache.NewStore(cfg.LocalTranscriptsPath()); err != nil {
			issues = append(issues, config.ConfigIssue{Severity: config.SeverityWarning, Message: "local transcript cache disabled: " + err.Error()})
		} else {
			if _, rerr := store.RetainNewest(cfg.LocalTranscriptsMax); rerr != nil {
				debug.Log("buildLocalCaches: transcript retain-newest: %v", rerr)
			}
			tr = store
		}
	}
	return lt, tr, issues
}

// buildHTTPClient returns an http.Client configured for connecting to the daemon
// plus any non-fatal setup issue. If TLSCACert is set in config, the system pool
// is extended with that CA so self-signed daemon certs are accepted without
// disabling cert verification; an unreadable CA falls back to the system roots
// and reports an issue.
func buildHTTPClient(cfg *config.Config) (*http.Client, []config.ConfigIssue) {
	if cfg.TLSCACert == "" {
		return http.DefaultClient, nil
	}
	pem, err := os.ReadFile(cfg.TLSCACert)
	if err != nil {
		return http.DefaultClient, []config.ConfigIssue{{
			Severity: config.SeverityError,
			Message:  "tls_ca_cert " + cfg.TLSCACert + " could not be read (" + err.Error() + "); falling back to system roots",
		}}
	}
	pool, _ := x509.SystemCertPool()
	if pool == nil {
		pool = x509.NewCertPool()
	}
	pool.AppendCertsFromPEM(pem)
	return &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{RootCAs: pool},
		},
	}, nil
}
