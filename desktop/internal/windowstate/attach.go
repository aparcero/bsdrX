package windowstate

import (
	"log"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// frameSource is the window behaviour the persistence layer reads. It is
// satisfied by *application.WebviewWindow and by fakes in tests.
type frameSource interface {
	Position() (int, int)
	Size() (int, int)
	IsMaximised() bool
	IsFullscreen() bool
}

// Attach persists the window frame to path for the next launch. Window move,
// resize, and state changes are recorded as they happen and written out after
// a short debounce, with a final flush when the window closes. Save failures
// are logged and never fatal: window state is a convenience, not data.
func Attach(window *application.WebviewWindow, path string, min Frame, debounce time.Duration) {
	p := &persister{source: window, path: path, min: min, debounce: debounce}
	for _, eventType := range []events.WindowEventType{
		events.Common.WindowDidMove,
		events.Common.WindowDidResize,
		events.Common.WindowMaximise,
		events.Common.WindowUnMaximise,
	} {
		window.OnWindowEvent(eventType, p.record)
	}
	window.OnWindowEvent(events.Common.WindowClosing, p.flush)
}

type persister struct {
	source   frameSource
	path     string
	min      Frame
	debounce time.Duration

	mu     sync.Mutex
	state  tracker
	timer  *time.Timer
	closed bool
}

func (p *persister) record(*application.WindowEvent) {
	x, y := p.source.Position()
	width, height := p.source.Size()
	maximised := p.source.IsMaximised()
	fullscreen := p.source.IsFullscreen()
	width, height = clampSize(width, height, p.min.Width, p.min.Height)

	p.mu.Lock()
	defer p.mu.Unlock()
	p.state.observe(Frame{X: x, Y: y, Width: width, Height: height}, maximised, fullscreen)
	if p.closed {
		return
	}
	if p.timer == nil {
		p.timer = time.AfterFunc(p.debounce, p.write)
	} else {
		p.timer.Reset(p.debounce)
	}
}

// flush performs the final synchronous write when the window closes. It is
// subscribed to the common WindowClosing event, which the framework maps
// from the platform close signals (including GTK's delete event on Linux).
func (p *persister) flush(*application.WindowEvent) {
	p.mu.Lock()
	p.closed = true
	if p.timer != nil {
		p.timer.Stop()
		p.timer = nil
	}
	state, ok := p.state.snapshot()
	p.mu.Unlock()
	if !ok {
		return
	}
	p.save(state)
}

// write is the debounce expiry callback.
func (p *persister) write() {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	state, ok := p.state.snapshot()
	p.mu.Unlock()
	if !ok {
		return
	}
	p.save(state)
}

func (p *persister) save(state State) {
	if err := Save(p.path, state); err != nil {
		log.Printf("window state: %v", err)
	}
}

// ReconcileWhenShown validates a restored placement once the window is live
// on a screen: the first show, move, or resize event triggers Reconcile. The
// event spread covers platforms that emit different startup events (WindowShow
// on Windows and macOS, load-completion move/resize on Linux).
func ReconcileWhenShown(window *application.WebviewWindow, min Frame) {
	var once sync.Once
	reconcile := func(*application.WindowEvent) {
		once.Do(func() { Reconcile(window, min) })
	}
	for _, eventType := range []events.WindowEventType{
		events.Common.WindowShow,
		events.Common.WindowDidMove,
		events.Common.WindowDidResize,
	} {
		window.OnWindowEvent(eventType, reconcile)
	}
}

// Reconcile checks the frame the window actually landed on against that
// screen's work area. A saved position that is no longer visible (for
// example, its monitor was disconnected) recenters the window, and a size
// larger than the work area is clamped. Windows without a screen (server
// mode) or currently maximised are left to the window manager.
func Reconcile(window *application.WebviewWindow, minFrame Frame) {
	if window.IsMaximised() {
		return
	}
	screen, err := window.GetScreen()
	if err != nil || screen == nil {
		return
	}
	area := Rect{
		X:      screen.WorkArea.X,
		Y:      screen.WorkArea.Y,
		Width:  screen.WorkArea.Width,
		Height: screen.WorkArea.Height,
	}

	x, y := window.Position()
	width, height := window.Size()
	frame := Frame{X: x, Y: y, Width: width, Height: height}

	adjusted, needsCenter := planAdjustment(frame, area, minFrame)
	if adjusted.Width != frame.Width || adjusted.Height != frame.Height {
		window.SetSize(adjusted.Width, adjusted.Height)
	}
	if needsCenter {
		window.Center()
	}
}
