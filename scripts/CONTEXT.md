# CONTEXT.md

## Scope
- This directory contains operational shell scripts for building, installing, and publishing `xcodecli`.

## Why This Exists
- These scripts are the destructive edge of the repository.
- The CLI code can be changed safely in isolation, but script changes affect installation paths, release flows, shared Homebrew tap state, and external automation.

## Key Files
- `./build-swift.sh`: local reproducible Swift build entrypoint; version/build-channel injection happens via edits to `../Sources/XcodeCLICore/Shared/Version.swift`.
- `./install.sh`: local-or-remote source installation flow, PATH guidance, and ref resolution.
- `./release_homebrew.sh`: shared tap formula generation, audit/build validation, local commit/push path, and tap safety checks.

## Local Rules
- Destructive steps come last.
- Release and Homebrew workflows are dry-run-first by default.
- `release_homebrew.sh` must continue to treat `oozoofrog/homebrew-tap` as a shared repository and only touch the shared tap formula path for `xcodecli`.
- `.tmp/` is an operational scratch area, not part of the context tree and not a documentation source of truth.
- Script usage examples must stay aligned with current release examples in `../README.md` and `../docs/releasing.md`.

## Change Coupling
- If `./build-swift.sh` changes version or output behavior, review:
  - `../README.md`
  - `../docs/agent-quickstart.md`
  - release instructions that mention build verification
- If `install.sh` changes refs, URLs, or PATH guidance, review:
  - install examples in `../README.md`
  - any version-tag examples
- If `./release_homebrew.sh` changes, review:
  - `../docs/releasing.md`
  - `../.github/workflows/homebrew-release.yml`
  - shared tap safety assumptions

## Verification Notes
- For build/install changes, run the script locally rather than assuming documentation is enough.
- For release script changes, verify the dry-run path before any push path.

## Child Contexts
- None. If the release flow expands further, prefer a deeper release-specific context rather than overloading this file.
