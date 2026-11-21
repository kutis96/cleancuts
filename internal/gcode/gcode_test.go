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

// TestCoordFormat pins the coordinate rendering: fixed precision with
// trailing-zero trim, -0 normalised to 0, and CoordPrecision honoured. The
// whole point is that float64 ulps from the core (e.g. -5.000000000000002
// from a 22.5° offset corner) must not leak into G-code.
func TestCoordFormat(t *testing.T) {
	w := &writer{o: DefaultOptions()}
	cases := []struct {
		v    float64
		want string
	}{
		{-5.000000000000002, "-5"}, // the real-world float artifact
		{5.0, "5"},                 // integer stays bare
		{-0.0, "0"},                // -0 normalised
		{0.123456789, "0.1235"},    // rounded to 1 µm
		{1.5, "1.5"},               // no trailing zeros
		{-0.00001, "0"},            // below precision rounds to 0
		{123.456789, "123.4568"},   // multi-digit rounding
		{1000.0, "1000"},           // no trailing dot
	}
	for _, c := range cases {
		if got := w.fmt(c.v); got != c.want {
			t.Errorf("fmt(%v) = %q, want %q", c.v, got, c.want)
		}
	}

	// CoordPrecision is honoured (2 decimals = 0.01 mm).
	w2 := &writer{o: DefaultOptions()}
	w2.o.CoordPrecision = 2
	if got := w2.fmt(1.234); got != "1.23" {
		t.Errorf("fmt(1.234) at precision 2 = %q, want %q", got, "1.23")
	}
}

// TestEmitTrimsFloatUlps runs a full emit through the same value that
// produced the real-world artifact, to pin the end-to-end behaviour.
func TestEmitTrimsFloatUlps(t *testing.T) {
	p := moves.Path{
		ID: "p", Operation: "profile", ToolID: "t1",
		Moves: []moves.Move{
			{Kind: moves.KindLinear, From: kernel.P3(0, 0, 0), To: kernel.P3(-5.000000000000002, -5, 0), Feed: 500},
		},
	}
	out, err := Emit(p, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "G1 X-5 Y-5 Z0 F500") {
		t.Errorf("float ulps leaked into G-code:\n%s", out)
	}
	if strings.Contains(out, "5.000000000000002") {
		t.Errorf("raw float64 value must not appear:\n%s", out)
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

// --- EmitProgram (ADR-019 §4) ---------------------------------------------

func TestEmitProgram(t *testing.T) {
	a := samplePath() // spindle 8000, wants flood
	b := samplePath() // spindle 12000, no coolant
	b.ID, b.Name = "q", "second feature"
	for i := range b.Moves {
		b.Moves[i].Spindle = 12000
		b.Moves[i].Coolant = moves.CoolantOff
	}
	link := moves.Path{
		ID:   "link-1",
		Name: "travel a → q",
		Moves: []moves.Move{{
			Kind:    moves.KindRapid,
			From:    kernel.P3(20, 10, 5),
			To:      kernel.P3(0, 0, 5),
			Comment: "travel to next feature",
		}},
	}
	prog := moves.Program{ID: "prog", Name: "two features", Paths: []moves.Path{a, link, b}}
	out, err := EmitProgram(prog, DefaultOptions())
	if err != nil {
		t.Fatalf("EmitProgram: %v", err)
	}

	for _, want := range []string{
		"; two features",   // the program name
		"; second feature", // the path name comment
		"S12000",           // spindle = the max across paths
		"M8",               // coolant on: one path wants it
		"G0 X0 Y0 Z5",      // the link rapid
		"M9", "M5", "M2",   // the footer
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in output:\n%s", want, out)
		}
	}
	// Footer order: coolant off, then spindle off, then program end.
	iM9, iM5, iM2 := strings.Index(out, "M9"), strings.Index(out, "M5"), strings.Index(out, "M2")
	if !(iM9 < iM5 && iM5 < iM2) {
		t.Errorf("footer order must be M9, M5, M2 (at %d, %d, %d)\n%s", iM9, iM5, iM2, out)
	}
	// Paths emit in order: a's first cutting move before the link, the link
	// before b's.
	iA, iLink, iB := strings.Index(out, "G1 X10 Y0 Z-10 F600"), strings.Index(out, "G0 X0 Y0 Z5"), strings.Index(out, "; second feature")
	if !(iA < iLink && iLink < iB) {
		t.Errorf("paths must emit in order (at %d, %d, %d)\n%s", iA, iLink, iB, out)
	}
}

func TestEmitProgramRejectsInvalid(t *testing.T) {
	cases := []struct {
		name string
		prog moves.Program
	}{
		{"no ID", moves.Program{Paths: []moves.Path{samplePath()}}},
		{"no paths", moves.Program{ID: "p"}},
		{"duplicate path IDs", moves.Program{ID: "p", Paths: []moves.Path{samplePath(), samplePath()}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := EmitProgram(c.prog, DefaultOptions()); err == nil {
				t.Fatal("expected a validation error")
			}
		})
	}
}
