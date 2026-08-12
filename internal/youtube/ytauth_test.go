package youtube

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// cookieLine builds one Netscape-format cookie record (7 tab-separated fields:
// domain, includeSubdomains, path, secure, expiry, name, value).
func cookieLine(domain, name, value string) string {
	return strings.Join([]string{domain, "TRUE", "/", "TRUE", "0", name, value}, "\t")
}

type parseCookieCase struct {
	name        string
	lines       []string
	wantSapisid string
	wantPairs   []string // "name=value" substrings that must appear in the header
	absentNames []string // "name=" prefixes that must NOT appear in the header
}

var parseCookieCases = []parseCookieCase{
	{
		name:        "valid SAPISID",
		lines:       []string{cookieLine(".youtube.com", "SAPISID", "sap_val")},
		wantSapisid: "sap_val",
		wantPairs:   []string{"SAPISID=sap_val"},
	},
	{
		name: "HttpOnly prefix stripped and accepted",
		// yt-dlp prefixes HttpOnly lines with "#HttpOnly_"; it must be stripped,
		// not treated as a comment.
		lines:       []string{"#HttpOnly_.youtube.com" + "\tTRUE\t/\tTRUE\t0\tHSID\thsid_val"},
		wantSapisid: "",
		wantPairs:   []string{"HSID=hsid_val"},
	},
	{
		name: "non-youtube domain dropped",
		lines: []string{
			cookieLine(".google.com", "SAPISID", "google_val"),
			cookieLine(".youtube.com", "PREF", "pref_val"),
		},
		wantSapisid: "", // google.com SAPISID must not be picked up
		wantPairs:   []string{"PREF=pref_val"},
		absentNames: []string{"SAPISID="},
	},
	{
		name: "duplicate name first occurrence wins",
		lines: []string{
			cookieLine(".youtube.com", "SAPISID", "first"),
			cookieLine(".youtube.com", "SAPISID", "second"),
		},
		wantSapisid: "first",
		wantPairs:   []string{"SAPISID=first"},
		absentNames: []string{"SAPISID=second"},
	},
	{
		name: "__Secure-3PAPISID preferred over SAPISID regardless of order",
		lines: []string{
			cookieLine(".youtube.com", "SAPISID", "sap_val"),
			cookieLine(".youtube.com", "__Secure-3PAPISID", "secure_val"),
		},
		wantSapisid: "secure_val",
		wantPairs:   []string{"SAPISID=sap_val", "__Secure-3PAPISID=secure_val"},
	},
	{
		name: "malformed and comment lines skipped",
		lines: []string{
			"# a comment",
			"",
			"youtube.com\tTRUE\t/", // < 7 fields
			cookieLine(".youtube.com", "SAPISID", "sap_val"),
		},
		wantSapisid: "sap_val",
		wantPairs:   []string{"SAPISID=sap_val"},
	},
	{
		name:        "no SAPISID leaves sapisid empty but still returns cookies",
		lines:       []string{cookieLine(".youtube.com", "PREF", "pref_val")},
		wantSapisid: "",
		wantPairs:   []string{"PREF=pref_val"},
		absentNames: []string{"SAPISID="},
	},
}

func TestParseCookieFile(t *testing.T) {
	for _, tt := range parseCookieCases {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "cookies.txt")
			content := "# Netscape HTTP Cookie File\n" + strings.Join(tt.lines, "\n") + "\n"
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatalf("write cookie file: %v", err)
			}

			header, sapisid, err := parseCookieFile(path)
			if err != nil {
				t.Fatalf("parseCookieFile: %v", err)
			}
			if sapisid != tt.wantSapisid {
				t.Errorf("sapisid = %q, want %q", sapisid, tt.wantSapisid)
			}
			for _, want := range tt.wantPairs {
				if !strings.Contains(header, want) {
					t.Errorf("header %q is missing %q", header, want)
				}
			}
			for _, absent := range tt.absentNames {
				if strings.Contains(header, absent) {
					t.Errorf("header %q must not contain %q", header, absent)
				}
			}
		})
	}
}

func TestParseCookieFileMissing(t *testing.T) {
	_, _, err := parseCookieFile(filepath.Join(t.TempDir(), "does-not-exist.txt"))
	if err == nil {
		t.Fatal("parseCookieFile on a missing file: got nil error, want non-nil")
	}
}
