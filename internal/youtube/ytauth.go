package youtube

import (
	"bytes"
	"context"
	"crypto/sha1" //nolint:gosec // SHA1 required by YouTube SAPISIDHASH protocol
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/EugeneShtoka/yt-tui/internal/config"
	"github.com/EugeneShtoka/yt-tui/internal/debug"
)

// This file is the auth/transport core of the YouTube innertube client: browser
// cookie extraction, cookie-file parsing, SAPISIDHASH signing, and the raw HTTP
// post. The domain verbs built on top (playlist/subscription operations) live in
// ytapi.go (M-3: auth/transport separated from the API surface).

// YTClient holds browser-extracted cookies and can make YouTube innertube API calls.
type YTClient struct {
	cookieHeader string
	sapisid      string
	hc           *http.Client // owned, not http.DefaultClient — isolates transport and is injectable in tests (L-6)
}

// newYTClient constructs a YTClient with an owned HTTP client. Requests are
// bounded by a per-call context deadline, so the client itself carries no
// Timeout.
func newYTClient(cookieHeader, sapisid string) *YTClient {
	return &YTClient{cookieHeader: cookieHeader, sapisid: sapisid, hc: &http.Client{}}
}

// cookieExtractionTimeout bounds browser cookie extraction, which shells out
// to yt-dlp's browser cookie-jar reader. That reader can hang indefinitely on
// a keyring/password prompt (e.g. a locked OS keyring) with no way to detect
// the hang other than a hard deadline.
const cookieExtractionTimeout = 60 * time.Second

// NewYTClient builds a YT API client authenticated via cookies.
// If cookies_file is set in config, it reads the Netscape cookie file directly.
// Otherwise it extracts cookies from the configured browser via yt-dlp.
func NewYTClient(ctx context.Context, cfg *config.Config) (*YTClient, error) {
	if cfg.CookiesFile != "" {
		cookieHeader, sapisid, err := parseCookieFile(cfg.CookiesFile)
		if err != nil {
			return nil, err
		}
		if sapisid == "" {
			return nil, fmt.Errorf("SAPISID not found in cookies_file; ensure it was exported while logged in to YouTube")
		}
		return newYTClient(cookieHeader, sapisid), nil
	}

	if cfg.Browser == "" {
		return nil, fmt.Errorf("no browser or cookies_file configured")
	}
	return newYTClientFromBrowser(ctx, cfg.Browser)
}

// newYTClientFromBrowser extracts YouTube cookies from the given browser by
// shelling out to yt-dlp (which writes them to a temporary Netscape cookie jar)
// and parses that jar into a client. The extraction is bounded by
// cookieExtractionTimeout since yt-dlp's browser cookie reader can hang on a
// locked OS keyring.
func newYTClientFromBrowser(ctx context.Context, browser string) (*YTClient, error) {
	f, err := os.CreateTemp("", "yt-tui-cookies-*.txt")
	if err != nil {
		return nil, fmt.Errorf("NewYTClient mktemp: %w", err)
	}
	cookiePath := f.Name()
	f.Close()
	// Remove the empty file so yt-dlp creates it itself — passing an empty file
	// to --cookies causes "does not look like a Netscape format cookies file". A
	// failed removal is logged: a leftover empty/stale file here would make the
	// yt-dlp call below fail or read stale cookies.
	if rmErr := os.Remove(cookiePath); rmErr != nil {
		debug.Log("NewYTClient: pre-remove temp cookie file %s: %v", cookiePath, rmErr)
	}
	defer func() {
		if rmErr := os.Remove(cookiePath); rmErr != nil {
			debug.Log("NewYTClient: cleanup temp cookie file %s: %v", cookiePath, rmErr)
		}
	}()

	ctx, cancel := context.WithTimeout(ctx, cookieExtractionTimeout)
	defer cancel()

	// Use the channels feed — same URL the app already uses for subscriptions,
	// so it's known to work. --flat-playlist --playlist-end 1 fetches minimal
	// data; the important side-effect is yt-dlp writing the cookie jar to file.
	cmd := exec.CommandContext(ctx, "yt-dlp",
		"--cookies-from-browser", browser,
		"--cookies", cookiePath,
		"--flat-playlist",
		"--playlist-end", "1",
		"--quiet",
		"--no-warnings",
		"https://www.youtube.com/feed/channels",
	)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err = cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("cookie extraction: %s", msg)
	}

	cookieHeader, sapisid, err := parseCookieFile(cookiePath)
	if err != nil {
		return nil, err
	}
	if sapisid == "" {
		return nil, fmt.Errorf("SAPISID not found; ensure browser is logged in to YouTube")
	}

	return newYTClient(cookieHeader, sapisid), nil
}

func parseCookieFile(path string) (cookieHeader, sapisid string, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", fmt.Errorf("parseCookieFile: %w", err)
	}

	seen := make(map[string]bool)
	var pairs []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		// yt-dlp prefixes HttpOnly cookie lines with "#HttpOnly_" — strip it.
		line = strings.TrimPrefix(line, "#HttpOnly_")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 7 {
			continue
		}
		domain := fields[0]
		// Only accept cookies scoped to youtube.com, not google.com etc.
		if !strings.HasSuffix(domain, "youtube.com") {
			continue
		}
		name := fields[5]
		value := fields[6]
		// Deduplicate: first occurrence wins (most specific domain first in yt-dlp output).
		if seen[name] {
			continue
		}
		seen[name] = true
		pairs = append(pairs, name+"="+value)
		// Prefer __Secure-3PAPISID for HTTPS innertube requests; fall back to SAPISID.
		if name == "__Secure-3PAPISID" {
			sapisid = value
		}
		if name == "SAPISID" && sapisid == "" {
			sapisid = value
		}
	}
	var names []string
	for _, p := range pairs {
		if i := strings.IndexByte(p, '='); i >= 0 {
			names = append(names, p[:i])
		}
	}
	debug.Log("parseCookieFile: cookies=%d names=%v sapisid_len=%d", len(pairs), names, len(sapisid))
	return strings.Join(pairs, "; "), sapisid, nil
}

func (c *YTClient) sapisidhash() string {
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	h := sha1.New() //nolint:gosec // required by YouTube SAPISIDHASH protocol
	h.Write([]byte(ts + " " + c.sapisid + " https://www.youtube.com"))
	hash := ts + "_" + hex.EncodeToString(h.Sum(nil))
	debug.Log("sapisidhash: ts=%s sapisid_prefix=%q", ts, c.sapisid[:min(6, len(c.sapisid))])
	return hash
}

func (c *YTClient) post(ctx context.Context, endpoint string, body map[string]any) ([]byte, error) {
	payload := map[string]any{
		"context": map[string]any{
			"client": map[string]any{
				"clientName":    "WEB",
				"clientVersion": "2.20231219.04.00",
				"hl":            "en",
			},
		},
	}
	for k, v := range body {
		payload[k] = v
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("post marshal: %w", err)
	}

	// Bound the request so a hung connection can't block the caller forever,
	// while still honoring an upstream cancel (shutting-down daemon / canceled RPC).
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://www.youtube.com/youtubei/v1/"+endpoint,
		bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("post request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "SAPISIDHASH "+c.sapisidhash())
	req.Header.Set("X-Origin", "https://www.youtube.com")
	req.Header.Set("X-Goog-AuthUser", "0")
	req.Header.Set("Referer", "https://www.youtube.com/")
	req.Header.Set("Cookie", c.cookieHeader)
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("post do: %w", err)
	}
	defer resp.Body.Close()

	respData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("post read: %w", err)
	}
	if resp.StatusCode >= 400 {
		preview := string(respData)
		if len(preview) > 120 {
			preview = preview[:120]
		}
		return nil, fmt.Errorf("YouTube API %d: %s", resp.StatusCode, preview)
	}
	return respData, nil
}
