# CONTEXT.md

## Scope
- This directory contains canonical long-form user documentation.
- It explains workflows that are too detailed for `CLAUDE.md` or `AGENTS.md`.

## Why This Exists
- The repository needs two different document types:
  - user-facing explanations and procedures
  - AI/operator-facing rules
- This directory owns the former. Context documents should point here rather than duplicate long procedures.

## Key Files
- `agent-quickstart.md`: first-time discovery path for agents and automation using `xcodecli`.
- `releasing.md`: canonical release, GitHub Release, and Homebrew flow.
- `implementation-spec.md`: full technical contract for reimplementation and agent reference (English).
- `implementation-spec.kr.md`: Korean translation of `implementation-spec.md`.

## Local Rules
- Keep these docs user-facing and procedural.
- Do not copy long CLI help text into these files; summarize and show representative commands.
- If CLI examples change, update the docs that teach those workflows rather than leaving the README as the only source.
- Version examples in this directory should track the current release line.

## Change Coupling
- CLI onboarding changes should review:
  - `./agent-quickstart.md`
  - `../README.md`
  - `../cmd/xcodecli/cli.go` help text
- Release flow changes should review:
  - `./releasing.md`
  - `../scripts/release_homebrew.sh`
  - `../.github/workflows/homebrew-release.yml`
  - version examples in `../README.md`

## Canonical Source Notes
- `../README.md` is the repository landing page.
- `./agent-quickstart.md` is the detailed first-time agent walkthrough.
- `./releasing.md` is the detailed release procedure.
- Context documents should link to these docs, not replace them.

## Verification Notes
- When commands or version strings change, scan docs for stale examples.
- Prefer small, representative command examples over exhaustive duplication of help output.
