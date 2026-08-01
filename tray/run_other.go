//go:build !darwin

package tray

import (
	"runtime"

	"fyne.io/systray"
)

// runTray runs the tray on its own OS-locked goroutine on Windows and Linux,
// where the tray is independent of the app's main UI loop. The tray window must
// be created and pumped on the same OS thread (a Windows requirement: window
// messages are delivered only to the creating thread), so we LockOSThread for
// the whole systray.Run lifetime.
//
// start brings the icon up (idempotent); end stops the tray loop, which returns
// from systray.Run on the locked thread.
func runTray(onReady func()) (start, end func()) {
	started := make(chan struct{})
	start = func() {
		go func() {
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
			close(started)
			systray.Run(onReady, nil)
		}()
	}
	end = func() {
		select {
		case <-started:
			systray.Quit()
		default:
			// Never started; nothing to tear down.
		}
	}
	return start, end
}
