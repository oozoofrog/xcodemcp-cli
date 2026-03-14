# Agent quick rules for `xcodemcp`

Use this repository when you need a CLI bridge into Xcode's MCP tools.

## Preconditions
- macOS only.
- Xcode must be running.
- At least one workspace or project window should be open in Xcode before using `tools` commands.
- The first `tools` request may automatically install a LaunchAgent at `~/Library/LaunchAgents/io.oozoofrog.xcodemcp.plist`.

## Recommended first-time flow
1. `./xcodemcp doctor --json`
2. `./xcodemcp agent status --json`
3. `./xcodemcp tools list --json`
4. `./xcodemcp tool inspect <name> --json`
5. `./xcodemcp tool call <name> --json '{...}'`

## Payload input
- Small payload: `--json '{...}'`
- File payload: `--json @payload.json`
- Piped payload: `--json-stdin`

## Failure triage
- Retry with `--debug` on `tools list`, `tool inspect`, or `tool call`.
- Check `./xcodemcp agent status --json` for LaunchAgent installation, socket reachability, and backend session state.
- If the agent is wedged, run `./xcodemcp agent stop` or `./xcodemcp agent uninstall` and retry.
