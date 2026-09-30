package windowstate

// tracker accumulates live window observations during a session. It keeps the
// last normal frame seen while not maximised or fullscreen, so a session that
// ends maximised still restores the frame the user had before maximising, and
// fullscreen geometry never overwrites the normal frame.
type tracker struct {
	normal    Frame
	maximised bool
	observed  bool
}

func (t *tracker) observe(frame Frame, maximised, fullscreen bool) {
	if fullscreen {
		return
	}
	t.observed = true
	t.maximised = maximised
	// Tiling window managers can maximise before any normal-size event. Keep a
	// usable initial frame in that case instead of persisting a zero-size state
	// that Load would discard on the next launch.
	if !maximised || t.normal.Width <= 0 || t.normal.Height <= 0 {
		t.normal = frame
	}
}

// snapshot returns the state to persist. It reports ok=false until a real
// window event has been observed, which keeps headless server-mode runs from
// overwriting a previous launch's state with placeholder geometry.
func (t *tracker) snapshot() (State, bool) {
	if !t.observed {
		return State{}, false
	}
	return State{Frame: t.normal, Maximised: t.maximised}, true
}
