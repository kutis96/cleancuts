// Package planning is CleanCuts's stage-before-generation (ADR-009): for
// each closed contour it decides the milling mode (climb or conventional),
// orients the contour so that mode is achieved, and picks a start vertex
// that lies outside the keep-out zones. It changes only the *traversal* of
// a contour — never its geometry — so generation (toolpath) and the
// never-over-cut guarantee (comp) are untouched. See docs/planning.md.
package planning

import (
	"fmt"
	"math"

	"github.com/kutis96/cleancuts/internal/kernel"
	"github.com/kutis96/cleancuts/internal/moves"
)

// SpindleRotation is the spindle direction as viewed from above (looking
// down −Z). Empty means CCW — the common "right-handed" mill convention.
type SpindleRotation string

const (
	// RotationCCW: the cutter turns counter-clockwise seen from above.
	RotationCCW SpindleRotation = "ccw"
	// RotationCW: the cutter turns clockwise seen from above (left-handed).
	RotationCW SpindleRotation = "cw"
)

// SpindleCCW reports whether the rotation is CCW (the empty value defaults
// to the common right-handed convention).
func (r SpindleRotation) SpindleCCW() bool {
	return r != RotationCW
}

// FeatureKind names which side of the contour holds the kept material. It is
// explicit input — never inferred — because it flips both the offset side
// (comp) and the climb/conventional derivation.
type FeatureKind int

const (
	// Profile: the kept material is INSIDE the contour; the tool runs
	// outside it (material removed around the part).
	Profile FeatureKind = iota
	// Pocket: the kept material is OUTSIDE the contour; the tool runs
	// inside it (the contour bounds a pocket wall).
	Pocket
)

// String returns a human-readable name for logs and errors.
func (k FeatureKind) String() string {
	if k == Pocket {
		return "pocket"
	}
	return "profile"
}

// KeepOut is a workholding zone the tool centre must never enter (a vise
// jaw, an edge clamp). A zone applies at a height iff its Z range covers
// that height; at a given height its footprint is the box projection or the
// cylinder disc. See docs/workholding.md.
type KeepOut struct {
	// Name identifies the zone in errors ("clamps over tab A").
	Name string
	// Box is a solid box in the canonical WCS (mm).
	Box *Box
	// Cyl is a vertical cylinder (centre in XY, radius, Z range).
	Cyl *Cylinder
}

// Box is an axis-aligned solid box in the canonical WCS (mm).
type Box struct {
	Min kernel.Point3
	Max kernel.Point3
}

// Cylinder is a vertical cylinder: centre in the XY plane, radius, Z range.
type Cylinder struct {
	Center kernel.Point2
	Radius float64
	ZMin   float64
	ZMax   float64
}

// Validate enforces the invariants of a KeepOut.
func (k KeepOut) Validate() error {
	if (k.Box == nil) == (k.Cyl == nil) {
		return fmt.Errorf("planning: keep-out %q needs exactly one of box or cylinder", orNone(k.Name))
	}
	if k.Box != nil {
		for _, n := range []struct {
			name     string
			min, max float64
		}{
			{"x", k.Box.Min.X, k.Box.Max.X},
			{"y", k.Box.Min.Y, k.Box.Max.Y},
			{"z", k.Box.Min.Z, k.Box.Max.Z},
		} {
			if n.min >= n.max {
				return fmt.Errorf("planning: keep-out %q box %s min must be < max (got %g >= %g)", orNone(k.Name), n.name, n.min, n.max)
			}
		}
	}
	if k.Cyl != nil {
		if k.Cyl.Radius <= 0 {
			return fmt.Errorf("planning: keep-out %q cylinder radius must be > 0 (got %g)", orNone(k.Name), k.Cyl.Radius)
		}
		if k.Cyl.ZMin >= k.Cyl.ZMax {
			return fmt.Errorf("planning: keep-out %q cylinder z_min must be < z_max", orNone(k.Name))
		}
	}
	return nil
}

// orNone renders "" as "<unnamed>" for messages.
func orNone(name string) string {
	if name == "" {
		return "<unnamed>"
	}
	return name
}

// Contains reports whether the point (x, y at height z) lies in the zone's
// footprint: the zone applies only if its Z range covers z. Boundary points
// count as inside — keep-out is a hard constraint, and treating the boundary
// as clear is the safe side for a tool centre (the tool radius adds real
// clearance on top).
func (k KeepOut) Contains(p kernel.Point2, z float64) bool {
	switch {
	case k.Box != nil:
		if z < k.Box.Min.Z || z > k.Box.Max.Z {
			return false
		}
		return p.X >= k.Box.Min.X && p.X <= k.Box.Max.X &&
			p.Y >= k.Box.Min.Y && p.Y <= k.Box.Max.Y
	case k.Cyl != nil:
		if z < k.Cyl.ZMin || z > k.Cyl.ZMax {
			return false
		}
		return p.Dist(k.Cyl.Center) <= k.Cyl.Radius
	}
	return false
}

// Params configure a planning decision for one feature.
type Params struct {
	// Kind is Profile (material inside) or Pocket (material outside).
	Kind FeatureKind
	// Spindle is the spindle rotation, viewed from above.
	Spindle SpindleRotation
	// Milling is the requested mode; empty falls back to DefaultMilling.
	Milling moves.Milling
	// DefaultMilling is the fallback mode; empty means climb.
	DefaultMilling moves.Milling
	// CutZ is the cutting depth, used to test keep-out Z ranges.
	CutZ float64
	// KeepOuts are the workholding zones the start point must avoid.
	KeepOuts []KeepOut
}

// Plan is the decided traversal: the chain oriented and rotated so the
// requested milling mode holds and the entry avoids the keep-outs, plus the
// resulting mode. Generation (toolpath.Loop) consumes it directly.
type Plan struct {
	// Chain is oriented (climb/conventional achieved) and rotated to start
	// at the chosen vertex.
	Chain kernel.Chain2
	// Milling is the mode the oriented chain achieves (== the request).
	Milling moves.Milling
	// Start is the chosen entry vertex.
	Start kernel.Point2
}

// PlanFeature orients a closed contour to achieve the requested milling mode
// and picks a start vertex outside the keep-out zones (ADR-009, Law 7).
// The contour's geometry is untouched: only its direction and start change.
func PlanFeature(c kernel.Chain2, p Params) (Plan, error) {
	if !c.IsClosed() {
		return Plan{}, fmt.Errorf("planning: %s needs a closed contour (got an open chain)", p.Kind)
	}
	if err := c.Validate(); err != nil {
		return Plan{}, fmt.Errorf("planning: contour: %w", err)
	}
	area := c.SignedArea()
	if math.Abs(area) < 1e-6 {
		return Plan{}, fmt.Errorf("planning: contour is degenerate (area %g); cannot plan a traversal", area)
	}
	for i, k := range p.KeepOuts {
		if err := k.Validate(); err != nil {
			return Plan{}, fmt.Errorf("planning: keep-out %d: %w", i, err)
		}
	}

	mode := p.Milling
	if mode == "" {
		mode = p.DefaultMilling
	}
	if mode == "" {
		mode = moves.MillingClimb
	}
	switch mode {
	case moves.MillingClimb, moves.MillingConventional:
	default:
		return Plan{}, fmt.Errorf("planning: unknown milling mode %q (want climb or conventional)", mode)
	}

	// Orient to achieve the requested mode (docs/planning.md table):
	// climb is achieved iff ((kind == Pocket) == (spindle == CCW)) == ccw.
	ccw := c.SignedArea() > 0
	climbAchieved := ((p.Kind == Pocket) == p.Spindle.SpindleCCW()) == ccw
	chain := c
	if (climbAchieved && mode == moves.MillingClimb) ||
		(!climbAchieved && mode == moves.MillingConventional) {
		chain = c
	} else {
		chain = c.Reversed()
	}

	// Pick the entry vertex outside the keep-out zones (hard constraint).
	start, err := PickStart(chain, p.CutZ, p.KeepOuts)
	if err != nil {
		return Plan{}, err
	}
	chain, err = Rotate(chain, start)
	if err != nil {
		return Plan{}, err
	}
	return Plan{Chain: chain, Milling: mode, Start: start}, nil
}

// PickStart returns the first vertex (in chain order) that is not inside any
// keep-out zone applicable at height z. If every vertex is inside a zone, it
// refuses with the offending zone named — the safe failure is "no plan",
// never an entry routed through the workholding.
func PickStart(c kernel.Chain2, z float64, keepOuts []KeepOut) (kernel.Point2, error) {
	for _, e := range c.Edges {
		v := e.Start()
		if inside := zoneContaining(v, z, keepOuts); inside == nil {
			return v, nil
		}
	}
	if len(keepOuts) > 0 {
		zones := make([]string, 0, len(keepOuts))
		for _, k := range keepOuts {
			zones = append(zones, orNone(k.Name))
		}
		return kernel.Point2{}, fmt.Errorf(
			"planning: every vertex of the contour lies in a keep-out zone at Z=%g (%s); no safe entry point — move the fixture or use a different feature",
			z, joinNames(zones))
	}
	return kernel.Point2{}, fmt.Errorf("planning: contour has no vertices to enter from")
}

// zoneContaining returns the first keep-out zone containing p at height z,
// or nil.
func zoneContaining(p kernel.Point2, z float64, keepOuts []KeepOut) *KeepOut {
	for i := range keepOuts {
		if keepOuts[i].Contains(p, z) {
			return &keepOuts[i]
		}
	}
	return nil
}

func joinNames(names []string) string {
	switch len(names) {
	case 0:
		return "no zones"
	case 1:
		return names[0]
	case 2:
		return names[0] + " and " + names[1]
	}
	all := names[0]
	for _, n := range names[1:] {
		all += ", " + n
	}
	return all
}

// Rotate reorders a closed chain's edges cyclically so the chain starts at
// the vertex v (which must be one of its vertices). The edge sequence and
// orientation — and therefore the milling mode — are preserved.
func Rotate(c kernel.Chain2, v kernel.Point2) (kernel.Chain2, error) {
	if !c.IsClosed() {
		return kernel.Chain2{}, fmt.Errorf("planning: Rotate needs a closed chain")
	}
	n := len(c.Edges)
	for i := 0; i < n; i++ {
		if c.Edges[i].Start().Eq(v, kernel.AbsTol) {
			out := make([]kernel.Edge2, 0, n)
			for k := 0; k < n; k++ {
				out = append(out, c.Edges[(i+k)%n])
			}
			return kernel.NewChain2(out)
		}
	}
	return kernel.Chain2{}, fmt.Errorf("planning: %v is not a vertex of the chain", v)
}
