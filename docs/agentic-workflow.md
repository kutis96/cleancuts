# Agentic workflow

How to drive CleanCuts development effectively using AI agents — the process
that complements the in-repo laws in [AGENTS.md](../AGENTS.md). The design goal:
any agent can pick up a well-scoped task, make a correct change, and hand it off
in a way a human (or the next agent) can verify in minutes.

## Why this works for agents

The repo is deliberately shaped so agent work is *verifiable and local*:

- **Narrow, pure packages** → an agent's change has a small blast radius and is
  easy to reason about.
- **Zero dependencies in the core** → no dependency-resolution surprises; behaviour
  is fully determined by source.
- **Explicit invariants + `Validate()`** → "correct" is checkable, not vibes.
- **Table-driven tests** → the spec *is* the tests; an agent can read them to
  learn the contract, and they double as the acceptance criteria.
- **A decision log (ADRs)** → agents don't re-derive settled choices or
  contradict earlier work.
- **Docs that state the *model*** (operations.md, tool-parameters.md) → an agent
  implements a defined equation, not an ambiguous idea.

## The unit of work: a "slice"

A **slice** is the smallest piece of work that is independently testable and
shippable. A slice:
- changes **one stage** of the pipeline (or adds one function to a stage),
- has **a clear definition of done** (a test that can pass),
- does **not require** a not-yet-built dependency to be meaningful, and
- leaves the build green.

Good slices (roughly in order): "STL importer for planar contours" · "2D offset
with mitre joins" · "fillet joins" · "climb/conventional derivation" ·
"trochoidal path generator" · "constant-chip-load feed scaling" · "corner
fillet in post-processing" · "lookahead feed scheduler."

Bad "slices": "do 2.5D" (not testable), "improve the kernel" (no definition of
done), "start the GUI" (out of the core's scope).

## A standard agent session

1. **Read first.** `AGENTS.md`, the relevant ADRs, and the doc for the area you're
   touching. Confirm the current phase in `roadmap.md`.
2. **State the slice** in 2–3 sentences: what, which stage, definition of done.
3. **Specify the model first** (design-first): if the slice adds or changes a
   decision, write/extend the ADR and the relevant `docs/` section *before*
   coding. The design is the durable artifact; the code follows it.
4. **Write the tests** (table-driven) that express the definition of done.
5. **Implement** until the tests pass and the four commands are green
   (`gofmt -l .`, `go vet ./...`, `go test ./...`, `go build ./...`).
6. **Update the docs** for the change (operations/architecture/ADR as needed).
7. **Commit as a handoff checkpoint** (see below), then **summarize**:
   what/why/which invariant, and any follow-up slice it enables.

### Verification commands (run and report all four)

```sh
gofmt -l . && go vet ./... && go test ./... && go build ./...
```

### Commits are handoff checkpoints

The repo lives at `github.com/kutis96/cleancuts`. Commit **as you go**, one focused
slice per commit, only at a green checkpoint (all four commands pass). A working,
committed state on `main` is the safety net against context loss: a compaction or
a new agent should find a green build plus docs/ADRs that capture the decisions,
not a wall of uncommitted, possibly-broken work. Use `bionic/…` branches for
risky work and keep `main` shippable. Full convention: see [AGENTS.md](../AGENTS.md).

If any fails, fix before claiming done. Paste the (short) tail of the test output
in the summary so a reviewer can see it without re-running.

## Handoff protocol

A handoff is complete when a reviewer (human or agent) can, **without context from
the author**, (a) understand the intent, (b) verify correctness, and (c) decide
the next slice. Concretely, a handoff includes:

- **The diff** (small and focused — one slice).
- **A short note:** what changed, why, which ADR/invariant it upholds, and the
  four-command result.
- **The tests** that pin the behaviour (the acceptance criteria).
- **Docs updated** so the *state of the world* matches the code.
- **Roadmap updated** if the slice's status changed.

Keep each handoff to one slice. A 60-line diff with 40 lines of tests and a
2-line note is a great handoff. A 600-line diff touching five packages is not.

## Reviewing an agent's work (for humans)

Check, in order:
1. Does it violate any **law** in AGENTS.md? (coordinate system, deps, format
   seam, invariants.)
2. Are the **tests real** (asserting analytic results and boundaries), not just
   "it ran"?
3. Did it **touch the spine** (`kernel`/`moves`/`gcode`)? If yes, is there an ADR?
4. Are the **docs** consistent with the code?
5. Can you **explain it** in a short paragraph? If the author's note can't, ask.

## Guardrails that prevent the classic agent failure modes

| Failure mode | Guardrail in this repo |
|---|---|
| Silently re-litigating a decision | ADR log + "write a new ADR" rule |
| "It works on my example" | Table-driven tests + analytic assertions + `Validate()` |
| Scope creep / big diffs | One-slice definition of done |
| Hidden coordinate/units bug | Single canonical frame law; mapping only in post-processing |
| Undocumented behaviour | Docs-as-part-of-done; the *model* is specified, not implied |
| Dependency drift | Zero-dep core law + dep ADR requirement |
| Error messages that mislead | Error contract (name the object + value) |

## Suggested division of labour

- **Human:** sets the phase, picks the next slice, makes the calls on open
  questions (module path, license, mesh-vs-B-rep, CI host), and does final review.
- **Agent(s):** implement slices, write tests, update docs, keep the build green,
  and produce clean handoffs. Multiple agents can work in parallel *as long as*
  their slices touch disjoint stages/packages (the pipeline is composable
  precisely so this is safe).

## Parallel work

Because stages are composable and depend only on `kernel`/`moves`, two slices can
run in parallel if they don't both edit the same package. Example: one agent does
"2D offset with mitre joins" (`internal/toolpath` + a new `comp` module) while
another does "machine post-processor for GRBL" (`internal/gcode` + a
`internal/post` module). A collision risk only exists when both need to change the
*format* — and that requires an ADR, so it's a deliberate, reviewed change, not an
accident.
