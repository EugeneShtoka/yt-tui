package command

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

// Scope controls whether a command is available globally or only in the active view.
type Scope int

const (
	ScopeGlobal Scope = iota
	ScopeView
)

// Command is a named action callable from the command bar (`:name args`).
type Command struct {
	Name     string
	Aliases  []string
	Help     string
	Scope    Scope
	Complete func(prefix string) []string // arg completion
	Run      func(args []string) tea.Cmd  // returns a cmd; its msg flows through Update
}

// Provider is implemented by tabs and overlays that expose view-local commands.
// View-local commands shadow global ones with the same name.
type Provider interface {
	Commands() []Command
}

// Registry holds the global command set. The root builds it once at startup.
type Registry struct {
	global []Command
}

// Register adds commands to the global set.
func (r *Registry) Register(cmds ...Command) {
	r.global = append(r.global, cmds...)
}

// Resolve finds a command by name or alias. Local commands shadow global ones.
func (r *Registry) Resolve(name string, local []Command) (Command, bool) {
	for _, c := range local {
		if c.Name == name {
			return c, true
		}
		for _, a := range c.Aliases {
			if a == name {
				return c, true
			}
		}
	}
	for _, c := range r.global {
		if c.Name == name {
			return c, true
		}
		for _, a := range c.Aliases {
			if a == name {
				return c, true
			}
		}
	}
	return Command{}, false
}

// Completions returns all visible command names (local ∪ global, local first).
func (r *Registry) Completions(local []Command) []string {
	seen := make(map[string]bool)
	var names []string
	for _, c := range local {
		if !seen[c.Name] {
			names = append(names, c.Name)
			seen[c.Name] = true
		}
	}
	for _, c := range r.global {
		if !seen[c.Name] {
			names = append(names, c.Name)
			seen[c.Name] = true
		}
	}
	return names
}

// All returns every visible command (local ∪ global, local first, deduped by
// name). It is the read model behind the `:help` listing.
func (r *Registry) All(local []Command) []Command {
	seen := make(map[string]bool)
	out := make([]Command, 0, len(local)+len(r.global))
	for _, c := range local {
		if !seen[c.Name] {
			out = append(out, c)
			seen[c.Name] = true
		}
	}
	for _, c := range r.global {
		if !seen[c.Name] {
			out = append(out, c)
			seen[c.Name] = true
		}
	}
	return out
}

// Complete returns completion candidates for a partially-typed command line.
// While the command name is still being typed (no space yet), it completes the
// name against the visible set. Once the name is complete (a space follows), it
// delegates to that command's Complete hook, prefixing each candidate with the
// command name so the caller can replace the whole line. Unknown commands and
// commands without a hook yield no candidates.
func (r *Registry) Complete(input string, local []Command) []string {
	name, rest, hasArg := strings.Cut(input, " ")
	if !hasArg {
		var out []string
		for _, n := range r.Completions(local) {
			if strings.HasPrefix(n, name) {
				out = append(out, n)
			}
		}
		return out
	}
	cmd, ok := r.Resolve(name, local)
	if !ok || cmd.Complete == nil {
		return nil
	}
	var out []string
	for _, arg := range cmd.Complete(strings.TrimLeft(rest, " ")) {
		out = append(out, name+" "+arg)
	}
	return out
}
