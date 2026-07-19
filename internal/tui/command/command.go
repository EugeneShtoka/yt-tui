package command

import tea "charm.land/bubbletea/v2"

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
