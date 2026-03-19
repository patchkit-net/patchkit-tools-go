# patchkit-tools-go

Go implementation of the PatchKit CLI tools for managing app versions — creating, uploading, diffing, and publishing game/app updates via the PatchKit API.

## Building

```bash
# Build for current platform
make build        # output: dist/pkt

# Build with CGo (native librsync + turbopatch support)
make build-cgo

# Cross-compile all platforms
make build-all

# Run tests
make test

# Run everything (lint + test + build)
make all
```

## Usage

```bash
# Check version
pkt --version

# Push a new version (auto-detects content vs diff mode)
pkt version push \
  --api-key <API_KEY> \
  --app <APP_SECRET> \
  --files ./my-build/ \
  --label "1.0.0" \
  --publish --wait

# See all commands
pkt --help
```

## Releasing

Releases are automated via GitHub Actions. When a version tag is pushed, the CI builds binaries for all platforms and creates a GitHub release with the assets attached.

### Steps

1. Ensure all changes are committed and pushed to `master`.
2. Tag the release:
   ```bash
   git tag v1.0.0
   git push origin v1.0.0
   ```
3. GitHub Actions will automatically:
   - Build `pkt` binaries for linux (amd64, arm64), macOS (amd64, arm64), and Windows (amd64)
   - Create a GitHub release at `https://github.com/patchkit-net/patchkit-tools-go/releases/tag/v1.0.0`
   - Attach all binaries to the release

4. The version string is embedded at build time via ldflags. After building from the tag, `pkt --version` will report:
   ```
   pkt version 1.0.0 (commit: abc1234, built: 2026-03-19T12:00:00Z)
   ```

### Version convention

Use semantic versioning: `vMAJOR.MINOR.PATCH` (e.g., `v1.0.0`, `v1.1.0`, `v2.0.0-rc1`).

## Architecture

- **Entry point**: `cmd/pkt/main.go` → Cobra CLI
- **CLI commands**: `internal/cli/` — version push, build, channel management
- **Diff pipeline**: `internal/diff/` — parallel delta generation with in-memory streaming
- **Native bindings**: `internal/native/` — pure Go librsync (default) + optional CGo librsync/turbopatch
- **Packers**: `internal/content/` (ZIP), `internal/pack1/` (gzip+AES-256-CBC)
- **Upload**: `internal/upload/` — S3 chunked upload with retry
- **Workflow**: `internal/workflow/` — orchestrates push/channel operations
