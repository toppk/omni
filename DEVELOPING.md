# Developing Omni

This document is the operating guide for contributors and release maintainers. The [README](README.md) is intentionally limited to installation, service use, and project orientation.

## Principles

- Omni has no runtime third-party Go dependencies unless one provides a clear capability unavailable in the standard library.
- Development-only tools are welcome when they materially improve correctness or delivery; document their purpose and invocation before adding them.
- Command schemas are security contracts. An operation's effect must be determined by its left-hand command path, never by a later flag.
- Credentials belong only in `~/.config/omni/credentials/credentials.toml`; never add credentials, `.env` files, or real configuration to the repository.

## Local requirements

- Go 1.26 or newer.
- Git.
- `curl` and `sha256sum` to test the Linux installer.

No package-manager bootstrap is required for the current project.

## Daily quality gate

```bash
# Format source files in place.
gofmt -w cmd internal

# Compile and run all tests.
go test ./...

# Run standard static checks.
go vet ./...

# Run the complete local gate used before commits.
./scripts/check

# Build a local development binary.
go build -o ./bin/omni ./cmd/omni
./bin/omni version  # omni dev
```

Some sandboxed environments have a read-only default Go build cache. In that case, prefix Go commands with `GOCACHE=/tmp/omni-go-cache`.

## Pre-commit hook

Opt in once per clone:

```bash
git config core.hooksPath .githooks
```

The hook runs `scripts/check`, which fails on unformatted Go source, vet findings, or failed tests. It does not modify files.

## Tests

Tests are colocated with packages under `internal/`. Keep provider HTTP tests local and deterministic by using an in-memory `http.RoundTripper`; do not require real credentials or network access in tests.

When adding a command, test at least:

- the command schema metadata and action-first path;
- policy behavior for its effect class;
- provider request shape and credential redaction where applicable;
- human and JSON discovery output for new service capabilities.

## Releases

Releases are tag-driven. The version is not stored in Go source: the release workflow compiles the Git tag into `internal/app.Version` with Go linker flags.

1. Add `release-notes/X.Y.Z.md` from [the template](release-notes/TEMPLATE.md). Every release note must cover Features, Bug fixes, Breaking changes, Security, and Upgrade notes.
2. Run `./scripts/check`.
3. Commit the change, create an annotated `vX.Y.Z` tag, and push the branch and tag.

```bash
git tag -a vX.Y.Z -m "Omni vX.Y.Z"
git push origin master vX.Y.Z
```

The [release workflow](.github/workflows/release.yml) tests and vets the tag, then builds a stripped static Linux x86_64 binary with `CGO_ENABLED=0`. It publishes:

```text
omni_linux_amd64
omni_linux_amd64.sha256
```

The workflow publishes the matching versioned release-note file rather than generated commit text. Verify a finished release with:

```bash
gh release download vX.Y.Z --pattern omni_linux_amd64 --dir /tmp/omni-verify
chmod +x /tmp/omni-verify/omni_linux_amd64
/tmp/omni-verify/omni_linux_amd64 --version
```

## Website and installer

The static GitHub Pages source is under `website/`. [Deploy Pages](.github/workflows/pages.yml) deploys it when that directory or its workflow changes on `master`.

The public installer is `https://toppk.github.io/omni/install.sh`. It downloads the latest release binary, fetches its release checksum, verifies SHA-256, and installs to `~/.local/bin` without `sudo`. It only replaces a target that identifies as an Omni release, rejects an unrelated `omni` already on `PATH`, and reports the before/after version.

After changing the installer, run:

```bash
sh -n website/install.sh
```

Keep the website's install command, the installer asset name, and the release workflow asset name synchronized.

## Documentation boundaries

- `README.md`: what Omni is, how to install it, and how to start using it.
- `DEVELOPING.md`: contributor workflow, testing, releases, Pages, and SDLC decisions.
- `docs/`: provider-specific operator and credential guidance.
- `release-notes/`: user-facing notes for each tagged release.
