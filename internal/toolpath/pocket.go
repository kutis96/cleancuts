// Pocket clearing: the area operation (ADR-022, ADR-026, ADR-027).
//
// Pocket clears a *region* (the tool-centre domain: the outer boundary
// offset inside by the tool radius, minus each boss offset outside by the
// tool radius — the planner applies the offsets, ADR-026) down to CutZ with
// a serpentine raster:
//
//   - rows at stepover s, placed so both ends of the region are within
//     s/2 <= R of a row (ADR-027 — full coverage by construction);
//   - each row's clearable intervals are (row ∩ region) − (row ∩ bosses),
//     computed analytically (line/edge intersection + even-odd pairing +
//     interval subtraction — no tessellation, ADR-002);
//   - rows are traversed bottom to top in a serpentine (even rows
//     left→right, odd rows right→left); a boss that splits a row leaves
//     disjoint intervals, bridged by a *lift* (retract, rapid, plunge) —
//     the gap is over kept material and is never cut through (Law 7);
//   - consecutive rows are joined by a *semicircle with the chord as
//     diameter* (the trochoidal element — an analytic arc, ADR-002),
//     gate-checked against the region and the bosses, degrading to a lift
//     with a diagnostic if no candidate passes (ADR-026);
//   - the first move of the path is a plunge from SafeZ (the ADR-020
//     producer contract the emitter relies on), and every interval after
//     a lift starts with a plunge into the face — intervals are clipped
//     outside the boss offsets, so a plunge never lands over kept material.
//
// Engagement and feed (ADR-022): a constant stepover gives a constant
// radial engagement a_e = R − √(R² − (s/2)²); every cutting move's feed is
// F_full · √(a_e/D) (underfeeding the joins — the safe side). Both are pure
// functions with their own tests.
package toolpath

import (
	"fmt"
	"math"
	"sort"

	"github.com/kutis96/cleancuts/internal/kernel"
	"github.com/kutis96/cleancuts/internal/moves"
)

// sliverMin is the width (mm) below which a clearable interval is dropped
// as residual material, with a diagnostic (ADR-026; the safe side, ADR-013).
const sliverMin = 0.1

// Diagnostic is a note about a safe resolution Pocket took (a dropped
// sliver, a join degraded to a lift). Diagnostics do not block the path;
// refusals are errors.
type Diagnostic struct {
	Level   string `json:"level"` // "info" | "warning"
	Message string `json:"message"`
}

// Engagement returns the radial engagement (mm) of a row cut at stepover s
// by a tool of radius R (ADR-022): the exact lens width
//
//	a_e = R − √(R² − (s/2)²)   (≈ s²/2R for small s),
//
// in (0, R] for 0 < s ≤ 2R; s = 2R (rows just touching) gives the full
// radius R. A constant stepover therefore gives a constant a_e — the
// "constant cutter engagement" request.
func Engagement(R, s float64) (float64, error) {
	if !kernel.IsFinite(R) || R <= 0 {
		return 0, fmt.Errorf("toolpath: Engagement: tool radius must be > 0 (got %g)", R)
	}
	if !kernel.IsFinite(s) || s <= 0 {
		return 0, fmt.Errorf("toolpath: Engagement: stepover must be > 0 (got %g)", s)
	}
	if s > 2*R+kernel.AbsTol {
		return 0, fmt.Errorf("toolpath: Engagement: stepover %g exceeds 2x the tool radius (%g) — the rows would not overlap and the region would not be covered; lower the stepover",
			s, 2*R)
	}
	half := s / 2
	if half > R {
		half = R // tolerance guard: s within AbsTol of 2R
	}
	return R - math.Sqrt(R*R-half*half), nil
}

// ChipLoadFeed returns the cutting feed (mm/min) for a move with radial
// engagement aE and tool diameter D, given the full-engagement feed F
// (ADR-022, the tool-parameters.md model):
//
//	F(a_e) = F · √(a_e / D).
//
// aE = D gives F and aE = 0 gives 0. The valid range is 0 ≤ aE ≤ D (Pocket's
// a_e ≤ R = D/2 always holds, so Pocket never exceeds the full feed).
func ChipLoadFeed(F, aE, D float64) (float64, error) {
	if !kernel.IsFinite(F) || F <= 0 {
		return 0, fmt.Errorf("toolpath: ChipLoadFeed: full feed must be > 0 (got %g)", F)
	}
	if !kernel.IsFinite(D) || D <= 0 {
		return 0, fmt.Errorf("toolpath: ChipLoadFeed: diameter must be > 0 (got %g)", D)
	}
	if !kernel.IsFinite(aE) || aE < 0 {
		return 0, fmt.Errorf("toolpath: ChipLoadFeed: engagement must be >= 0 (got %g)", aE)
	}
	if aE > D+kernel.AbsTol {
		return 0, fmt.Errorf("toolpath: ChipLoadFeed: engagement %g exceeds the diameter %g", aE, D)
	}
	return F * math.Sqrt(aE/D), nil
}

// PocketParams configures a pocket clearing path. Region and Bosses are
// the *tool-centre* boundaries (the planner has already applied the
// offsets: region = outer offset inside by R, each boss offset outside by
// R — ADR-026). Every tool-centre position stays inside Region and outside
// every Boss, which is the primary clearance (Law 7) by construction.
type PocketParams struct {
	// Operation is recorded on the path; empty defaults to "pocket".
	Operation string
	// Name is the display name; empty defaults to "pocket clear".
	Name string
	// ToolID is recorded on the path for post-processing and inspection.
	ToolID string

	// Milling is the climb/conventional mode recorded on cutting moves.
	Milling moves.Milling
	// FeedFull is the full-engagement feed (mm/min); each cutting move's
	// feed is ChipLoadFeed(FeedFull, a_e, 2R) (ADR-022). Must be > 0.
	FeedFull float64
	// Spindle is the spindle speed (RPM).
	Spindle float64
	// Coolant is the coolant state during cutting.
	Coolant moves.Coolant
	// SafeZ is the rapid safe height (mm); must be above CutZ.
	SafeZ float64
	// CutZ is the cutting depth (mm).
	CutZ float64

	// Radius is the tool radius R (mm); must be > 0.
	Radius float64
	// Stepover is the row spacing s (mm); must satisfy 0 < s <= 2R
	// (coverage, ADR-022/027), else a precise refusal.
	Stepover float64
	// Region is the closed tool-centre domain; the tool stays inside (or
	// on) it.
	Region kernel.Chain2
	// Bosses are the closed kept-material boundaries (already offset
	// outside by R); the tool stays outside (or on) each.
	Bosses []kernel.Chain2
}

// validate enforces the invariants of PocketParams (Law 5).
func (p PocketParams) validate() error {
	if !kernel.IsFinite(p.Radius) || p.Radius <= 0 {
		return fmt.Errorf("toolpath: Pocket radius must be > 0 (got %g)", p.Radius)
	}
	if !kernel.IsFinite(p.Stepover) || p.Stepover <= 0 {
		return fmt.Errorf("toolpath: Pocket stepover must be > 0 (got %g)", p.Stepover)
	}
	if p.Stepover > 2*p.Radius+kernel.AbsTol {
		return fmt.Errorf("toolpath: Pocket stepover %g exceeds 2x the tool radius (%g) — the rows would not overlap and the region would not be covered",
			p.Stepover, 2*p.Radius)
	}
	if !kernel.IsFinite(p.FeedFull) || p.FeedFull <= 0 {
		return fmt.Errorf("toolpath: Pocket full feed must be > 0 (got %g)", p.FeedFull)
	}
	if !kernel.IsFinite(p.SafeZ) || !kernel.IsFinite(p.CutZ) || p.CutZ >= p.SafeZ {
		return fmt.Errorf("toolpath: Pocket CutZ (%g) must be strictly below SafeZ (%g)", p.CutZ, p.SafeZ)
	}
	if err := p.Region.Validate(); err != nil {
		return fmt.Errorf("toolpath: Pocket region: %w", err)
	}
	if !p.Region.IsClosed() {
		return fmt.Errorf("toolpath: Pocket region must be a closed loop (got an open chain)")
	}
	for i, b := range p.Bosses {
		if err := b.Validate(); err != nil {
			return fmt.Errorf("toolpath: Pocket boss %d: %w", i, err)
		}
		if !b.IsClosed() {
			return fmt.Errorf("toolpath: Pocket boss %d must be a closed loop (got an open chain)", i)
		}
	}
	return nil
}

// interval is a clearable x-range on one row (mm).
type interval struct{ lo, hi float64 }

// row is one raster row: its height, its traversal direction, and its
// clearable intervals sorted left to right.
type row struct {
	y    float64
	dir  int // +1: left→right; -1: right→left (serpentine parity, ADR-026)
	ints []interval
}

// order returns the intervals in traversal order.
func (r row) order() []interval {
	if r.dir > 0 {
		return r.ints
	}
	out := make([]interval, len(r.ints))
	for i, iv := range r.ints {
		out[len(r.ints)-1-i] = iv
	}
	return out
}

// startOf / endOf give an interval's traversal entry and exit x.
func (r row) startOf(iv interval) float64 {
	if r.dir > 0 {
		return iv.lo
	}
	return iv.hi
}

func (r row) endOf(iv interval) float64 {
	if r.dir > 0 {
		return iv.hi
	}
	return iv.lo
}

// Pocket generates the verified pocket-clearing path (ADR-026, ADR-027)
// plus the safe-resolution diagnostics (dropped slivers, joins degraded to
// lifts). The first move is a plunge from SafeZ at the entry point (the
// ADR-020 producer contract), and the last move is a retract to SafeZ.
func Pocket(p PocketParams) (moves.Path, []Diagnostic, error) {
	if err := p.validate(); err != nil {
		return moves.Path{}, nil, err
	}
	aE, err := Engagement(p.Radius, p.Stepover)
	if err != nil {
		return moves.Path{}, nil, err
	}
	feed, err := ChipLoadFeed(p.FeedFull, aE, 2*p.Radius)
	if err != nil {
		return moves.Path{}, nil, err
	}

	minX, minY, maxX, maxY := chainBBox(p.Region)
	height := maxY - minY
	width := maxX - minX
	if width <= kernel.AbsTol || height <= kernel.AbsTol {
		return moves.Path{}, nil, fmt.Errorf("toolpath: Pocket region is degenerate (%gx%g mm bbox) — it has no clearable area", width, height)
	}

	var diags []Diagnostic
	slivers := 0
	// --- Rows (ADR-027) and their clearable intervals ----------------------
	rows := make([]row, 0)
	for i, y := range rowYs(minY, maxY, p.Stepover, p.Radius) {
		outer, err := rowIntervals(p.Region, y)
		if err != nil {
			return moves.Path{}, nil, fmt.Errorf("toolpath: Pocket row at y=%g: %w", y, err)
		}
		clearable := outer
		for _, b := range p.Bosses {
			bi, err := rowIntervals(b, y)
			if err != nil {
				return moves.Path{}, nil, fmt.Errorf("toolpath: Pocket row at y=%g against boss: %w", y, err)
			}
			clearable = subtractIntervals(clearable, bi)
		}
		kept := clearable[:0]
		for _, iv := range clearable {
			if iv.hi-iv.lo < sliverMin {
				slivers++
				diags = append(diags, Diagnostic{Level: "warning",
					Message: fmt.Sprintf("row y=%g: dropped a %g mm sliver (below %g mm) — residual material, the safe side", y, iv.hi-iv.lo, sliverMin)})
				continue
			}
			kept = append(kept, iv)
		}
		if len(kept) == 0 {
			continue // this row has nothing to cut (fully inside bosses)
		}
		dir := 1
		if i%2 == 1 {
			dir = -1
		}
		rows = append(rows, row{y: y, dir: dir, ints: kept})
	}
	if len(rows) == 0 {
		return moves.Path{}, nil, fmt.Errorf("toolpath: Pocket region has no clearable rows at stepover %g — the tool does not fit the region (choose a smaller tool or a smaller stepover)", p.Stepover)
	}

	// --- Traversal (ADR-026): serpentine with lifts and join arcs ---------
	operation := p.Operation
	if operation == "" {
		operation = "pocket"
	}
	name := p.Name
	if name == "" {
		name = "pocket clear"
	}
	cut := func(x, y float64) kernel.Point3 { return kernel.Point3{X: x, Y: y, Z: p.CutZ} }
	safe := func(x, y float64) kernel.Point3 { return kernel.Point3{X: x, Y: y, Z: p.SafeZ} }
	plunge := func(x, y float64) moves.Move {
		return moves.Move{Kind: moves.KindRapid, From: safe(x, y), To: cut(x, y),
			Spindle: p.Spindle, Coolant: p.Coolant, Comment: "plunge"}
	}
	retract := func(x, y float64) moves.Move {
		return moves.Move{Kind: moves.KindRapid, From: cut(x, y), To: safe(x, y), Comment: "retract"}
	}
	rapid := func(x1, y1, x2, y2 float64) moves.Move {
		return moves.Move{Kind: moves.KindRapid, From: safe(x1, y1), To: safe(x2, y2), Comment: "rapid"}
	}
	line := func(x1, y1, x2, y2 float64) moves.Move {
		return moves.Move{Kind: moves.KindLinear, From: cut(x1, y1), To: cut(x2, y2),
			Feed: feed, Spindle: p.Spindle, Coolant: p.Coolant, Milling: p.Milling, Comment: "row"}
	}
	// The first move of the path is a plunge at safe Z (ADR-020).
	first := rows[0]
	f0 := first.order()[0]
	out := moves.Path{
		ID:        "pocket",
		Name:      name,
		Operation: operation,
		ToolID:    p.ToolID,
		Pass:      1,
	}
	out.Moves = append(out.Moves,
		plunge(first.startOf(f0), first.y),
		line(first.startOf(f0), first.y, first.endOf(f0), first.y))
	// The remaining intervals of row 0 (a boss split it): lift between —
	// the gap is over kept material and is never cut through (Law 7).
	{
		o := first.order()
		for j, iv := range o[1:] {
			out.Moves = append(out.Moves,
				retract(first.endOf(o[j]), first.y),
				rapid(first.endOf(o[j]), first.y, first.startOf(iv), first.y),
				plunge(first.startOf(iv), first.y),
				line(first.startOf(iv), first.y, first.endOf(iv), first.y))
			diags = append(diags, Diagnostic{Level: "info",
				Message: "row 0: lifted between intervals — the gap is over kept material and is never cut through"})
		}
	}

	// The remaining rows: a join arc (or a lift) then the row's intervals.
	for i := 1; i < len(rows); i++ {
		prev := rows[i-1]
		cur := rows[i]
		prevEnd := prev.order()[len(prev.order())-1]
		curStart := cur.order()[0]
		p1 := kernel.P2(prev.endOf(prevEnd), prev.y)
		p2 := kernel.P2(cur.startOf(curStart), cur.y)
		if e, ok := joinArc(p1, p2, p.Region, p.Bosses); ok {
			a, _ := e.AsArc()
			kind := moves.KindArcCCW
			if a.CW {
				kind = moves.KindArcCW
			}
			out.Moves = append(out.Moves, moves.Move{
				Kind:    kind,
				From:    cut(p1.X, p1.Y),
				To:      cut(p2.X, p2.Y),
				Center:  cut(a.Center.X, a.Center.Y),
				CW:      a.CW,
				Feed:    feed,
				Spindle: p.Spindle,
				Coolant: p.Coolant,
				Milling: p.Milling,
				Comment: "row join",
			})
		} else {
			diags = append(diags, Diagnostic{Level: "info",
				Message: fmt.Sprintf("join from row %d to row %d: no safe semicircle — degraded to a lift", i-1, i)})
			out.Moves = append(out.Moves,
				retract(p1.X, p1.Y),
				rapid(p1.X, p1.Y, p2.X, p2.Y),
				plunge(p2.X, p2.Y))
		}
		// The row's intervals, with a lift between consecutive ones.
		o := cur.order()
		for j, iv := range o {
			if j > 0 {
				diags = append(diags, Diagnostic{Level: "info",
					Message: fmt.Sprintf("row %d: lifted between intervals — the gap is over kept material and is never cut through", i)})
				out.Moves = append(out.Moves,
					retract(cur.endOf(o[j-1]), cur.y),
					rapid(cur.endOf(o[j-1]), cur.y, cur.startOf(iv), cur.y),
					plunge(cur.startOf(iv), cur.y))
			}
			out.Moves = append(out.Moves, line(cur.startOf(iv), cur.y, cur.endOf(iv), cur.y))
		}
	}

	// Final retract.
	last := rows[len(rows)-1]
	lastEnd := last.order()[len(last.order())-1]
	out.Moves = append(out.Moves, retract(last.endOf(lastEnd), last.y))

	out.Metadata = map[string]string{
		"strategy":     "trochoidal-raster",
		"stepover":     fmt.Sprintf("%g", p.Stepover),
		"a_e":          fmt.Sprintf("%g", aE),
		"feed_model":   "F_full * sqrt(a_e / D)",
		"feed_cutting": fmt.Sprintf("%g", feed),
		"rows":         fmt.Sprintf("%d", len(rows)),
		"slivers":      fmt.Sprintf("%d", slivers),
	}
	if err := out.Validate(); err != nil {
		return moves.Path{}, nil, fmt.Errorf("toolpath: Pocket produced an invalid path: %w", err)
	}
	return out, diags, nil
}

// rowYs returns the raster row heights for a region spanning [minY, maxY]
// (ADR-027): the ADR-026 progression minY + s/2 + i·s (while it stays at
// least s/2 below the top), with the coverage guarantees — a single middle
// row when the region is at most one stepover tall, and a final row at
// maxY − s/2 when the progression would leave the top R uncovered. Every
// point of the region is then within s/2 <= R of a row.
func rowYs(minY, maxY, s, R float64) []float64 {
	h := maxY - minY
	if h <= s+kernel.AbsTol {
		return []float64{minY + h/2}
	}
	var rows []float64
	for i := 0; ; i++ {
		y := minY + s/2 + float64(i)*s
		if y > maxY-s/2+kernel.AbsTol {
			break
		}
		rows = append(rows, y)
	}
	if len(rows) == 0 { // unreachable when h > s; keep the function total
		rows = []float64{minY + h/2}
	}
	if maxY-rows[len(rows)-1] > R+kernel.AbsTol {
		rows = append(rows, maxY-s/2)
	}
	return rows
}

// chainBBox returns a conservative bounding box of the chain: exact for
// line edges, centre ± radius for arcs (the arc's true extent is smaller;
// the extra margin only adds rows that interval clipping then empties).
func chainBBox(c kernel.Chain2) (minX, minY, maxX, maxY float64) {
	minX, minY = math.Inf(1), math.Inf(1)
	maxX, maxY = math.Inf(-1), math.Inf(-1)
	upt := func(x, y float64) {
		if x < minX {
			minX = x
		}
		if y < minY {
			minY = y
		}
		if x > maxX {
			maxX = x
		}
		if y > maxY {
			maxY = y
		}
	}
	for _, e := range c.Edges {
		if e.IsLine() {
			l, _ := e.AsLine()
			upt(l.Start.X, l.Start.Y)
			upt(l.End.X, l.End.Y)
		} else {
			a, _ := e.AsArc()
			upt(a.Center.X-a.Radius, a.Center.Y-a.Radius)
			upt(a.Center.X+a.Radius, a.Center.Y+a.Radius)
		}
	}
	return
}

// rowIntervals returns the sorted, disjoint x-intervals where the horizontal
// line y = const crosses the interior of the closed chain c (the even-odd
// rule). Vertex handling (ADR-026): a vertex on the row counts a crossing
// iff the two adjacent edges leave opposite sides of the row — implemented
// per edge as "strictly-below differs", which counts a local extremum on
// the row as two (even — a touch, no interior) and a local minimum as zero,
// and a collinear boundary edge as nothing (the safe measure-zero reading).
// An odd count (an unhandled grazing degeneracy) is a precise refusal, not
// a guess.
func rowIntervals(c kernel.Chain2, y float64) ([]interval, error) {
	xs := make([]float64, 0, 8)
	for _, e := range c.Edges {
		switch {
		case e.IsLine():
			l, _ := e.AsLine()
			belowA := l.Start.Y < y-kernel.AbsTol
			belowB := l.End.Y < y-kernel.AbsTol
			if belowA == belowB {
				continue // same side, or a collinear edge on the row
			}
			t := (y - l.Start.Y) / (l.End.Y - l.Start.Y)
			xs = append(xs, l.Start.X+t*(l.End.X-l.Start.X))
		default:
			a, _ := e.AsArc()
			// A long segment across the arc's span: every point of the arc
			// lies within centre ± radius, so onSegment holds for true hits.
			line := kernel.Line2{
				Start: kernel.P2(a.Center.X-a.Radius-1, y),
				End:   kernel.P2(a.Center.X+a.Radius+1, y),
			}
			for _, pt := range kernel.LineArcIntersect(line, a) {
				// A point at the circle's top or bottom is a tangency: the
				// row touches the arc without crossing the interior (0, not
				// 1, crossings — the even-odd rule).
				if math.Abs(math.Abs(pt.Y-a.Center.Y)-a.Radius) <= kernel.AbsTol {
					continue
				}
				xs = append(xs, pt.X)
			}
		}
	}
	if len(xs)%2 != 0 {
		return nil, fmt.Errorf("row at y=%g grazes the boundary (odd intersection count %d) — adjust the stepover so the row clears the boundary", y, len(xs))
	}
	sort.Float64s(xs)
	out := make([]interval, 0, len(xs)/2)
	for i := 0; i+1 < len(xs); i += 2 {
		if xs[i+1]-xs[i] > kernel.AbsTol {
			out = append(out, interval{lo: xs[i], hi: xs[i+1]})
		}
	}
	return out, nil
}

// subtractIntervals returns the sorted, disjoint intervals that lie in a
// but not in b (b is merged first; a is assumed sorted and disjoint).
func subtractIntervals(a, b []interval) []interval {
	bm := mergeIntervals(b)
	out := make([]interval, 0, len(a))
	for _, av := range a {
		cur := av.lo
		for _, bv := range bm {
			if bv.hi <= cur {
				continue
			}
			if bv.lo >= av.hi {
				break
			}
			if bv.lo > cur {
				out = append(out, interval{lo: cur, hi: bv.lo})
			}
			if bv.hi > cur {
				cur = bv.hi
			}
			if cur >= av.hi {
				break
			}
		}
		if cur < av.hi {
			out = append(out, interval{lo: cur, hi: av.hi})
		}
	}
	return out
}

// mergeIntervals merges overlapping or touching intervals into a sorted,
// disjoint list.
func mergeIntervals(ivs []interval) []interval {
	if len(ivs) == 0 {
		return nil
	}
	s := make([]interval, len(ivs))
	copy(s, ivs)
	sort.Slice(s, func(i, j int) bool { return s[i].lo < s[j].lo })
	out := []interval{s[0]}
	for _, v := range s[1:] {
		last := &out[len(out)-1]
		if v.lo <= last.hi+kernel.AbsTol {
			if v.hi > last.hi {
				last.hi = v.hi
			}
		} else {
			out = append(out, v)
		}
	}
	return out
}

// joinArc builds the row-join semicircle with diameter P1P2 (ADR-026): of
// the two semicircles, the one that stays inside (or on) the region and
// outside (or on) every boss is used — the offset boundaries already embed
// the tool radius, so no crossing plus the bulge on the right side means
// clearance >= R everywhere on the arc (Law 7). The candidate whose bulge
// is inside the region is preferred. ok=false means no candidate passed;
// the caller degrades to a lift (the safe fallback, with a diagnostic).
func joinArc(p1, p2 kernel.Point2, region kernel.Chain2, bosses []kernel.Chain2) (kernel.Edge2, bool) {
	if p1.Eq(p2, kernel.AbsTol) {
		return kernel.Edge2{}, false
	}
	r := p1.Dist(p2) / 2
	if r <= kernel.AbsTol {
		return kernel.Edge2{}, false
	}
	mid := kernel.P2((p1.X+p2.X)/2, (p1.Y+p2.Y)/2)
	var cands []kernel.Edge2
	for _, cw := range []bool{false, true} {
		e, err := kernel.NewArcEdge(mid, p1, p2, cw)
		if err != nil {
			continue // sweep out of (0, 2π): degenerate chord
		}
		cands = append(cands, e)
	}
	type scored struct {
		e           kernel.Edge2
		bulgeInside bool
	}
	var passed []scored
	for _, e := range cands {
		if !joinGate(e, p1, p2, region, bosses) {
			continue
		}
		a, _ := e.AsArc()
		passed = append(passed, scored{e: e, bulgeInside: kernel.PointInContour(a.PointAt(0.5), region)})
	}
	if len(passed) == 0 {
		return kernel.Edge2{}, false
	}
	for _, s := range passed {
		if s.bulgeInside {
			return s.e, true
		}
	}
	return passed[0].e, true
}

// joinGate reports whether the arc stays inside (or on) the region and
// outside (or on) every boss: no edge crossing of any boundary, plus the
// arc's midpoint (its bulge point) on the right side of each boundary — a
// connected arc that never crosses a closed boundary stays entirely on the
// side its midpoint is on. "On" the boundary means the tool disc is
// tangent to the true boundary (the offsets embed R) — the safe limit.
//
// The arc's own endpoints (p1, p2) are exempt from the crossing test:
// they sit on a boundary by construction (a row interval ends where the row
// meets the region or a boss boundary, tangent to the true wall), and the
// join simply departs from that already-legal attachment point.
func joinGate(e kernel.Edge2, p1, p2 kernel.Point2, region kernel.Chain2, bosses []kernel.Chain2) bool {
	a, _ := e.AsArc()
	bulge := a.PointAt(0.5)
	for _, re := range region.Edges {
		for _, pt := range kernel.CurveIntersect(e, re) {
			if !pt.Eq(p1, kernel.AbsTol) && !pt.Eq(p2, kernel.AbsTol) {
				return false
			}
		}
	}
	if !kernel.PointInContour(bulge, region) && kernel.PointToChainDist(bulge, region) > kernel.AbsTol {
		return false // the arc's middle is outside the region
	}
	for _, b := range bosses {
		for _, be := range b.Edges {
			for _, pt := range kernel.CurveIntersect(e, be) {
				if !pt.Eq(p1, kernel.AbsTol) && !pt.Eq(p2, kernel.AbsTol) {
					return false
				}
			}
		}
		if kernel.PointInContour(bulge, b) {
			return false // the arc's middle is inside kept material
		}
	}
	return true
}
