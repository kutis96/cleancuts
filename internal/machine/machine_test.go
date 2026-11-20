package machine

import (
	"encoding/json"
	"testing"
)

func validMachine() Machine {
	return Machine{
		ID: "grbl1", Name: "GRBL 3-axis", Dialect: DialectGRBL, Units: "mm",
		Axes: []Axis{
			{Letter: "X", Min: 0, Max: 200, MaxFeed: 5000, MaxRapid: 5000},
			{Letter: "Y", Min: 0, Max: 200, MaxFeed: 5000, MaxRapid: 5000},
			{Letter: "Z", Min: -50, Max: 0, MaxFeed: 1000, MaxRapid: 2500},
		},
		SpindleMaxRPM: 10000, WCS: "G54", SafeHeight: 5, ToolChange: "T",
		PostProcessor: PostProcessor{
			SpindleStart: 3, SpindleStop: 5, ProgramEnd: 2,
			CoolantOn: 8, CoolantOff: 9, ArcFormat: "ij",
		},
	}
}

func TestMachineValidate(t *testing.T) {
	if err := validMachine().Validate(); err != nil {
		t.Fatalf("valid machine failed: %v", err)
	}

	m := validMachine()
	m.Units = "feet"
	if err := m.Validate(); err == nil {
		t.Error("expected units error")
	}

	m = validMachine()
	m.Axes[0].Min, m.Axes[0].Max = 100, 50
	if err := m.Validate(); err == nil {
		t.Error("expected axis range error")
	}

	m = validMachine()
	m.PostProcessor.ArcFormat = "xyz"
	if err := m.Validate(); err == nil {
		t.Error("expected arc_format error")
	}

	// Spindle rotation (planning input, ADR-009).
	m = validMachine()
	m.SpindleRotation = "clockwise"
	if err := m.Validate(); err == nil {
		t.Error("expected spindle_rotation error")
	}
	m = validMachine()
	m.SpindleRotation = "cw"
	if err := m.Validate(); err != nil {
		t.Errorf("cw spindle rotation should be valid, got %v", err)
	}

	// Feed/rapid limit invariants (ADR-010).
	m = validMachine()
	m.MinFeed, m.MaxFeed = 9000, 8000
	if err := m.Validate(); err == nil {
		t.Error("expected min_feed > max_feed error")
	}
	m = validMachine()
	m.MinRapid, m.MaxRapid = 9000, 8000
	if err := m.Validate(); err == nil {
		t.Error("expected min_rapid > max_rapid error")
	}
	m = validMachine()
	m.MaxFeed, m.MinFeed, m.MaxRapid, m.MinRapid = 8000, 100, 8000, 0
	if err := m.Validate(); err != nil {
		t.Errorf("valid feed/rapid limits should pass, got %v", err)
	}
}

func TestMachineJSONRoundTrip(t *testing.T) {
	m := validMachine()
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	var back Machine
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	if back.Dialect != DialectGRBL || len(back.Axes) != 3 || back.PostProcessor.ArcFormat != "ij" {
		t.Errorf("round trip mismatch: %+v", back)
	}
	if err := back.Validate(); err != nil {
		t.Errorf("round-tripped machine should validate: %v", err)
	}

	// Spindle rotation defaults to CCW when absent (right-handed convention).
	if !validMachine().SpindleCCW() {
		t.Error("empty spindle_rotation should default to CCW")
	}
	cw := validMachine()
	cw.SpindleRotation = "cw"
	if cw.SpindleCCW() {
		t.Error("cw machine should report SpindleCCW == false")
	}
}
