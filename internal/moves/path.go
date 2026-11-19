package moves

import (
	"fmt"
	"math"
	"time"

	"github.com/kutis96/cleancuts/internal/kernel"
)

// Path is a single continuous chain of Moves producing one pass of one
// feature. It is the unit of post-processing, inspection, and override.
type Path struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Operation string `json:"operation"` // e.g. "profile", "pocket", "face"
	ToolID    string `json:"tool_id"`
	Pass      int    `json:"pass"`
	Moves     []Move `json:"moves"`
	// Metadata carries non-structural, free-form context (e.g. strategy name,
	// engagement model) without polluting the core fields.
	Metadata  map[string]string `json:"metadata"`
	CreatedAt time.Time         `json:"created_at"`
}

// Validate enforces the structural invariants of a Path: a non-empty ID, at
// least one move, each move valid, and continuity between consecutive moves.
func (p Path) Validate() error {
	if p.ID == "" {
		return fmt.Errorf("moves: path requires a non-empty ID")
	}
	if len(p.Moves) == 0 {
		return fmt.Errorf("moves: path %q has no moves", p.ID)
	}
	prev := p.Moves[0].From
	for i, m := range p.Moves {
		if err := m.Validate(); err != nil {
			return fmt.Errorf("moves: path %q move %d: %w", p.ID, i, err)
		}
		if i > 0 && !prev.Eq(m.From, kernel.AbsTol) {
			return fmt.Errorf("moves: path %q move %d: From %v does not continue previous To %v",
				p.ID, i, m.From, prev)
		}
		prev = m.To
	}
	return nil
}

// Length returns the total cutting length of the path in mm. Rapid moves are
// excluded; arcs are counted by their arc length.
func (p Path) Length() float64 {
	var total float64
	for _, m := range p.Moves {
		switch m.Kind {
		case KindLinear:
			total += m.From.Dist(m.To)
		case KindArcCCW, KindArcCW:
			r := m.Center.Dist(m.From)
			a0 := angleFrom(m.Center, m.From)
			a1 := angleFrom(m.Center, m.To)
			var sweep float64
			if m.CW {
				sweep = kernel.NormalizeAngle(a0 - a1)
			} else {
				sweep = kernel.NormalizeAngle(a1 - a0)
			}
			total += r * sweep
		}
	}
	return total
}

// angleFrom returns the angle (rad) of point p about centre c in the XY plane.
func angleFrom(c, p kernel.Point3) float64 {
	return kernel.NormalizeAngle(math.Atan2(p.Y-c.Y, p.X-c.X))
}
