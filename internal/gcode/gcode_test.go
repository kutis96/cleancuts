package gcode

import (
	"strings"
	"testing"

	"github.com/kutis96/cleancuts/internal/kernel"
	"github.com/kutis96/cleancuts/internal/moves"
)

func samplePath() moves.Path {
	return moves.Path{
		ID: "p", Operation: "profile", ToolID: "t1",
		Moves: []moves.Move{
			{Kind: moves.KindRapid, From: kernel.P3(0, 0, 5), To: kernel.P3(0, 0, -10), Spindle: 8000, Coolant: moves.CoolantFlood},
			{Kind: moves.KindLinear, From: kernel.P3(0, 0, -10), To: kernel.P3(10, 0, -10), Feed: 600, Spindle: 8000, Milling: moves.MillingClimb},
			{Kind: moves.KindArcCCW, From: kernel.P3(10, 0, -10), To: kernel.P3(20, 10, -10), Center: kernel.P3(10, 10, -10), Feed: 600, Spindle: 8000},
		},
	}
}

func TestEmitArms(t *testing.T) {
	out, err := Emit(samplePath(), DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"S8000",
		"M3",
		"M8",
		"G0 X0 Y0 Z-10",
		"G1 X10 Y0 Z-10 F600",
		"G3 X20 Y10 Z-10 I0 J10 F600",
		"M9",
		"M5",
		"M2",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in output:\n%s", want, out)
		}
	}
}

func TestEmitTessellated(t *testing.T) {
	o := DefaultOptions()
	o.UseArms = false
	o.ArcTolDeg = 45
	out, err := Emit(samplePath(), o)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "G3 ") {
		t.Errorf("should not contain G3 when tessellated:\n%s", out)
	}
	// The 90° arc at 45°/step yields 2 G1 segments (plus the one linear).
	if n := strings.Count(out, "G1 "); n < 2 {
		t.Errorf("expected >= 2 G1 tessellation segments, got %d:\n%s", n, out)
	}
}

func TestEmitRejectsInvalid(t *testing.T) {
	bad := moves.Path{ID: "bad", Moves: []moves.Move{
		{Kind: moves.KindLinear, From: kernel.P3(0, 0, 0), To: kernel.P3(1, 0, 0), Feed: 0},
	}}
	if _, err := Emit(bad, DefaultOptions()); err == nil {
		t.Fatal("expected a validation error to propagate")
	}
}
