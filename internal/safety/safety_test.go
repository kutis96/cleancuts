package safety

import (
	"strings"
	"testing"

	"github.com/kutis96/cleancuts/internal/kernel"
	"github.com/kutis96/cleancuts/internal/moves"
	"github.com/kutis96/cleancuts/internal/planning"
)

func boxZone(name string, x0, y0, x1, y1, z0, z1 float64) planning.KeepOut {
	return planning.KeepOut{
		Name: name,
		Box:  &planning.Box{Min: kernel.P3(x0, y0, z0), Max: kernel.P3(x1, y1, z1)},
	}
}

func cylZone(name string, cx, cy, r, z0, z1 float64) planning.KeepOut {
	return planning.KeepOut{
		Name: name,
		Cyl:  &planning.Cylinder{Center: kernel.P2(cx, cy), Radius: r, ZMin: z0, ZMax: z1},
	}
}

func lineChain(t *testing.T, a, b kernel.Point2) kernel.Chain2 {
	t.Helper()
	e, err := kernel.NewLineEdge(a, b)
	if err != nil {
		t.Fatal(err)
	}
	c, err := kernel.NewChain2([]kernel.Edge2{e})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func arcChain(t *testing.T, c, a, b kernel.Point2, cw bool) kernel.Chain2 {
	t.Helper()
	e, err := kernel.NewArcEdge(c, a, b, cw)
	if err != nil {
		t.Fatal(err)
	}
	ch, err := kernel.NewChain2([]kernel.Edge2{e})
	if err != nil {
		t.Fatal(err)
	}
	return ch
}

func TestSegmentClearsZone(t *testing.T) {
	c := lineChain(t, kernel.P2(0, 0), kernel.P2(10, 0))
	zone := boxZone("jaw", 20, 20, 30, 30, -20, 20)
	if err := CheckChain(c, 0, 2, []planning.KeepOut{zone}); err != nil {
		t.Fatalf("clear segment should pass, got: %v", err)
	}
}

func TestSegmentHitsZone(t *testing.T) {
	// The segment y=0 passes straight through the box.
	c := lineChain(t, kernel.P2(0, 0), kernel.P2(10, 0))
	zone := boxZone("jaw", 4, -3, 8, 1, -20, 20)
	err := CheckChain(c, 0, 2, []planning.KeepOut{zone})
	if err == nil {
		t.Fatal("a segment through the box must be rejected")
	}
	if !strings.Contains(err.Error(), "jaw") {
		t.Fatalf("error should name the zone, got: %v", err)
	}
}

func TestSegmentTangentIsACollision(t *testing.T) {
	// The box's near edge is exactly r from the segment: tangent contact
	// with a clamp is a collision, and the safe side is to refuse.
	c := lineChain(t, kernel.P2(0, 0), kernel.P2(10, 0))
	zone := boxZone("clamp", 0, 2, 10, 4, -20, 20)
	if err := CheckChain(c, 0, 2, []planning.KeepOut{zone}); err == nil {
		t.Fatal("tangent contact must be rejected")
	}
	// 0.1 mm of clearance is enough.
	if err := CheckChain(c, 0, 1.9, []planning.KeepOut{zone}); err != nil {
		t.Fatalf("0.1mm clearance should pass, got: %v", err)
	}
}

func TestSegmentVsDisc(t *testing.T) {
	// Disc centre (5,3), radius 1: closest point (5,2), distance 2 from the
	// segment y=0.
	c := lineChain(t, kernel.P2(0, 0), kernel.P2(10, 0))
	zone := cylZone("post", 5, 3, 1, -20, 20)
	if err := CheckChain(c, 0, 2, []planning.KeepOut{zone}); err == nil {
		t.Fatal("tangent contact with the disc must be rejected")
	}
	if err := CheckChain(c, 0, 1, []planning.KeepOut{zone}); err != nil {
		t.Fatalf("1mm clearance should pass, got: %v", err)
	}
}

func TestArcVsRectangle(t *testing.T) {
	// Quarter arc R=10 about the origin; the box's nearest corner (8,8) is
	// sqrt(2) from the arc's 45° point (7.071, 7.071).
	c := arcChain(t, kernel.P2(0, 0), kernel.P2(10, 0), kernel.P2(0, 10), false)
	zone := boxZone("clamp", 8, 8, 12, 12, -20, 20)
	if err := CheckChain(c, 0, 2, []planning.KeepOut{zone}); err == nil {
		t.Fatal("arc clipping the box must be rejected")
	}
	// Clearance sqrt(2) ≈ 1.414: r=1 passes, r=1.4142 (touch) does not.
	if err := CheckChain(c, 0, 1, []planning.KeepOut{zone}); err != nil {
		t.Fatalf("1mm clearance should pass, got: %v", err)
	}
}

func TestArcVsDisc(t *testing.T) {
	// Quarter arc R=10 about the origin (from (10,0) to (0,10)); disc
	// centre (5,0) radius 2: the arc point (10,0) is 3 from the disc edge.
	c := arcChain(t, kernel.P2(0, 0), kernel.P2(10, 0), kernel.P2(0, 10), false)
	zone := cylZone("post", 5, 0, 2, -20, 20)
	if err := CheckChain(c, 0, 3, []planning.KeepOut{zone}); err == nil {
		t.Fatal("tangent contact with the disc must be rejected")
	}
	if err := CheckChain(c, 0, 2, []planning.KeepOut{zone}); err != nil {
		t.Fatalf("2mm clearance should pass, got: %v", err)
	}
}

func TestZoneAboveCutIsIgnored(t *testing.T) {
	c := lineChain(t, kernel.P2(0, 0), kernel.P2(10, 0))
	// The clamp floats above the cutting depth: it must not apply.
	zone := boxZone("float", 0, 0, 10, 10, 5, 20)
	if err := CheckChain(c, 0, 0.5, []planning.KeepOut{zone}); err != nil {
		t.Fatalf("a zone above the cut must be ignored, got: %v", err)
	}
}

func TestCheckPath(t *testing.T) {
	zone := boxZone("jaw", -5, -5, 15, 15, -20, 20)

	// A path that dives into the zone must be rejected, naming the move.
	path := moves.Path{ID: "p", Name: "t", Operation: "test", Moves: []moves.Move{
		{Kind: moves.KindRapid, From: kernel.P3(20, 0, 5), To: kernel.P3(20, 0, -10)},
		{Kind: moves.KindLinear, From: kernel.P3(20, 0, -10), To: kernel.P3(5, 0, -10),
			Feed: 500, Milling: moves.MillingClimb},
	}}
	err := CheckPath(path, 2, []planning.KeepOut{zone})
	if err == nil {
		t.Fatal("a path into the zone must be rejected")
	}
	if !strings.Contains(err.Error(), "move 1") {
		t.Fatalf("error should name the move, got: %v", err)
	}

	// A clear path passes.
	clear := moves.Path{ID: "q", Name: "t", Operation: "test", Moves: []moves.Move{
		{Kind: moves.KindRapid, From: kernel.P3(20, 0, 5), To: kernel.P3(20, 0, -10)},
		{Kind: moves.KindLinear, From: kernel.P3(20, 0, -10), To: kernel.P3(25, 0, -10),
			Feed: 500, Milling: moves.MillingClimb},
	}}
	if err := CheckPath(clear, 2, []planning.KeepOut{zone}); err != nil {
		t.Fatalf("clear path should pass, got: %v", err)
	}
}

func TestVerticalPlungeIntoZone(t *testing.T) {
	// A vertical plunge at (5,5) — inside the zone footprint — through the
	// zone's Z range must be rejected.
	zone := boxZone("tab-clamp", 0, 0, 10, 10, 0, 20)
	path := moves.Path{ID: "v", Name: "t", Operation: "test", Moves: []moves.Move{
		{Kind: moves.KindRapid, From: kernel.P3(5, 5, 30), To: kernel.P3(5, 5, -5)},
	}}
	if err := CheckPath(path, 2, []planning.KeepOut{zone}); err == nil {
		t.Fatal("a plunge through the zone must be rejected")
	}
	// A plunge at (20,20) is clear.
	path = moves.Path{ID: "v2", Name: "t", Operation: "test", Moves: []moves.Move{
		{Kind: moves.KindRapid, From: kernel.P3(20, 20, 30), To: kernel.P3(20, 20, -5)},
	}}
	if err := CheckPath(path, 2, []planning.KeepOut{zone}); err != nil {
		t.Fatalf("a clear plunge should pass, got: %v", err)
	}
}

func TestArcThroughRectangleIsZeroClearance(t *testing.T) {
	// A semicircle about (5,5) R=5 from (10,5) to (0,5) passes through the
	// box (its apex is (5,10), inside the box): clearance 0, rejected even
	// for a point tool.
	c := arcChain(t, kernel.P2(5, 5), kernel.P2(10, 5), kernel.P2(0, 5), false)
	zone := boxZone("block", 4, 8, 6, 12, -20, 20)
	if err := CheckChain(c, 0, 0, []planning.KeepOut{zone}); err == nil {
		t.Fatal("an arc through the box must be rejected")
	}
}
