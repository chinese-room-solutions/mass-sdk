package proclife

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestWatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		lost     func() bool
		interval time.Duration
		cancel   time.Duration // 0 leaves ctx alive
		wantCall bool
	}{
		{
			name:     "parent gone",
			lost:     func() bool { return true },
			interval: time.Millisecond,
			wantCall: true,
		},
		{
			name:     "parent alive until cancelled",
			lost:     func() bool { return false },
			interval: time.Millisecond,
			cancel:   20 * time.Millisecond,
		},
		{
			name:   "unsupported platform returns at once",
			cancel: time.Hour, // never reached: a nil check must not block
		},
		{
			name:     "zero interval falls back to the default",
			lost:     func() bool { return false },
			interval: 0,
			cancel:   20 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if tt.cancel > 0 {
				time.AfterFunc(tt.cancel, cancel)
			}

			var calls atomic.Int32
			done := make(chan struct{})
			go func() {
				defer close(done)
				watch(ctx, tt.interval, tt.lost, func() { calls.Add(1) })
			}()

			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Fatal("watch did not return")
			}

			if tt.wantCall {
				require.Equal(t, int32(1), calls.Load())
			} else {
				require.Zero(t, calls.Load())
			}
		})
	}
}

// TestParentLost pins the per-platform half: the test binary's own parent is
// alive, so a supported platform must report false rather than orphan us.
func TestParentLost(t *testing.T) {
	t.Parallel()

	lost := parentLost()
	if lost == nil {
		return
	}
	require.False(t, lost())
}
