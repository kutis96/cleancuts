package moves

import (
	"encoding/json"
	"math"
	"strings"
	"testing"

	"github.com/kutis96/cleancuts/internal/kernel"
)

func validRapid() Move {
	return Move{
		Kind:    KindRapid,
		From:    kernel.P3(0, 0, 5),
		To:      kernel.P3(0, 0, -10),
		Spindle: 8000,
		Coolant: CoolantFlood,
	}
}

func validLinear() Move {
	return Move{
		Kind:    KindLinear,
		From:    kernel.P3(0, 0, -10),
		To:      kernel.P3(10, 0, -10),
		Feed:    600,
		Spindle: 8000,
		Milling: MillingClimb,
	}
}

func validArc() Move {
	// CCW quarter arc from (10,0,-10) to (20,10,-10) about centre (10,10,-10),
	// radius 10 — continues the linear move above.
	return Move{
		Kind:    KindArcCCW,
		From:    kernel.P3(10, 0, -10),
		To:      kernel.P3(20, 10, -10),
		Center:  kernel.P3(10, 10, -10),
		CW:      false,
		Feed:    600,
		Spindle: 8000,
		Milling: MillingClimb,
	}
}

func TestMoveValidateValid(t *testing.T) {
	for name, m := range map[string]Move{
		"rapid":  validRapid(),
		"linear": validLinear(),
		"arc":    validArc(),
	} {
		if err := m.Validate(); err != nil {
			t.Errorf("%s: unexpected error: %v", name, err)
		}
	}
}

func TestMoveValidateInvalid(t *testing.T) {
	cases := map[string]Move{
		"rapid with feed":  {Kind: KindRapid, From: kernel.P3(0, 0, 0), To: kernel.P3(0, 0, 1), Feed: 100},
		"linear zero feed": {Kind: KindLinear, From: kernel.P3(0, 0, 0), To: kernel.P3(1, 0, 0), Feed: 0},
		"linear zero len":  {Kind: KindLinear, From: kernel.P3(0, 0, 0), To: kernel.P3(0, 0, 0), Feed: 100},
		"arc zero feed":    {Kind: KindArcCCW, From: kernel.P3(0, 0, 0), To: kernel.P3(1, 0, 0), Center: kernel.P3(0, 1, 0), Feed: 0},
		"arc bad radius":   {Kind: KindArcCCW, From: kernel.P3(0, 0, 0), To: kernel.P3(1, 0, 0), Center: kernel.P3(0, 1, 0), Feed: 100},
		"arc kind/cw":      {Kind: KindArcCCW, From: kernel.P3(0, 0, 0), To: kernel.P3(1, 0, 0), Center: kernel.P3(0, 1, 0), Feed: 100, CW: true},
		"unknown kind":     {Kind: Kind("fly"), From: kernel.P3(0, 0, 0), To: kernel.P3(1, 0, 0)},
		"nan coord":        {Kind: KindRapid, From: kernel.P3(0, 0, 0), To: kernel.P3(1, 0, math.NaN())},
	}
	for name, m := range cases {
		if err := m.Validate(); err == nil {
			t.Errorf("%s: expected an error, got none", name)
		}
	}
}

func TestPathValidateContinuity(t *testing.T) {
	p := Path{
		ID: "p1",
		Moves: []Move{
			validRapid(),
			validLinear(),
		},
	}
	if err := p.Validate(); err != nil {
		t.Fatalf("expected valid path, got %v", err)
	}

	broken := Path{
		ID: "p2",
		Moves: []Move{
			validRapid(),
			{Kind: KindLinear, From: kernel.P3(50, 0, -10), To: kernel.P3(60, 0, -10), Feed: 600},
		},
	}
	if err := broken.Validate(); err == nil {
		t.Fatal("expected continuity error, got none")
	}

	empty := Path{ID: "p3", Moves: nil}
	if err := empty.Validate(); err == nil {
		t.Fatal("expected empty-path error, got none")
	}

	noid := Path{Moves: []Move{validRapid()}}
	if err := noid.Validate(); err == nil {
		t.Fatal("expected missing-ID error, got none")
	}
}

func TestPathLength(t *testing.T) {
	p := Path{
		ID: "len",
		Moves: []Move{
			validRapid(),  // excluded
			validLinear(), // 10 mm
			validArc(),    // quarter circle r=10 -> (π/2)*10
		},
	}
	if err := p.Validate(); err != nil {
		t.Fatalf("path should be valid: %v", err)
	}
	// 10 (linear) + 15.708 (arc)
	if got := p.Length(); !kernel.Equal(got, 10+15.7079632679, 1e-6) {
		t.Errorf("length = %v want ~%v", got, 10+15.7079632679)
	}
}

func TestPathJSONRoundTrip(t *testing.T) {
	p := Path{
		ID:        "profile",
		Name:      "2D profile",
		Operation: "profile",
		ToolID:    "endmill-10",
		Pass:      1,
		Moves:     []Move{validRapid(), validLinear(), validArc()},
		Metadata:  map[string]string{"strategy": "climb"},
	}
	if err := p.Validate(); err != nil {
		t.Fatalf("path should be valid: %v", err)
	}
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back Path
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.ID != p.ID || back.Operation != p.Operation || len(back.Moves) != len(p.Moves) {
		t.Errorf("round-trip mismatch: %+v", back)
	}
	if back.Moves[2].Kind != KindArcCCW || back.Moves[2].Center.X != 10 {
		t.Errorf("arc not preserved: %+v", back.Moves[2])
	}
	if !strings.Contains(string(data), `"kind":"arc_ccw"`) {
		t.Errorf("expected kind tag in JSON, got: %s", string(data))
	}
	if err := back.Validate(); err != nil {
		t.Errorf("round-tripped path should validate: %v", err)
	}
}

// --- Program -------------------------------------------------------------

// simplePath builds a valid one-move path for program tests.
func simplePath(id string, x float64) Path {
	return Path{
		ID:   id,
		Name: id,
		Moves: []Move{{
			Kind: KindRapid,
			From: kernel.P3(0, 0, 10),
			To:   kernel.P3(x, 0, 0),
		}},
	}
}

func TestProgramValidate(t *testing.T) {
	ok := Program{ID: "prog", Paths: []Path{simplePath("a", 1), simplePath("b", 2)}}
	if err := ok.Validate(); err != nil {
		t.Fatalf("valid program rejected: %v", err)
	}

	cases := []struct {
		name    string
		prog    Program
		wantSub string
	}{
		{"no ID", Program{Paths: []Path{simplePath("a", 1)}}, "non-empty ID"},
		{"no paths", Program{ID: "p"}, "no paths"},
		{"invalid path inside", Program{ID: "p", Paths: []Path{{ID: ""}}}, "path 0"},
		{"duplicate path IDs", Program{ID: "p", Paths: []Path{simplePath("a", 1), simplePath("a", 2)}}, "unique"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.prog.Validate()
			if err == nil || !strings.Contains(err.Error(), c.wantSub) {
				t.Fatalf("want error mentioning %q, got %v", c.wantSub, err)
			}
		})
	}
}

func TestProgramJSONRoundTrip(t *testing.T) {
	prog := Program{
		ID:   "prog-1",
		Name: "a test program",
		Paths: []Path{
			simplePath("a", 1),
			Path{
				ID:   "link-1",
				Name: "travel",
				Moves: []Move{{
					Kind:    KindRapid,
					From:    kernel.P3(1, 0, 0),
					To:      kernel.P3(2, 0, 10),
					Comment: "travel to next feature",
				}},
			},
			simplePath("b", 3),
		},
		Metadata: map[string]string{"cut_z": "0", "safe_z": "25"},
	}
	data, err := json.Marshal(prog)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back Program
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.ID != prog.ID || len(back.Paths) != 3 || back.Metadata["safe_z"] != "25" {
		t.Fatalf("round-trip mismatch: %+v", back)
	}
	if back.Paths[1].Moves[0].Comment != "travel to next feature" {
		t.Fatalf("link move lost: %+v", back.Paths[1].Moves[0])
	}
	if err := back.Validate(); err != nil {
		t.Fatalf("round-tripped program should validate: %v", err)
	}
}
