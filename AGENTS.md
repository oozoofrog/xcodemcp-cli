# Agent quick rules for `xcodecli`

Use this repository when you need a CLI bridge into Xcode's MCP tools.

## Preconditions
- macOS only.
- Xcode must be running.
- At least one workspace or project window should be open in Xcode before using `tools` commands.
- The first `tools` request may automatically install a LaunchAgent at `~/Library/LaunchAgents/io.oozoofrog.xcodecli.plist`.

## Recommended first-time flow
1. `./xcodecli agent guide "build Unicody"`
2. `./xcodecli agent demo --json`
3. `./xcodecli doctor --json`
4. `./xcodecli tools list --json`
5. `./xcodecli tool call <name> --json '{...}'`

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
- Check `./xcodecli agent status --json` for LaunchAgent installation, socket reachability, and backend session state.
- If the agent is wedged, run `./xcodecli agent stop` or `./xcodecli agent uninstall` and retry.
