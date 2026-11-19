// Package tool defines the portable tool model and library, loaded from
// JSON. A Tool describes a physical cutting tool and the process parameters
// needed to drive it safely.
//
// The tool model is deliberately data-only and dependency-free so that a
// library of tool JSON files can be carried across machines and operators
// exactly like a physical tool crib. See docs/tool-parameters.md for the full
// parameter reference and the reasoning behind each field.
package tool

import (
	"fmt"
)

// Kind is the geometric family of a tool tip.
type Kind string

const (
	KindFlatEndMill  Kind = "flat_end_mill"
	KindBallNose     Kind = "ball_nose"
	KindBullNose     Kind = "bull_nose"
	KindTaperEndMill Kind = "taper_end_mill"
	KindChamfer      Kind = "chamfer"
	KindDrill        Kind = "drill"
	KindFaceMill     Kind = "face_mill"
)

// FeedPerTooth is a recommended feed-per-tooth (fz) for a specific material.
type FeedPerTooth struct {
	Material string  `json:"material"` // e.g. "aluminum-6061", "steel-1018"
	FZ       float64 `json:"fz"`       // mm per tooth
	VC       float64 `json:"vc"`       // cutting speed (m/min); 0 = unspecified
	Note     string  `json:"note,omitempty"`
}

// Tool is a single cutting tool definition. All linear dimensions are in mm.
type Tool struct {
	ID            string         `json:"id"`
	Name          string         `json:"name"`
	Kind          Kind           `json:"kind"`
	Diameter      float64        `json:"diameter"`      // cutting diameter
	Flutes        int            `json:"flutes"`        // number of teeth
	CornerRadius  float64        `json:"corner_radius"` // bull-nose corner radius
	TaperAngle    float64        `json:"taper_angle"`   // half-angle for tapered tools (deg)
	FluteLength   float64        `json:"flute_length"`  // cutting length
	OverallLength float64        `json:"overall_length"`
	ShankDiameter float64        `json:"shank_diameter"`
	Material      string         `json:"material"` // "carbide", "hss", ...
	Coating       string         `json:"coating,omitempty"`
	MaxRPM        float64        `json:"max_rpm"`
	Feeds         []FeedPerTooth `json:"feeds,omitempty"`
}

// Radius returns the cutting radius (mm).
func (t Tool) Radius() float64 { return t.Diameter / 2 }

// Validate enforces the invariants of a Tool.
func (t Tool) Validate() error {
	if t.ID == "" {
		return fmt.Errorf("tool: ID is required")
	}
	if t.Diameter <= 0 {
		return fmt.Errorf("tool %q: diameter must be > 0 (got %g)", t.ID, t.Diameter)
	}
	if t.Flutes < 0 {
		return fmt.Errorf("tool %q: flutes must be >= 0 (got %d)", t.ID, t.Flutes)
	}
	if t.CornerRadius < 0 {
		return fmt.Errorf("tool %q: corner radius must be >= 0 (got %g)", t.ID, t.CornerRadius)
	}
	if t.CornerRadius > t.Diameter/2 {
		return fmt.Errorf("tool %q: corner radius %g exceeds radius %g", t.ID, t.CornerRadius, t.Diameter/2)
	}
	if t.TaperAngle < 0 || t.TaperAngle >= 90 {
		return fmt.Errorf("tool %q: taper angle must be in [0, 90) deg (got %g)", t.ID, t.TaperAngle)
	}
	if t.MaxRPM < 0 {
		return fmt.Errorf("tool %q: max_rpm must be >= 0 (got %g)", t.ID, t.MaxRPM)
	}
	for i, f := range t.Feeds {
		if f.FZ <= 0 {
			return fmt.Errorf("tool %q: feeds[%d] fz must be > 0 (got %g)", t.ID, i, f.FZ)
		}
		if f.VC < 0 {
			return fmt.Errorf("tool %q: feeds[%d] vc must be >= 0 (got %g)", t.ID, i, f.VC)
		}
	}
	return nil
}

// Library is a named set of Tools (a portable tool library / tool crib).
type Library struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Tools []Tool `json:"tools"`
}

// Get returns the tool with the given ID.
func (l Library) Get(id string) (Tool, bool) {
	for _, t := range l.Tools {
		if t.ID == id {
			return t, true
		}
	}
	return Tool{}, false
}

// Validate enforces library-level invariants, including unique tool IDs.
func (l Library) Validate() error {
	if l.ID == "" {
		return fmt.Errorf("tool: library ID is required")
	}
	seen := make(map[string]bool, len(l.Tools))
	for _, t := range l.Tools {
		if err := t.Validate(); err != nil {
			return err
		}
		if seen[t.ID] {
			return fmt.Errorf("tool: duplicate tool ID %q in library %q", t.ID, l.ID)
		}
		seen[t.ID] = true
	}
	return nil
}
