# CLAUDE.md

## Project Overview

patchkit-tools-go is a Go CLI (`pkt`) for managing PatchKit app versions — creating, uploading, diffing, and publishing game/app updates via the PatchKit API (`api.patchkit.net`). It is the Go rewrite of the Ruby patchkit-tools.

## Building and Testing

```bash
make build          # Build for current platform → dist/pkt
make test           # Run tests with race detector
make lint           # go vet
make all            # lint + test + build
make build-all      # Cross-compile all platforms
```

Build tags:
- Default: pure Go librsync, no CGo dependencies
- `-tags cgo_librsync`: use native librsync via CGo
- `-tags cgo_turbopatch`: use native turbopatch via CGo

## Architecture

### Entry Point
`cmd/pkt/main.go` → sets version/commit/date from ldflags → `cli.Execute()`

### Key Packages
| Package | Purpose |
|---------|---------|
| `internal/cli/` | Cobra command definitions (version push, build, channel, app, config) |
| `internal/workflow/` | Orchestrates multi-step operations (push, channel-push) |
| `internal/diff/` | Parallel delta pipeline — computes deltas in memory via goroutines |
| `internal/native/` | Rsync/TurboPatch interfaces with pure Go + CGo implementations |
| `internal/content/` | ZIP packager for content archives |
| `internal/pack1/` | Pack1 format: gzip + AES-256-CBC encryption |
| `internal/upload/` | S3 chunked upload with retry and backoff |
| `internal/api/` | PatchKit REST API client |
| `internal/lock/` | Distributed lock via API |
| `internal/hash/` | xxHash32 implementation |
| `internal/config/` | YAML config via Viper |

### Diff Pipeline (in-memory streaming)
Delta generation avoids writing temporary files to disk (prevents Windows AV corruption):
1. Worker goroutines compute deltas via `DeltaToWriter` → `bytes.Buffer`
2. `DeltaEntry` carries either `FilePath` (added files on disk) or `Data` (in-memory delta bytes)
3. Packers accept `map[string]DeltaEntry` and handle both types transparently

### Version / Release

Version is embedded at build time via ldflags in `cmd/pkt/main.go`:
```
-X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)
```

**To release a new version:**
1. Commit and push all changes to `master`
2. Tag: `git tag v1.0.0 && git push origin v1.0.0`
3. GitHub Actions builds binaries for linux/darwin/windows and creates a release

The CI workflow (`.github/workflows/ci.yml`) runs on push/PR: vet, test with race detector, build check.
The release workflow (`.github/workflows/release.yml`) runs on `v*` tags: cross-compile + GitHub release.

## Test Accounts

Test credentials are stored in SOT (`patchkit.documentation`, record #53 REDACTED). Use the Windows gzip test app (`REDACTED_APP_SECRET`) with API key `REDACTED_API_KEY` for integration testing.
