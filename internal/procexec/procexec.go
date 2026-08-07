// Package procexec provides a thin injection seam over os/exec so subprocess
// orchestration (yt-dlp fetch, yt-dlp download) can be unit-tested without a
// real binary on PATH. Production code uses OS{}; tests supply a fake Runner
// that returns canned stdout/stderr and a Wait error.
package procexec

import (
	"context"
	"io"
	"os/exec"
)

// Cmd is the subset of *exec.Cmd the app relies on. *exec.Cmd satisfies it
// directly, so the OS runner is a zero-cost wrapper.
type Cmd interface {
	StdoutPipe() (io.ReadCloser, error)
	StderrPipe() (io.ReadCloser, error)
	Start() error
	Wait() error
}

// Runner constructs a Cmd for a program + args. It is the seam callers depend
// on instead of exec.CommandContext directly.
type Runner interface {
	Command(ctx context.Context, name string, args ...string) Cmd
}

// OS is the production Runner: it execs real binaries via os/exec.
type OS struct{}

// Command returns an *exec.Cmd (which satisfies Cmd) bound to ctx.
func (OS) Command(ctx context.Context, name string, args ...string) Cmd {
	return exec.CommandContext(ctx, name, args...)
}
