package command

import (
	"reflect"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func testCommands() []Command {
	return []Command{
		{Name: "quit", Aliases: []string{"q"}, Help: "quit"},
		{Name: "tab", Help: "switch tab", Complete: func(prefix string) []string {
			names := []string{"Feed", "Local", "History"}
			var out []string
			for _, n := range names {
				if hasPrefix(n, prefix) {
					out = append(out, n)
				}
			}
			return out
		}},
		{Name: "download", Help: "download a url"},
	}
}

// hasPrefix is a tiny local helper mirroring strings.HasPrefix without the
// import, so the fixture stays self-contained.
func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

func TestRegistryResolvePrefersLocal(t *testing.T) {
	var r Registry
	r.Register(Command{Name: "quit", Help: "global quit"})
	local := []Command{{Name: "quit", Help: "local quit"}}

	got, ok := r.Resolve("quit", local)
	if !ok {
		t.Fatal("quit did not resolve")
	}
	if got.Help != "local quit" {
		t.Errorf("local command should shadow global; got Help=%q", got.Help)
	}
}

func TestRegistryResolveByAlias(t *testing.T) {
	var r Registry
	r.Register(testCommands()...)
	if _, ok := r.Resolve("q", nil); !ok {
		t.Error("alias q should resolve to quit")
	}
	if _, ok := r.Resolve("nope", nil); ok {
		t.Error("unknown name must not resolve")
	}
}

func TestCompleteCommandNames(t *testing.T) {
	var r Registry
	r.Register(testCommands()...)

	// Bare prefix completes command names (local ∪ global).
	got := r.Complete("d", nil)
	want := []string{"download"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Complete(%q) = %v, want %v", "d", got, want)
	}

	// Empty input offers every command name.
	if names := r.Complete("", nil); len(names) != 3 {
		t.Errorf("Complete(\"\") = %v, want 3 names", names)
	}
}

func TestCompleteDelegatesToArgHook(t *testing.T) {
	var r Registry
	r.Register(testCommands()...)

	// Once the command name is complete (trailing space), completion delegates
	// to that command's Complete hook on the arg prefix, prefixed with the name.
	got := r.Complete("tab Lo", nil)
	want := []string{"tab Local"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Complete(%q) = %v, want %v", "tab Lo", got, want)
	}
}

func TestCompleteUnknownCommandArgYieldsNothing(t *testing.T) {
	var r Registry
	r.Register(testCommands()...)
	if got := r.Complete("nope arg", nil); got != nil {
		t.Errorf("Complete on unknown command = %v, want nil", got)
	}
	// A command without a Complete hook yields no arg completions.
	if got := r.Complete("download foo", nil); got != nil {
		t.Errorf("Complete on hookless command = %v, want nil", got)
	}
}

func TestAllDedupesLocalFirst(t *testing.T) {
	var r Registry
	r.Register(Command{Name: "quit", Help: "global"}, Command{Name: "tab"})
	local := []Command{{Name: "quit", Help: "local"}}

	all := r.All(local)
	if len(all) != 2 {
		t.Fatalf("All returned %d commands, want 2 (deduped)", len(all))
	}
	if all[0].Name != "quit" || all[0].Help != "local" {
		t.Errorf("local quit must come first and shadow global; got %+v", all[0])
	}
}

// ensure the package's tea dependency stays used even if the fixtures change.
var _ tea.Cmd
