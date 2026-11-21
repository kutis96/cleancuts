// Command cleancuts is the CleanCuts command-line interface. It wraps the
// machine-independent core and provides:
//
//	cleancuts version    print the version
//	cleancuts demo       generate a sample 2D profile and emit generic G-code
//	cleancuts generate   plan a part (part + stock + fixture + tool + machine
//	                     JSON) into .moves.json + .nc — the phase-2 workflow
//	cleancuts help       print usage
//
// The CLI is intentionally dependency-free (stdlib only) so the core stays
// trivially portable — including a future WASM build. See README.md and
// docs/roadmap.md for what arrives in later phases.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/kutis96/cleancuts/internal/gcode"
	"github.com/kutis96/cleancuts/internal/kernel"
	"github.com/kutis96/cleancuts/internal/moves"
	"github.com/kutis96/cleancuts/internal/toolpath"
)

const version = "0.1.0"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "cleancuts:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		usage()
		return nil
	}
	switch args[0] {
	case "version", "--version", "-v":
		fmt.Println("cleancuts", version)
		return nil
	case "help", "--help", "-h":
		usage()
		return nil
	case "demo":
		return demo(args[1:])
	case "generate":
		return generate(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "cleancuts: unknown command %q\n\n", args[0])
		usage()
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func usage() {
	fmt.Fprint(os.Stderr, strings.ReplaceAll(`
usage: cleancuts <command> [flags]

commands:
  version    print the version
  demo       generate a sample 2D profile and print generic G-code
  generate   plan a part from JSON configs into .moves.json + .nc
  help       print this help
`, "\t", "  "))
}

func demo(args []string) error {
	fs := flag.NewFlagSet("demo", flag.ContinueOnError)
	noArms := fs.Bool("no-arms", false, "tessellate arcs into G1 line segments")
	feed := fs.Float64("feed", 600, "cutting feed rate (mm/min)")
	safeZ := fs.Float64("safe-z", 5, "rapid safe height (mm)")
	cutZ := fs.Float64("cut-z", -10, "cutting depth (mm)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	// A 100 x 60 mm rounded rectangle (10 mm corners) to profile, in the
	// canonical Z-up WCS. Lines and arcs travel as analytic moves; the
	// emitter decides whether the controller gets G2/G3 or G1 tessellation.
	chain, err := roundedRect(100, 60, 10)
	if err != nil {
		return err
	}
	p, err := toolpath.Loop(chain, toolpath.LoopParams{
		Operation: "profile",
		Name:      "demo rounded rectangle",
		Milling:   moves.MillingClimb,
		Feed:      *feed,
		Spindle:   8000,
		SafeZ:     *safeZ,
		CutZ:      *cutZ,
		Coolant:   moves.CoolantFlood,
		ToolID:    "endmill-10mm-4f",
	})
	if err != nil {
		return err
	}

	opts := gcode.DefaultOptions()
	opts.UseArms = !*noArms
	opts.HeaderLines = []string{"% CleanCuts demo — 2D profile of a 100x60 rounded rectangle"}
	out, err := gcode.Emit(p, opts)
	if err != nil {
		return err
	}
	fmt.Print(out)
	return nil
}

// roundedRect builds a CCW rounded-rectangle chain (lines + quarter arcs).
func roundedRect(w, h, r float64) (kernel.Chain2, error) {
	P := kernel.P2
	edges := []kernel.Edge2{
		mustEdge(P(r, 0), P(w-r, 0)),
		mustArc(P(w-r, r), P(w-r, 0), P(w, r)),
		mustEdge(P(w, r), P(w, h-r)),
		mustArc(P(w-r, h-r), P(w, h-r), P(w-r, h)),
		mustEdge(P(w-r, h), P(r, h)),
		mustArc(P(r, h-r), P(r, h), P(0, h-r)),
		mustEdge(P(0, h-r), P(0, r)),
		mustArc(P(r, r), P(0, r), P(r, 0)),
	}
	return kernel.NewChain2(edges)
}

func mustEdge(a, b kernel.Point2) kernel.Edge2 {
	e, err := kernel.NewLineEdge(a, b)
	if err != nil {
		panic(err) // demo fixture: a bad constant is a bug, not user input
	}
	return e
}

func mustArc(c, a, b kernel.Point2) kernel.Edge2 {
	e, err := kernel.NewArcEdge(c, a, b, false)
	if err != nil {
		panic(err)
	}
	return e
}
