# Developing Omni

This document is the operating guide for contributors and release maintainers. The [README](README.md) is intentionally limited to installation, service use, and project orientation.

## Principles

- Omni has no runtime third-party Go dependencies unless one provides a clear capability unavailable in the standard library.
- Development-only tools are welcome when they materially improve correctness or delivery; document their purpose and invocation before adding them.
- Command schemas are security contracts. An operation's effect must be determined by its left-hand command path, never by a later flag.
- Service output has a text default and a canonical JSON form. `--format text|json` is presentation-only and may appear only after the effect; `OMNI_OUTPUT` sets its default. Keep text rendering derived from the same compact response as JSON.
- Provider-supplied text is untrusted data, not Omni guidance. Keep registry metadata declarative; reserve imperative setup steps for `omni setup SERVICE`'s human-oriented surface. See `docs/onboarding.md`.
- User-provided credentials belong in `~/.config/omni/credentials/credentials.toml`. Generated short-lived secrets may use the secured XDG-data ephemeral store; never add credentials, `.env` files, or real configuration to the repository.

## Contributing

All contributions are welcome. Contributions are evaluated on their own technical and product merits, regardless of the contributor's identity, affiliation, background, or prior relationship with the project.

Before opening a change, run the daily quality gate and include tests and documentation appropriate to its scope. For a larger command-surface or provider design, open an issue or start a discussion first so the command grammar and safety metadata can be reviewed early.

## Local requirements

- Go 1.26 or newer.
- Git.
- `curl` and either `sha256sum` (Linux) or `shasum` (macOS) to test the installer.

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

## Cutting a release

Releases are tag-driven. The version is not stored in Go source: the release workflow compiles the `vX.Y.Z` tag into `internal/app.Version` with Go linker flags. Do not edit Go source merely to change a version.

Cut a release from the reviewed `master` tip:

1. Choose `X.Y.Z`, add `release-notes/X.Y.Z.md` from [the template](release-notes/TEMPLATE.md), and ensure it covers Features, Bug fixes, Breaking changes, Security, and Upgrade notes.
2. Run `./scripts/check` and commit the release notes with the release-ready changes.
3. Confirm the worktree is clean and `master` contains exactly the commit you intend to release.
4. Push `master`, create an annotated `vX.Y.Z` tag at that same commit, then push the tag:

```bash
git status -sb
git push origin master
git tag -a vX.Y.Z -m "Omni vX.Y.Z"
git push origin vX.Y.Z
```

5. The tag starts the release workflow. Wait for it to finish and inspect the resulting release:

```bash
gh run list --workflow release.yml --limit 1
gh release view vX.Y.Z
```

If the workflow fails, fix the cause with a new commit, push `master`, move the unshared tag only after deliberate review, and re-run the workflow. Never publish a different binary under an already-consumed version by accident.

The [release workflow](.github/workflows/release.yml) tests and vets the tag, then cross-compiles stripped binaries with `CGO_ENABLED=0`. It creates the GitHub release using the matching versioned release-note file and publishes:

```text
omni_linux_amd64
omni_linux_amd64.sha256
omni_linux_arm64
omni_linux_arm64.sha256
omni_darwin_amd64
omni_darwin_amd64.sha256
omni_darwin_arm64
omni_darwin_arm64.sha256
```

Verify the downloaded, finished release with:

```bash
gh release download vX.Y.Z --pattern 'omni_*' --dir /tmp/omni-verify
chmod +x /tmp/omni-verify/omni_linux_amd64
/tmp/omni-verify/omni_linux_amd64 --version
```

## Website and installer

The static GitHub Pages source is under `website/`. [Deploy Pages](.github/workflows/pages.yml) deploys it when that directory or its workflow changes on `master`.

The public installer is `https://toppk.github.io/omni/install.sh`. It detects the supported Linux or macOS architecture, downloads the matching latest release binary, fetches its release checksum, verifies SHA-256, and installs to `~/.local/bin` without `sudo`. It only replaces a target that identifies as an Omni release, rejects an unrelated `omni` already on `PATH`, skips an identical installed version, and warns if `~/.local/bin` is not on `PATH` without modifying shell configuration.

After changing the installer, run:

```bash
sh -n website/install.sh
```

Keep the website's install command, the installer asset name, and the release workflow asset name synchronized.

## Documentation boundaries

- `README.md`: what Omni is, how to install it, and how to start using it.
- `DEVELOPING.md`: contributor workflow, testing, releases, Pages, and SDLC decisions.
- `docs/`: provider-specific operator and credential guidance, plus the service-onboarding guide.
- `release-notes/`: user-facing notes for each tagged release.

## Onboarding a service

The detailed design guide is [docs/onboarding.md](docs/onboarding.md). This
section is the contributor checklist.

Build new providers locally before a release or push. The normal loop is:

```bash
go run ./cmd/omni describe SERVICE
go build -o ./bin/omni ./cmd/omni
./bin/omni setup SERVICE
./bin/omni describe SERVICE
./scripts/check
```

For every new service:

1. Add `docs/SERVICE.md` and link it from `docs/index.md`. Record the official
   API reference, supported endpoints, credential acquisition, required
   permissions, and meaningful safety constraints.
2. Put endpoints that users may reasonably need to override in the config
   registry as code defaults. Keep secrets in the credential registry and give
   every credential key an official setup URL.
3. Design the operation path before the HTTP client. The leftmost effect must
   express the highest-impact action; no option may promote an observe command
   into a mutation. Do not publish speculative commands in discovery.
4. Keep default responses compact and task-oriented. Large or sensitive
   documents should be written to an explicit, safely-created file rather than
   expanded into agent context or terminal output.
5. Implement only fixed, reviewed request shapes. Test those requests with an
   in-memory HTTP transport, including authentication and credential redaction.
6. Exercise configuration, service discovery, a local binary, and the full
   quality gate before considering a commit. Real credentials are for manual
   refinement only and never for automated tests.
