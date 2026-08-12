package procexec

import (
	"context"
	"io"
	"strings"
)

// FakeRunner is a test double for Runner. New builds the Cmd for each
// invocation; args lets a test vary output per call (e.g. per pagination page).
type FakeRunner struct {
	New func(args []string) Cmd
}

// Command records nothing beyond delegating to New; the context is ignored so
// tests stay deterministic. Use FakeCmd.WaitFn if a test needs to observe ctx.
func (f FakeRunner) Command(_ context.Context, _ string, args ...string) Cmd {
	return f.New(args)
}

// FakeCmd is a canned process: it serves Stdout/Stderr from strings and reports
// a start/wait outcome. It satisfies Cmd.
type FakeCmd struct {
	Stdout   string
	Stderr   string
	StartErr error
	// WaitErr is returned by Wait when WaitFn is nil.
	WaitErr error
	// WaitFn, if set, is called by Wait — use it to block (to hold a slot
	// "active" while testing concurrency) or to compute the exit error.
	WaitFn func() error
}

func (c *FakeCmd) StdoutPipe() (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader(c.Stdout)), nil
}

func (c *FakeCmd) StderrPipe() (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader(c.Stderr)), nil
}

func (c *FakeCmd) Start() error { return c.StartErr }

func (c *FakeCmd) Wait() error {
	if c.WaitFn != nil {
		return c.WaitFn()
	}
	return c.WaitErr
}
