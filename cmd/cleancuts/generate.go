package main

// generate is the phase-2 CLI (ADR-019 §5): it loads a part recipe, the
// mesh, the stock, the fixture, a tool, and a machine, runs the first-cut
// planner, and writes the inspectable intermediate format (.moves.json)
// plus machine G-code (.nc). The CLI is where I/O lives (Law 4) — every
// decision is made by the tested core packages.

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kutis96/cleancuts/internal/fixture"
	"github.com/kutis96/cleancuts/internal/gcode"
	"github.com/kutis96/cleancuts/internal/machine"
	"github.com/kutis96/cleancuts/internal/mesh"
	"github.com/kutis96/cleancuts/internal/moves"
	"github.com/kutis96/cleancuts/internal/part"
	"github.com/kutis96/cleancuts/internal/planner"
	"github.com/kutis96/cleancuts/internal/stock"
	"github.com/kutis96/cleancuts/internal/tool"
)

func generate(args []string) error {
	fs := flag.NewFlagSet("generate", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), strings.ReplaceAll(`
usage: cleancuts generate --part P.json --stock S.json --tool T.json --machine M.json [flags]

plans the part's 2.5D features and writes .moves.json (inspectable) + .nc (G-code).

required:
  --part       part recipe JSON (see schemas/part.schema.json)
  --stock      stock JSON (see schemas/stock.schema.json)
  --tool       tool or tool-library JSON (see schemas/tool*.schema.json)
  --machine    machine JSON (see schemas/machine.schema.json)

optional:
  --fixture    fixture/workholding JSON (keep-out zones; default: none)
  --tool-id    tool ID (default: the single tool, or the only tool in a library)
  --feed       cutting feed rate mm/min (default 500; the phase-3 ProcessTable
               will resolve this from the tool's feeds[] per material)
  --spindle    spindle speed RPM (default 8000)
  --coolant    off | flood | mist | through (default off)
  --milling    climb | conventional (default climb)
  --out        .moves.json output (default <part-id>.moves.json)
  --nc         .nc output (default <part-id>.nc)
`, "\t", "  "))
	}
	partFile := fs.String("part", "", "part recipe JSON")
	stockFile := fs.String("stock", "", "stock JSON")
	fixtureFile := fs.String("fixture", "", "fixture JSON (keep-out zones)")
	toolFile := fs.String("tool", "", "tool or tool-library JSON")
	toolID := fs.String("tool-id", "", "tool ID")
	machineFile := fs.String("machine", "", "machine JSON")
	feed := fs.Float64("feed", 500, "cutting feed rate (mm/min)")
	spindle := fs.Float64("spindle", 8000, "spindle speed (RPM)")
	coolant := fs.String("coolant", "off", "coolant: off, flood, mist, through")
	milling := fs.String("milling", "climb", "milling mode: climb or conventional")
	outFile := fs.String("out", "", "output .moves.json (default <part-id>.moves.json)")
	ncFile := fs.String("nc", "", "output .nc (default <part-id>.nc)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	for name, v := range map[string]*string{
		"--part": partFile, "--stock": stockFile, "--tool": toolFile, "--machine": machineFile,
	} {
		if *v == "" {
			fs.Usage()
			return fmt.Errorf("%s is required", name)
		}
	}

	// --- Load (I/O lives here and only here — Law 4) -----------------------
	pr, err := loadJSON[part.Part](*partFile)
	if err != nil {
		return err
	}
	if err := pr.Validate(); err != nil {
		return fmt.Errorf("%s: %w", *partFile, err)
	}
	meshData, err := os.ReadFile(partMeshPath(*partFile, pr.Mesh))
	if err != nil {
		return fmt.Errorf("part %q: reading the mesh file: %w", pr.ID, err)
	}
	m, err := parseMesh(pr.Mesh, meshData)
	if err != nil {
		return fmt.Errorf("part %q: %w", pr.ID, err)
	}
	if err := m.Validate(); err != nil {
		return fmt.Errorf("part %q: mesh: %w", pr.ID, err)
	}
	if !m.IsClosedMesh() {
		fmt.Fprintf(os.Stderr, "warning: part %q mesh is not a closed 2-manifold (open edges) — planning will proceed, but the geometry may be broken\n", pr.ID)
	}

	st, err := loadJSON[stock.Stock](*stockFile)
	if err != nil {
		return err
	}
	if err := st.Validate(); err != nil {
		return fmt.Errorf("%s: %w", *stockFile, err)
	}

	fx := fixture.Fixture{ID: "none"}
	if *fixtureFile != "" {
		fx, err = loadJSON[fixture.Fixture](*fixtureFile)
		if err != nil {
			return err
		}
	}
	keepOuts, err := fx.KeepOuts()
	if err != nil {
		return fmt.Errorf("fixture: %w", err)
	}

	tl, err := loadTool(*toolFile, *toolID)
	if err != nil {
		return err
	}

	mch, err := loadJSON[machine.Machine](*machineFile)
	if err != nil {
		return err
	}
	if err := mch.Validate(); err != nil {
		return fmt.Errorf("%s: %w", *machineFile, err)
	}

	// --- Plan (the tested core decides) ------------------------------------
	res, err := planner.Plan(planner.Params{
		Part:     pr,
		Mesh:     m,
		Stock:    st,
		KeepOuts: keepOuts,
		Tool:     tl,
		Machine:  mch,
		Feed:     *feed,
		Spindle:  *spindle,
		Milling:  moves.Milling(*milling),
		Coolant:  moves.Coolant(*coolant),
	})
	if err != nil {
		return err
	}
	prog, err := res.ToProgram(pr.ID, keepOuts, tl.Radius())
	if err != nil {
		return err
	}
	prog.Name = programName(pr)

	// --- Write --------------------------------------------------------------
	outPath := *outFile
	if outPath == "" {
		outPath = pr.ID + ".moves.json"
	}
	ncPath := *ncFile
	if ncPath == "" {
		ncPath = pr.ID + ".nc"
	}
	if err := writeJSON(outPath, prog); err != nil {
		return err
	}
	nc, err := gcode.EmitProgram(prog, gcodeOptions(mch))
	if err != nil {
		return err
	}
	if err := os.WriteFile(ncPath, []byte(nc), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", ncPath, err)
	}

	// --- Summary (short, kind, actionable) ----------------------------------
	fmt.Printf("planned %q:\n", prog.Name)
	fmt.Printf("  z: slice %g · cut %g · safe %g\n", res.SliceZ, res.CutZ, res.SafeZ)
	for _, f := range res.Features {
		mode := "—"
		for _, mv := range f.Path.Moves {
			if mv.Milling != "" {
				mode = string(mv.Milling)
				break
			}
		}
		fmt.Printf("  %d. %s (%s, %g mm²) — %s\n",
			f.Feature.Index+1, f.Path.ID, f.Feature.Kind, f.Feature.Area, mode)
	}
	for _, d := range res.Diagnostics {
		fmt.Printf("  note: %s\n", d.Message)
	}
	fmt.Printf("wrote %s (%d paths) and %s\n", outPath, len(prog.Paths), ncPath)
	return nil
}

// programName gives the program (and its header) a meaningful name.
func programName(pr part.Part) string {
	name := pr.Name
	if name == "" {
		name = pr.ID
	}
	return name + " — first cut"
}

// partMeshPath resolves the recipe's mesh file: relative paths resolve
// against the recipe's own directory (a recipe travels with its geometry).
func partMeshPath(partFile, meshRef string) string {
	if filepath.IsAbs(meshRef) {
		return meshRef
	}
	return filepath.Join(filepath.Dir(partFile), meshRef)
}

// parseMesh picks the parser by file extension (the mesh formats are
// self-describing, but the extension is the contract — ADR-017).
func parseMesh(name string, data []byte) (mesh.Mesh, error) {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".stl":
		return mesh.ParseSTL(data)
	case ".3mf":
		return mesh.Parse3MF(data)
	default:
		return mesh.Mesh{}, fmt.Errorf("unsupported mesh extension %q (want .stl or .3mf)", filepath.Ext(name))
	}
}

// loadJSON reads a JSON file into T with precise errors.
func loadJSON[T any](file string) (T, error) {
	var out T
	data, err := os.ReadFile(file)
	if err != nil {
		return out, fmt.Errorf("reading %s: %w", file, err)
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return out, fmt.Errorf("parsing %s: %w", file, err)
	}
	return out, nil
}

// loadTool accepts a single-tool file or a library (selected by ID, or the
// only tool when there is exactly one).
func loadTool(file, id string) (tool.Tool, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return tool.Tool{}, fmt.Errorf("reading %s: %w", file, err)
	}
	var probe struct {
		Tools []json.RawMessage `json:"tools"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return tool.Tool{}, fmt.Errorf("parsing %s: %w", file, err)
	}
	if len(probe.Tools) > 0 {
		var lib tool.Library
		if err := json.Unmarshal(data, &lib); err != nil {
			return tool.Tool{}, fmt.Errorf("parsing %s as a library: %w", file, err)
		}
		if err := lib.Validate(); err != nil {
			return tool.Tool{}, fmt.Errorf("%s: %w", file, err)
		}
		if id == "" {
			if len(lib.Tools) != 1 {
				return tool.Tool{}, fmt.Errorf("%s: a library with %d tools needs --tool-id (IDs: %s)",
					file, len(lib.Tools), toolIDs(&lib))
			}
			id = lib.Tools[0].ID
		}
		t, ok := lib.Get(id)
		if !ok {
			return tool.Tool{}, fmt.Errorf("%s: no tool with id %q (IDs: %s)", file, id, toolIDs(&lib))
		}
		return t, nil
	}
	var t tool.Tool
	if err := json.Unmarshal(data, &t); err != nil {
		return tool.Tool{}, fmt.Errorf("parsing %s as a single tool: %w", file, err)
	}
	if id != "" && t.ID != id {
		return tool.Tool{}, fmt.Errorf("%s: tool id %q does not match --tool-id %q", file, t.ID, id)
	}
	if err := t.Validate(); err != nil {
		return tool.Tool{}, fmt.Errorf("%s: %w", file, err)
	}
	return t, nil
}

func toolIDs(lib *tool.Library) string {
	ids := make([]string, 0, len(lib.Tools))
	for _, t := range lib.Tools {
		ids = append(ids, t.ID)
	}
	return strings.Join(ids, ", ")
}

// gcodeOptions maps the machine's post-processor config onto the generic
// emitter (the dialect lives in the machine JSON, ADR-007).
func gcodeOptions(m machine.Machine) gcode.Options {
	o := gcode.DefaultOptions()
	o.HeaderLines = append([]string{
		fmt.Sprintf("%% CleanCuts — %s", m.Name),
		"G21", // millimetres
		"G90", // absolute positioning
	}, m.PostProcessor.HeaderLines...)
	o.FooterLines = m.PostProcessor.FooterLines
	o.SpindleStart = m.PostProcessor.SpindleStart
	o.SpindleStop = m.PostProcessor.SpindleStop
	o.ProgramEnd = m.PostProcessor.ProgramEnd
	o.CoolantOn = m.PostProcessor.CoolantOn
	o.CoolantOff = m.PostProcessor.CoolantOff
	return o
}

// writeJSON writes v indented (the .moves.json file is read by humans).
func writeJSON(file string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding %s: %w", file, err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(file, data, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", file, err)
	}
	return nil
}
