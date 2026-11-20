# Workholding & keep-out zones

The part is held by something. In a real setup that's a vise, clamps, tabs, or a
fixture — geometry the tool must **never touch**. A collision with an edge clamp
or a vise jaw destroys the tool, the clamp, and usually the part, and can be a
safety hazard. So we treat "don't hit the clamp" as a first-class, declared,
verifiable property (ADR-012), not a hope.

## What a keep-out zone is

A **keep-out zone** is a **volume in the canonical WCS** (relative to the
stock/setup origin) that the **tool envelope** must never enter. It is a **hard
constraint**: a violation is a program-level error, not a warning.

This generalises "never over-cut the part" (ADR-010) to a broader rule:

> **The tool envelope must never intersect any keep-out volume — the part, the
> fixture, or any explicit no-go region.**

The distinction that matters:

| Thing | What it is | Tool relation |
|---|---|---|
| **Removed material** | stock that gets milled away | the *target* of cutting |
| **Kept material** | the part (and any stock that stays) | must not be cut |
| **Keep-out zone** | clamps, vise jaws, tabs, no-go regions | must **never be touched** |

Kept material and keep-out zones are both "don't touch," but keep-out zones are
*additional* geometry you declare explicitly (a clamp is not part of the part).

## The model

Keep-out zones live in a **fixture / workholding config** (portable JSON),
alongside the part, stock, tool, and machine. The volumes are simple, portable
solids:

- **box** — opposite corners `min`/`max` (clamps, vise jaws). The common case.
- **cylinder** — base centre + axis direction + radius + length.
- **mesh** — a solid mesh (STL/3MF) for a complex fixture, in the canonical frame.

A **complete, shareable setup is a recipe**:

```
part  +  stock  +  fixture(keep-out)  +  tool  +  machine   →   program
  (JSON)  (JSON)      (JSON)              (JSON)   (JSON)
```

This is the unit a shop keeps in a crib and moves between machines — and the unit
the planner and the collision check consume. Tool and machine configs already
exist; the fixture config is the new piece (see
[schemas/fixture.schema.json](../schemas/fixture.schema.json) and
[examples/fixture/clamp_setup.json](../examples/fixture/clamp_setup.json)).

## How it is enforced

Two independent layers, so a problem is caught early *and* verified:

1. **Planning routes around keep-out volumes.** Entry/exit points are chosen
   clear of clamps; retract heights are above the fixture; adaptive/trochoidal
   paths don't dive into a clamp region. This *prevents* violations by
   construction.
2. **Collision check verifies the whole program** (phase 4). For every point of
   every move, the tool **envelope** (the real tip surface, ADR-011 — not just the
   centre) is tested against all keep-out volumes *and* the kept-part volume.
   Any intersection **rejects the program** with a precise message (which move,
   which volume, at what point). This is the dry-run gate before emission.

The collision predicate — *tool envelope vs. a set of volumes* — is a pure,
independently testable function: a toolpath that clips a keep-out box is
rejected; one that clears it is accepted. Adding a clamp to the setup is a **data
change** (a new keep-out volume), not a code change (ADR-007).

### The envelope test (implemented in `internal/safety`)

At a cutting height `z`, a zone applies iff its Z range covers `z`; its XY
footprint is the box projection (a rectangle) or the cylinder disc. The tool
envelope of a move at that height is the sweep of its tip disc (radius `r`,
the tool's tip-envelope radius at working Z — ADR-011):

- **linear move** → a **capsule** (the segment plus the disc of radius `r`);
- **arc move** → an **annular sector** (annulus radii `R−r`…`R+r` over the
  arc's span).

The gate is the exact 2D distance between that sweep and the zone footprint:
**distance < `r` rejects the move** (tangent contact also rejects — touching a
clamp is a collision, and the safe side is to refuse). The distance is
analytic: point/segment-to-rectangle and -disc distances, segment-to-arc via
the foot of the perpendicular and the arc endpoints, arc-to-rectangle via its
four sides — all in the standard library, all unit-tested. `safety.CheckChain`
gates a planned tool-centre chain; `safety.CheckPath` gates a finished
`moves.Path` (including rapids and vertical plunges, which must clear the
fixture too). The error names the move, the zone, and the distance — precise
enough to fix the setup, not vague enough to guess.

## Example (a part in a vise)

`examples/fixture/clamp_setup.json` declares a 100×60 part held by front/back
vise jaws plus one edge clamp over a tab:

```json
{
  "id": "clamp-setup-100x60",
  "keep_out": [
    { "type": "box", "box": { "min": {"x":-20,"y":-20,"z":0}, "max": {"x":120,"y":-5,"z":20} },
      "note": "vise jaw (front) — never enter" },
    { "type": "box", "box": { "min": {"x":-20,"y":65,"z":0}, "max": {"x":120,"y":80,"z":20} },
      "note": "vise jaw (back) — never enter" },
    { "type": "box", "box": { "min": {"x":40,"y":24,"z":8}, "max": {"x":60,"y":36,"z":30} },
      "note": "edge clamp over a tab — keep clear" }
  ]
}
```

The planner will route the toolpath and retracts to stay out of these boxes, and
the collision check will reject any program whose tool envelope enters them.

## Status

- **v0.1:** the fixture config model, schema, and example are defined (the
  contract).
- **Phase 2 (in progress):** the planner picks entries clear of the zones
  (`internal/planning`), and `internal/safety` implements the analytic
  envelope test above (`CheckChain`, `CheckPath`). Both layers now exist;
  the `generate` command (slice L) wires fixture JSON into them.
- **Phase 4:** the gate extends to the kept-part volume and mesh keep-outs
  (stock/mesh slices), and to full 3D tool envelopes (ballnoses, tapers).
- **The point of declaring it now:** the *design* is settled (what a keep-out
  zone is, how it's specified, and that it's a hard constraint enforced in
  planning + collision), so when the routing and collision modules are built they
  are built to a known target.
