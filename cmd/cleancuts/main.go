// Command cleancuts is the CleanCuts command-line interface. It wraps the
// machine-independent core and, in v0.1, provides:
//
//	cleancuts version     print the version
//	cleancuts demo        generate a sample 2D profile and emit generic G-code
//	cleancuts help        print usage
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

	// A 100 x 60 mm rectangle to profile, in the canonical Z-up WCS.
	poly := []kernel.Point2{
		kernel.P2(0, 0), kernel.P2(100, 0), kernel.P2(100, 60), kernel.P2(0, 60),
	}
	p, err := toolpath.Profile(poly, toolpath.ProfileParams{
		Milling: moves.MillingClimb,
		Feed:    *feed,
		Spindle: 8000,
		SafeZ:   *safeZ,
		CutZ:    *cutZ,
		Coolant: moves.CoolantFlood,
		ToolID:  "endmill-10mm-4f",
	})
	if err != nil {
		return err
	}

	opts := gcode.DefaultOptions()
	opts.UseArms = !*noArms
	opts.HeaderLines = []string{"% CleanCuts demo — 2D profile of a 100x60 rectangle"}
	out, err := gcode.Emit(p, opts)
	if err != nil {
		return err
	}
	fmt.Print(out)
	return nil
}
