// Package machine defines the portable machine and post-processor
// configuration, loaded from JSON. A Machine describes a physical CNC mill
// and the G-code dialect its controller expects, so toolpaths and tool
// libraries can be carried across machines unchanged.
//
// The machine config is the single source of truth for dialect-specific
// behaviour: axis mapping and travel, spindle and coolant M-codes, WCS, and
// arc format. The generic gcode emitter and (later) machine-specific
// post-processors read from here rather than hard-coding dialect details,
// which is what keeps post-processing portable and testable. See
// docs/gcode.md.
package machine

import "fmt"

// Dialect names a known controller family. Unknown dialects are allowed and
// simply treated as "generic".
type Dialect string

const (
	DialectGRBL     Dialect = "grbl"
	DialectLinuxCNC Dialect = "linuxcnc"
	DialectFanuc    Dialect = "fanuc"
	DialectGeneric  Dialect = "generic"
)

// Axis describes one linear axis and its travel/feeds.
type Axis struct {
	Letter   string  `json:"letter"`
	Min      float64 `json:"min"`       // mm
	Max      float64 `json:"max"`       // mm
	MaxFeed  float64 `json:"max_feed"`  // mm/min
	MaxRapid float64 `json:"max_rapid"` // mm/min
}

// PostProcessor holds the dialect-specific codes and program framing.
type PostProcessor struct {
	SpindleStart int      `json:"spindle_start"` // M3
	SpindleStop  int      `json:"spindle_stop"`  // M5
	ProgramEnd   int      `json:"program_end"`   // M2
	CoolantOn    int      `json:"coolant_on"`    // M8
	CoolantOff   int      `json:"coolant_off"`   // M9
	ArcFormat    string   `json:"arc_format"`    // "ij" (relative) or "absolute"
	HeaderLines  []string `json:"header_lines,omitempty"`
	FooterLines  []string `json:"footer_lines,omitempty"`
}

// Machine is a full machine + post-processor description.
type Machine struct {
	ID            string        `json:"id"`
	Name          string        `json:"name"`
	Dialect       Dialect       `json:"dialect"`
	Units         string        `json:"units"` // "mm" or "inch"
	Axes          []Axis        `json:"axes"`
	SpindleMaxRPM float64       `json:"spindle_max_rpm"`
	WCS           string        `json:"wcs"`         // e.g. "G54"
	SafeHeight    float64       `json:"safe_height"` // mm
	ToolChange    string        `json:"tool_change"` // e.g. "T"
	PostProcessor PostProcessor `json:"post_processor"`
}

// Validate enforces machine-level invariants.
func (m Machine) Validate() error {
	if m.ID == "" {
		return fmt.Errorf("machine: ID is required")
	}
	if m.Units != "mm" && m.Units != "inch" {
		return fmt.Errorf("machine %q: units must be \"mm\" or \"inch\" (got %q)", m.ID, m.Units)
	}
	if len(m.Axes) == 0 {
		return fmt.Errorf("machine %q: at least one axis is required", m.ID)
	}
	for _, a := range m.Axes {
		if a.Min >= a.Max {
			return fmt.Errorf("machine %q: axis %s min (%g) must be < max (%g)", m.ID, a.Letter, a.Min, a.Max)
		}
		if a.MaxFeed <= 0 || a.MaxRapid <= 0 {
			return fmt.Errorf("machine %q: axis %s max_feed and max_rapid must be > 0", m.ID, a.Letter)
		}
	}
	if m.SpindleMaxRPM < 0 {
		return fmt.Errorf("machine %q: spindle_max_rpm must be >= 0 (got %g)", m.ID, m.SpindleMaxRPM)
	}
	if m.PostProcessor.ArcFormat != "ij" && m.PostProcessor.ArcFormat != "absolute" {
		return fmt.Errorf("machine %q: post_processor.arc_format must be \"ij\" or \"absolute\" (got %q)",
			m.ID, m.PostProcessor.ArcFormat)
	}
	return nil
}
