package windowstate

// Rect is a desktop work area in logical pixels, mirroring the subset of the
// framework's screen geometry this package reasons about.
type Rect struct {
	X      int
	Y      int
	Width  int
	Height int
}

// visibilityMargin is the minimum overlap, in each axis, that a restored
// window must keep with a visible work area. It is sized so the title bar
// stays grabbable and the window can always be dragged back on-screen.
const visibilityMargin = 60

// RestorePlan describes how to open the window given the persisted state.
type RestorePlan struct {
	// Frame holds the size (already clamped to the minimum) and, when
	// UsePosition is true, the position to open at.
	Frame Frame
	// UsePosition reports whether Frame.X/Frame.Y carry a usable position.
	// When false the window is placed by the platform default.
	UsePosition bool
	// Maximised opens the window maximised.
	Maximised bool
}

// PlanRestore decides the initial window frame. Without usable persisted
// state it returns defaults; otherwise it clamps the saved size to the
// configured minimum and passes the saved position through for validation
// against the screen the window actually lands on (see Reconcile).
func PlanRestore(saved State, ok bool, defaults, min Frame) RestorePlan {
	if !ok {
		return RestorePlan{Frame: defaults}
	}
	width, height := clampSize(saved.Frame.Width, saved.Frame.Height, min.Width, min.Height)
	return RestorePlan{
		Frame:       Frame{X: saved.Frame.X, Y: saved.Frame.Y, Width: width, Height: height},
		UsePosition: true,
		Maximised:   saved.Maximised,
	}
}

func clampSize(width, height, minWidth, minHeight int) (int, int) {
	if width < minWidth {
		width = minWidth
	}
	if height < minHeight {
		height = minHeight
	}
	return width, height
}

// planAdjustment decides corrections for a frame that has already landed on a
// screen: a size larger than the work area is clamped (to at least minFrame),
// and needsCenter reports whether the frame is not visibly on the work area
// and should be recentered instead.
func planAdjustment(frame Frame, area Rect, minFrame Frame) (adjusted Frame, needsCenter bool) {
	adjusted = frame
	if adjusted.Width > area.Width || adjusted.Height > area.Height {
		adjusted.Width, adjusted.Height = min(adjusted.Width, area.Width), min(adjusted.Height, area.Height)
		adjusted.Width, adjusted.Height = clampSize(adjusted.Width, adjusted.Height, minFrame.Width, minFrame.Height)
	}
	return adjusted, !visible(adjusted, area)
}

// visible reports whether the frame keeps at least visibilityMargin of
// overlap with the work area in both axes, so enough of the window (including
// its title bar) is on-screen to see and drag.
func visible(frame Frame, area Rect) bool {
	return axisVisible(frame.X, frame.X+frame.Width, area.X, area.X+area.Width) &&
		axisVisible(frame.Y, frame.Y+frame.Height, area.Y, area.Y+area.Height)
}

func axisVisible(start, end, areaStart, areaEnd int) bool {
	overlap := min(end, areaEnd) - max(start, areaStart)
	return overlap >= visibilityMargin
}
