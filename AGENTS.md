# AGENTS.md — working agreement for AI agents (and humans)

You are working on **CleanCuts**, a CAM core in Go with zero external
dependencies in `internal/`. Read this before making changes. The goal is that
any capable agent can make a correct, well-tested, well-documented change without
a human holding their hand — and that a human can review it in minutes.

## The laws (do not violate)

These are settled. If you believe one must change, **write a new ADR**
(`docs/design-decisions.md`) explaining the new context — do not just change the
code.

1. **One coordinate system.** Canonical WCS: right-handed, **Z-up**, **millimetres**
   (ADR-001). No subsystem re-frames or resamples coordinates. Frame/units mapping
   happens exactly once, in post-processing, driven by the machine config.
2. **Analytic geometry.** Lines and arcs are values. Never represent a toolpath as
   a tessellated polyline inside the core; tessellation is a post-processing choice
   (ADR-002).
3. **No external dependencies in `internal/`.** Standard library only (ADR-004).
   New deps need an ADR and a real reason.
4. **The intermediate format is the seam.** Generation produces `moves.Path`;
   post-processing consumes it. Don't let a generator reach into a post-processor or
   vice versa (ADR-003, ADR-008).
5. **Invariants are checked, not assumed.** Every public type has `Validate()`;
   generators validate their output before returning (ADR-006, ADR-007).
6. **Config is data.** Tools and machines are JSON with stable lowercase snake_case
   keys. Adding a machine or a tool is a data change, not a code change (ADR-007).

## Build / test / verify (always run all three)

```sh
gofmt -l .            # must print nothing (or fix the listed files)
go vet ./...          # must be clean
go test ./...         # must pass; add tests for your change
go build ./...        # must build
```

A change is not done until all four are green **and** you added or updated tests
that cover it.

## Conventions

- **Package = capability.** Small packages with one responsibility. A package
  depends on `kernel` and at most the format it produces/consumes. No cycles, no
  god packages. (See `docs/architecture.md`.)
- **Errors are precise and kind.** Name the package, the object, and the offending
  value: `tool "t1": corner radius 6 exceeds radius 5`. Wrap with `%w`. Never
  return a bare `errors.New("failed")`. Never include stack traces or internal
  jargon in the message a human reads.
- **Tests are table-driven and self-documenting.** Name the case. Assert analytic
  results to tight tolerance (we don't resample). Include the degenerate/boundary
  cases, not just the happy path. Every JSON type gets a round-trip test.
- **Comments explain *why*, not *what*.** Doc-comment every exported symbol. If a
  function's name already says what it does, the comment should say why it exists
  or which invariant it upholds.
- **Keep the core pure.** No I/O, no globals, no `init()` side effects, no
  package-level mutable state in `internal/`. The CLI (`cmd/`) is where I/O lives.

## How to add a feature (checklist)

1. **Locate the stage.** Which pipeline stage does this belong to? (geometry →
   features → generation → format → post-processing.) If it's a new strategy, it's
   a new function/module in `internal/toolpath` (or a subpackage) that returns
   `moves.Path`.
2. **Don't touch the spine to fit your feature.** If you find yourself needing to
   change `kernel`/`moves`/`gcode`, stop and write an ADR first.
3. **Write the tests first** (or at least in the same change). Table-driven. Cover
   valid, invalid, and boundary inputs.
4. **Document it.** Add the *model* to `docs/operations.md`, the package note to
   `docs/architecture.md` if you added a package, and an ADR if you changed a
   decision.
5. **Run the four commands above.** All green.
6. **Keep AGENTS.md and the roadmap honest.** If your change changes the "current
   phase" or a convention, update the docs in the same change.

## What not to do

- Don't add a third-party dependency to `internal/` to save yourself 20 lines.
- Don't introduce a second coordinate system, a second units convention, or a
  hidden origin offset.
- Don't represent toolpaths as float arrays of points in the core.
- Don't swallow errors or return `nil` from a fallible path "for now."
- Don't leave a TODO that is actually a skipped requirement. If it's not done, say
  so in the roadmap, not in a comment.
- Don't re-litigate a settled ADR silently. Write the new ADR.

## Where things live

- **Decisions & their rationale:** `docs/design-decisions.md` (ADR log).
- **Structure & extension guide:** `docs/architecture.md`.
- **Tool parameters:** `docs/tool-parameters.md`.
- **Strategies & their models:** `docs/operations.md`.
- **Format & post-processing:** `docs/gcode.md`.
- **Plan & current status:** `docs/roadmap.md`.
- **JSON contracts:** `schemas/`.
- **Examples:** `examples/`.

## Definition of done (current phase)

For any change in the current phase: the four build/test commands are green, tests
cover the change, the docs reflect it, and the change is explainable in a short
paragraph (what, why, which invariant it upholds). If you can't write that
paragraph, the change isn't ready.
