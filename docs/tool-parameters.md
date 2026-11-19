# Tool parameters

A tool definition is the single most important piece of *process* data in CAM.
Geometry (diameter, flutes, corner radius) tells the toolpath generator **what the
cutter can do**; process parameters (feed, speed, material) tell it **how fast and
how hard**. Both are needed to produce G-code that is not just geometrically
correct but physically sane. This document specifies what to store, why, and how
each feeds the pipeline.

## The two halves of a tool definition

```
┌──────────────────────────────────────────────────────────────────────┐
│                              Tool                                    │
│                                                                      │
│  GEOMETRY (what the cutter is)        PROCESS (how it cuts)          │
│  ─────────────────────────────        ─────────────────────          │
│  kind, diameter, flutes,              material, coating, max_rpm,    │
│  corner_radius, taper_angle,          feeds[] (per material):        │
│  flute_length, overall_length,        fz, vc, notes                  │
│  shank_diameter                                             │
└──────────────────────────────────────────────────────────────────────┘
        │                                            │
        ▼                                            ▼
  drives offset, reach,          drives feed/speed, constant-chip-load,
  corner join, stock-out         tool-load, safe parameters
```

### Geometry parameters

| Field | Type | Meaning | Drives |
|---|---|---|---|
| `kind` | enum | Tip family: `flat_end_mill`, `ball_nose`, `bull_nose`, `taper_end_mill`, `chamfer`, `drill`, `face_mill` | Which operations are valid, and the tip geometry used in offset/compensation and 3D scallop math. |
| `diameter` | mm | Cutting diameter (major). | Tool radius = `diameter/2`; the offset amount in 2D compensation; engagement and chip-load math. |
| `flutes` | int | Number of teeth (flutes). | Feed-per-tooth → table feed conversion (`feed = n · flutes · fz`); chip load and tool-load. |
| `corner_radius` | mm | Bull-nose corner radius (flat end mill with rounded corners). Valid only if `≤ diameter/2`. | Corner-join geometry in 2D offset; which features (e.g. sharp internal corners) the tool can reach. |
| `taper_angle` | deg | Half-angle for a tapered tool. Valid in `[0, 90)`. | Effective radius as a function of Z (a tapered tool's diameter varies with height) — needed for compensation and 3D. |
| `flute_length` | mm | Cutting length. | Reach/engagement limits; a pass cannot engage more material than this without risk. |
| `overall_length` | mm | Shank + flute. | Collisions with the vise/stock top; Z reach. |
| `shank_diameter` | mm | Shank diameter. | Collet fit, rigidity, and collision checks near the stock. |

**Notes.**

- `corner_radius` generalises the tip: a flat end mill has `corner_radius = 0`, a
  ball nose has `corner_radius = diameter/2`, a bull nose is in between. Storing
  `corner_radius` (rather than a boolean "ball nose") lets the offset/compensation
  module use one code path for all three.
- A `drill` is *not* a milling tool: it removes material axially, so most 2D
  toolpath strategies don't apply to it. We still store it in the same model for a
  uniform tool crib, but the planner will route drilling to its own operation
  later.

### Process parameters

| Field | Type | Meaning | Drives |
|---|---|---|---|
| `material` | string | Tool material: `carbide`, `hss`, `carbide-coated`, … | Stiffness, allowable speeds/feeds (bounds the process table). |
| `coating` | string | e.g. `TiAlN`, `AlCrN`, `none`. | Dry-cutting ability, allowable speeds. |
| `max_rpm` | rpm | Maximum safe spindle speed. | Upper bound on `n`; the planner must clamp `n ≤ max_rpm`. |
| `feeds[]` | list | Recommended feed-per-tooth (`fz`) and cutting speed (`vc`) **per workpiece material**. | The primary inputs to feed/speed calculation and to constant-chip-load scaling. |

`feeds[]` entries:

| Field | Type | Meaning |
|---|---|---|
| `material` | string | Workpiece material key, e.g. `aluminum-6061`, `steel-1018`. |
| `fz` | mm/tooth | Feed per tooth. The fundamental chip-thickness driver. |
| `vc` | m/min | Cutting (peripheral) speed; `n = vc · 1000 / (π · diameter)` gives RPM. `0` = "use fz only". |
| `note` | string | Free-form guidance (e.g. "use flood coolant", "climb preferred"). |

## The feed/speed model (and why `fz` is the anchor)

Everything in milling feed/speed hangs off **feed per tooth, `fz`**, because `fz`
*is* the chip thickness driver. The chain is:

```
fz  (mm/tooth)
 │
 ├── feed rate   F = n · flutes · fz          (mm/min)
 │
 └── with vc:    n = vc · 1000 / (π · D)      (rpm)
                 F = vc · 1000 · flutes · fz / (π · D)
```

- **`vc` (cutting speed)** is a property of the tool–workpiece pair; it sets the
  peripheral speed, and therefore the spindle speed `n`.
- **`fz` (feed per tooth)** sets the chip thickness and, with `n` and `flutes`,
  the table feed `F`.
- Storing **both** `fz` and `vc` (when available) lets the planner either
  *speed-limited* (`vc` → `n`, then `fz` → `F`) or *feed-limited* (`fz` → `F`),
  and then **clamp `n ≤ max_rpm`**. Whichever limit bites first wins.

**Constant chip-load (the friend's request).** When the radial engagement `a_e`
varies along a pass (as it does in trochoidal/adaptive clearing), holding `F`
constant makes the chip thickness vary — heavy where engagement is deep, light
where it is shallow. To keep the *chip thickness* (hence the load, heat, and
vibration) roughly constant, scale the feed by engagement:

```
F_engaged ≈ F_full · sqrt( a_e / D )        (constant chip-thickness model)
```

So as `a_e` drops toward zero, `F` rises; as `a_e` → D (full slot), `F` →
`F_full`. This is a first-order model; the adaptive module (phase 3) will compute
`a_e` per segment from the geometry and apply the scaling. The point here is that
`fz`/`D`/`flutes` are exactly the inputs that model needs, which is why they are
first-class tool fields.

## What the planner is allowed to assume

A tool definition must be **self-sufficient**: given a `Tool` and a workpiece
material, the planner can compute a safe `n` and `F` without any other source.
Concretely, the planner will:

1. Look up `feeds[]` for the workpiece material (fallback: a conservative default
   with a warning, or reject).
2. Compute `n` from `vc` (if present) else choose a safe `n`.
3. Clamp `n ≤ max_rpm`.
4. Compute `F = n · flutes · fz`.
5. Apply constant-chip-load scaling per engagement (phase 3+).
6. Record the resulting `spindle` and `feed` on the generated moves.

## Reference JSON schema

See [schemas/tool.schema.json](../schemas/tool.schema.json). A minimal tool:

```json
{
  "id": "endmill-10mm-4f",
  "name": "10 mm 4-flute carbide end mill",
  "kind": "flat_end_mill",
  "diameter": 10.0,
  "flutes": 4,
  "corner_radius": 0.0,
  "taper_angle": 0.0,
  "flute_length": 25.0,
  "overall_length": 75.0,
  "shank_diameter": 10.0,
  "material": "carbide",
  "coating": "TiAlN",
  "max_rpm": 24000,
  "feeds": [
    { "material": "aluminum-6061", "fz": 0.15, "vc": 200, "note": "climb, flood coolant" },
    { "material": "steel-1018",    "fz": 0.08, "vc": 80,  "note": "climb, flood coolant" }
  ]
}
```

## Invariants enforced by `tool.Tool.Validate()`

- `id` is present.
- `diameter > 0`.
- `flutes ≥ 0`.
- `corner_radius` in `[0, diameter/2]`.
- `taper_angle` in `[0, 90)`.
- `max_rpm ≥ 0`.
- every `feeds[i].fz > 0` and `feeds[i].vc ≥ 0`.

These are the "the file parsed, and it also makes physical sense" guarantees
(see ADR-007).

## v0.1 status

- **Implemented:** the full `Tool` and `Library` model, `Validate()`, JSON
  round-trip, and `Radius()`. The planner (phase 2+) will consume `feeds[]`,
  `max_rpm`, and `flutes` to drive feed/speed and constant-chip-load.
- **Planned:** a `ProcessTable` that resolves `feeds[]` for a workpiece material
  into a concrete `(n, F)` with clamping; collision-aware reach checks using
  `flute_length`/`overall_length`/`shank_diameter`; and per-operation feed
  overrides.
