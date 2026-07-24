# Omni

Omni is a dependency-light Go CLI that exposes service APIs through command paths whose safety class is visible at the left edge of the command.

```text
omni observe trello board list
omni observe trello card get CARD_ID
omni create trello card create LIST_ID --name "Plan release"
omni delete trello card delete CARD_ID
```

The first word after `omni` is always the effect level. A flag must never turn an `observe` command into a mutating operation. This makes approval rules and local policy reliable without parsing arbitrary arguments.

## Start here

```bash
go test ./...
go run ./cmd/omni configure init
go run ./cmd/omni describe --format=json
```

Check the installed release version with `omni version`, `omni --version`, or `omni -V`. Local builds report `dev`; release builds receive their version from the Git tag during compilation.

`configure init` creates the only two configuration files used by the initial skeleton:

```text
~/.config/omni/settings.toml
~/.config/omni/credentials/credentials.toml
```

Populate Trello in one command, rather than editing files manually:

```bash
omni setup trello
omni configure trello --default-board BOARD_ID --api-key API_KEY --api-token API_TOKEN

# Inspect every registry key, its type, and whether it has a code default.
omni configure describe
omni configure help trello.api-key
```

The Trello command stores ordinary settings in `settings.toml` and secrets only in `credentials/credentials.toml`. Commands acknowledge keys but never print secret values. Shell arguments can remain in history or be visible to local processes, so use a short-lived shell or clear the relevant history entry after using the convenience form.

`omni configure help trello.api-key` prints Trello's official API introduction and credential guide: [developer.atlassian.com/cloud/trello/guides/rest-api/api-introduction](https://developer.atlassian.com/cloud/trello/guides/rest-api/api-introduction/). Advanced registry setters remain available through `omni configure set` and `omni configure secret set`.

For example, `trello.api-url` is an optional setting that defaults to `https://api.trello.com/1`. Point it at a compatible mock server with `omni configure trello --api-url URL`; this override applies to the native Trello client without changing code.

See [credential setup](docs/credentials.md) before adding API credentials. Trello's core board/list/card operations are live native API calls; Tailscale is the next planned integration.

## Install

Linux x86_64 releases are distributed as a single static binary with a SHA-256 checksum. After the next release, install the latest version without elevated permissions:

```bash
curl -fsSL https://toppk.github.io/omni/install.sh | sh
```

The script downloads `omni_linux_amd64`, verifies its release checksum, and installs it to `~/.local/bin`. Set `OMNI_INSTALL_DIR` to choose another destination. Read the script before piping it to your shell, or download the binary and checksum directly from [GitHub Releases](https://github.com/toppk/omni/releases/latest).

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

## Capability discovery

Omni exposes a service-level operation catalog for both people and agents:

```bash
omni describe
omni describe trello
omni describe trello --format=json
omni describe observe trello card get --format=json
```

The service JSON includes stable `operation_id` values, action-first command tokens, effect metadata, summaries, descriptions, response descriptions, arguments, options, and credential requirements. This is intentionally shaped like MCP tool enumeration while remaining a local CLI contract; the default terminal view is a human-oriented service manual.

## Developing

Contributor workflow, testing, release process, GitHub Pages, installer maintenance, and SDLC policies are in [DEVELOPING.md](DEVELOPING.md).

## Policies

Use `OMNI_POLICY=read-only` or `omni --policy read-only …` to reject all non-observe commands locally. Other initial modes are `no-delete` and `unattended-safe`.

## Design

The project brief is in [universal-cli-project.md](universal-cli-project.md).

## License

BSD 3-Clause. See [LICENSE](LICENSE).
