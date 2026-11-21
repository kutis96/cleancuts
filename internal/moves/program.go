package moves

import "fmt"

// Program is an ordered group of Paths forming one machining program
// (ADR-019): the level above Path and below G-code. It is what the CLI
// writes to `.moves.json`, what the future GUI renders, and what
// gcode.EmitProgram renders. Paths are executed in order; link paths (the
// rapids between features) sit between feature paths as first-class,
// overridable moves (ADR-019 §2).
type Program struct {
	// ID is the stable program identifier.
	ID string `json:"id"`
	// Name is a human-readable name (the part + operation, e.g.
	// "plate-60x40-hole20 — first cut").
	Name string `json:"name,omitempty"`
	// Paths run in order: a feature path (plunge → cutting loop → retract)
	// or a link path (one rapid between features).
	Paths []Path `json:"paths"`
	// Metadata carries non-structural context (Z model, part/tool IDs,
	// strategy notes) without polluting the core fields.
	Metadata map[string]string `json:"metadata,omitempty"`
}

// Validate enforces the program invariants (Law 5): a non-empty ID, at
// least one path, every path valid, and unique path IDs — paths are
// addressable objects the user inspects and overrides (ADR-003, ADR-019
// §1), so duplicates are a contract break, not a cosmetic issue.
func (p Program) Validate() error {
	if p.ID == "" {
		return fmt.Errorf("moves: program requires a non-empty ID")
	}
	if len(p.Paths) == 0 {
		return fmt.Errorf("moves: program %q has no paths", p.ID)
	}
	seen := make(map[string]bool, len(p.Paths))
	for i, path := range p.Paths {
		if err := path.Validate(); err != nil {
			return fmt.Errorf("moves: program %q path %d: %w", p.ID, i, err)
		}
		if seen[path.ID] {
			return fmt.Errorf("moves: program %q has two paths with ID %q — path IDs must be unique (paths are addressable)", p.ID, path.ID)
		}
		seen[path.ID] = true
	}
	return nil
}
