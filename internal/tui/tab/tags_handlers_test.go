package tab

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
)

func updateTags(tg Tags, msg tea.Msg) (Tags, tea.Cmd) {
	m, cmd := tg.Update(msg)
	return m.(Tags), cmd
}

// loadedTags builds a sized Tags tab with one subscribed channel tagged
// "gonews" that owns one video, ready for key-handler assertions.
func loadedTags(t *testing.T) Tags {
	t.Helper()
	tg := NewTags(context.Background(), &fakeBackend{}, testKeys(), false, TagsOpts{Mode: "subscribed", StaleDays: 30})
	tg, _ = updateTags(tg, sized(80, 24))
	tg, _ = updateTags(tg, tagsDataMsg{
		chans:     []domain.Channel{{ID: "sub", Name: "Sub", State: domain.SubYT, Tags: []string{"gonews"}}},
		subVideos: []domain.Video{{ID: "v1", ChannelID: "sub", Channel: "Sub", Title: "Vid"}},
	})
	return tg
}

func TestTagsAccessors(t *testing.T) {
	tg := loadedTags(t)
	if tg.ID() != tuipkg.TabTags {
		t.Errorf("ID() = %v, want TabTags", tg.ID())
	}
	if tg.Title() != "Tags" {
		t.Errorf("Title() = %q, want Tags", tg.Title())
	}
	if tg.Loading() {
		t.Error("Loading() should be false after tagsDataMsg")
	}
	if len(tg.ShortHelp()) == 0 {
		t.Error("ShortHelp() should list at least the drill/panel-mode bindings")
	}
	// In the list pane, SelectedVideo returns the highlighted tag's latest video.
	if v, ok := tg.SelectedVideo(); !ok || v.ID != "v1" {
		t.Errorf("SelectedVideo() in list pane = (%q, %v), want (v1, true)", v.ID, ok)
	}
}

func TestTagsDrillIntoTagEntersVideoPane(t *testing.T) {
	tg := loadedTags(t)

	tg, _ = updateTags(tg, tea.KeyPressMsg{Code: tea.KeyEnter})

	if tg.pane != 1 {
		t.Fatalf("drill should enter the video pane (pane 1), got pane %d", tg.pane)
	}
	if tg.tagSel != "gonews" {
		t.Errorf("tagSel = %q, want gonews", tg.tagSel)
	}
}

func TestTagsVideoPanePlayEmitsPlayVideoMsg(t *testing.T) {
	tg := loadedTags(t)
	tg, _ = updateTags(tg, tea.KeyPressMsg{Code: tea.KeyEnter}) // drill → pane 1

	_, cmd := updateTags(tg, tea.KeyPressMsg{Text: "p"})

	play, ok := runCmd(cmd).(tuipkg.PlayVideoMsg)
	if !ok {
		t.Fatalf("Play should emit PlayVideoMsg, got %#v", runCmd(cmd))
	}
	if play.Video.ID != "v1" {
		t.Errorf("played video = %q, want v1", play.Video.ID)
	}
}

func TestTagsVideoPaneBackReturnsToList(t *testing.T) {
	tg := loadedTags(t)
	tg, _ = updateTags(tg, tea.KeyPressMsg{Code: tea.KeyEnter}) // drill → pane 1
	if tg.pane != 1 {
		t.Fatal("setup: expected to be in the video pane")
	}

	tg, _ = updateTags(tg, tea.KeyPressMsg{Code: tea.KeyEscape})

	if tg.pane != 0 {
		t.Errorf("Escape in the video pane should return to the list pane, got pane %d", tg.pane)
	}
}

func TestTagsModePickerOpensAndCommits(t *testing.T) {
	tg := loadedTags(t)

	// PanelMode opens the picker; while open it intercepts input.
	tg, _ = updateTags(tg, tea.KeyPressMsg{Text: "M"})
	if !tg.InterceptsInput() {
		t.Fatal("PanelMode should open the mode picker (InterceptsInput true)")
	}

	// Committing the picker selection closes it and returns to the list pane.
	tg, _ = updateTags(tg, tea.KeyPressMsg{Code: tea.KeyEnter})
	if tg.InterceptsInput() {
		t.Error("committing the picker should close it")
	}
	if tg.pane != 0 {
		t.Errorf("after picker commit pane = %d, want 0", tg.pane)
	}
}
