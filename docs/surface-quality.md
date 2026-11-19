# Surface quality & 3D machining

This is the analysis behind "how fine should it go, how many passes, which
strategy" — framed as **configuration the software exposes**, not constants it
hard-codes (ADR-011). The reader may not be a machinist; the goal is to give the
software the *right knobs* and the *relationships* between them, so the choices
are made deliberately and the results are predictable.

## Why 3D is harder than 2.5D (and what that means for the design)

In 2.5D the tool moves on a plane at a fixed Z. The only "geometry" is a 2D
contour offset by a *radius*; the tool reduces to a number. Clean and testable.

In 3D the tool moves over a **surface**, and at every point several things become
functions of the *local surface normal and curvature*:

| Concern | 2.5D | 3D |
|---|---|---|
| Tool shape | a radius | a **tip envelope** `r(z)` (surface of revolution) |
| Compensation | offset a line/arc | **place the tool surface** tangent to the workpiece |
| Clearance | distance to a contour | distance from the tool *volume* to the kept volume |
| "How fine" | stepover (constant) | **adaptive stepover** driven by local curvature |
| Engagement/chip load | per-edge constant | varies continuously along the path |

The *hard* modules in 3D are therefore: **(a) tool-surface placement/offset**
(the classic "tool placement" problem, which is non-unique and singular at high
curvature), **(b) volume collision** (tool envelope vs. workpiece + workholding),
and **(c) adaptive stepover** (curvature-driven). The design answer (ADR-011):
keep the *pipeline* the same and add these as **separate, heavily-tested
modules** — so 3D is "add modules," not "re-architect." 2.5D is the degenerate
case (Z constant) and exercises the same spine now.

## The core 3D primitive: the tool tip envelope

Every milling tip is a **surface of revolution** about the spindle axis. Describe
it by the cutting radius as a function of height above the tip, `r(z)`:

| Tip | `r(z)` (z = height above the tip) |
|---|---|
| **Flat end mill** | `D/2` over the cutting length, else `0` (a step). |
| **Ball-nose** | spherical cap of radius `R = D/2`: `r(z) = √(R² − (R − z)²)` for `0 ≤ z ≤ R`; `R` above that. |
| **Bull-nose** | corner-radius cap `c`: `r(z) = √(c² − (c − z)²)` for `0 ≤ z ≤ c`; `D/2` above. |
| **Taper** | cone: `r(z) = r_tip + z·tan θ`. |

This one function is what:
- **2D offset** uses (`r` at the working Z),
- **3D compensation** uses (offset the surface, not a point),
- **scallop height** uses (the tip curvature), and
- **engagement / chip load** uses (the engaged arc length depends on the shape).

The current `Tool` fields (`kind`, `diameter`, `corner_radius`, `taper_angle`)
already *encode* this envelope — a flat mill has `corner_radius = 0`, a ball-nose
has `corner_radius = D/2`, a bull-nose is in between, a taper sets `taper_angle`.
So the tool model is **already 3D-ready**; the `tipRadius(z)` helper is a pure,
testable addition. Flute count (`flutes`) feeds the feed/chip-load math in every
case (see [tool-parameters.md](tool-parameters.md)).

## The finish-quality knob: scallop height ↔ stepover

For a ball-nose (or bull-nose cap) finishing a surface, the **peak-to-valley
texture left between adjacent passes** — the *scallop height* `h` — is set by the
lateral **stepover** `s`. This is *the* relationship that answers "how fine":

```
h = R − √( R² − (s/2)² )          (solve for s given a target h)
s = 2·√( 2·R·h − h² )
```

- A machinist sets a **target scallop height** `h_max` (e.g. `0.05 mm` for a fine
  finish, `0.2 mm` for a rough-ish pass) and the tool derives the stepover `s`.
  Equivalently they set `s` and the tool reports `h`. Either way it is a defined,
  testable function — not a fudge.
- For a bull-nose the same holds on the cap, with `R` = the corner radius.
- The **finishing allowance** is what makes this honest: roughing leaves
  `finishing_allowance` (e.g. `0.1–0.3 mm`) of stock so the finish pass isn't
  cutting into the valleys of the rough marks. The finish pass then takes that
  allowance in small, high-precision steps to hit `h_max`.

## The pass structure: rough → (semi-finish) → finish

"How many passes" decomposes into a **stage structure**, each with its own
goal, feed, and tolerance:

| Stage | Goal | Typical DOF / stepover | Strategy | Tolerance |
|---|---|---|---|---|
| **Rough** | Remove most material, control load | large depth-of-cut, large stepover | adaptive / trochoidal (load control) | clear the feature to `finishing_allowance` |
| **Semi-finish** *(optional)* | Reduce residual, raise precision | medium | Z-level / scallop | tighten toward the finish band |
| **Finish** | Hit the target surface | small depth-of-cut, stepover from `h_max` | scallop / flow / contour-parallel | within `surface_tolerance` of nominal |

- **Convergence:** a rough/adaptive pass stops when its engagement (and thus
  removal) drops below a `min_engagement` — this bounds "how many passes" rather
  than fixing a count.
- **Depth-of-cut per pass** is load-limited (chip volume per tooth), so it is set
  by the tool's `fz`/`vc` and the engagement model, clamped by `max_rpm`
  (see [tool-parameters.md](tool-parameters.md)).

## Organic shapes: adaptive (curvature-driven) stepover

On a flat surface a constant stepover gives a constant scallop. On an organic
surface the *same* stepover gives a *worse* scallop where the surface is more
curved. The fix is to make the stepover a **function of local curvature**: keep
the scallop at `h_max` by shrinking the stepover as curvature rises.

- Model: `s(local) = 2·√(2·R·h_max − h_max²)` adjusted by the local radius of
  curvature (smaller curvature radius → smaller stepover).
- This is a **per-point** computation along the finish path and is a well-scoped,
  testable module (input: surface point + normal + curvature + tool + `h_max` →
  output: stepover / next path point).
- It is the difference between "a pass every 0.5 mm" (bad on domes) and "a pass
  wherever the surface needs it" (good on domes).

## Steep vs. flat regions: per-region strategy

A single strategy rarely fits an organic part. The planner should be **region
aware**, switching strategy by local steepness:

- **Flat / shallow** (surface normal near +Z): scallop / flow — smooth, even
  finish.
- **Steep / near-vertical** (normal far from +Z): **contour-parallel** or
  **pencil** milling — follow the surface normal so the tool stays engaged and
  the finish is even on the wall.
- **`steep_angle_threshold`** is the knob that decides the switch (e.g. a face
  steeper than 45° from horizontal is "steep").
- **Undercuts / concavities** may need a smaller tool than the roughing tool to
  reach; the planner selects from the tool crib by reach, not just diameter.

## The configuration a machinist sets (the exposed knobs)

These are the inputs the planner should accept (a `SurfaceSpec`, or per-operation
overrides) rather than hide:

| Knob | Meaning | Default posture |
|---|---|---|
| `target_scallop` (or `target_stepover`) | Finish texture — pick one, derive the other. | required for a finish op |
| `rough_depth_of_cut` + `rough_strategy` | How much to take per rough pass; `adaptive`/`trochoidal` recommended. | load-limited |
| `finishing_allowance` | Stock left for the finish pass (e.g. `0.1–0.3 mm`). | required if multi-stage |
| `finish_strategy` | `scallop` / `flow` / `contour_parallel`. | by region |
| `finish_depth_of_cut` + `finish_stepover` | The final cut; stepover from `target_scallop` if not given. | derived |
| `steep_angle_threshold` | Normal angle below which a region is "steep." | e.g. 45° |
| `surface_tolerance` | Allowed deviation from the nominal surface. | bounds finish accuracy |
| `max_passes` / `min_engagement` | Stop conditions for convergence. | `min_engagement` recommended |
| `max_passes_per_op` | Hard cap (safety / time). | optional |

A machinist who is not yet fluent can start from two numbers — **how fine
(`target_scallop`)** and **how deep per pass (`rough_depth_of_cut`)** — and the
planner fills in the rest from the tool envelope and the surface. As they learn,
they gain more knobs. That progression is the point of exposing config instead of
hard-coding it.

## What this means for the architecture

- **New modules (3D):** `surface` (Z-height / normal / curvature queries over the
  part), `placement` (tool-surface offset), `collision` (tool volume vs. kept
  volume + workholding), `stepover` (adaptive, curvature-driven). Each is a
  separate, heavily-tested package with its own ADR when it introduces new
  geometry.
- **Config:** a `SurfaceSpec` (the knobs above) is part of the planner's input and
  is *portable JSON*, like the tool and machine configs. A part + `SurfaceSpec` +
  tool + machine is a complete, shareable "recipe."
- **Format:** no change needed for 2.5D; 3D arcs add an explicit plane normal
  (ADR-011) so CW/CCW is unambiguous.
- **Tests:** the scallop↔stepover relation, the `tipRadius(z)` per tip family,
  the convergence stop, and the region/steepness switch each get their own
  table-driven tests with analytic assertions.

## v0.1 status

None of the 3D modules exist yet — v0.1 is 2.5D (profile). But the **design is
3D-ready** (ADR-011): the spine, the `moves` format, and the tool model are the
same ones 3D will use. The `tipRadius(z)` helper, the `SurfaceSpec`, and the
four 3D modules are the planned additions, and their *models* are specified here
so they are built to a known target rather than discovered. See
[roadmap.md](roadmap.md) for the phase in which each lands.
