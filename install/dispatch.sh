#!/bin/sh
# Terminal dispatcher for a bundled console app (AppImage / .app). A
# double-clicked binary has no controlling terminal, so a TUI wizard would EOF on
# input and the window would flash closed. This opens the user's terminal, sized
# to the wizard, and runs the app inside it.
#
# $1 is the absolute path to the wrapped binary; remaining args are forwarded.
# Two cases run INLINE (no new window):
#   1. forwarded args present — e.g. a sudo re-exec "$APPIMAGE --install …".
#      Args mean "do this here", and sudo may leave stdin not-a-tty, so we must
#      NOT gate on -t 0 or elevation would loop into a fresh wizard window.
#   2. already attached to a terminal (launched from a shell).
app="$1"; shift
cols=__COLS__; rows=__ROWS__
if [ "$#" -gt 0 ]; then exec "$app" "$@"; fi
if [ -t 0 ] && [ -t 1 ]; then exec "$app"; fi

run_konsole() {
    # konsole 25.x restores a remembered window size and ignores geometry flags
    # unless restore is disabled. A throwaway XDG home whose konsolerc sets
    # RememberWindowSize=false makes it honour --qwindowgeometry; pixels are
    # derived from the grid (~8 px/col, ~18 px/row + frame pad).
    d=$(mktemp -d) || return 1
    mkdir -p "$d/cfg" "$d/data/konsole"
    printf '[KonsoleWindow]\nRememberWindowSize=false\n' > "$d/cfg/konsolerc"
    printf '[General]\nName=BundleSetup\nTerminalColumns=%s\nTerminalRows=%s\n' \
        "$cols" "$rows" > "$d/data/konsole/BundleSetup.profile"
    px_w=$((cols * 8 + 31)); px_h=$((rows * 18 + 4))
    # The throwaway XDG home is konsole's alone — the wrapped app must see the
    # real one, or it would derive its data dir under /tmp. Snapshot the real
    # values FIRST (a command-prefix can't both override XDG_* and read its old
    # value — the override wins), then restore them in konsole's child shell.
    REAL_XDG_CONFIG_HOME="$XDG_CONFIG_HOME"; REAL_XDG_DATA_HOME="$XDG_DATA_HOME"
    export REAL_XDG_CONFIG_HOME REAL_XDG_DATA_HOME
    # konsole connects only stdin to the pty under `-e`; stdout/stderr are left
    # disconnected. The TUI writes its frames to stdout and probes it for a tty
    # (styling), so without this the wizard renders to a dead fd and the window
    # looks empty / flashes. Point stdout+stderr at the controlling terminal
    # (/dev/tty inside konsole) so the form actually appears.
    XDG_CONFIG_HOME="$d/cfg" XDG_DATA_HOME="$d/data" \
        konsole --separate --hide-menubar --profile BundleSetup \
        --qwindowgeometry "${px_w}x${px_h}" -e /bin/sh -c '
            if [ -n "$REAL_XDG_CONFIG_HOME" ]; then export XDG_CONFIG_HOME="$REAL_XDG_CONFIG_HOME"; else unset XDG_CONFIG_HOME; fi
            if [ -n "$REAL_XDG_DATA_HOME" ]; then export XDG_DATA_HOME="$REAL_XDG_DATA_HOME"; else unset XDG_DATA_HOME; fi
            unset REAL_XDG_CONFIG_HOME REAL_XDG_DATA_HOME
            exec "$1" >/dev/tty 2>&1' sh "$app"
    rc=$?; rm -rf "$d"; return $rc
}

# macOS: drive Terminal.app via AppleScript — open a window, run the app, poll until
# the tab's shell exits, then close the window (no dead "[Process completed]").
#
# The double window: launching Terminal.app cold (it wasn't already running) makes
# it auto-open a blank default window; a bare `do script` then opens ANOTHER for the
# wizard — two windows. Disposing the blank one after the fact is racy: its login
# shell may still be mid-startup (sourcing /etc/profile → path_helper), so `exit`
# queues behind that and the close prompts "terminate -bash (2), path_helper?".
#
# So on a cold launch we don't open a second window at all — we run the wizard IN the
# blank window the launch just made. It's safe to target precisely because we know
# Terminal wasn't running (captured BEFORE we touch it): the front window is ours,
# not some shell the user already had open (which is why blind `in window 1` was
# wrong). `do script … in <that window>` waits for its shell prompt before injecting,
# so no startup race. On a warm launch there's no blank window, so a plain `do script`
# opens the one window we need. Either way exactly one window, and it ends via
# `; exit` so its close is prompt-free.
#
# We DON'T size the window here. Terminal.app honours the CSI "8t" window-resize
# escape, so the wizard sizes itself to its own grid on entry (FormSpec.ResizeOnEnter
# on darwin) — the same mechanism Windows uses. Driving the size from here instead is
# unreliable (AppleScript's `set number of columns` lands on the wrong/unsettled
# window or a profile pins it) and a throwaway `.terminal` profile would import itself
# into the user's Terminal preferences. Letting the app resize itself needs neither.
if [ "$(uname -s)" = Darwin ]; then
    osascript \
      -e "set wasRunning to application \"Terminal\" is running" \
      -e "tell application \"Terminal\"" \
      -e "  activate" \
      -e "  set cmd to quoted form of \"$app\" & \"; exit\"" \
      -e "  if wasRunning then" \
      -e "    set w to do script cmd" \
      -e "  else" \
      -e "    set w to do script cmd in front window" \
      -e "  end if" \
      -e "  set win to (first window whose tabs contains w)" \
      -e "  repeat" \
      -e "    delay 0.2" \
      -e "    if not (busy of w) then" \
      -e "      if (count of (processes of w)) is 0 then exit repeat" \
      -e "    end if" \
      -e "  end repeat" \
      -e "  try" \
      -e "    close win saving no" \
      -e "  end try" \
      -e "end tell"
    exit $?
fi

for t in konsole gnome-terminal xfce4-terminal ptyxis kitty alacritty xterm; do
    command -v "$t" >/dev/null 2>&1 || continue
    case "$t" in
        konsole)        run_konsole; exit $? ;;
        gnome-terminal) exec "$t" --geometry="${cols}x${rows}" -- "$app" ;;
        xfce4-terminal) exec "$t" --geometry="${cols}x${rows}" -x "$app" ;;
        ptyxis)         exec "$t" -- "$app" ;;
        kitty)          exec "$t" -o initial_window_width=${cols}c -o initial_window_height=${rows}c "$app" ;;
        alacritty)      exec "$t" -o window.dimensions.columns=$cols -o window.dimensions.lines=$rows -e "$app" ;;
        xterm)          exec "$t" -geometry "${cols}x${rows}" -e "$app" ;;
    esac
done
command -v xmessage >/dev/null 2>&1 && exec xmessage "No terminal emulator found."
echo "No terminal emulator found." >&2; exit 1
