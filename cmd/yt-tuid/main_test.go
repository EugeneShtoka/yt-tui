package main

import "testing"

// fakeWaiter / fakeStopper record the teardown call order so the test can prove
// the shutdown ordering invariant (stop before enrichment join) and that both
// background writers are joined.
type fakeWaiter struct{ order *[]string }

func (f fakeWaiter) WaitEnrichment() { *f.order = append(*f.order, "wait") }

type fakeStopper struct{ order *[]string }

func (f fakeStopper) Stop() { *f.order = append(*f.order, "stop-dl") }

// TestTeardownStopsBothWritersInOrder is the M-1 regression: teardown must run
// on every serve return, canceling the enrichment context, then joining the
// download workers and the enrichment pass — so nothing is still writing when
// the deferred database.Close() runs. Before the fix this path only ran on the
// signal branch, leaking the serve-error case.
func TestTeardownStopsBothWritersInOrder(t *testing.T) {
	var order []string
	stopCalled := false
	stop := func() {
		stopCalled = true
		order = append(order, "cancel")
	}

	teardown(stop, fakeWaiter{order: &order}, fakeStopper{order: &order})

	if !stopCalled {
		t.Fatal("teardown must cancel the enrichment context (stop)")
	}
	// stop() must precede WaitEnrichment: the enrichment loop only exits once its
	// context is canceled, so joining before canceling would hang.
	cancelIdx, waitIdx := indexOf(order, "cancel"), indexOf(order, "wait")
	if cancelIdx == -1 || waitIdx == -1 {
		t.Fatalf("expected both cancel and wait to run, got %v", order)
	}
	if cancelIdx > waitIdx {
		t.Errorf("cancel must precede WaitEnrichment, got order %v", order)
	}
	if indexOf(order, "stop-dl") == -1 {
		t.Errorf("teardown must stop the downloader, got order %v", order)
	}
}

func indexOf(s []string, want string) int {
	for i, v := range s {
		if v == want {
			return i
		}
	}
	return -1
}
