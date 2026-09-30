package windowstate

import "testing"

func TestTrackerSnapshotWithoutObservation(t *testing.T) {
	var tr tracker

	if state, ok := tr.snapshot(); ok {
		t.Fatalf("snapshot() ok = true before any observation, state = %+v", state)
	}
}

func TestTrackerTracksNormalFrames(t *testing.T) {
	var tr tracker
	first := Frame{X: 10, Y: 20, Width: 920, Height: 680}
	second := Frame{X: 30, Y: 40, Width: 800, Height: 600}

	tr.observe(first, false, false)
	tr.observe(second, false, false)

	state, ok := tr.snapshot()
	if !ok {
		t.Fatal("snapshot() ok = false after observation")
	}
	if state.Frame != second {
		t.Fatalf("snapshot() frame = %+v, want the latest normal frame %+v", state.Frame, second)
	}
	if state.Maximised {
		t.Fatal("snapshot() maximised = true for a normal session")
	}
}

func TestTrackerKeepsLastNormalFrameWhileMaximised(t *testing.T) {
	var tr tracker
	normal := Frame{X: 10, Y: 20, Width: 920, Height: 680}
	maximised := Frame{X: 0, Y: 0, Width: 1920, Height: 1040}

	tr.observe(normal, false, false)
	tr.observe(maximised, true, false)

	state, ok := tr.snapshot()
	if !ok {
		t.Fatal("snapshot() ok = false after observation")
	}
	if state.Frame != normal {
		t.Fatalf("snapshot() frame = %+v, want the pre-maximise frame %+v", state.Frame, normal)
	}
	if !state.Maximised {
		t.Fatal("snapshot() maximised = false while the session ends maximised")
	}
}

func TestTrackerResumesNormalFramesAfterUnmaximise(t *testing.T) {
	var tr tracker
	before := Frame{X: 10, Y: 20, Width: 920, Height: 680}
	after := Frame{X: 50, Y: 60, Width: 700, Height: 500}

	tr.observe(before, false, false)
	tr.observe(Frame{X: 0, Y: 0, Width: 1920, Height: 1040}, true, false)
	tr.observe(after, false, false)

	state, _ := tr.snapshot()
	if state.Frame != after || state.Maximised {
		t.Fatalf("snapshot() = %+v, want frame %+v unmaximised", state, after)
	}
}

func TestTrackerIgnoresFullscreenFrames(t *testing.T) {
	var tr tracker

	tr.observe(Frame{X: 0, Y: 0, Width: 1920, Height: 1080}, false, true)
	if _, ok := tr.snapshot(); ok {
		t.Fatal("snapshot() ok = true after only a fullscreen observation")
	}

	normal := Frame{X: 10, Y: 20, Width: 920, Height: 680}
	tr.observe(normal, false, false)
	tr.observe(Frame{X: 0, Y: 0, Width: 1920, Height: 1080}, false, true)

	state, _ := tr.snapshot()
	if state.Frame != normal {
		t.Fatalf("snapshot() frame = %+v, want fullscreen geometry to leave the normal frame untouched at %+v", state.Frame, normal)
	}
}

func TestFirstMaximisedObservationHasUsableFrame(t *testing.T) {
	var state tracker
	state.observe(Frame{Width: 1600, Height: 1000}, true, false)
	saved, ok := state.snapshot()
	if !ok || saved.Frame.Width <= 0 || saved.Frame.Height <= 0 || !saved.Maximised {
		t.Fatalf("unrestorable maximised state: %+v", saved)
	}
}
