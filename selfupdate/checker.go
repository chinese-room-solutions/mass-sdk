package selfupdate

import (
	"context"
	"sync"
	"time"

	"github.com/rs/zerolog"
)

const (
	// DefaultInterval is how often a running app re-asks for the newest release.
	// A release is a rare event and the answer costs a redirect, so this is
	// tuned to "an app left open all day notices before the user gives up",
	// not to promptness.
	DefaultInterval = 6 * time.Hour
	// checkTimeout bounds one check. Nothing waits on the background ones, so
	// this only stops a hung connection from pinning the goroutine; a manual
	// check is answered inside an HTTP request, where it is the whole budget.
	checkTimeout = 30 * time.Second
)

// Status is what an app knows about a newer release right now: the running
// build, the newer tag (empty when current), when the answer was taken, and
// why the last attempt failed.
//
// Err is the load-bearing field for the operator: a check that can't reach the
// release host must not read as "up to date". Being offline is normal, so it is
// reported rather than logged loudly.
type Status struct {
	Version   string    `json:"version"`
	Available string    `json:"available"`
	CheckedAt time.Time `json:"checked_at,omitempty"`
	Err       string    `json:"error,omitempty"`
}

// Checker owns an app's answer to "is there a newer release?" — refreshed in
// the background by [Checker.Run] and on demand by [Checker.Check], read by
// [Checker.Status] from any goroutine.
//
// One check at startup is not enough: the app's UI can render before the first
// answer arrives, and an app left running outlives releases published after it
// started.
type Checker struct {
	// Version is the running build, as stamped at link time.
	Version string
	// BaseURL is the release repository, e.g.
	// "https://github.com/chinese-room-solutions/mass".
	BaseURL string
	// Interval is the background re-check cadence; zero means DefaultInterval.
	Interval time.Duration
	// Logger receives the background checks' outcomes.
	Logger zerolog.Logger
	// OnFound, when set, is called once each time a newer tag appears —
	// including the first time it is found. Apps use it to push the news to an
	// already-open window.
	OnFound func(tag string)

	mu     sync.Mutex
	status Status
}

// Status returns the last known answer.
func (c *Checker) Status() Status {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.status
	s.Version = c.Version
	return s
}

// Available is the newer release tag, or "" when none is known.
func (c *Checker) Available() string { return c.Status().Available }

// Check asks the release repository now and records the answer. It returns the
// resulting status; the error is returned as well as recorded so a caller that
// asked for this check (an operator pressing a button) can report it.
func (c *Checker) Check(ctx context.Context) (Status, error) {
	ctx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()

	latest, err := Latest(ctx, c.BaseURL)
	tag := ""
	if err == nil && IsNewer(c.Version, latest) {
		tag = latest
	}
	return c.record(tag, err), err
}

// record stores one check's outcome and fires OnFound for a tag that wasn't
// already known. A failed check keeps the previously found tag: the release
// didn't disappear because the network did.
func (c *Checker) record(tag string, err error) Status {
	c.mu.Lock()
	prev := c.status.Available
	if err == nil {
		c.status.Available = tag
	}
	c.status.CheckedAt = time.Now()
	c.status.Err = ""
	if err != nil {
		c.status.Err = err.Error()
	}
	s := c.status
	c.mu.Unlock()

	s.Version = c.Version
	if err == nil && tag != "" && tag != prev && c.OnFound != nil {
		c.OnFound(tag)
	}
	return s
}

// Run checks immediately and then every Interval until ctx ends. It is the
// caller's goroutine: start it with `go checker.Run(ctx)`.
func (c *Checker) Run(ctx context.Context) {
	interval := c.Interval
	if interval <= 0 {
		interval = DefaultInterval
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		c.log(c.Check(ctx))
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// log reports a background check at debug level — an unreachable release host
// is the normal state of an offline machine, and the operator sees the failure
// in Status either way. A newly found release is worth an info line.
func (c *Checker) log(s Status, err error) {
	switch {
	case err != nil:
		c.Logger.Debug().Err(err).Str("url", c.BaseURL).Msg("checking for a newer release")
	case s.Available != "":
		c.Logger.Info().Str("running", c.Version).Str("available", s.Available).
			Msg("a newer release is available")
	default:
		c.Logger.Debug().Str("running", c.Version).Msg("up to date")
	}
}
