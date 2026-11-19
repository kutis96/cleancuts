package toolpath

import (
	"testing"

	"github.com/kutis96/cleancuts/internal/kernel"
	"github.com/kutis96/cleancuts/internal/moves"
)

func rect() []kernel.Point2 {
	return []kernel.Point2{
		kernel.P2(0, 0), kernel.P2(100, 0), kernel.P2(100, 60), kernel.P2(0, 60),
	}
}

func TestProfile(t *testing.T) {
	p, err := Profile(rect(), ProfileParams{
		Milling: moves.MillingClimb,
		Feed:    600,
		Spindle: 8000,
		SafeZ:   5,
		CutZ:    -10,
		Coolant: moves.CoolantFlood,
		ToolID:  "endmill-10",
	})
	if err != nil {
		t.Fatalf("Profile: %v", err)
	}
	if err := p.Validate(); err != nil {
		t.Fatalf("path should validate: %v", err)
	}

	// 1 plunge + 4 cutting + 1 retract = 6 moves.
	if len(p.Moves) != 6 {
		t.Fatalf("expected 6 moves, got %d", len(p.Moves))
	}
	if p.Moves[0].Kind != moves.KindRapid || p.Moves[5].Kind != moves.KindRapid {
		t.Errorf("first/last moves should be rapid, got %v / %v", p.Moves[0].Kind, p.Moves[5].Kind)
	}
	for i := 1; i <= 4; i++ {
		m := p.Moves[i]
		if m.Kind != moves.KindLinear {
			t.Errorf("cutting move %d should be linear, got %v", i, m.Kind)
		}
		if m.To.Z != -10 || m.From.Z != -10 {
			t.Errorf("cutting move %d should be at CutZ, got %v -> %v", i, m.From, m.To)
		}
		if m.Milling != moves.MillingClimb {
			t.Errorf("cutting move %d should be climb, got %v", i, m.Milling)
		}
	}
	// Perimeter of a 100 x 60 rectangle.
	if got := p.Length(); got != 2*(100+60) {
		t.Errorf("cutting length = %v want %v", got, 2*(100+60))
	}
}

func TestProfileErrors(t *testing.T) {
	base := ProfileParams{Feed: 600, SafeZ: 5, CutZ: -10}

	if _, err := Profile([]kernel.Point2{kernel.P2(0, 0), kernel.P2(1, 0)}, base); err == nil {
		t.Error("expected error for < 3 vertices")
	}
	if _, err := Profile(rect(), ProfileParams{Feed: 0, SafeZ: 5, CutZ: -10}); err == nil {
		t.Error("expected error for zero feed")
	}
	if _, err := Profile(rect(), ProfileParams{Feed: 100, SafeZ: -5, CutZ: 10}); err == nil {
		t.Error("expected error for CutZ >= SafeZ")
	}
	if _, err := Profile(rect(), base); err != nil {
		t.Errorf("valid profile should succeed, got %v", err)
	}
}
