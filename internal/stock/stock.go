// Package stock models the finite solid of material the part is milled
// from (ADR-015). Stock is config data (Law 6): a portable JSON document —
// a box, a vertical cylinder, or a prism (a closed 2D footprint of lines
// and arcs, extruded over a Z range) — all in the canonical WCS (Z-up, mm,
// ADR-001).
//
// The package answers the three questions the rest of the pipeline needs
// (docs/stock.md):
//
//   - SectionAt(z): the closed XY boundary of the material at height z;
//   - TopAt(x, y):  the stock's top height above (x, y) — a plunge must
//     start above it (Law 7);
//   - Contains(p):  whether a WCS point is inside the material.
//
// Fits is a convenience consumer of those primitives: the "does the part
// fit this stock" gate the generate command runs before planning.
//
// The package depends only on kernel, keeps no mutable state, and never
// re-frames coordinates (Law 1). A mesh stock is a later extension behind
// the same SectionAt/TopAt/Contains surface (ADR-015).
package stock

import (
	"fmt"

	"github.com/kutis96/cleancuts/internal/kernel"
)

// Shape names the solid kind a Stock describes. Exactly one matching
// geometry field is set; Validate enforces the pairing.
type Shape string

const (
	// ShapeBox: an axis-aligned box given by opposite corners.
	ShapeBox Shape = "box"
	// ShapeCylinder: a vertical cylinder (centre XY, radius, Z range).
	ShapeCylinder Shape = "cylinder"
	// ShapePrism: a closed 2D footprint extruded over a Z range.
	ShapePrism Shape = "prism"
)

// Box is an axis-aligned solid box in the canonical WCS (mm).
type Box struct {
	Min kernel.Point3 `json:"min"`
	Max kernel.Point3 `json:"max"`
}

// Cylinder is a vertical cylinder: centre in the XY plane, radius, Z range.
type Cylinder struct {
	Center kernel.Point2 `json:"center"`
	Radius float64       `json:"radius"`
	MinZ   float64       `json:"min_z"`
	MaxZ   float64       `json:"max_z"`
}

// LineEdge is a straight footprint edge.
type LineEdge struct {
	Start kernel.Point2 `json:"start"`
	End   kernel.Point2 `json:"end"`
}

// ArcEdge is a circular footprint arc (never a full circle — ADR-014).
type ArcEdge struct {
	Center kernel.Point2 `json:"center"`
	Start  kernel.Point2 `json:"start"`
	End    kernel.Point2 `json:"end"`
	CW     bool          `json:"cw"`
}

// Edge is one prism-footprint edge: exactly one of Line or Arc.
type Edge struct {
	Line *LineEdge `json:"line,omitempty"`
	Arc  *ArcEdge  `json:"arc,omitempty"`
}

// Prism is a closed 2D footprint (lines and arcs) extruded over a Z range.
type Prism struct {
	Edges []Edge  `json:"edges"`
	MinZ  float64 `json:"min_z"`
	MaxZ  float64 `json:"max_z"`
}

// Stock is the solid of material a part is milled from. Exactly one of
// Box, Cylinder, and Prism is set, matching Shape.
type Stock struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`

	Shape Shape `json:"shape"`

	Box      *Box      `json:"box,omitempty"`
	Cylinder *Cylinder `json:"cylinder,omitempty"`
	Prism    *Prism    `json:"prism,omitempty"`
}

// Validate enforces the stock invariants: an ID, exactly one geometry
// field, and a Shape that names it (Law 5).
func (s Stock) Validate() error {
	if s.ID == "" {
		return fmt.Errorf("stock: id is required")
	}
	set := 0
	for _, b := range []bool{s.Box != nil, s.Cylinder != nil, s.Prism != nil} {
		if b {
			set++
		}
	}
	if set != 1 {
		return fmt.Errorf("stock %q: needs exactly one of box, cylinder, prism (found %d)", s.ID, set)
	}
	switch {
	case s.Box != nil:
		if s.Shape != ShapeBox {
			return shapeMismatch(s.ID, s.Shape, ShapeBox)
		}
		return validateBox(s.ID, s.Box)
	case s.Cylinder != nil:
		if s.Shape != ShapeCylinder {
			return shapeMismatch(s.ID, s.Shape, ShapeCylinder)
		}
		return validateCylinder(s.ID, s.Cylinder)
	default:
		if s.Shape != ShapePrism {
			return shapeMismatch(s.ID, s.Shape, ShapePrism)
		}
		return validatePrism(s.ID, s.Prism)
	}
}

func shapeMismatch(id string, got, want Shape) error {
	return fmt.Errorf("stock %q: shape is %q but the %q geometry is set (set shape %q)", id, got, want, want)
}

func validateBox(id string, b *Box) error {
	if !b.Min.Finite() || !b.Max.Finite() {
		return fmt.Errorf("stock %q: box corners must be finite (min %v, max %v)", id, b.Min, b.Max)
	}
	for _, n := range []struct {
		name     string
		min, max float64
	}{
		{"x", b.Min.X, b.Max.X},
		{"y", b.Min.Y, b.Max.Y},
		{"z", b.Min.Z, b.Max.Z},
	} {
		if n.min >= n.max {
			return fmt.Errorf("stock %q: box %s min (%g) must be < max (%g)", id, n.name, n.min, n.max)
		}
	}
	return nil
}

func validateCylinder(id string, c *Cylinder) error {
	if !c.Center.Finite() {
		return fmt.Errorf("stock %q: cylinder centre must be finite (got %v)", id, c.Center)
	}
	if !kernel.IsFinite(c.Radius) || c.Radius <= kernel.AbsTol {
		return fmt.Errorf("stock %q: cylinder radius must be > 0 (got %g)", id, c.Radius)
	}
	if !kernel.IsFinite(c.MinZ) || !kernel.IsFinite(c.MaxZ) {
		return fmt.Errorf("stock %q: cylinder z range must be finite (got %g..%g)", id, c.MinZ, c.MaxZ)
	}
	if c.MinZ >= c.MaxZ {
		return fmt.Errorf("stock %q: cylinder min_z (%g) must be < max_z (%g)", id, c.MinZ, c.MaxZ)
	}
	return nil
}

func validatePrism(id string, p *Prism) error {
	if !kernel.IsFinite(p.MinZ) || !kernel.IsFinite(p.MaxZ) {
		return fmt.Errorf("stock %q: prism z range must be finite (got %g..%g)", id, p.MinZ, p.MaxZ)
	}
	if p.MinZ >= p.MaxZ {
		return fmt.Errorf("stock %q: prism min_z (%g) must be < max_z (%g)", id, p.MinZ, p.MaxZ)
	}
	if len(p.Edges) < 3 {
		return fmt.Errorf("stock %q: prism footprint needs at least 3 edges (got %d)", id, len(p.Edges))
	}
	if _, err := prismChain(p); err != nil {
		return fmt.Errorf("stock %q: prism footprint: %v", id, err)
	}
	return nil
}

// toKernel converts one JSON footprint edge to the kernel contour
// primitive, enforcing exactly-one and the kernel's own invariants.
func (e Edge) toKernel() (kernel.Edge2, error) {
	switch {
	case e.Line != nil && e.Arc == nil:
		return kernel.NewLineEdge(e.Line.Start, e.Line.End)
	case e.Arc != nil && e.Line == nil:
		return kernel.NewArcEdge(e.Arc.Center, e.Arc.Start, e.Arc.End, e.Arc.CW)
	}
	return kernel.Edge2{}, fmt.Errorf("edge needs exactly one of line or arc")
}

// prismChain builds the kernel chain for a prism footprint.
func prismChain(p *Prism) (kernel.Chain2, error) {
	edges := make([]kernel.Edge2, len(p.Edges))
	for i, e := range p.Edges {
		ke, err := e.toKernel()
		if err != nil {
			return kernel.Chain2{}, fmt.Errorf("edge %d: %v", i, err)
		}
		edges[i] = ke
	}
	chain, err := kernel.NewChain2(edges)
	if err != nil {
		return kernel.Chain2{}, err
	}
	if !chain.IsClosed() {
		return kernel.Chain2{}, fmt.Errorf("footprint must be a closed contour (start %v, end %v)",
			chain.Start(), chain.End())
	}
	return chain, nil
}

// zRange returns the stock's Z span. It validates the stock first.
func (s Stock) zRange() (minZ, maxZ float64, err error) {
	if err := s.Validate(); err != nil {
		return 0, 0, err
	}
	switch {
	case s.Box != nil:
		return s.Box.Min.Z, s.Box.Max.Z, nil
	case s.Cylinder != nil:
		return s.Cylinder.MinZ, s.Cylinder.MaxZ, nil
	default:
		return s.Prism.MinZ, s.Prism.MaxZ, nil
	}
}

// SectionAt returns the closed XY boundary of the stock's material at
// height z, oriented counter-clockwise (positive SignedArea) so the
// material interior is on the left of the travel. It is the single source
// of truth for "what material exists at this height" (ADR-015): planning,
// the safety gate, and later roughing call this instead of re-deriving
// geometry.
//
// z must lie within the stock's Z span (inclusive); otherwise the error
// says where the stock spans.
func (s Stock) SectionAt(z float64) (kernel.Chain2, error) {
	zmin, zmax, err := s.zRange()
	if err != nil {
		return kernel.Chain2{}, err
	}
	if z < zmin-kernel.AbsTol || z > zmax+kernel.AbsTol {
		return kernel.Chain2{}, fmt.Errorf("stock %q: no material at z=%g (stock spans z=%g..%g)",
			s.ID, z, zmin, zmax)
	}
	switch {
	case s.Box != nil:
		b := s.Box
		corners := []kernel.Point2{
			kernel.P2(b.Min.X, b.Min.Y),
			kernel.P2(b.Max.X, b.Min.Y),
			kernel.P2(b.Max.X, b.Max.Y),
			kernel.P2(b.Min.X, b.Max.Y),
		}
		return rectangle(corners)
	case s.Cylinder != nil:
		c := s.Cylinder
		r := c.Radius
		east := kernel.P2(c.Center.X+r, c.Center.Y)
		west := kernel.P2(c.Center.X-r, c.Center.Y)
		// A circle is two half-arcs — an arc is never a full circle (ADR-014).
		e1, err := kernel.NewArcEdge(c.Center, east, west, false)
		if err != nil {
			return kernel.Chain2{}, err
		}
		e2, err := kernel.NewArcEdge(c.Center, west, east, false)
		if err != nil {
			return kernel.Chain2{}, err
		}
		return kernel.NewChain2([]kernel.Edge2{e1, e2})
	default:
		chain, err := prismChain(s.Prism)
		if err != nil {
			return kernel.Chain2{}, err
		}
		if chain.SignedArea() < 0 {
			chain = chain.Reversed() // canonical CCW output
		}
		return chain, nil
	}
}

// rectangle builds a CCW rectangle chain from min, x-max, y-max, x-min
// corner order (any consistent CCW order works; the caller supplies it).
func rectangle(corners []kernel.Point2) (kernel.Chain2, error) {
	edges := make([]kernel.Edge2, len(corners))
	for i, c := range corners {
		edges[i], _ = kernel.NewLineEdge(c, corners[(i+1)%len(corners)])
	}
	return kernel.NewChain2(edges)
}

// TopAt returns the top Z of the stock above the point (x, y). For the v1
// shapes (extrusions) this is the stock's Z-max; a mesh stock will return
// the true surface height behind this same call (ADR-015). A plunge must
// start above this value: a plunge below it digs a crater in material that
// should be left (Law 7).
func (s Stock) TopAt(x, y float64) (float64, error) {
	_, zmax, err := s.zRange()
	if err != nil {
		return 0, err
	}
	if !s.inFootprint(x, y) {
		return 0, fmt.Errorf("stock %q: point (%g, %g) is outside the stock footprint", s.ID, x, y)
	}
	return zmax, nil
}

// inFootprint reports whether (x, y) is inside or on the stock's XY
// footprint. Boundary counts as inside — "is there material here" is
// answered on the safe side.
func (s Stock) inFootprint(x, y float64) bool {
	switch {
	case s.Box != nil:
		b := s.Box
		return x >= b.Min.X-kernel.AbsTol && x <= b.Max.X+kernel.AbsTol &&
			y >= b.Min.Y-kernel.AbsTol && y <= b.Max.Y+kernel.AbsTol
	case s.Cylinder != nil:
		return kernel.P2(x, y).Dist(s.Cylinder.Center) <= s.Cylinder.Radius+kernel.AbsTol
	default:
		chain, err := prismChain(s.Prism)
		if err != nil {
			return false
		}
		p := kernel.P2(x, y)
		return kernel.PointInContour(p, chain) || kernel.PointToChainDist(p, chain) < kernel.AbsTol
	}
}

// Contains reports whether the WCS point p lies in the stock's material
// (inside or on the boundary — the safe reading for "is there material
// here"). An invalid stock contains nothing.
func (s Stock) Contains(p kernel.Point3) bool {
	if !p.Finite() {
		return false
	}
	zmin, zmax, err := s.zRange()
	if err != nil {
		return false
	}
	return p.Z >= zmin-kernel.AbsTol && p.Z <= zmax+kernel.AbsTol && s.inFootprint(p.X, p.Y)
}

// Fits is the "does this setup make sense" gate (ADR-015): the part's
// closed contour at height partZ must lie inside the stock's section there
// — every vertex and every edge midpoint inside or on the section
// boundary — and the part's top (partTop) must not be above the stock's
// top. It is a precise consumer of SectionAt, not a hidden assumption
// inside a generator.
//
// The vertex-and-midpoint test catches any contour point that leaves the
// material, including an arc bulging past a straight stock boundary. It is
// a sanity gate on the setup, not a substitute for the safety gate
// (ADR-012), which verifies the actual tool envelope.
func (s Stock) Fits(part kernel.Chain2, partZ, partTop float64) error {
	if err := s.Validate(); err != nil {
		return err
	}
	if err := part.Validate(); err != nil {
		return fmt.Errorf("stock %q: part contour: %v", s.ID, err)
	}
	if !part.IsClosed() {
		return fmt.Errorf("stock %q: part contour is not closed (start %v, end %v)",
			s.ID, part.Start(), part.End())
	}
	zmin, zmax, _ := s.zRange() // validated above
	if partZ < zmin-kernel.AbsTol {
		return fmt.Errorf("stock %q: part at z=%g is below the stock (stock spans z=%g..%g)",
			s.ID, partZ, zmin, zmax)
	}
	if partZ > zmax+kernel.AbsTol {
		return fmt.Errorf("stock %q: part at z=%g is above the stock (stock spans z=%g..%g)",
			s.ID, partZ, zmin, zmax)
	}
	if partTop > zmax+kernel.AbsTol {
		return fmt.Errorf("stock %q: part top z=%g is above the stock top z=%g",
			s.ID, partTop, zmax)
	}
	section, err := s.SectionAt(partZ)
	if err != nil {
		return err
	}
	for _, e := range part.Edges {
		for _, p := range []kernel.Point2{e.Start(), e.PointAt(0.5)} {
			if !inOrOn(section, p) {
				return fmt.Errorf("stock %q: part point (%g, %g) at z=%g is outside the stock section",
					s.ID, p.X, p.Y, partZ)
			}
		}
	}
	return nil
}

// inOrOn reports whether p is strictly inside c or on its boundary
// (within the kernel tolerance) — the "inside the material, on the safe
// side" reading used by Fits.
func inOrOn(c kernel.Chain2, p kernel.Point2) bool {
	return kernel.PointInContour(p, c) || kernel.PointToChainDist(p, c) < kernel.AbsTol
}
