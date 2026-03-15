# Releasing xcodecli

## Standard release flow

1. Merge changes into `main`.
2. Run the test/build checks locally:
   - `go test ./...`
   - `./scripts/build.sh`
   - `./xcodecli version`
3. Create and push an annotated tag like `v0.4.0`.
4. Publish the GitHub Release for that tag.
5. The Homebrew release workflow updates the shared `oozoofrog/homebrew-tap` repository automatically.

## Homebrew automation

`xcodecli` is distributed through the shared `oozoofrog/homebrew-tap` repository:

```bash
brew tap oozoofrog/tap
brew install oozoofrog/tap/xcodecli
```

The automation path is:

- GitHub Release published in `oozoofrog/xcodecli`
- `.github/workflows/homebrew-release.yml` runs
- `scripts/release_homebrew.sh <tag> --push` updates the tap formula
- `oozoofrog/homebrew-tap/Formula/xcodecli.rb` is committed and pushed
- other formulas/casks in the shared tap are left untouched

## Manual recovery / local dry-run

Use this if the release workflow fails or before publishing a new version:

```bash
./scripts/release_homebrew.sh v0.4.0 --tap-dir .tmp/homebrew-tap --dry-run
```

To commit locally in the tap repo without pushing:

```bash
./scripts/release_homebrew.sh v0.4.0 --tap-dir .tmp/homebrew-tap
```

To clone the tap automatically and push the update:

```bash
HOMEBREW_TAP_GITHUB_TOKEN=... ./scripts/release_homebrew.sh v0.4.0 --push
```

## Required GitHub secret

Add this repository secret in `oozoofrog/xcodecli`:

- `HOMEBREW_TAP_GITHUB_TOKEN`

The token must be able to push to `oozoofrog/homebrew-tap`.

## Shared tap safety rules

- Treat `oozoofrog/homebrew-tap` as a shared repository for multiple projects.
- Only `Formula/xcodecli.rb` should be created or updated by the `xcodecli` release flow.
- Validation temporarily backs up and restores only `Formula/xcodecli.rb` inside the local Homebrew tap checkout.
- If the tap clone has unrelated local changes, the release script should stop instead of mixing `xcodecli` changes with other tap edits.
- If validation or push fails, recover by re-running `./scripts/release_homebrew.sh <tag> --tap-dir <tap clone> --dry-run` and checking only the `Formula/xcodecli.rb` diff.
