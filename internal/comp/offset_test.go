package comp

import (
	"errors"
	"math"
	"testing"

	"github.com/kutis96/cleancuts/internal/kernel"
)

func TestOffsetEdgeLine(t *testing.T) {
	// Bottom edge of a CCW square: (0,0) -> (10,0). Interior is +Y.
	e, _ := kernel.NewLineEdge(kernel.P2(0, 0), kernel.P2(10, 0))

	t.Run("outside offset goes away from interior", func(t *testing.T) {
		ne, err := offsetEdge(e, 2, Outside, true)
		if err != nil {
			t.Fatal(err)
		}
		l, _ := ne.AsLine()
		if !l.Start.Eq(kernel.P2(0, -2), 1e-9) || !l.End.Eq(kernel.P2(10, -2), 1e-9) {
			t.Fatalf("offset = %v -> %v, want (0,-2) -> (10,-2)", l.Start, l.End)
		}
	})
	t.Run("inside offset goes into interior", func(t *testing.T) {
		ne, err := offsetEdge(e, 2, Inside, true)
		if err != nil {
			t.Fatal(err)
		}
		l, _ := ne.AsLine()
		if !l.Start.Eq(kernel.P2(0, 2), 1e-9) || !l.End.Eq(kernel.P2(10, 2), 1e-9) {
			t.Fatalf("offset = %v -> %v, want (0,2) -> (10,2)", l.Start, l.End)
		}
	})

	// The same physical edge traversed the other way (CW contour): outside
	// must land on the SAME side — orientation must not flip the cut.
	eRev, _ := kernel.NewLineEdge(kernel.P2(10, 0), kernel.P2(0, 0))
	ne, err := offsetEdge(eRev, 2, Outside, false)
	if err != nil {
		t.Fatal(err)
	}
	l, _ := ne.AsLine()
	if !l.Start.Eq(kernel.P2(10, -2), 1e-9) || !l.End.Eq(kernel.P2(0, -2), 1e-9) {
		t.Fatalf("reversed outside offset = %v -> %v, want (10,-2) -> (0,-2)", l.Start, l.End)
	}
}

func TestOffsetEdgeArc(t *testing.T) {
	// A CCW contour whose boundary includes a CCW quarter arc (a convex
	// bump): centre (0,0), from (10,0) to (0,10), R=10.
	bump, err := kernel.NewArcEdge(kernel.P2(0, 0), kernel.P2(10, 0), kernel.P2(0, 10), false)
	if err != nil {
		t.Fatal(err)
	}
	t.Run("outside offset of convex arc is R+r", func(t *testing.T) {
		ne, err := offsetEdge(bump, 2, Outside, true)
		if err != nil {
			t.Fatal(err)
		}
		a, _ := ne.AsArc()
		if !kernel.EqualTol(a.Radius, 12) {
			t.Fatalf("radius = %g, want 12", a.Radius)
		}
		if !a.Center.Eq(kernel.P2(0, 0), 1e-9) {
			t.Fatalf("centre = %v, want (0,0)", a.Center)
		}
		if a.CW {
			t.Fatalf("direction must be preserved (CCW)")
		}
	})
	t.Run("inside offset of convex arc is R-r", func(t *testing.T) {
		ne, err := offsetEdge(bump, 2, Inside, true)
		if err != nil {
			t.Fatal(err)
		}
		a, _ := ne.AsArc()
		if !kernel.EqualTol(a.Radius, 8) {
			t.Fatalf("radius = %g, want 8", a.Radius)
		}
	})

	// A concave notch in a CCW contour: a semicircular bite from the top
	// edge, centre (5,10), R=2, running (7,10) -> (5,8) -> (3,10), which
	// sweeps CW — the opposite sense to the contour. Outside is the side
	// away from the material, i.e. INTO the notch, so it shrinks the radius.
	notch, err := kernel.NewArcEdge(kernel.P2(5, 10), kernel.P2(7, 10), kernel.P2(3, 10), true)
	if err != nil {
		t.Fatal(err)
	}
	t.Run("outside offset of concave arc is R-r", func(t *testing.T) {
		ne, err := offsetEdge(notch, 1, Outside, true)
		if err != nil {
			t.Fatal(err)
		}
		a, _ := ne.AsArc()
		if !kernel.EqualTol(a.Radius, 1) {
			t.Fatalf("radius = %g, want 1", a.Radius)
		}
		if !a.Center.Eq(kernel.P2(5, 10), 1e-9) {
			t.Fatalf("centre = %v, want (5,10)", a.Center)
		}
	})
	t.Run("inside offset of concave arc is R+r", func(t *testing.T) {
		ne, err := offsetEdge(notch, 1, Inside, true)
		if err != nil {
			t.Fatal(err)
		}
		a, _ := ne.AsArc()
		if !kernel.EqualTol(a.Radius, 3) {
			t.Fatalf("radius = %g, want 3", a.Radius)
		}
	})
}

func TestOffsetEdgeArcTooLarge(t *testing.T) {
	// A convex arc R=5 with an inside offset of r=5 degenerates (R-r = 0).
	bump, err := kernel.NewArcEdge(kernel.P2(0, 0), kernel.P2(5, 0), kernel.P2(0, 5), false)
	if err != nil {
		t.Fatal(err)
	}
	_, err = offsetEdge(bump, 5, Inside, true)
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("err = %v, want ErrTooLarge", err)
	}
	_, err = offsetEdge(bump, 6, Inside, true)
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("err = %v, want ErrTooLarge", err)
	}
	// A concave notch R=5 with an outside offset of r=6 also degenerates:
	// the tool simply does not fit in the notch.
	notch, _ := kernel.NewArcEdge(kernel.P2(5, 10), kernel.P2(10, 10), kernel.P2(0, 10), true)
	if _, err := offsetEdge(notch, 6, Outside, true); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("err = %v, want ErrTooLarge", err)
	}
}

func TestOffsetOptionsValidate(t *testing.T) {
	cases := []struct {
		name    string
		o       Options
		wantErr bool
	}{
		{"valid", Options{Radius: 5, Direction: Outside, Join: JoinMitre}, false},
		{"zero radius", Options{Radius: 0}, true},
		{"negative radius", Options{Radius: -1}, true},
		{"nan radius", Options{Radius: math.NaN()}, true},
		{"bad direction", Options{Radius: 1, Direction: 7}, true},
		{"bad join", Options{Radius: 1, Join: 99}, true},
		{"negative mitre limit", Options{Radius: 1, MitreLimit: -1}, true},
		{"negative fillet radius", Options{Radius: 1, FilletRadius: -1}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.o.validate(); (err != nil) != tc.wantErr {
				t.Fatalf("validate err = %v, wantErr = %v", err, tc.wantErr)
			}
		})
	}
}

func TestOffsetDefaults(t *testing.T) {
	o := Options{Radius: 4}
	if got := o.mitreLimitMM(); got != DefaultMitreLimit*4 {
		t.Fatalf("default mitre limit = %g, want %g", got, DefaultMitreLimit*4)
	}
	if got := o.filletRadius(); got != 4 {
		t.Fatalf("default fillet radius = %g, want 4", got)
	}
	o.MitreLimit = 3
	o.FilletRadius = 7
	if got := o.mitreLimitMM(); got != 12 {
		t.Fatalf("mitre limit = %g, want 12", got)
	}
	if got := o.filletRadius(); got != 7 {
		t.Fatalf("fillet radius = %g, want 7", got)
	}
}
