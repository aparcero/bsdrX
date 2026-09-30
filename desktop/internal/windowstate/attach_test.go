package windowstate

import (
	"path/filepath"
	"testing"
	"time"
)

// fakeWindow is a stand-in for *application.WebviewWindow's frame read-back.
type fakeWindow struct {
	x, y, width, height   int
	maximised, fullscreen bool
}

func (f *fakeWindow) Position() (int, int) { return f.x, f.y }
func (f *fakeWindow) Size() (int, int)     { return f.width, f.height }
func (f *fakeWindow) IsMaximised() bool    { return f.maximised }
func (f *fakeWindow) IsFullscreen() bool   { return f.fullscreen }

func newTestPersister(t *testing.T, source frameSource, debounce time.Duration) *persister {
	t.Helper()
	return &persister{
		source:   source,
		path:     filepath.Join(t.TempDir(), stateFilename),
		min:      Frame{Width: 640, Height: 520},
		debounce: debounce,
	}
}

func loadState(t *testing.T, path string) (State, bool) {
	t.Helper()
	state, ok, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	return state, ok
}

func TestPersisterFlushWritesObservedFrame(t *testing.T) {
	window := &fakeWindow{x: 10, y: 20, width: 920, height: 680}
	p := newTestPersister(t, window, time.Hour)

	p.record(nil)
	p.flush(nil)

	state, ok := loadState(t, p.path)
	if !ok {
		t.Fatal("flush() wrote nothing after an observation")
	}
	want := State{Frame: Frame{X: 10, Y: 20, Width: 920, Height: 680}}
	if state != want {
		t.Fatalf("persisted state = %+v, want %+v", state, want)
	}
}

func TestPersisterFlushWithoutObservationWritesNothing(t *testing.T) {
	window := &fakeWindow{}
	p := newTestPersister(t, window, time.Hour)

	p.flush(nil)

	if _, ok := loadState(t, p.path); ok {
		t.Fatal("flush() wrote state without any window observation")
	}
}

func TestPersisterKeepsNormalFrameWhileMaximised(t *testing.T) {
	window := &fakeWindow{x: 10, y: 20, width: 920, height: 680}
	p := newTestPersister(t, window, time.Hour)

	p.record(nil)
	window.maximised = true
	window.x, window.y, window.width, window.height = 0, 0, 1920, 1040
	p.record(nil)
	p.flush(nil)

	state, ok := loadState(t, p.path)
	if !ok {
		t.Fatal("flush() wrote nothing")
	}
	want := State{Frame: Frame{X: 10, Y: 20, Width: 920, Height: 680}, Maximised: true}
	if state != want {
		t.Fatalf("persisted state = %+v, want the pre-maximise frame %+v", state, want)
	}
}

func TestPersisterClampsBelowMinimumSize(t *testing.T) {
	window := &fakeWindow{x: 0, y: 0, width: 300, height: 200}
	p := newTestPersister(t, window, time.Hour)

	p.record(nil)
	p.flush(nil)

	state, _ := loadState(t, p.path)
	if state.Frame.Width != 640 || state.Frame.Height != 520 {
		t.Fatalf("persisted frame = %+v, want size clamped to the minimum 640x520", state.Frame)
	}
}

func TestPersisterWritesAfterDebounceQuiescence(t *testing.T) {
	window := &fakeWindow{x: 0, y: 0, width: 920, height: 680}
	p := newTestPersister(t, window, 10*time.Millisecond)

	p.record(nil)
	window.x, window.y = 40, 30
	p.record(nil)

	deadline := time.Now().Add(2 * time.Second)
	for {
		state, ok := loadState(t, p.path)
		if ok {
			if state.Frame.X != 40 || state.Frame.Y != 30 {
				t.Fatalf("persisted frame = %+v, want the latest observation", state.Frame)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("debounced save did not land within 2s of quiescence")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestPersisterStopsWritingAfterClose(t *testing.T) {
	window := &fakeWindow{x: 10, y: 20, width: 920, height: 680}
	p := newTestPersister(t, window, 10*time.Millisecond)

	p.record(nil)
	p.flush(nil)

	window.x, window.y = 999, 999
	p.record(nil)
	time.Sleep(50 * time.Millisecond)

	state, _ := loadState(t, p.path)
	if state.Frame.X == 999 {
		t.Fatal("persister wrote after close")
	}
}
