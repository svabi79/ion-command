package motion

import "testing"

func TestHoverHoldsPreviousCourse(t *testing.T) {
	got := BlendCourse(90, 180, 0.4, 12)
	if got != 12 {
		t.Fatalf("hover must hold previous course, got %v", got)
	}
}

func TestSlowUsesTrack(t *testing.T) {
	got := BlendCourse(40, 90, 10, 0)
	if got != 40 {
		t.Fatalf("slow aircraft must use reported track, got %v", got)
	}
}

func TestFastUsesChord(t *testing.T) {
	got := BlendCourse(10, 80, 30, 0)
	if got != 80 {
		t.Fatalf("cruise must use chord, got %v", got)
	}
}

func TestSlewCapAndIgnoredTurn(t *testing.T) {
	if TurnRateDegS(0, 90, 3, 1) != 0 {
		t.Fatal("turn rate below 10 kt must be ignored")
	}
	got := ApplySlew(0, 90, 10, 1)
	if got > 13 {
		t.Fatalf("low-speed slew cap ~12 deg/s, got %v", got)
	}
}

func TestCoastWindow(t *testing.T) {
	if CoastSeconds(0) != GraceSeconds {
		t.Fatalf("full grace expected")
	}
	if CoastSeconds(90) != 0 {
		t.Fatalf("grace expired")
	}
	if CoastSeconds(400) != 0 {
		t.Fatalf("hard coast expired")
	}
}
