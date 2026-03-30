# CONTEXT.md

## Scope
- This directory contains operational shell scripts for building, installing, and publishing `xcodecli`.

## Why This Exists
- These scripts are the destructive edge of the repository.
- The CLI code can be changed safely in isolation, but script changes affect installation paths, release flows, shared Homebrew tap state, and external publishing.

## Key Files
- [scripts/build-swift.sh](./build-swift.sh): local reproducible Swift build entrypoint; version/build-channel injection happens via edits to [Sources/XcodeCLICore/Shared/Version.swift](../Sources/XcodeCLICore/Shared/Version.swift).
- [scripts/install.sh](./install.sh): local-or-remote source installation flow, PATH guidance, and ref resolution.
- [scripts/release.sh](./release.sh): standard local release entrypoint for version bump, verification, tag push, Homebrew sync, and GitHub Release creation.
- [scripts/release_homebrew.sh](./release_homebrew.sh): low-level shared tap formula generation, audit/build validation, local commit/push path, and tap safety checks.

## Local Rules
- Destructive steps come last.
- Release and Homebrew workflows are local-first and dry-run-first.
- `release.sh` must remain the canonical high-level release entrypoint.
- `release_homebrew.sh` must continue to treat `oozoofrog/homebrew-tap` as a shared repository and only touch the shared tap formula path for `xcodecli`.
- `.tmp/` is an operational scratch area, not part of the context tree and not a documentation source of truth.
- Script usage examples must stay aligned with current release examples in [README.md](../README.md) and [docs/releasing.md](../docs/releasing.md).

## Change Coupling
- If [scripts/build-swift.sh](./build-swift.sh) changes version or output behavior, review:
  - [README.md](../README.md)
  - [docs/agent-quickstart.md](../docs/agent-quickstart.md)
  - release instructions that mention build verification
- If `install.sh` changes refs, URLs, or PATH guidance, review:
  - install examples in [README.md](../README.md)
  - any version-tag examples
- If [scripts/release.sh](./release.sh) changes, review:
  - [docs/releasing.md](../docs/releasing.md)
  - [README.md](../README.md)
  - shared release assumptions
- If [scripts/release_homebrew.sh](./release_homebrew.sh) changes, review:
  - [docs/releasing.md](../docs/releasing.md)
  - [scripts/release.sh](./release.sh)
  - shared tap safety assumptions

## Verification Notes
- For build/install changes, run the script locally rather than assuming documentation is enough.
- For release script changes, verify the dry-run path before any push path.
- For Homebrew release changes, verify the dry-run path before any push path.

## Child Contexts
- None. If the release flow expands further, prefer a deeper release-specific context rather than overloading this file.
