package term

// spinnerFrames are braille dots — smooth, single-cell, widely rendered.
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// Spinner is a single-line spinner for a slow step, drawn on its Phase's
// transient row: Tick advances and redraws the frame in place, Finish replaces the
// row with a "<mark> <label>" result. When styling is off it prints a plain
// "<label>..." once and no frames, so a non-TTY run isn't spammed with carriage
// returns. Not goroutine-safe; drive it from one goroutine.
type Spinner struct {
	p     *Phase
	label string
	frame int
	done  bool
}

// Spinner starts a spinner for label on p's row, drawing the first frame
// immediately so the line appears before the work starts. Call Finish exactly
// once; p.Close() covers an early return that never gets there.
//
// The label can't carry the phase's centering itself — the frame and the final
// mark are drawn in front of it, and they change the row's width — so the row is
// centered by the phase, like every other row it prints.
func (p *Phase) Spinner(label string) *Spinner {
	s := &Spinner{p: p, label: label}
	if !StylingEnabled() {
		p.Line(label + "...")
		return s
	}
	s.draw()
	return s
}

// Tick advances to the next frame and redraws the row. Cheap; call it in a loop or
// between sub-steps. No-op when styling is off or already finished.
func (s *Spinner) Tick() {
	if s.done || !StylingEnabled() {
		return
	}
	s.frame = (s.frame + 1) % len(spinnerFrames)
	s.draw()
}

// Finish replaces the spinner row with the step's result: OKMark+label on success,
// FailMark+label otherwise. Call exactly once.
func (s *Spinner) Finish(success bool) {
	if s.done {
		return
	}
	s.done = true
	mark := OKMark()
	if !success {
		mark = FailMark()
	}
	s.p.Line(mark + s.label)
}

func (s *Spinner) draw() {
	s.p.transientRow(Cyan(spinnerFrames[s.frame]) + " " + s.label)
}
