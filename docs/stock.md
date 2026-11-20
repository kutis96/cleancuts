# Stock (the material the part is milled from)

The part is milled *from* a finite block of material (ADR-015). Stock is a
citizen of the setup recipe — `part + stock + fixture + tool + machine` — and
is **data** (Law 6): a portable JSON document, kept in a crib like a tool.

## What the core needs to know about stock

1. **What material exists at a height** — the XY section at the cutting depth
   (roughing, part-fits checks).
2. **How high the stock top is at a point** — a plunge must start *above* the
   stock top at the entry point (a plunge below the top digs a crater in
   stock that should be left for the cut).
3. **That the part fits** — the part's footprint at its Z must lie inside the
   stock's section there, and the stock top must be above the part top.

Everything else (roughing strategies, stock-to-stock tooling) builds on these
three answers.

## The shapes (v1)

| Shape | Parameters (canonical WCS, mm) | Section |
|---|---|---|
| **box** | opposite corners `min`/`max` | a rectangle (4 lines) |
| **cylinder** | centre XY, radius, Z range | a circle (two half-arcs — an arc is never a full circle, ADR-014) |
| **prism** | a closed `kernel.Chain2` footprint, Z range | the footprint |

A **mesh stock** (STL/3MF block) is a later extension behind the same
surface, using the `mesh` slice's `SectionAt` — nothing downstream changes
(ADR-015).

## The primitives (all pure, all tested)

- **`SectionAt(z)`** → the closed XY chain of the cross-section at height `z`,
  or an error when `z` is outside the stock's Z range. One function, one
  source of truth (ADR-015).
- **`TopAt(x, y)`** → the top Z of the stock above the point (x, y), or an
  error if the point is outside the footprint. For v1 shapes this is the
  Z-max of the shape (they are extrusions); a mesh stock returns the true
  surface height.
- **`Contains(p)`** → whether a WCS point is inside the stock (footprint ×
  Z range).
- **`Validate()`** → exactly one shape, sane ranges.

## The fits check (a consumer of the primitives)

`Stock.Fits(part, partZ, partTop)` — the part's closed contour at height
`partZ` lies inside the stock's section there: **every vertex and every edge
midpoint is inside or on the section boundary** (on the boundary counts as
inside — a flush setup is legal), and `partTop` is not above the stock's top
(flush is legal; higher is not). The midpoint test catches an arc that
bulges past a straight stock wall even though its vertices are inside. A
precise error names the offending point and the stock. This is the "does
this setup make sense" gate the `generate` command runs before planning —
a sanity gate, not a substitute for the safety gate (ADR-012), which
verifies the actual tool envelope.

## Config (JSON)

See [schemas/stock.schema.json](../schemas/stock.schema.json) and
[examples/stock/](../examples/stock/). Example:

```json
{
  "id": "block-120x80x20",
  "name": "120 x 80 x 20 mm aluminium block",
  "shape": "box",
  "box": {
    "min": { "x": -10, "y": -10, "z": 0 },
    "max": { "x": 110, "y": 70, "z": 20 }
  }
}
```

## Status

**v0.1 (this slice):** box / cylinder / prism with `SectionAt`, `TopAt`,
`Contains`, `Fits`, JSON schema, and examples — green and tested.
**Later:** mesh stock behind the same surface (true `TopAt` from the surface);
stock-aware roughing (phase 3); stock allowance / finish stock as a
*parameter* of the part, not a second solid.
