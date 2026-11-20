// Package safety is CleanCuts's keep-out collision gate (ADR-012): it
// verifies that the tool's envelope — the sweep of its tip disc, not just
// its centre — never enters a keep-out zone (vise jaws, edge clamps). A
// violation is a hard error naming the move and the zone; the safe failure
// is refusing the path, never emitting one that clips a clamp.
//
// The test is exact 2D geometry at the move's height: a zone applies iff
// its Z range covers that height, and its footprint is the box projection
// (rectangle) or the cylinder disc. A linear move sweeps a capsule; an arc
// move sweeps an annular sector. See docs/workholding.md.
package safety

import (
	"fmt"
	"math"

	"github.com/kutis96/cleancuts/internal/kernel"
	"github.com/kutis96/cleancuts/internal/moves"
	"github.com/kutis96/cleancuts/internal/planning"
)

// contactTol is the margin (mm) by which "touching" counts as colliding:
// the safe side is to refuse tangent contact with a clamp.
const contactTol = 1e-9

// CheckChain reports whether the tool-centre chain at height z, with tip
// radius r, clears every keep-out zone. It returns a precise error naming
// the first offending edge and zone. A chain that touches a zone (distance
// ≤ r) is a collision.
func CheckChain(c kernel.Chain2, z, r float64, zones []planning.KeepOut) error {
	if r < 0 {
		return fmt.Errorf("safety: tool radius must be >= 0 (got %g)", r)
	}
	for i, e := range c.Edges {
		if err := checkEdge(e, z, r, zones); err != nil {
			return fmt.Errorf("safety: chain edge %d: %w", i, err)
		}
	}
	return nil
}

// CheckPath reports whether every planar move of a finished path clears the
// keep-out zones at that move's height. Vertical (plunge/retract) moves are
// tested as a point against the zones whose Z range overlaps the travel.
func CheckPath(p moves.Path, r float64, zones []planning.KeepOut) error {
	for i, m := range p.Moves {
		if err := checkMove(m, r, zones); err != nil {
			return fmt.Errorf("safety: path %q move %d: %w", p.ID, i, err)
		}
	}
	return nil
}

// checkMove gates one move.
func checkMove(m moves.Move, r float64, zones []planning.KeepOut) error {
	switch {
	case m.From.Z == m.To.Z: // planar move at a single height
		switch {
		case m.Kind == moves.KindArcCCW || m.Kind == moves.KindArcCW:
			a := kernel.NewArc2(kernel.P2(m.Center.X, m.Center.Y),
				kernel.P2(m.From.X, m.From.Y), kernel.P2(m.To.X, m.To.Y), m.CW)
			return checkArc(a, m.From.Z, r, zones)
		default:
			return checkSegment(kernel.P2(m.From.X, m.From.Y), kernel.P2(m.To.X, m.To.Y), m.From.Z, r, zones)
		}
	default: // vertical move: a point at (X,Y) through a Z interval
		lo, hi := m.From.Z, m.To.Z
		if lo > hi {
			lo, hi = hi, lo
		}
		pt := kernel.P2(m.From.X, m.From.Y)
		for _, zn := range zones {
			if zOverlap(zn, lo, hi) && pointInZone(pt, zn) {
				return fmt.Errorf("vertical move at (%g, %g) through Z %g..%g lies in keep-out %s",
					pt.X, pt.Y, lo, hi, zoneName(zn))
			}
		}
		return nil
	}
}

// checkEdge gates one chain edge.
func checkEdge(e kernel.Edge2, z, r float64, zones []planning.KeepOut) error {
	if e.IsLine() {
		l, _ := e.AsLine()
		return checkSegment(l.Start, l.End, z, r, zones)
	}
	a, _ := e.AsArc()
	return checkArc(a, z, r, zones)
}

// checkSegment reports whether a capsule of radius r about segment AB (at
// height z) stays clear of the zones.
func checkSegment(a, b kernel.Point2, z, r float64, zones []planning.KeepOut) error {
	for _, zn := range zones {
		if !zoneApplies(zn, z) {
			continue
		}
		d := distSegmentZone(a, b, zn)
		if d < r+contactTol {
			return fmt.Errorf("tool of radius %g at Z=%g enters keep-out %s (clearance %g < %g)",
				r, z, zoneName(zn), d, r)
		}
	}
	return nil
}

// checkArc reports whether an annular-sector sweep of the arc by a tip disc
// of radius r (at height z) stays clear of the zones.
func checkArc(a kernel.Arc2, z, r float64, zones []planning.KeepOut) error {
	for _, zn := range zones {
		if !zoneApplies(zn, z) {
			continue
		}
		d := distArcZone(a, zn)
		if d < r+contactTol {
			return fmt.Errorf("tool of radius %g at Z=%g enters keep-out %s (clearance %g < %g)",
				r, z, zoneName(zn), d, r)
		}
	}
	return nil
}

// zoneApplies reports whether the zone's Z range covers height z.
func zoneApplies(zn planning.KeepOut, z float64) bool {
	switch {
	case zn.Box != nil:
		return z >= zn.Box.Min.Z && z <= zn.Box.Max.Z
	case zn.Cyl != nil:
		return z >= zn.Cyl.ZMin && z <= zn.Cyl.ZMax
	}
	return false
}

// zOverlap reports whether the zone's Z range intersects [lo, hi].
func zOverlap(zn planning.KeepOut, lo, hi float64) bool {
	switch {
	case zn.Box != nil:
		return hi >= zn.Box.Min.Z && lo <= zn.Box.Max.Z
	case zn.Cyl != nil:
		return hi >= zn.Cyl.ZMin && lo <= zn.Cyl.ZMax
	}
	return false
}

// pointInZone reports whether the XY point is in the zone's footprint.
func pointInZone(p kernel.Point2, zn planning.KeepOut) bool {
	switch {
	case zn.Box != nil:
		return p.X >= zn.Box.Min.X && p.X <= zn.Box.Max.X &&
			p.Y >= zn.Box.Min.Y && p.Y <= zn.Box.Max.Y
	case zn.Cyl != nil:
		return p.Dist(zn.Cyl.Center) <= zn.Cyl.Radius
	}
	return false
}

func zoneName(zn planning.KeepOut) string {
	if zn.Name != "" {
		return fmt.Sprintf("%q", zn.Name)
	}
	return "<unnamed>"
}

// --- exact 2D distances (the analytic core of the gate) -------------------

// distSegmentZone returns the distance from segment AB to the zone's
// footprint (rectangle or disc).
func distSegmentZone(a, b kernel.Point2, zn planning.KeepOut) float64 {
	if zn.Box != nil {
		return distSegmentRect(a, b, xy(zn.Box.Min), xy(zn.Box.Max))
	}
	return distSegmentDisc(a, b, zn.Cyl.Center, zn.Cyl.Radius)
}

// distArcZone returns the distance from the arc to the zone's footprint.
func distArcZone(a kernel.Arc2, zn planning.KeepOut) float64 {
	if zn.Box != nil {
		return distArcRect(a, xy(zn.Box.Min), xy(zn.Box.Max))
	}
	return distArcDisc(a, zn.Cyl.Center, zn.Cyl.Radius)
}

// xy projects a WCS point to the XY plane.
func xy(p kernel.Point3) kernel.Point2 { return kernel.P2(p.X, p.Y) }

// distPointRect is the distance from a point to a rectangle (solid).
func distPointRect(p, mn, mx kernel.Point2) float64 {
	dx := math.Max(0, math.Max(mn.X-p.X, p.X-mx.X))
	dy := math.Max(0, math.Max(mn.Y-p.Y, p.Y-mx.Y))
	return math.Hypot(dx, dy)
}

// distPointDisc is the distance from a point to a disc (solid).
func distPointDisc(p, c kernel.Point2, r float64) float64 {
	return math.Max(0, p.Dist(c)-r)
}

// distPointSegment is the distance from a point to a finite segment.
func distPointSegment(p, a, b kernel.Point2) float64 {
	ab := b.Sub(a)
	l2 := ab.Dot(ab)
	if l2 < kernel.AbsTol {
		return p.Dist(a)
	}
	t := (p.Sub(a)).Dot(ab) / l2
	t = math.Min(1, math.Max(0, t))
	return p.Dist(a.Add(ab.Scale(t)))
}

// footOnSegment returns the point of segment AB closest to p.
func footOnSegment(p, a, b kernel.Point2) kernel.Point2 {
	ab := b.Sub(a)
	l2 := ab.Dot(ab)
	if l2 < kernel.AbsTol {
		return a
	}
	t := (p.Sub(a)).Dot(ab) / l2
	t = math.Min(1, math.Max(0, t))
	return a.Add(ab.Scale(t))
}

// segIntersectsRect reports whether segment AB crosses the rectangle.
// Slab method (Liang–Barsky).
func segIntersectsRect(a, b, mn, mx kernel.Point2) bool {
	dx, dy := b.X-a.X, b.Y-a.Y
	t0, t1 := 0.0, 1.0
	for _, p := range []struct {
		p, d, lo, hi float64
	}{
		{a.X, dx, mn.X, mx.X},
		{a.Y, dy, mn.Y, mx.Y},
	} {
		if math.Abs(p.d) < kernel.AbsTol {
			if p.p < p.lo || p.p > p.hi {
				return false
			}
			continue
		}
		tA := (p.lo - p.p) / p.d
		tB := (p.hi - p.p) / p.d
		if tA > tB {
			tA, tB = tB, tA
		}
		t0 = math.Max(t0, tA)
		t1 = math.Min(t1, tB)
		if t0 > t1 {
			return false
		}
	}
	return true
}

// distSegmentRect is the exact distance between a segment and a rectangle.
func distSegmentRect(a, b, mn, mx kernel.Point2) float64 {
	if segIntersectsRect(a, b, mn, mx) {
		return 0
	}
	best := distPointRect(a, mn, mx)
	if d := distPointRect(b, mn, mx); d < best {
		best = d
	}
	for _, v := range []kernel.Point2{
		kernel.P2(mn.X, mn.Y), kernel.P2(mx.X, mn.Y),
		kernel.P2(mn.X, mx.Y), kernel.P2(mx.X, mx.Y),
	} {
		if d := distPointSegment(v, a, b); d < best {
			best = d
		}
	}
	return best
}

// distSegmentDisc is the exact distance between a segment and a disc.
func distSegmentDisc(a, b, c kernel.Point2, r float64) float64 {
	// min_{q on AB, p in disc} |q-p| = max(0, dist(c, AB) - r)
	return math.Max(0, distPointSegment(c, a, b)-r)
}

// angleInSpan reports whether the point's angle about the arc's centre lies
// within the arc's swept span (endpoints inclusive).
func angleInSpan(a kernel.Arc2, p kernel.Point2) bool {
	theta := kernel.NormalizeAngle(math.Atan2(p.Y-a.Center.Y, p.X-a.Center.X))
	var d float64
	if a.CW {
		d = kernel.NormalizeAngle(a.A0 - theta)
	} else {
		d = kernel.NormalizeAngle(theta - a.A0)
	}
	return d <= a.Sweep()+1e-9
}

// distPointArc is the exact distance from a point to an arc.
func distPointArc(p kernel.Point2, a kernel.Arc2) float64 {
	best := math.Min(p.Dist(a.Start()), p.Dist(a.End()))
	if angleInSpan(a, p) {
		if d := math.Abs(p.Dist(a.Center) - a.Radius); d < best {
			best = d
		}
	}
	return best
}

// distSegmentArc is the exact distance between a segment and an arc: the
// closest pair is either (segment endpoint, arc), (segment, arc endpoint),
// or the radial point at the foot of the perpendicular from the centre —
// whichever the span admits.
func distSegmentArc(a, b kernel.Point2, arc kernel.Arc2) float64 {
	// Exact intersection: kernel's finite line–arc test.
	le, _ := kernel.NewLineEdge(a, b)
	l, _ := le.AsLine()
	if len(kernel.LineArcIntersect(l, arc)) > 0 {
		return 0
	}
	best := math.Min(distPointArc(a, arc), distPointArc(b, arc))
	if d := distPointSegment(arc.Start(), a, b); d < best {
		best = d
	}
	if d := distPointSegment(arc.End(), a, b); d < best {
		best = d
	}
	foot := footOnSegment(arc.Center, a, b)
	if angleInSpan(arc, foot) {
		if d := math.Abs(foot.Dist(arc.Center) - arc.Radius); d < best {
			best = d
		}
	}
	return best
}

// distArcRect is the exact distance between an arc and a rectangle: an arc
// through the rectangle's interior crosses a side (→ 0); otherwise the
// closest pair lies on one of the four sides or at an arc endpoint.
func distArcRect(arc kernel.Arc2, mn, mx kernel.Point2) float64 {
	sides := [4][2]kernel.Point2{
		{kernel.P2(mn.X, mn.Y), kernel.P2(mx.X, mn.Y)},
		{kernel.P2(mx.X, mn.Y), kernel.P2(mx.X, mx.Y)},
		{kernel.P2(mn.X, mx.Y), kernel.P2(mx.X, mx.Y)},
		{kernel.P2(mx.X, mx.Y), kernel.P2(mn.X, mn.Y)},
	}
	best := math.Min(distPointRect(arc.Start(), mn, mx), distPointRect(arc.End(), mn, mx))
	for _, s := range sides {
		if d := distSegmentArc(s[0], s[1], arc); d < best {
			best = d
		}
	}
	return best
}

// distArcDisc is the exact distance between an arc and a disc.
//
//	min_{p on arc, q in disc} |p−q| = max(0, min_{p on arc} |p−c| − r)
//
// and the arc point nearest the disc centre c is either an arc endpoint or
// the radial point in c's direction — if that direction lies in the span.
func distArcDisc(arc kernel.Arc2, c kernel.Point2, r float64) float64 {
	m := math.Min(c.Dist(arc.Start()), c.Dist(arc.End()))
	if angleInSpan(arc, c) {
		if d := math.Abs(c.Dist(arc.Center) - arc.Radius); d < m {
			m = d
		}
	}
	return math.Max(0, m-r)
}
