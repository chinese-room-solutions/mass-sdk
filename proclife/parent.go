// Package proclife ties a process's lifetime to the one that started it.
//
// A MASS runtime gateway is a hashicorp/go-plugin subprocess, and go-plugin
// hands the child the host's own stdin rather than a pipe — so a gateway never
// sees an EOF when MASS dies and would otherwise keep running, holding a model
// resident, with nobody left to talk to. [WatchParent] is that missing signal.
//
// MASS also puts every gateway it launches into a Windows job object, which the
// OS enforces however MASS dies; this package is what covers the platforms that
// have no equivalent.
package proclife

import (
	"context"
	"time"
)

// DefaultInterval is how often [WatchParent] looks, when the caller passes 0.
// A gateway that outlives MASS by a few seconds costs nothing; one that outlives
// it forever pins a model in memory.
const DefaultInterval = 3 * time.Second

// WatchParent blocks until ctx is cancelled or the process that started this one
// exits, calling onOrphan in the latter case. Callers run it in their own
// goroutine, whose exit path is either of those two.
//
// On Windows it returns immediately: the parent pid there is a recorded, stale
// value that survives the parent's death and can be recycled onto an unrelated
// process, so polling it proves nothing. MASS's job object covers that platform.
func WatchParent(ctx context.Context, interval time.Duration, onOrphan func()) {
	watch(ctx, interval, parentLost(), onOrphan)
}

// watch is WatchParent with the "has the parent gone?" test injected, so tests
// don't need to orphan a real process.
func watch(ctx context.Context, interval time.Duration, lost func() bool, onOrphan func()) {
	if lost == nil {
		return
	}
	if interval <= 0 {
		interval = DefaultInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if lost() {
				onOrphan()
				return
			}
		}
	}
}
