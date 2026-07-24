# Omni

Omni is a dependency-light Go CLI that exposes service APIs through command paths whose safety class is visible at the left edge of the command.

```text
omni observe tailscale device list
omni observe trello board list
omni authorize tailscale device authorize DEVICE_ID
omni delete trello card delete CARD_ID
```

The first word after `omni` is always the effect level. A flag must never turn an `observe` command into a mutating operation. This makes approval rules and local policy reliable without parsing arbitrary arguments.

## Start here

```bash
go test ./...
go run ./cmd/omni configure init
go run ./cmd/omni describe --format=json
```

`configure init` creates the only two configuration files used by the initial skeleton:

```text
~/.config/omni/settings.toml
~/.config/omni/credentials/credentials.toml
```

Populate configuration using lower-case registry-style keys, rather than editing files manually:

```bash
omni configure set trello.default-board-id BOARD_ID
omni configure secret set trello.api-key API_KEY
omni configure secret set trello.api-token API_TOKEN

# Convenience form for both Trello secrets.
omni configure trello auth --api-key API_KEY --api-token API_TOKEN

# Inspect every registry key, its type, and whether it has a code default.
omni configure describe
omni configure help trello.api-key
```

The `set` tree stores ordinary settings in `settings.toml`; `secret set` and `trello auth` store secrets only in `credentials/credentials.toml`. Commands acknowledge keys but never print secret values. Shell arguments can remain in history or be visible to local processes, so use a short-lived shell or clear the relevant history entry after using the convenience form.

`omni configure help trello.api-key` prints Trello's official API introduction and credential guide: [developer.atlassian.com/cloud/trello/guides/rest-api/api-introduction](https://developer.atlassian.com/cloud/trello/guides/rest-api/api-introduction/).

For example, `trello.api-url` is an optional setting that defaults to `https://api.trello.com/1`. Point it at a compatible mock server with `omni configure set trello.api-url URL`; this override applies to the native Trello client without changing code.

See [credential setup](docs/credentials.md) before adding API credentials. Tailscale is currently a registered command surface; Trello's core board/list/card operations are live native API calls.

## Trello commands

After adding a Trello API key and token, commands return JSON suitable for people, shell tools, and agents:

```bash
omni observe trello board list
omni observe trello board overview BOARD_ID
omni observe trello list list BOARD_ID
omni observe trello card get CARD_ID

omni create trello card create LIST_ID --name "Plan release" --description "Initial draft"
omni move trello card move CARD_ID LIST_ID --position bottom
omni archive trello card archive CARD_ID
omni delete trello card delete CARD_ID
```

The first four are `observe` commands. The change operations have their own action-level prefix, so no flag can transform an approved observation into a modification.

## Development workflow

Omni uses only the standard Go toolchain for its initial quality gate—no linter dependency or framework is required.

```bash
# Format source files in place.
gofmt -w cmd internal

# Compile and run all unit tests.
go test ./...

# Run Go's standard static checks.
go vet ./...

# Compile a release binary.
go build -o ./bin/omni ./cmd/omni
```

Run the full pre-commit gate with:

```bash
./scripts/check
```

To use that gate automatically before every local commit, opt in once per clone:

```bash
git config core.hooksPath .githooks
```

The hook checks formatting without changing files, then runs `go vet` and `go test`. A writable Go build cache is required; if a sandboxed environment makes the default cache read-only, use `GOCACHE=/tmp/omni-go-cache` for the command.

## Policies

Use `OMNI_POLICY=read-only` or `omni --policy read-only …` to reject all non-observe commands locally. Other initial modes are `no-delete` and `unattended-safe`.

## Design

The project brief is in [universal-cli-project.md](universal-cli-project.md).
