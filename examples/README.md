# Examples

Portable config and toolpath examples that validate against the schemas in
`../schemas/`.

| File | What it is |
|---|---|
| [`parts/plate_60x40x10_hole20.json`](parts/plate_60x40x10_hole20.json) | A part *recipe* (ADR-018) over the example STL: a 60 × 40 × 10 plate with a 20 × 20 through-hole. `slice_z`/`cut_z` are omitted — the defaults apply. |
| [`parts/plate_60x40x10_hole20.stl`](parts/plate_60x40x10_hole20.stl) | The example part's geometry (pinned by the mesh and part tests). |
| [`stock/block_120x80x20.json`](stock/block_120x80x20.json) | A 120 × 80 × 20 mm box of stock. |
| [`stock/cylinder_d60x15.json`](stock/cylinder_d60x15.json) | A Ø60 × 15 mm cylindrical stock. |
| [`stock/prism_rounded_80x50x12.json`](stock/prism_rounded_80x50x12.json) | A prismatic stock: a rounded-rectangle footprint extruded. |
| [`fixture/clamp_setup.json`](fixture/clamp_setup.json) | Workholding: vise jaws + an edge clamp as keep-out volumes (ADR-012). |
| [`tools/library.json`](tools/library.json) | A tool crib: a flat 4-flute end mill, a ball-nose, and a bull-nose, each with per-material feed/speed recommendations. |
| [`machines/grbl_3axis.json`](machines/grbl_3axis.json) | A typical GRBL 3-axis mill (mm) with its post-processor codes. |
| [`machines/linuxcnc_3axis.json`](machines/linuxcnc_3axis.json) | A LinuxCNC 3-axis mill (inch) with dialect-specific framing. |
| [`moves/demo_profile.json`](moves/demo_profile.json) | A 2D profile of a 100 × 60 mm rectangle in the intermediate format — inspect this to see what a user can override. |

## Using them

These are reference data. In v0.1 the CLI's `demo` command generates an
equivalent profile programmatically:

```sh
go run ./cmd/cleancuts demo
```

A later CLI (`cleancuts generate --part … --stock … --tool … --machine …`) will
load these JSON files directly and emit `.moves.json` + `.nc` output.

The schemas are the contract:

- `../schemas/part.schema.json` — a part recipe over a mesh (ADR-018)
- `../schemas/stock.schema.json` — the stock solid (ADR-015)
- `../schemas/fixture.schema.json` — workholding / keep-out zones (ADR-012)
- `../schemas/tool.schema.json` — one tool
- `../schemas/tool_library.schema.json` — a tool crib
- `../schemas/machine.schema.json` — a machine + post-processor
- `../schemas/moves.schema.json` — the intermediate toolpath format
