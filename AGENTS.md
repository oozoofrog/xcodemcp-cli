# Agent quick rules for `xcodemcp`

Use this repository when you need a CLI bridge into Xcode's MCP tools.

## Preconditions
- macOS only.
- Xcode must be running.
- At least one workspace or project window should be open in Xcode before using `tools` commands.
- The first `tools` request may automatically install a LaunchAgent at `~/Library/LaunchAgents/io.oozoofrog.xcodemcp.plist`.

## Recommended first-time flow
1. `./xcodemcp agent guide "build Unicody"`
2. `./xcodemcp agent demo --json`
3. `./xcodemcp doctor --json`
4. `./xcodemcp tools list --json`
5. `./xcodemcp tool call <name> --json '{...}'`

## Workflow guidance first
- Start with `agent guide` when you already know the user's intent and need to learn the right tool sequence.
- Use `agent demo` when you want safe live discovery of windows and the tool catalog before choosing a workflow.
- Fall back to `tool inspect` only when you need schema reassurance or a less common payload shape.

## Payload input
- Small payload: `--json '{...}'`
- File payload: `--json @payload.json`
- Piped payload: `--json-stdin`

## Failure triage
- Retry with `--debug` on `tools list`, `tool inspect`, or `tool call`.
- Check `./xcodemcp agent status --json` for LaunchAgent installation, socket reachability, and backend session state.
- If the agent is wedged, run `./xcodemcp agent stop` or `./xcodemcp agent uninstall` and retry.
