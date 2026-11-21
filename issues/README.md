# Issues

Findings, known bugs, and doc drift — one file per item.

- `open/` — items that still need work.
- `closed/` — resolved items, kept for the record (with resolution note + commit).

## Conventions

- Filenames: `NNN-short-kebab-slug.md`. `NNN` increases monotonically
  across **both** folders; never reuse a number (a superseded issue is
  closed, and the new issue gets the next number).
- Every issue follows the template below.
- **At the start of a session:** read every file in `issues/open/` before
  starting work. If an issue overlaps your task, work on it, or say
  explicitly why you're deferring it. Never silently delete an open issue.
- **New issues:** bugs you discover, doc drift, and TODOs worth tracking
  get a new number and land in `issues/open/`.
- **Closing:** move the file to `issues/closed/`, set Status to `closed`,
  and fill in Resolution (date + commit).

## Template

```md
# NNN: Title

- **Status:** open
- **Type:** bug | docs | design | housekeeping
- **Priority:** high | medium | low
- **Found:** YYYY-MM-DD (by whom / which review)
- **Files:** relative paths

## Summary
What is wrong, in one or two sentences.

## Evidence
How it was confirmed (test run, repro steps, spec citation).

## Suggested fix
Concrete change(s).

## Acceptance criteria
What "done" looks like (tests, CI, docs).
```
