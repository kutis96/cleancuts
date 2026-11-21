// Package fixture is CleanCuts's portable workholding model (ADR-012,
// ADR-019 §3): the keep-out zones a tool must never enter (vise jaws,
// edge clamps), as schema-backed JSON (schemas/fixture.schema.json is the
// contract, Law 6). KeepOuts maps the volumes to the planner's hard
// constraints (planning.KeepOut); the v1 mapping covers boxes and vertical
// cylinders — anything else is a precise refusal, because planning against
// a zone the safety gate cannot verify is a Law 7 violation.
//
// The package depends only on kernel and planning (for the shared KeepOut
// vocabulary), keeps no mutable state, and never re-frames coordinates
// (Law 1).
package fixture

import (
	"fmt"

	"github.com/kutis96/cleancuts/internal/kernel"
	"github.com/kutis96/cleancuts/internal/planning"
)

// VolumeKind names the shape of a workholding volume.
type VolumeKind string

const (
	// KindBox: an axis-aligned solid box.
	KindBox VolumeKind = "box"
	// KindCylinder: a finite cylinder (v1: vertical only, see KeepOuts).
	KindCylinder VolumeKind = "cylinder"
	// KindMesh: a complex fixture as a solid mesh file (v1: refused at
	// mapping — the gate cannot verify a mesh zone).
	KindMesh VolumeKind = "mesh"
)

// Box is an axis-aligned solid in the canonical WCS (mm).
type Box struct {
	Min kernel.Point3 `json:"min"`
	Max kernel.Point3 `json:"max"`
}

// Cylinder is a finite cylinder as the fixture schema defines it: base
// centre, axis direction, radius, length (canonical WCS, mm).
type Cylinder struct {
	Center kernel.Point3 `json:"center"`
	Axis   kernel.Point3 `json:"axis"`
	Radius float64       `json:"radius"`
	Length float64       `json:"length"`
}

// MeshVolume names a solid mesh file (STL/3MF) in the canonical frame.
type MeshVolume struct {
	File string `json:"file"`
}

// Volume is one workholding volume: exactly one of box / cylinder / mesh,
// named by Type (the schema's oneOf, mirrored as a Go invariant).
type Volume struct {
	Type VolumeKind  `json:"type"`
	Box  *Box        `json:"box,omitempty"`
	Cyl  *Cylinder   `json:"cylinder,omitempty"`
	Mesh *MeshVolume `json:"mesh,omitempty"`
	Note string      `json:"note,omitempty"`
}

// Fixture is the portable workholding config (ADR-012): KeepOut are the
// hard safety zones (the tool envelope must never enter them); Workholding
// is the fixture's own solids, for visualization and the phase-4 collision
// gate — the planner does not map them to constraints in v1.
type Fixture struct {
	ID          string   `json:"id"`
	Name        string   `json:"name,omitempty"`
	KeepOut     []Volume `json:"keep_out"`
	Workholding []Volume `json:"workholding"`
}

// Validate enforces the fixture invariants (Law 5): an ID, and each volume
// well-formed with exactly one geometry matching its Type.
func (f Fixture) Validate() error {
	if f.ID == "" {
		return fmt.Errorf("fixture: id is required")
	}
	for i, v := range f.KeepOut {
		if err := v.Validate(); err != nil {
			return fmt.Errorf("fixture %q: keep_out[%d]: %w", f.ID, i, err)
		}
	}
	for i, v := range f.Workholding {
		if err := v.Validate(); err != nil {
			return fmt.Errorf("fixture %q: workholding[%d]: %w", f.ID, i, err)
		}
	}
	return nil
}

// Validate enforces the volume invariants: a known Type with exactly one
// matching geometry field, and sane dimensions.
func (v Volume) Validate() error {
	geos := 0
	for _, set := range []bool{v.Box != nil, v.Cyl != nil, v.Mesh != nil} {
		if set {
			geos++
		}
	}
	switch v.Type {
	case KindBox:
		if v.Box == nil {
			return fmt.Errorf("volume: type box needs a box")
		}
		if geos != 1 {
			return fmt.Errorf("volume: a box volume must set exactly one geometry (found %d)", geos)
		}
		return validateBox(v.Box)
	case KindCylinder:
		if v.Cyl == nil {
			return fmt.Errorf("volume: type cylinder needs a cylinder")
		}
		if geos != 1 {
			return fmt.Errorf("volume: a cylinder volume must set exactly one geometry (found %d)", geos)
		}
		return validateCylinder(v.Cyl)
	case KindMesh:
		if v.Mesh == nil || v.Mesh.File == "" {
			return fmt.Errorf("volume: type mesh needs a mesh file")
		}
		if geos != 1 {
			return fmt.Errorf("volume: a mesh volume must set exactly one geometry (found %d)", geos)
		}
		return nil
	default:
		return fmt.Errorf("volume: unknown type %q (want box, cylinder, or mesh)", v.Type)
	}
}

func validateBox(b *Box) error {
	for _, n := range []struct {
		name     string
		min, max float64
	}{
		{"x", b.Min.X, b.Max.X},
		{"y", b.Min.Y, b.Max.Y},
		{"z", b.Min.Z, b.Max.Z},
	} {
		if n.min >= n.max {
			return fmt.Errorf("box %s min must be < max (got %g >= %g)", n.name, n.min, n.max)
		}
	}
	return nil
}

func validateCylinder(c *Cylinder) error {
	if !c.Center.Finite() || !c.Axis.Finite() {
		return fmt.Errorf("cylinder: center and axis must be finite")
	}
	if c.Radius <= 0 {
		return fmt.Errorf("cylinder: radius must be > 0 (got %g)", c.Radius)
	}
	if c.Length <= 0 {
		return fmt.Errorf("cylinder: length must be > 0 (got %g)", c.Length)
	}
	return nil
}

// KeepOuts maps the fixture's keep-out volumes to the planner's hard
// constraints (ADR-019 §3). Boxes map directly; cylinders map iff vertical
// (axis along ±Z: base centre + length become the Z range). A non-vertical
// axis or a mesh volume is a precise refusal — the v1 safety gate verifies
// boxes and vertical cylinders exactly, and planning against anything else
// would be a Law 7 violation.
func (f Fixture) KeepOuts() ([]planning.KeepOut, error) {
	if err := f.Validate(); err != nil {
		return nil, err
	}
	out := make([]planning.KeepOut, 0, len(f.KeepOut))
	for i, v := range f.KeepOut {
		ko, err := mapKeepOut(f.ID, i, v)
		if err != nil {
			return nil, err
		}
		out = append(out, ko)
	}
	return out, nil
}

func mapKeepOut(fid string, i int, v Volume) (planning.KeepOut, error) {
	name := v.Note
	if name == "" {
		name = fmt.Sprintf("keep-out %d", i)
	}
	switch v.Type {
	case KindBox:
		ko := planning.KeepOut{
			Name: name,
			Box:  &planning.Box{Min: v.Box.Min, Max: v.Box.Max},
		}
		if err := ko.Validate(); err != nil {
			return planning.KeepOut{}, fmt.Errorf("fixture %q: keep_out[%d]: %v", fid, i, err)
		}
		return ko, nil
	case KindCylinder:
		c := v.Cyl
		if c.Axis.X != 0 || c.Axis.Y != 0 || c.Axis.Z == 0 {
			return planning.KeepOut{}, fmt.Errorf(
				"fixture %q: keep_out[%d] has a non-vertical cylinder axis (%v) — v1 keep-outs are boxes and vertical cylinders; model the clamp as a box or make it vertical",
				fid, i, c.Axis)
		}
		lo, hi := c.Center.Z, c.Center.Z+c.Length
		if c.Axis.Z < 0 {
			lo, hi = c.Center.Z-c.Length, c.Center.Z
		}
		ko := planning.KeepOut{
			Name: name,
			Cyl: &planning.Cylinder{
				Center: kernel.P2(c.Center.X, c.Center.Y),
				Radius: c.Radius,
				ZMin:   lo,
				ZMax:   hi,
			},
		}
		if err := ko.Validate(); err != nil {
			return planning.KeepOut{}, fmt.Errorf("fixture %q: keep_out[%d]: %v", fid, i, err)
		}
		return ko, nil
	case KindMesh:
		return planning.KeepOut{}, fmt.Errorf(
			"fixture %q: keep_out[%d] is a mesh volume (%s) — v1 cannot map mesh keep-outs; use a box or a vertical cylinder",
			fid, i, v.Mesh.File)
	default:
		return planning.KeepOut{}, fmt.Errorf("fixture %q: unknown keep_out[%d] type %q", fid, i, v.Type)
	}
}
