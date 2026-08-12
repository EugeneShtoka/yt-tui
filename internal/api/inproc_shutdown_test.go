package api

import (
	"context"
	"testing"
	"time"
)

// buildTranscriptNoteShared must balance its transcriptWG Add/Done even on the
// early-return path (no transcript store), so shutdown can't hang waiting on a
// leaked counter. Guards the singleflight wrapper's WaitGroup bookkeeping.
func TestBuildTranscriptNoteSharedBalancesWaitGroup(t *testing.T) {
	p := &InProc{} // transcripts nil → buildTranscriptNote returns false immediately
	if p.buildTranscriptNoteShared(context.Background(), "abc", "") {
		t.Fatal("want false with no transcript store")
	}
	done := make(chan struct{})
	go func() { p.WaitEnrichment(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("WaitEnrichment hung — shared build leaked a transcriptWG count")
	}
}

// TestWaitEnrichmentDrainsTranscriptBuilds guards the M-1 fix: WaitEnrichment
// must block until every in-flight fire-and-forget transcript-note build has
// finished, so shutdown can't race the transcript store's teardown. It stands
// in for such a build with the exact Add(1)/Done pairing maybeSaveTranscript
// uses. If someone drops the transcriptWG.Wait() from WaitEnrichment this fails.
func TestWaitEnrichmentDrainsTranscriptBuilds(t *testing.T) {
	p := &InProc{} // enrichDone nil → WaitEnrichment only has the transcriptWG to drain

	started := make(chan struct{})
	release := make(chan struct{})
	p.transcriptWG.Add(1)
	go func() {
		defer p.transcriptWG.Done()
		close(started)
		<-release // hold the "build" open until the test releases it
	}()
	<-started

	returned := make(chan struct{})
	go func() {
		p.WaitEnrichment()
		close(returned)
	}()

	// While the build is in flight, WaitEnrichment must not return.
	select {
	case <-returned:
		t.Fatal("WaitEnrichment returned before the in-flight transcript build finished")
	case <-time.After(50 * time.Millisecond):
	}

	close(release) // build completes → Done()
	select {
	case <-returned:
	case <-time.After(2 * time.Second):
		t.Fatal("WaitEnrichment did not return after the transcript build finished")
	}
}

// TestWaitEnrichmentDrainsBackgroundMaintenance guards L-2: WaitEnrichment must
// also block on bgWG, the group tracking detached maintenance goroutines (the
// thumbnail recrop sweep), so shutdown can't race the thumbnail store teardown.
// If someone drops the bgWG.Wait() from WaitEnrichment this fails.
func TestWaitEnrichmentDrainsBackgroundMaintenance(t *testing.T) {
	p := &InProc{} // enrichDone nil → WaitEnrichment only has the WaitGroups to drain

	started := make(chan struct{})
	release := make(chan struct{})
	p.bgWG.Add(1)
	go func() {
		defer p.bgWG.Done()
		close(started)
		<-release // hold the "recrop" open until the test releases it
	}()
	<-started

	returned := make(chan struct{})
	go func() {
		p.WaitEnrichment()
		close(returned)
	}()

	select {
	case <-returned:
		t.Fatal("WaitEnrichment returned before the in-flight maintenance goroutine finished")
	case <-time.After(50 * time.Millisecond):
	}

	close(release)
	select {
	case <-returned:
	case <-time.After(2 * time.Second):
		t.Fatal("WaitEnrichment did not return after the maintenance goroutine finished")
	}
}

// TestWaitEnrichmentNoBackgroundWork returns immediately when nothing is running
// (no enrichment started, no transcript builds) — the common no-op shutdown.
func TestWaitEnrichmentNoBackgroundWork(t *testing.T) {
	p := &InProc{}
	done := make(chan struct{})
	go func() { p.WaitEnrichment(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("WaitEnrichment blocked with no background work")
	}
}
