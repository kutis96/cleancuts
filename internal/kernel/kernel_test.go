package kernel

import (
	"math"
	"testing"
)

// feq reports whether a and b agree within rel+abs relative tolerance,
// suitable for asserting on derived floating point results in tests.
func feq(t *testing.T, name string, got, want float64) {
	t.Helper()
	if !Equal(got, want, 1e-9) {
		t.Errorf("%s = %v, want %v", name, got, want)
	}
}

func TestVec2Arithmetic(t *testing.T) {
	cases := []struct {
		name string
		a, b Vec2
		want Vec2
		op   func(Vec2, Vec2) Vec2
	}{
		{"add", V2(1, 2), V2(3, 4), V2(4, 6), func(a, b Vec2) Vec2 { return a.Add(b) }},
		{"sub", V2(3, 4), V2(1, 2), V2(2, 2), func(a, b Vec2) Vec2 { return a.Sub(b) }},
		{"scale", V2(2, 3), V2(1, 1), V2(4, 6), func(a, _ Vec2) Vec2 { return a.Scale(2) }},
	}
	for _, c := range cases {
		got := c.op(c.a, c.b)
		if !got.Eq(c.want, AbsTol) {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

func TestVec2DotCross(t *testing.T) {
	feq(t, "dot", V2(1, 2).Dot(V2(3, 4)), 11)
	feq(t, "cross ccw", V2(1, 0).Cross(V2(0, 1)), 1)
	feq(t, "cross cw", V2(0, 1).Cross(V2(1, 0)), -1)
}

func TestVec2NormPerpRotate(t *testing.T) {
	got := V2(3, 4).Len()
	feq(t, "len", got, 5)

	n := V2(3, 4).Norm()
	feq(t, "norm len", n.Len(), 1)

	lp := V2(1, 0).LeftPerp()
	if !lp.Eq(V2(0, 1), AbsTol) {
		t.Errorf("leftPerp = %v want (0,1)", lp)
	}
	rp := V2(1, 0).RightPerp()
	if !rp.Eq(V2(0, -1), AbsTol) {
		t.Errorf("rightPerp = %v want (0,-1)", rp)
	}

	r := V2(1, 0).Rotate(math.Pi / 2)
	if !r.Eq(V2(0, 1), 1e-9) {
		t.Errorf("rotate 90deg = %v want ~(0,1)", r)
	}
}

func TestPoint2(t *testing.T) {
	a, b := P2(1, 2), P2(4, 6)
	if !a.Add(V2(1, 1)).Eq(P2(2, 3), AbsTol) {
		t.Error("add failed")
	}
	if !b.Sub(a).Eq(V2(3, 4), AbsTol) {
		t.Error("sub failed")
	}
	feq(t, "dist", a.Dist(b), 5)
	if !a.Eq(P2(1, 2), AbsTol) {
		t.Error("eq same failed")
	}
	if a.Eq(b, AbsTol) {
		t.Error("eq distinct should be false")
	}
}

func TestVec3Point3(t *testing.T) {
	a, b := V3(1, 2, 2), V3(2, 2, 1)
	feq(t, "dot", a.Dot(b), 1*2+2*2+2*1)
	c := a.Cross(b)
	want := V3(2*1-2*2, 2*2-1*1, 1*2-2*2)
	if !c.Eq(want, AbsTol) {
		t.Errorf("cross = %v want %v", c, want)
	}

	p, q := P3(0, 0, 0), P3(1, 2, 2)
	feq(t, "dist", p.Dist(q), math.Sqrt(1+4+4))
}

func TestLine2(t *testing.T) {
	l := NewLine2(P2(0, 0), P2(2, 0))
	feq(t, "len", l.Len(), 2)
	if !l.Midpoint().Eq(P2(1, 0), AbsTol) {
		t.Error("midpoint failed")
	}
	if !l.PointAt(0.25).Eq(P2(0.5, 0), AbsTol) {
		t.Error("pointAt failed")
	}
	// Clamping.
	if !l.PointAt(1.5).Eq(P2(2, 0), AbsTol) {
		t.Error("pointAt clamp failed")
	}
}

func TestArc2CCW(t *testing.T) {
	a := Arc2{Center: P2(0, 0), Radius: 1, A0: 0, A1: math.Pi / 2, CW: false}
	if !a.Start().Eq(P2(1, 0), 1e-9) {
		t.Errorf("start = %v want (1,0)", a.Start())
	}
	if !a.End().Eq(P2(0, 1), 1e-9) {
		t.Errorf("end = %v want (0,1)", a.End())
	}
	feq(t, "sweep", a.Sweep(), math.Pi/2)
	feq(t, "len", a.Len(), math.Pi/2)
	if !a.PointAt(0.5).Eq(P2(math.Sqrt2/2, math.Sqrt2/2), 1e-9) {
		t.Errorf("pointAt(0.5) = %v want (~0.707,~0.707)", a.PointAt(0.5))
	}
}

func TestArc2CW(t *testing.T) {
	a := Arc2{Center: P2(0, 0), Radius: 1, A0: math.Pi / 2, A1: 0, CW: true}
	if !a.Start().Eq(P2(0, 1), 1e-9) {
		t.Errorf("start = %v want (0,1)", a.Start())
	}
	if !a.End().Eq(P2(1, 0), 1e-9) {
		t.Errorf("end = %v want (1,0)", a.End())
	}
	feq(t, "len", a.Len(), math.Pi/2)
	if !a.PointAt(0.5).Eq(P2(math.Sqrt2/2, math.Sqrt2/2), 1e-9) {
		t.Errorf("pointAt(0.5) = %v", a.PointAt(0.5))
	}
}

func TestNewArc2(t *testing.T) {
	a := NewArc2(P2(0, 0), P2(1, 0), P2(0, 1), false)
	feq(t, "radius", a.Radius, 1)
	if !a.Start().Eq(P2(1, 0), 1e-9) || !a.End().Eq(P2(0, 1), 1e-9) {
		t.Errorf("endpoints wrong: start %v end %v", a.Start(), a.End())
	}
}

func TestNormalize(t *testing.T) {
	feq(t, "norm0", NormalizeAngle(0), 0)
	feq(t, "norm2pi", NormalizeAngle(2*math.Pi), 0)
	feq(t, "normneg", NormalizeAngle(-math.Pi/2), 3*math.Pi/2)
	feq(t, "pi", NormalizePi(math.Pi/2), math.Pi/2)
	feq(t, "pi wrap", NormalizePi(3*math.Pi/2), -math.Pi/2)
}

func TestClampEqualFinite(t *testing.T) {
	if Clamp(5, 0, 3) != 3 || Clamp(-1, 0, 3) != 0 || Clamp(2, 0, 3) != 2 {
		t.Error("clamp failed")
	}
	if !Equal(1, 1+1e-12, 1e-9) {
		t.Error("equal should be true")
	}
	if Equal(1, 2, 1e-9) {
		t.Error("equal should be false")
	}
	if !IsFinite(1.234) {
		t.Error("finite failed")
	}
	if IsFinite(math.NaN()) || IsFinite(math.Inf(1)) {
		t.Error("nan/inf should not be finite")
	}
}

func TestLineLineIntersect(t *testing.T) {
	a := NewLine2(P2(0, 0), P2(1, 0))
	b := NewLine2(P2(0, 1), P2(0, -1)) // vertical x=0
	p, ok := LineLineIntersect(a, b)
	if !ok {
		t.Fatal("expected intersection")
	}
	if !p.Eq(P2(0, 0), AbsTol) {
		t.Errorf("intersection = %v want (0,0)", p)
	}

	par := NewLine2(P2(0, 1), P2(1, 1))
	if _, ok := LineLineIntersect(a, par); ok {
		t.Error("parallel lines should not intersect")
	}
}

func TestSegmentsIntersect(t *testing.T) {
	cross := []Line2{
		NewLine2(P2(0, 0), P2(1, 1)),
		NewLine2(P2(0, 1), P2(1, 0)),
	}
	if !SegmentsIntersect(cross[0], cross[1]) {
		t.Error("crossing segments should intersect")
	}

	parallel := []Line2{
		NewLine2(P2(0, 0), P2(1, 0)),
		NewLine2(P2(0, 1), P2(1, 1)),
	}
	if SegmentsIntersect(parallel[0], parallel[1]) {
		t.Error("parallel segments should not intersect")
	}

	touch := []Line2{
		NewLine2(P2(0, 0), P2(1, 0)),
		NewLine2(P2(1, 0), P2(1, 1)),
	}
	if !SegmentsIntersect(touch[0], touch[1]) {
		t.Error("touching-endpoint segments should intersect")
	}
}
