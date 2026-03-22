# Agent quick rules for `xcodecli`

Use this repository when you need a CLI bridge into Xcode's MCP tools.

## Read order
1. `CLAUDE.md` for global constraints, commands, and override rules.
2. This file for portable collaboration and first-time triage rules.
3. The closest `CONTEXT.md` to the files you are changing.

## Global constraints
- Runtime and environment prerequisites live in `CLAUDE.md`.
- Use this file for collaboration rules and quick recovery flow, not as a second source of truth for platform constraints.

## First-time flow
1. `./xcodecli agent guide "build Unicody"`
2. `./xcodecli agent demo --json`
3. `./xcodecli doctor --json`
4. `./xcodecli tools list --json`
5. `./xcodecli tool call <name> --json '{...}'`

## Collaboration rules
- Prefer `agent guide` before guessing a tool sequence.
- Use `agent demo` for safe live discovery.
- Fall back to `tool inspect` only when you need schema reassurance.
- Keep payloads as JSON objects:
  - inline: `--json '{...}'`
  - file: `--json @payload.json`
  - stdin: `--json-stdin`

## Output Contract
- Report changed files or touched documents explicitly.
- Include the verification command you ran and whether it passed.
- Call out remaining risks, stale references, or follow-up cleanup separately.

## Review Rules
- Findings should cite the relevant file, section, or command.
- Prioritize correctness, broken references, and drift over stylistic praise.
- If no issues remain after verification, say so explicitly instead of implying it.

## Failure triage
- Retry with `--debug` on `tools list`, `tool inspect`, or `tool call`.
- Check `./xcodecli agent status --json` for LaunchAgent and socket state.
- If the local runtime is wedged, run `./xcodecli agent stop` or `./xcodecli agent uninstall` and retry.
