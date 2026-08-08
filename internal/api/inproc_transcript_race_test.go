package api

import (
	"context"
	"sync"
	"testing"
	"time"
)

// TestBuildTranscriptNoteShared_CtxCancelDoesNotRaceWaitEnrichment guards H-1.
// When a caller's ctx cancels before the shared build finishes, the transcriptWG
// registration must already have happened on the caller's own stack — not on the
// singleflight worker goroutine — so a subsequent WaitEnrichment cannot race the
// Add. The previous code added inside the DoChan closure and, on this exact
// ctx-cancel path, would panic "sync: WaitGroup misuse: Add called concurrently
// with Wait" (reliably across the loop below). The fix must instead let
// WaitEnrichment drain every build cleanly. Run under -race in CI.
func TestBuildTranscriptNoteShared_CtxCancelDoesNotRaceWaitEnrichment(t *testing.T) {
	p := &InProc{} // transcripts nil → the build is a no-op, but still runs on the tracked goroutine

	for i := 0; i < 200; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // caller has already given up before the build is scheduled
		p.buildTranscriptNoteShared(ctx, "vid", "")

		done := make(chan struct{})
		go func() { p.WaitEnrichment(); close(done) }()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatalf("iter %d: WaitEnrichment hung — a build leaked a transcriptWG count", i)
		}
	}
}

// TestBuildTranscriptNoteShared_ConcurrentSameKeyDrains fires many concurrent
// builds for the same video id — the collapse path singleflight exists for — and
// asserts WaitEnrichment drains them all without a WaitGroup race or hang. It
// exercises the Add/Done bookkeeping under heavy concurrency (M-7). Run under
// -race in CI.
func TestBuildTranscriptNoteShared_ConcurrentSameKeyDrains(t *testing.T) {
	p := &InProc{}
	const n = 64

	var callers sync.WaitGroup
	callers.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer callers.Done()
			p.buildTranscriptNoteShared(context.Background(), "same-id", "")
		}()
	}
	callers.Wait() // every caller returned; their tracked builds may still be finishing

	done := make(chan struct{})
	go func() { p.WaitEnrichment(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("WaitEnrichment hung after concurrent same-key builds")
	}
}
