// Package planner is CleanCuts's first-cut operation planner (ADR-018 §3):
// the thin composition that maps
//
//	part + mesh + stock + fixture + tool + machine + process
//
// to an ordered, verified list of moves.Path — one per milled feature —
// with a single coherent Z model (cut_z / safe_z) and process checks.
// Per feature it runs exactly (ADR-008: the planner re-derives no
// geometry):
//
//	comp.Offset (direction by feature kind, tool radius)
//	  → per chain planning.PlanFeature (milling mode + keep-out-safe entry)
//	  → toolpath.Loop (plunge, cutting moves, retract)
//	  → safety.CheckPath (the tool envelope clears every keep-out)
//
// planner is the one package that fans in over the pipeline; it is small,
// stateless, and its output (Result) is a value the program layer
// (moves.Program) and the future GUI can both consume. See docs/part.md.
package planner

import (
	"fmt"
	"strconv"

	"github.com/kutis96/cleancuts/internal/comp"
	"github.com/kutis96/cleancuts/internal/kernel"
	"github.com/kutis96/cleancuts/internal/machine"
	"github.com/kutis96/cleancuts/internal/mesh"
	"github.com/kutis96/cleancuts/internal/moves"
	"github.com/kutis96/cleancuts/internal/part"
	"github.com/kutis96/cleancuts/internal/planning"
	"github.com/kutis96/cleancuts/internal/safety"
	"github.com/kutis96/cleancuts/internal/stock"
	"github.com/kutis96/cleancuts/internal/tool"
	"github.com/kutis96/cleancuts/internal/toolpath"
)

// DefaultSafeClearance is the default rapid clearance above the stock top,
// in mm (ADR-018 §4). 5 mm is a common shop-floor safe height.
const DefaultSafeClearance = 5.0

// Diagnostic is a note about a safe resolution the plan took (e.g. a
// partially dropped offset region — residual material, the safe side).
// Diagnostics do not block the plan; refusals are errors.
type Diagnostic struct {
	// Level is "info" or "warning" (mirrors comp's diagnostic levels).
	Level string
	// Feature is the part.Feature index the note belongs to.
	Feature int
	Message string
}

// FeaturePath is one milled feature: the verified toolpath plus the entry —
// the plunge/retract column XY. The entry is the data the program layer
// needs to emit and verify the linking rapid between features (ADR-018 §4).
type FeaturePath struct {
	Feature part.Feature
	Path    moves.Path
	Entry   kernel.Point2
}

// Result is the verified first-cut plan (ADR-018): the Z model, one
// self-contained, safety-checked path per feature (plunge → cutting loop →
// retract), and any safe-resolution diagnostics. Linking rapids between
// features are NOT here — they belong to the program layer (slice L), which
// materializes them at SafeZ from the FeaturePath.Entry values and verifies
// them with safety.
type Result struct {
	// SliceZ is the height the part was sectioned at.
	SliceZ float64
	// CutZ is the tool-tip height of the cut.
	CutZ float64
	// SafeZ is the rapid safe height (stock top + clearance).
	SafeZ float64
	// Features are ordered: profiles first (largest first), then the voids
	// they enclose, then islands (part.Detect's order).
	Features []FeaturePath
	// Diagnostics are safe-resolution notes (residual material, …).
	Diagnostics []Diagnostic
}

// Validate enforces the result invariants (Law 5): a non-empty feature
// list, a sane Z model, and a structurally valid path per feature.
func (r Result) Validate() error {
	if len(r.Features) == 0 {
		return fmt.Errorf("planner: result has no features")
	}
	if !(r.CutZ < r.SafeZ) {
		return fmt.Errorf("planner: result has CutZ %g not below SafeZ %g", r.CutZ, r.SafeZ)
	}
	if r.SliceZ <= r.CutZ {
		return fmt.Errorf("planner: result has SliceZ %g not above CutZ %g", r.SliceZ, r.CutZ)
	}
	for i, f := range r.Features {
		if err := f.Path.Validate(); err != nil {
			return fmt.Errorf("planner: feature %d: %w", i, err)
		}
	}
	return nil
}

// Params is everything Plan needs. The geometry inputs are values (the CLI
// resolves the Part's mesh file and the JSON configs — core purity, Law 4);
// Feed/Spindle/Milling/Coolant are the resolved process (ADR-018 §5:
// explicit, checked, refused — not clamped; clamping is phase 4).
type Params struct {
	Part     part.Part
	Mesh     mesh.Mesh
	Stock    stock.Stock
	KeepOuts []planning.KeepOut
	Tool     tool.Tool
	Machine  machine.Machine

	// Feed is the cutting feed rate (mm/min).
	Feed float64
	// Spindle is the spindle speed (RPM).
	Spindle float64
	// Milling is the requested mode for all features; empty means climb
	// (the ADR-005 default). Per-feature override is a later extension —
	// the seam (planning.Params.Milling) already exists.
	Milling moves.Milling
	// Coolant is the coolant state during cutting; empty means off.
	Coolant moves.Coolant

	// CutZ overrides the part's cut_z (which itself defaults to the part's
	// minimum Z). Nil = no override. A pointer, not a 0 sentinel: the WCS
	// is arbitrary and z=0 is a legitimate height.
	CutZ *float64
	// SafeClearance is the rapid clearance above the stock top (mm);
	// <= 0 uses DefaultSafeClearance.
	SafeClearance float64
}

// Plan maps the setup to an ordered, verified first-cut plan (ADR-018).
// Any setup problem is a precise refusal naming the offending value — the
// safe failure is no program, never a program that over-cuts (Law 7).
func Plan(p Params) (Result, error) {
	if err := p.Part.Validate(); err != nil {
		return Result{}, err
	}
	if err := p.Mesh.Validate(); err != nil {
		return Result{}, err
	}
	if err := p.Stock.Validate(); err != nil {
		return Result{}, err
	}
	if err := p.Tool.Validate(); err != nil {
		return Result{}, err
	}
	if err := p.Machine.Validate(); err != nil {
		return Result{}, err
	}
	// The core plans in millimetres (ADR-001); units mapping is a
	// post-processing concern and v1 implements only the identity (mm) map.
	// Refuse inch machines up front: comparing mm process values against
	// inch limits is a category error, and emitting G20 over mm numbers
	// would scale the whole program by 25.4x (over-cut). Inch support is
	// deferred (roadmap) — the safe failure is no program.
	if p.Machine.Units != "mm" {
		return Result{}, fmt.Errorf("planner: machine %q is configured for %s; v1 plans in millimetres and supports only mm machines (inch units are a deferred post-processing concern, see docs/gcode.md)",
			p.Machine.ID, p.Machine.Units)
	}
	for i, k := range p.KeepOuts {
		if err := k.Validate(); err != nil {
			return Result{}, fmt.Errorf("planner: keep-out %d: %w", i, err)
		}
	}
	if !kernel.IsFinite(p.Feed) || p.Feed <= 0 {
		return Result{}, fmt.Errorf("planner: feed must be > 0 (got %g)", p.Feed)
	}
	if !kernel.IsFinite(p.Spindle) || p.Spindle <= 0 {
		return Result{}, fmt.Errorf("planner: spindle must be > 0 (got %g)", p.Spindle)
	}
	mode := p.Milling
	if mode == "" {
		mode = moves.MillingClimb
	}
	switch mode {
	case moves.MillingClimb, moves.MillingConventional:
	default:
		return Result{}, fmt.Errorf("planner: unknown milling mode %q (want climb or conventional)", mode)
	}
	coolant := p.Coolant
	if coolant == "" {
		coolant = moves.CoolantOff
	}
	switch coolant {
	case moves.CoolantOff, moves.CoolantFlood, moves.CoolantMist, moves.CoolantThrough:
	default:
		return Result{}, fmt.Errorf("planner: unknown coolant %q (want off, flood, mist, or through)", coolant)
	}
	if p.SafeClearance < 0 {
		return Result{}, fmt.Errorf("planner: safe clearance must be >= 0 (got %g)", p.SafeClearance)
	}
	clearance := p.SafeClearance
	if clearance <= 0 {
		clearance = DefaultSafeClearance
	}

	// --- The Z model (ADR-018 §4) -----------------------------------------
	sliceZ, err := p.Part.EffectiveSliceZ(p.Mesh)
	if err != nil {
		return Result{}, err
	}
	cutZ, err := p.Part.EffectiveCutZ(p.Mesh)
	if err != nil {
		return Result{}, err
	}
	if p.CutZ != nil {
		cutZ = *p.CutZ
	}
	if !kernel.IsFinite(cutZ) {
		return Result{}, fmt.Errorf("planner: cut_z must be finite (got %g)", cutZ)
	}
	stockMin, stockMax, err := p.Stock.ZRange()
	if err != nil {
		return Result{}, err
	}
	if cutZ >= sliceZ {
		return Result{}, fmt.Errorf("planner: cut_z (%g) must be strictly below slice_z (%g) — the cut must remove material below the section",
			cutZ, sliceZ)
	}
	if cutZ < stockMin-kernel.AbsTol {
		return Result{}, fmt.Errorf("planner: cut_z (%g) is below the stock bottom (z=%g) — the tool would cut into the table; raise cut_z or change the stock",
			cutZ, stockMin)
	}
	if cutZ > stockMax+kernel.AbsTol {
		return Result{}, fmt.Errorf("planner: cut_z (%g) is above the stock top (z=%g) — the tool would cut air; lower cut_z",
			cutZ, stockMax)
	}
	safeZ := stockMax + clearance

	// The tool's flute must cover the depth it is asked to cut: a short
	// flute would leave the top of the wall uncut (a silent wrong part).
	// partTop is the mesh's max Z: intentionally over-conservative for a
	// pocket that sits below a raised boss elsewhere (the flute must cover
	// boss-top to cut-bottom even though only the pocket depth is cut).
	// That is a safe-side over-refusal, acceptable in v1 — the correct
	// fix is per-feature top Z once multi-pass Z scheduling exists.
	partTop := p.Mesh.Bounds().Max.Z
	if p.Tool.FluteLength > 0 && partTop-cutZ > p.Tool.FluteLength+kernel.AbsTol {
		return Result{}, fmt.Errorf("planner: tool %q flute length %g cannot cover the cut: the part top is at z=%g and the cut is at z=%g — the top %g mm would be left uncut; use a longer flute or a smaller cut_z",
			p.Tool.ID, p.Tool.FluteLength, partTop, cutZ, partTop-p.Tool.FluteLength-cutZ)
	}

	// --- Features and fit ---------------------------------------------------
	features, err := part.Detect(p.Mesh, sliceZ)
	if err != nil {
		return Result{}, err
	}
	for _, f := range features {
		if err := p.Stock.Fits(f.Contour, sliceZ, partTop); err != nil {
			return Result{}, fmt.Errorf("planner: %s: %w", f, err)
		}
	}

	// --- Process checks (refused, not clamped — ADR-018 §5) -----------------
	if p.Machine.MinFeed > 0 && p.Feed < p.Machine.MinFeed {
		return Result{}, fmt.Errorf("planner: feed %g is below the machine %q minimum %g — raise the feed",
			p.Feed, p.Machine.ID, p.Machine.MinFeed)
	}
	if p.Machine.MaxFeed > 0 && p.Feed > p.Machine.MaxFeed {
		return Result{}, fmt.Errorf("planner: feed %g is above the machine %q maximum %g — lower the feed",
			p.Feed, p.Machine.ID, p.Machine.MaxFeed)
	}
	if p.Machine.SpindleMaxRPM > 0 && p.Spindle > p.Machine.SpindleMaxRPM {
		return Result{}, fmt.Errorf("planner: spindle %g RPM is above the machine %q maximum %g — lower the spindle speed",
			p.Spindle, p.Machine.ID, p.Machine.SpindleMaxRPM)
	}
	if p.Tool.MaxRPM > 0 && p.Spindle > p.Tool.MaxRPM {
		return Result{}, fmt.Errorf("planner: spindle %g RPM is above tool %q maximum %g — lower the spindle speed or use a different tool",
			p.Spindle, p.Tool.ID, p.Tool.MaxRPM)
	}

	// --- Per-feature composition (ADR-018 §3) --------------------------------
	label := p.Part.Name
	if label == "" {
		label = p.Part.ID
	}
	out := Result{SliceZ: sliceZ, CutZ: cutZ, SafeZ: safeZ}
	for _, f := range features {
		if err := planFeature(p, &out, f, mode, coolant, safeZ, cutZ, label); err != nil {
			return Result{}, err
		}
	}
	if err := out.Validate(); err != nil {
		return Result{}, fmt.Errorf("planner: produced an invalid result: %w", err)
	}
	return out, nil
}

// planFeature runs the per-feature pipeline for one feature and appends the
// verified path(s) to out (ADR-018 §3 and §6).
func planFeature(
	p Params,
	out *Result,
	f part.Feature,
	mode moves.Milling,
	coolant moves.Coolant,
	safeZ, cutZ float64,
	label string,
) error {
	// Direction by kind (ADR-018 §2): a material contour is cut with the
	// tool outside it; a void contour with the tool inside it.
	direction := comp.Outside
	if f.Kind == planning.Pocket {
		direction = comp.Inside
	}
	res, err := comp.Offset(f.Contour, comp.Options{
		Radius:    p.Tool.Radius(),
		Direction: direction,
	})
	if err != nil {
		return fmt.Errorf("planner: %s cannot be milled with tool %q (radius %g mm): %v — choose a smaller tool or change the setup",
			f, p.Tool.ID, p.Tool.Radius(), err)
	}
	if len(res.Chains) == 0 {
		detail := "the tool does not fit this feature"
		for _, d := range res.Diagnostics {
			detail = d.Message
			break
		}
		return fmt.Errorf("planner: %s is unreachable with tool %q (radius %g mm): %s — choose a smaller tool or change the setup",
			f, p.Tool.ID, p.Tool.Radius(), detail)
	}
	// Partial drops are diagnostics: residual material is the safe side
	// (ADR-013), phase 3's roughing is where the rest gets cut.
	for _, d := range res.Diagnostics {
		out.Diagnostics = append(out.Diagnostics, Diagnostic{Level: d.Level, Feature: f.Index, Message: d.Message})
	}

	sz := safeZ
	for ci, chain := range res.Chains {
		featurePlan, err := planning.PlanFeature(chain, planning.Params{
			Kind:     f.Kind,
			Spindle:  planning.SpindleRotation(p.Machine.SpindleRotation),
			Milling:  mode,
			CutZ:     cutZ,
			SafeZ:    &sz,
			KeepOuts: p.KeepOuts,
		})
		if err != nil {
			return fmt.Errorf("planner: %s (chain %d of %d): %v", f, ci+1, len(res.Chains), err)
		}
		name := fmt.Sprintf("%s — %s %d", label, f.Kind, f.Index)
		if len(res.Chains) > 1 {
			name = fmt.Sprintf("%s (%d/%d)", name, ci+1, len(res.Chains))
		}
		path, err := toolpath.Loop(featurePlan.Chain, toolpath.LoopParams{
			Operation: f.Kind.String(),
			Name:      name,
			Milling:   featurePlan.Milling,
			Feed:      p.Feed,
			Spindle:   p.Spindle,
			SafeZ:     safeZ,
			CutZ:      cutZ,
			Coolant:   coolant,
			ToolID:    p.Tool.ID,
		})
		if err != nil {
			return fmt.Errorf("planner: %s (chain %d of %d): %v", f, ci+1, len(res.Chains), err)
		}
		// Addressable ID (ADR-019 §1): paths are objects the user
		// inspects and overrides, so every path has a unique, meaningful ID.
		path.ID = fmt.Sprintf("%s-%d", f.Kind, f.Index)
		if len(res.Chains) > 1 {
			path.ID = fmt.Sprintf("%s-%d-c%d", f.Kind, f.Index, ci+1)
		}
		// The envelope gate (ADR-012): the tool's sweep, not just its
		// centre, must clear every keep-out — a hard constraint.
		if err := safety.CheckPath(path, p.Tool.Radius(), p.KeepOuts); err != nil {
			return fmt.Errorf("planner: %s (chain %d of %d): %v", f, ci+1, len(res.Chains), err)
		}
		out.Features = append(out.Features, FeaturePath{
			Feature: f,
			Path:    path,
			Entry:   featurePlan.Start,
		})
	}
	return nil
}

// ToProgram groups the plan's feature paths into an executable program
// (ADR-019 §2): the linking rapid between consecutive features is
// materialized as a standalone one-move path and verified against the
// keep-outs — a travel through a clamp is a refusal naming the zones, not
// a silent collision. A zero-length link (consecutive features sharing an
// entry) is omitted, not emitted. The Z model rides along in metadata for
// inspection and the GUI.
func (r Result) ToProgram(programID string, keepOuts []planning.KeepOut, toolRadius float64) (moves.Program, error) {
	if programID == "" {
		return moves.Program{}, fmt.Errorf("planner: program ID is required")
	}
	if err := r.Validate(); err != nil {
		return moves.Program{}, fmt.Errorf("planner: result: %w", err)
	}
	if toolRadius < 0 {
		return moves.Program{}, fmt.Errorf("planner: tool radius must be >= 0 (got %g)", toolRadius)
	}
	for i, k := range keepOuts {
		if err := k.Validate(); err != nil {
			return moves.Program{}, fmt.Errorf("planner: keep-out %d: %w", i, err)
		}
	}

	paths := make([]moves.Path, 0, 2*len(r.Features)-1)
	for i, fp := range r.Features {
		if i > 0 {
			link, err := linkPath(r.Features[i-1], fp, i, keepOuts, toolRadius)
			if err != nil {
				return moves.Program{}, err
			}
			if link != nil {
				paths = append(paths, *link)
			}
		}
		paths = append(paths, fp.Path)
	}

	prog := moves.Program{
		ID:    programID,
		Paths: paths,
		Metadata: map[string]string{
			"slice_z": strconv.FormatFloat(r.SliceZ, 'g', -1, 64),
			"cut_z":   strconv.FormatFloat(r.CutZ, 'g', -1, 64),
			"safe_z":  strconv.FormatFloat(r.SafeZ, 'g', -1, 64),
		},
	}
	if err := prog.Validate(); err != nil {
		return moves.Program{}, fmt.Errorf("planner: %w", err)
	}
	return prog, nil
}

// linkPath builds the verified travel between two consecutive features
// (ADR-019 §2), or nil when they share the same retract/plunge point.
func linkPath(prev, next FeaturePath, idx int, keepOuts []planning.KeepOut, toolRadius float64) (*moves.Path, error) {
	from := prev.Path.Moves[len(prev.Path.Moves)-1].To // the retract top
	to := next.Path.Moves[0].From                      // the plunge top
	if from.Eq(to, kernel.AbsTol) {
		return nil, nil
	}
	link := moves.Path{
		ID:        fmt.Sprintf("link-%d", idx),
		Name:      fmt.Sprintf("travel %s → %s", prev.Path.ID, next.Path.ID),
		Operation: "travel",
		Moves: []moves.Move{{
			Kind:    moves.KindRapid,
			From:    from,
			To:      to,
			Comment: "travel to next feature",
		}},
	}
	if err := link.Validate(); err != nil {
		return nil, fmt.Errorf("planner: travel between %s and %s: %w", prev.Path.ID, next.Path.ID, err)
	}
	// The envelope gate (ADR-012) on the travel too: a rapid through a
	// clamp destroys the setup — refuse, naming the zones.
	if err := safety.CheckPath(link, toolRadius, keepOuts); err != nil {
		return nil, fmt.Errorf("planner: travel between %s and %s: %w", prev.Path.ID, next.Path.ID, err)
	}
	return &link, nil
}
