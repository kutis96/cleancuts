# Examples

Portable config and toolpath examples that validate against the schemas in
`../schemas/`.

| File | What it is |
|---|---|
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

- `../schemas/tool.schema.json` — one tool
- `../schemas/tool_library.schema.json` — a tool crib
- `../schemas/machine.schema.json` — a machine + post-processor
- `../schemas/moves.schema.json` — the intermediate toolpath format
