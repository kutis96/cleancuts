package tool

import (
	"math"
	"testing"
)

func TestTipRadiusFlatEndMill(t *testing.T) {
	tm := Tool{ID: "flat", Kind: KindFlatEndMill, Diameter: 10, FluteLength: 20}
	cases := []struct {
		name   string
		height float64
		want   float64
	}{
		{"at the tip", 0, 5},
		{"mid flute", 10, 5},
		{"at the flute end", 20, 5},
		{"above the flute", 25, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tm.TipRadius(tc.height)
			if err != nil {
				t.Fatal(err)
			}
			if math.Abs(got-tc.want) > 1e-9 {
				t.Fatalf("TipRadius(%g) = %g, want %g", tc.height, got, tc.want)
			}
		})
	}
	if _, err := tm.TipRadius(-1); err == nil {
		t.Fatalf("negative height must be an error")
	}
}

func TestTipRadiusBallNose(t *testing.T) {
	tm := Tool{ID: "ball", Kind: KindBallNose, Diameter: 10}
	// Sphere radius 5 centred at height 5: r(h) = sqrt(25 − (5−h)²).
	cases := []struct {
		height float64
		want   float64
	}{
		{0, 0}, // the tip point
		{2.5, math.Sqrt(25 - 6.25)},
		{5, 5}, // equator
		{10, 0},
		{12, 0}, // above the ball
	}
	for _, tc := range cases {
		got, err := tm.TipRadius(tc.height)
		if err != nil {
			t.Fatal(err)
		}
		if math.Abs(got-tc.want) > 1e-9 {
			t.Fatalf("TipRadius(%g) = %g, want %g", tc.height, got, tc.want)
		}
	}
}

func TestTipRadiusBullNose(t *testing.T) {
	// D = 10 (R=5), corner radius 2: flat radius R-cr = 3 from the tip up to
	// height 3, then the torus corner (radius 2, centred at (3,5)) up to 5.
	tm := Tool{ID: "bull", Kind: KindBullNose, Diameter: 10, CornerRadius: 2}
	cases := []struct {
		height float64
		want   float64
	}{
		{0, 3},
		{3, 3}, // end of the flat
		{4, 3 + math.Sqrt(4-1)},
		{5, 5}, // top edge
		{6, 0},
	}
	for _, tc := range cases {
		got, err := tm.TipRadius(tc.height)
		if err != nil {
			t.Fatal(err)
		}
		if math.Abs(got-tc.want) > 1e-9 {
			t.Fatalf("TipRadius(%g) = %g, want %g", tc.height, got, tc.want)
		}
	}
}

func TestTipRadiusTaper(t *testing.T) {
	// Half-angle 45°: radius grows as height · tan(45°) = height.
	tm := Tool{ID: "taper", Kind: KindTaperEndMill, TaperAngle: 45}
	for _, tc := range []struct{ h, want float64 }{{0, 0}, {3, 3}, {10, 10}} {
		got, err := tm.TipRadius(tc.h)
		if err != nil {
			t.Fatal(err)
		}
		if math.Abs(got-tc.want) > 1e-9 {
			t.Fatalf("TipRadius(%g) = %g, want %g", tc.h, got, tc.want)
		}
	}
	// A zero-angle "taper" is a nonsense config; refuse, don't guess.
	bad := Tool{ID: "bad", Kind: KindTaperEndMill, TaperAngle: 0}
	if _, err := bad.TipRadius(1); err == nil {
		t.Fatalf("zero taper angle must be an error")
	}
}

func TestTipRadiusUnsupportedKinds(t *testing.T) {
	for _, kind := range []Kind{KindDrill, KindChamfer, KindFaceMill} {
		tm := Tool{ID: "x", Kind: kind, Diameter: 10}
		if _, err := tm.TipRadius(1); err == nil {
			t.Fatalf("kind %q must be refused, not guessed", kind)
		}
	}
}
