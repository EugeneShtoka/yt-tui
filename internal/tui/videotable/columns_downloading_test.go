package videotable

import (
	"strings"
	"testing"

	"github.com/EugeneShtoka/yt-tui/internal/tui/render"
)

// dlRow is a minimal row satisfying the DlDurationCol constraint.
type dlRow struct{ secs int }

func (r dlRow) GetDurationSecs() int { return r.secs }

// TestDlDurationColHonorsConfiguredFormat locks in the M-2 fix: the Downloading
// tab's duration column must format seconds through render.Duration (config-aware),
// not carry a pre-formatted string. Before the fix, download durations rendered in
// a fixed h:mm:ss format regardless of the user's DurationFormat, diverging from
// every other duration column.
func TestDlDurationColHonorsConfiguredFormat(t *testing.T) {
	prev := render.ActiveDurFmt()
	t.Cleanup(func() { render.SetDurFmt(prev) })

	col := DlDurationCol[dlRow]()
	cell := func(secs int) string {
		return strings.TrimSpace(col.Cell(dlRow{secs: secs}, 0).(string))
	}

	render.SetDurFmt(render.DurFmtHHMMSS) // always-padded hours
	if got, want := cell(3665), "01:01:05"; got != want {
		t.Errorf("HH:MM:SS format: cell(3665) = %q, want %q", got, want)
	}

	render.SetDurFmt(render.DurFmthhmm) // compact, hours dropped under 1h
	if got, want := cell(305), "5"; got != want {
		t.Errorf("hh:mm format: cell(305) = %q, want %q", got, want)
	}
}
