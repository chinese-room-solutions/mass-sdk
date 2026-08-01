// Package tray adds a system-tray (status-notifier) icon to a MASS app GUI so
// the window can fold away to the tray instead of staying on the taskbar. It is
// a thin, webview-agnostic facade over fyne.io/systray: the app supplies an icon
// and two callbacks (show the window, quit the app) and gets back start/end
// thunks to drive from its webview lifecycle.
//
// Threading is the crux and is handled per-OS (see run_*.go):
//   - Windows: the tray window must be created AND pumped on one OS-locked
//     thread. We run systray's own loop on a dedicated locked goroutine, fully
//     independent of the webview's main thread. (Using the external-loop API
//     here is broken: it creates the window on the caller's thread but pumps on
//     a different goroutine, so clicks never arrive.)
//   - Linux (KDE StatusNotifierItem over D-Bus): also independent of the GTK
//     loop, so the same dedicated-loop approach works.
//   - macOS: the status item needs the main NSApplication, so the webview's
//     already-running main loop must host it via systray's external-loop API.
//
// Targets: Windows, macOS, and KDE Linux.
package tray

import "fyne.io/systray"

// Options configures the tray icon and its menu.
type Options struct {
	// Title is the tray tooltip (and the menubar label on macOS).
	Title string
	// IconPNG is the tray icon as PNG bytes. It is converted to the format each
	// platform's tray expects (.ico on Windows; PNG as-is on Linux/macOS).
	IconPNG []byte
	// OnShow re-shows and focuses the app window. Wired to a left-click on the
	// icon (which always shows, never hides).
	OnShow func()
	// OnToggle shows the window if hidden, hides it if shown. Wired to the
	// "Show/Hide" menu item. The window owns the visibility state so a left
	// click, a minimize, and this toggle stay in sync.
	OnToggle func()
	// OnQuit fully shuts the app down. Wired to the "Quit" menu item.
	OnQuit func()
}

// ControllerInterface lets the app update the tray after it is registered.
type ControllerInterface interface {
	// SetTooltip changes the icon's hover text.
	SetTooltip(string)
}

type controller struct{}

func (controller) SetTooltip(s string) { systray.SetTooltip(s) }

// Register wires the tray into the app. It returns:
//
//	start — call once the webview window exists, to bring the icon up;
//	end   — call on app shutdown, to remove the icon and stop the tray;
//	ctrl  — to update the tray later.
//
// The menu is built on the systray ready callback: a "Show/Hide" item (also the
// left-click action) and a "Quit" item. The callbacks fire on the tray's loop
// goroutine, so OnShow/OnQuit must hand UI work to the webview via its dispatch.
func Register(opts Options) (start, end func(), ctrl ControllerInterface) {
	onReady := func() {
		systray.SetTooltip(opts.Title)
		if icon := trayIcon(opts.IconPNG); len(icon) > 0 {
			systray.SetIcon(icon)
		}

		// Left-click the icon → show the window (on platforms where a click
		// opens the menu instead, e.g. macOS, this is ignored).
		if opts.OnShow != nil {
			systray.SetOnTapped(opts.OnShow)
		}

		mShow := systray.AddMenuItem("Show/Hide", "Show or hide the window")
		mQuit := systray.AddMenuItem("Quit", "Quit the application")
		go func() {
			for {
				select {
				case _, ok := <-mShow.ClickedCh:
					if !ok {
						return
					}
					if opts.OnToggle != nil {
						opts.OnToggle()
					}
				case _, ok := <-mQuit.ClickedCh:
					if !ok {
						return
					}
					if opts.OnQuit != nil {
						opts.OnQuit()
					}
					return
				}
			}
		}()
	}

	start, end = runTray(onReady)
	return start, end, controller{}
}
