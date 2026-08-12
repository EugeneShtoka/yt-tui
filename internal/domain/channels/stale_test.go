package channels

import (
	"testing"
	"time"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

func TestIsStale(t *testing.T) {
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	fresh := now.Add(-10 * 24 * time.Hour).Unix() // 10 days ago
	old := now.Add(-40 * 24 * time.Hour).Unix()   // 40 days ago
	tagged := []string{"tech"}

	cases := []struct {
		name string
		ch   domain.Channel
		days int
		want bool
	}{
		{
			name: "tagged unsubscribed old = stale",
			ch:   domain.Channel{Tags: tagged, State: domain.SubNone, LastActivityAt: old},
			days: 30, want: true,
		},
		{
			name: "tagged unsubscribed never-active = stale",
			ch:   domain.Channel{Tags: tagged, State: domain.SubNone, LastActivityAt: 0},
			days: 30, want: true,
		},
		{
			name: "tagged unsubscribed recent = fresh",
			ch:   domain.Channel{Tags: tagged, State: domain.SubNone, LastActivityAt: fresh},
			days: 30, want: false,
		},
		{
			name: "untagged never counts",
			ch:   domain.Channel{State: domain.SubNone, LastActivityAt: old},
			days: 30, want: false,
		},
		{
			name: "subscribed exempt even when old",
			ch:   domain.Channel{Tags: tagged, State: domain.SubYT, LastActivityAt: old},
			days: 30, want: false,
		},
		{
			name: "local-subscribed exempt",
			ch:   domain.Channel{Tags: tagged, State: domain.SubLocal, LastActivityAt: old},
			days: 30, want: false,
		},
		{
			name: "blocked exempt",
			ch:   domain.Channel{Tags: tagged, State: domain.SubNone, Blocked: true, LastActivityAt: old},
			days: 30, want: false,
		},
		{
			name: "non-positive threshold disables",
			ch:   domain.Channel{Tags: tagged, State: domain.SubNone, LastActivityAt: old},
			days: 0, want: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsStale(tc.ch, now, tc.days); got != tc.want {
				t.Fatalf("IsStale(%+v) = %v, want %v", tc.ch, got, tc.want)
			}
		})
	}
}

// TestIsStaleBoundary checks the cutoff is exclusive at exactly the threshold.
func TestIsStaleBoundary(t *testing.T) {
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	cutoff := now.Add(-30 * 24 * time.Hour).Unix()
	tagged := []string{"tech"}
	// Exactly at the cutoff second is not yet older than the threshold.
	atCutoff := domain.Channel{Tags: tagged, State: domain.SubNone, LastActivityAt: cutoff}
	if IsStale(atCutoff, now, 30) {
		t.Fatalf("channel active exactly at cutoff should not be stale")
	}
	justBefore := domain.Channel{Tags: tagged, State: domain.SubNone, LastActivityAt: cutoff - 1}
	if !IsStale(justBefore, now, 30) {
		t.Fatalf("channel active one second before cutoff should be stale")
	}
}
