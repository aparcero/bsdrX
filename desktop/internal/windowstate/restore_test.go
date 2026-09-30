package windowstate

import "testing"

var planMin = Frame{Width: 640, Height: 520}

func TestPlanRestoreWithoutSavedStateUsesDefaults(t *testing.T) {
	defaults := Frame{Width: 920, Height: 680}

	plan := PlanRestore(State{}, false, defaults, planMin)

	if plan.Frame != defaults {
		t.Fatalf("plan frame = %+v, want defaults %+v", plan.Frame, defaults)
	}
	if plan.UsePosition {
		t.Fatal("plan UsePosition = true without saved state")
	}
	if plan.Maximised {
		t.Fatal("plan Maximised = true without saved state")
	}
}

func TestPlanRestorePassesSavedFrameThrough(t *testing.T) {
	saved := State{Frame: Frame{X: -1920, Y: 40, Width: 800, Height: 600}, Maximised: true}

	plan := PlanRestore(saved, true, Frame{Width: 920, Height: 680}, planMin)

	want := Frame{X: -1920, Y: 40, Width: 800, Height: 600}
	if plan.Frame != want {
		t.Fatalf("plan frame = %+v, want %+v", plan.Frame, want)
	}
	if !plan.UsePosition {
		t.Fatal("plan UsePosition = false with a saved position")
	}
	if !plan.Maximised {
		t.Fatal("plan Maximised = false with a saved maximised state")
	}
}

func TestPlanRestoreClampsSizeToMinimum(t *testing.T) {
	saved := State{Frame: Frame{X: 100, Y: 50, Width: 300, Height: 200}}

	plan := PlanRestore(saved, true, Frame{Width: 920, Height: 680}, planMin)

	want := Frame{X: 100, Y: 50, Width: planMin.Width, Height: planMin.Height}
	if plan.Frame != want {
		t.Fatalf("plan frame = %+v, want %+v", plan.Frame, want)
	}
}

func TestVisible(t *testing.T) {
	area := Rect{X: 0, Y: 0, Width: 1920, Height: 1040}
	tests := []struct {
		name  string
		frame Frame
		want  bool
	}{
		{"inside the work area", Frame{X: 100, Y: 100, Width: 920, Height: 680}, true},
		{"on another disconnected monitor", Frame{X: 2500, Y: 100, Width: 920, Height: 680}, false},
		{"exactly the visibility margin visible", Frame{X: area.Width - visibilityMargin, Y: 100, Width: 920, Height: 680}, true},
		{"less than the visibility margin visible", Frame{X: area.Width - 40, Y: 100, Width: 920, Height: 680}, false},
		{"title bar above the screen", Frame{X: 100, Y: -700, Width: 920, Height: 680}, false},
		{"below the work area", Frame{X: 100, Y: 1500, Width: 920, Height: 680}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := visible(tt.frame, area); got != tt.want {
				t.Fatalf("visible(%+v) = %v, want %v", tt.frame, got, tt.want)
			}
		})
	}
}

func TestPlanAdjustmentKeepsHealthyFrames(t *testing.T) {
	area := Rect{X: 0, Y: 0, Width: 1920, Height: 1040}
	frame := Frame{X: 100, Y: 100, Width: 920, Height: 680}

	adjusted, needsCenter := planAdjustment(frame, area, planMin)

	if adjusted != frame {
		t.Fatalf("adjusted = %+v, want unchanged %+v", adjusted, frame)
	}
	if needsCenter {
		t.Fatal("needsCenter = true for a fully visible frame")
	}
}

func TestPlanAdjustmentClampsOversizedFrames(t *testing.T) {
	area := Rect{X: 0, Y: 0, Width: 1280, Height: 720}
	frame := Frame{X: 0, Y: 0, Width: 1920, Height: 1080}

	adjusted, needsCenter := planAdjustment(frame, area, planMin)

	want := Frame{X: 0, Y: 0, Width: area.Width, Height: area.Height}
	if adjusted != want {
		t.Fatalf("adjusted = %+v, want %+v", adjusted, want)
	}
	if needsCenter {
		t.Fatal("needsCenter = true for an oversized but visible frame")
	}
}

func TestPlanAdjustmentRecentersInvisibleFrames(t *testing.T) {
	area := Rect{X: 0, Y: 0, Width: 1920, Height: 1040}
	frame := Frame{X: 2500, Y: 100, Width: 920, Height: 680}

	adjusted, needsCenter := planAdjustment(frame, area, planMin)

	if adjusted != frame {
		t.Fatalf("adjusted = %+v, want size untouched %+v", adjusted, frame)
	}
	if !needsCenter {
		t.Fatal("needsCenter = false for a frame outside the work area")
	}
}

func TestPlanAdjustmentOnScreenSmallerThanMinimum(t *testing.T) {
	area := Rect{X: 0, Y: 0, Width: 600, Height: 400}
	frame := Frame{X: 0, Y: 0, Width: 920, Height: 680}

	adjusted, _ := planAdjustment(frame, area, planMin)

	want := Frame{X: 0, Y: 0, Width: planMin.Width, Height: planMin.Height}
	if adjusted != want {
		t.Fatalf("adjusted = %+v, want clamped to minimum %+v", adjusted, want)
	}
}
