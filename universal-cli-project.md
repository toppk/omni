# A Universal CLI for Humans and Agents

## Executive Summary

The software industry is rapidly exposing existing services to AI agents through MCP servers, plugins, SDKs, chatbots, browser automation, and custom integrations. This is producing a fragmented and increasingly risky supply chain.

The underlying services generally do not need another API. Trello, Tailscale, Cloudflare, AWS, GitHub, and similar platforms already have capable APIs. What is missing is a consistent **operational interface** that both humans and agents can use, discover, audit, and constrain.

This project proposes an open-source command-line framework and curated CLI that:

- exposes existing service APIs through stable, human-readable commands;
- attaches explicit safety and effect semantics to every operation;
- enforces local modes such as read-only or no-delete;
- emits machine-readable command descriptions;
- generates approval rules for agents such as Codex and Claude Code;
- favors curated, reviewed integrations over an unrestricted plugin ecosystem;
- reuses strong existing CLIs where they already exist;
- ships as a small, easily deployed Go binary.

The objective is not to replace APIs or reproduce MCP over a different transport. It is to create the missing command-oriented layer between existing APIs, human operators, and autonomous agents.

---

## The Problem

Every major platform is trying to make its services usable by AI agents. The mechanisms vary:

- MCP servers;
- REST and GraphQL APIs;
- SDKs;
- provider-specific CLIs;
- browser automation;
- chatbots;
- first-party agent integrations;
- third-party plugins and connectors.

The result is a collection of parallel integration paths with inconsistent behavior, permission models, maintenance standards, and security assumptions.

Every agent repeatedly has to determine:

- How is this service authenticated?
- Which operations are available?
- Does this action merely read data?
- Does it create, update, move, archive, or delete something?
- Is the operation reversible?
- Should the user be asked before execution?
- Can it safely run unattended?
- Does a particular flag change the command's effect?
- Can a user establish a persistent approval policy for it?

Agents currently infer these answers from documentation, help text, HTTP verbs, command names, and prior user approvals. That inference is unreliable.

The missing foundation is not another way to transmit JSON. It is a common operational vocabulary with enforceable semantics.

---

## Existing APIs Already Work

The project does not require providers to redesign or conform their APIs.

REST, GraphQL, gRPC, and provider-specific protocols already solve machine-to-machine communication. Existing SDKs also remain valuable for application development.

A Trello integration can call the Trello API. A Tailscale integration can call the Tailscale API. An AWS integration can delegate to the AWS CLI or SDK. The provider interface remains an implementation detail behind a stable command tree.

This avoids repeating the historical mistake of attempting to make every server participate in one universal, self-describing API system.

OpenAPI can describe request and response shapes, but it generally does not express the operational safety semantics needed by an autonomous agent. HTTP methods offer hints, but they are not a dependable authorization or approval model. A `POST` may perform a harmless query, while a nominally simple update may trigger a large and irreversible workflow.

The semantics therefore belong at the operational interface—the command—where the human or agent actually chooses an action.

---

## Why Not Build Everything Around MCP?

MCP solves a useful discovery and tool-invocation problem, but it also commonly requires someone to build, deploy, authenticate, monitor, and maintain an additional service that translates requests into an API that already exists.

Even after an MCP server is created, humans still need an operational interface. They will expect to type commands, compose them with shell tools, place them in scripts, review them before execution, and run them during incidents.

The CLI is therefore not merely a fallback for MCP. It is independently necessary.

A command-line tool also has several advantages as an agent interface:

- it is inspectable before execution;
- it leaves an ordinary process and audit trail;
- it works in local development, CI, containers, and remote shells;
- it can be invoked by any agent that can run a process;
- it does not require a long-running integration service;
- it remains useful to humans when the agent is unavailable;
- its permission rules can be represented as stable command prefixes.

MCP may remain one consumer or projection of the command schema, but it should not be the foundation that every provider must implement.

---

## A Common Command-Line Layer

The project standardizes the command-oriented layer rather than the underlying APIs.

The commands for Trello do not need to look identical to the commands for Tailscale. Their resource models are different and should remain recognizable.

They should, however, share consistent framework behavior:

```text
<tool> trello board list
<tool> trello card get <card>
<tool> trello card create ...
<tool> tailscale device list
<tool> tailscale device authorize <device>
<tool> tailscale acl validate <file>
<tool> tailscale acl apply <file>
```

The commonality is not a lowest-common-denominator resource model. It is:

- command organization;
- output conventions;
- authentication handling;
- machine-readable discovery;
- error structure;
- effect classification;
- approval metadata;
- policy enforcement;
- audit behavior.

Existing high-quality CLIs should be reused rather than reimplemented. AWS is the clearest example: its CLI already exposes an enormous surface. The framework may wrap, classify, constrain, or describe selected AWS commands without attempting to reproduce all of AWS.

Native integrations are most valuable where a provider has a capable API but no complete administrative CLI.

Trello and Tailscale are strong first integrations:

- Trello has a mature API but lacks a comprehensive official CLI.
- Tailscale has a strong CLI for controlling the local node, but Tailnet-wide administration and configuration remain primarily API- or console-oriented.

---

## Self-Describing Operations

Every operation should carry structured metadata describing what it can do.

A minimal effect vocabulary might include:

- `observe` — reads remote or local state without modifying it;
- `create` — creates a new resource;
- `update` — changes an existing resource;
- `move` — changes placement, ownership, ordering, or workflow state;
- `archive` — removes a resource from normal use but preserves it;
- `delete` — removes a resource, possibly irreversibly;
- `execute` — triggers work or code;
- `transfer` — moves data, money, ownership, or authority;
- `authorize` — changes credentials, access, membership, or policy;
- `administer` — performs a broad or provider-specific privileged operation;
- `unbounded` — exposes a generic request or escape hatch whose effect cannot be known from the command path alone.

Additional properties can refine the classification:

- local or remote effect;
- reversible or irreversible;
- idempotent, conditionally idempotent, or non-idempotent;
- supports dry-run;
- requires elevated credentials;
- can affect one resource or many;
- may expose secrets or private data;
- safe for unattended execution;
- requires explicit confirmation;
- safe to retry.

This metadata should be part of the command definition, not documentation added later.

---

## ACL and Approval Semantics

### Approval Systems Match Command Text

Popular agents already support persistent approval rules for shell commands.

Codex uses prefix-oriented rules such as:

```python
prefix_rule(
    pattern=["gh", "pr", "view"],
    decision="allow",
)
```

Claude Code supports shell permission patterns such as:

```text
Bash(gh pr view:*)
```

These mechanisms are useful, but they operate primarily on the text at the beginning of the command.

In practice, they favor a **big-endian command grammar**:

> The most security-significant information must appear first, while arguments toward the right should only identify targets or provide values.

Once a prefix is approved, approval systems often assume that the remaining arguments do not materially change the safety of the operation.

That assumption is not valid for many ordinary command lines.

### The `uv run` Problem

Consider:

```text
uv run ls
uv run python
```

The first may be a narrowly understandable directory listing. The second launches a general-purpose interpreter capable of reading files, modifying the repository, executing network requests, or deleting data.

A rule approving:

```text
uv run
```

cannot distinguish them.

The command prefix identifies a launcher, not the eventual operation. The safety-defining verb appears too far to the right, and even `uv run python --version` differs fundamentally from `uv run python arbitrary_script.py`.

The same problem appears with:

```text
bash -c ...
python -c ...
podman exec ...
docker run ...
ssh host ...
aws ...
curl ...
```

These commands are execution envelopes. Their early tokens do not determine their final effect.

### Safety Must Be Decidable From the Left

The universal CLI should deliberately organize commands so that safety becomes more specific as early as possible:

```text
<tool> trello board list
<tool> trello board get
<tool> trello board create
<tool> trello board update
<tool> trello board archive
<tool> trello board delete
```

An approval matcher can safely distinguish:

```text
<tool> trello board list ...
```

from:

```text
<tool> trello board delete ...
```

The resource identifier and output formatting flags may appear later because they do not change the fundamental effect.

The command tree should be designed so that a safe prefix never contains a later option that turns it into a destructive command.

Bad:

```text
<tool> trello board list --archive-matching
<tool> tailscale device list --delete-expired
```

Better:

```text
<tool> trello board list
<tool> trello board archive-matching
<tool> tailscale device list
<tool> tailscale device delete-expired
```

A mutating operation should be a distinct command path, not a surprising flag attached to a read operation.

### Arguments Must Not Change the Effect Class

Arguments to the right may influence scope and therefore risk, but they should not change the command's basic effect class.

For example:

```text
<tool> trello card archive CARD_ID
```

is always an archive operation.

However, scope still matters:

```text
<tool> trello card archive CARD_ID
<tool> trello card archive --all-matching QUERY
```

The second command may deserve a stronger approval policy because it can affect many resources. Ideally, bulk operations should also have a distinct prefix:

```text
<tool> trello card archive-one CARD_ID
<tool> trello card archive-matching QUERY
```

or:

```text
<tool> trello card archive CARD_ID
<tool> trello cards archive-matching QUERY
```

This allows approval policy to distinguish singular and bulk effects without parsing arbitrary arguments.

### Escape Hatches Are Unbounded

A generic command such as:

```text
<tool> trello request --method DELETE --path ...
```

cannot be classified safely from its command prefix. Its effect is determined by later arguments and potentially by request content.

Such facilities may be useful for expert humans, but they should be explicitly classified as `unbounded` and excluded from generated auto-approval rules.

The same principle applies to:

- arbitrary shell execution;
- arbitrary HTTP requests;
- embedded scripting;
- provider-specific raw query languages;
- arbitrary plugin loading;
- commands that execute user-provided templates or code.

The framework should not pretend that every capability can be safely reduced to a prefix rule.

### Command Grammar Is Part of the Security Model

Naming and command layout are therefore not cosmetic concerns. They determine whether agent approval systems can express useful policy.

The command grammar should follow these rules:

1. Put provider, resource, and effect-defining verb before identifiers and values.
2. Never let a read-only prefix acquire mutating behavior through a flag.
3. Give bulk operations separate command paths where practical.
4. Keep general-purpose execution and raw-request facilities visibly separate.
5. Preserve stable prefixes across releases.
6. Treat moving an operation beneath an approved prefix as a security-sensitive compatibility change.
7. Generate rules for leaf commands or carefully reviewed subtrees, never for the entire executable by default.

---

## Three Layers of Policy

Operation semantics, CLI enforcement, and agent approval are related but distinct.

### 1. Operation Semantics

The command definition states what the operation does:

```yaml
path: [trello, board, list]
effect: observe
cardinality: many
approval: automatic
```

or:

```yaml
path: [trello, board, delete]
effect: delete
cardinality: one
reversible: false
approval: always
```

### 2. CLI Runtime Policy

The executable enforces a user-selected policy independently of the agent:

```bash
TOOL_POLICY=read-only <tool> trello board delete BOARD_ID
```

must fail.

Possible policy modes include:

- read-only;
- no-delete;
- no-bulk;
- no-auth-changes;
- no-unbounded;
- interactive;
- unattended-safe;
- custom policy file.

This is essential because an agent's allowlist is an approval mechanism, not a complete security boundary.

### 3. Agent Approval Policy

The CLI exports rules in the syntax understood by a particular agent:

```bash
<tool> policy export codex
<tool> policy export claude
```

Read-only operations can be emitted as automatic approvals. Destructive, privileged, bulk, or unbounded operations may be omitted, denied, or emitted as ask-required entries where the agent supports that distinction.

The effective path is:

```text
Agent command matcher
        ↓
CLI semantic policy
        ↓
Provider credentials and ACLs
        ↓
Remote API
```

Each layer addresses a different failure mode.

---

## Generated Agent Rules

The command schema should be the source of truth for:

- Codex approval rules;
- Claude Code permissions;
- future agent-specific formats;
- machine-readable discovery;
- shell completion;
- help text;
- documentation;
- policy validation;
- audit descriptions.

For example, this command definition:

```go
Command{
    Path:        []string{"trello", "board", "list"},
    Effect:      EffectObserve,
    Cardinality: CardinalityMany,
    Approval:    ApprovalAutomatic,
}
```

could generate a Codex rule:

```python
prefix_rule(
    pattern=["<tool>", "trello", "board", "list"],
    decision="allow",
)
```

and a Claude Code permission:

```text
Bash(<tool> trello board list:*)
```

A delete operation would not be included in the default automatic-approval profile.

Export profiles could include:

```text
read-only
safe-unattended
developer
operator
administrator
custom
```

Generated output should include comments or a companion manifest recording:

- framework version;
- integration version;
- policy profile;
- generated command paths;
- omitted command paths and reasons.

This makes approval configuration reviewable rather than magical.

---

## Runtime Safety Modes

Environment variables are convenient for agents and CI:

```bash
TOOL_POLICY=read-only
TOOL_ALLOW_BULK=false
TOOL_ALLOW_UNBOUNDED=false
```

Command-line equivalents should also exist:

```bash
<tool> --policy read-only trello board list
```

The framework must define precedence clearly, for example:

1. immutable administrator policy;
2. project policy;
3. environment policy;
4. command-line restriction.

A lower layer may further restrict policy but should not silently broaden it.

The CLI should fail closed when:

- a command lacks effect metadata;
- an integration is incompatible with the policy engine;
- a raw provider operation cannot be classified;
- generated policy metadata is stale or invalid.

---

## Discoverability

Agents should not scrape prose help to understand the command surface.

The CLI should expose structured discovery:

```bash
<tool> describe --format json
<tool> describe trello card create --format json
<tool> policy explain trello card create
```

The response can include:

- command path;
- summary;
- arguments and flags;
- effect classification;
- cardinality;
- reversibility;
- idempotency;
- authentication requirements;
- required provider scopes;
- approval recommendation;
- policy compatibility;
- dry-run support;
- output schema.

This can support direct CLI use today and other projections, including MCP, later.

---

## Supply-Chain Security

An unrestricted plugin ecosystem would recreate one of the central problems the project is intended to solve.

Allowing arbitrary packages to add credentials, commands, and agent-visible metadata creates opportunities for:

- dependency confusion;
- malicious updates;
- abandoned integrations;
- credential theft;
- misleading safety metadata;
- command-prefix collisions;
- inconsistent behavior;
- transitive dependency growth.

The primary distribution should therefore favor a curated integration repository with centralized review and release discipline.

This does not require excluding smaller providers.

Because the framework is open source, a provider can build its own compatible executable using the same command schema and metadata conventions. Agents can discover and consume that metadata even when the provider-specific implementation is distributed separately.

Trust should remain explicit. A third-party executable should not automatically inherit the trust level of the curated primary CLI merely because it uses the same schema.

---

## Why Go?

This project is a CLI platform, not an embeddable application library.

Go is a strong fit.

### Development Velocity

Go has a small language, fast compilation, strong tooling, straightforward concurrency, and relatively low implementation ceremony. This matters because integration work will involve a large amount of API modeling, authentication, pagination, error handling, and compatibility maintenance.

### Security

Go is memory-safe for ordinary application code. Its rich standard library reduces dependency count and therefore reduces supply-chain exposure.

The language does not eliminate security problems, but it provides a safer default than a low-level language while avoiding the dependency culture common in many JavaScript and TypeScript projects.

### Deployment

Go normally produces a single native executable.

There is no language runtime to install, no virtual environment, and no package graph to resolve on the target machine. That is ideal for:

- human operators;
- coding agents;
- CI systems;
- containers;
- ephemeral hosts;
- remote servers.

### Performance

The workload is primarily network-bound. HTTP latency and provider response time dominate. Go provides more than sufficient performance while keeping development and maintenance costs lower than Rust or Zig for this application.

### Why Not Rust?

Rust offers excellent safety and control, but this project does not require zero-cost abstractions or fine-grained memory management. Compile times, implementation complexity, and ecosystem integration effort could reduce delivery velocity.

Rust remains a good choice for security-sensitive components that genuinely require its stronger low-level guarantees, but it is not necessary for the main CLI.

### Why Not Zig?

Zig is attractive for small binaries and explicit systems programming, but its ecosystem and high-level API client tooling are less mature. The project benefits more from stable HTTP, authentication, encoding, testing, and CLI infrastructure than from low-level control.

### Why Not TypeScript?

TypeScript offers strong ecosystem momentum around agents, but typical Node.js distribution and dependency graphs conflict with the goals of a small, auditable, stable, single-binary operational tool.

The project is intended to provide commands, not libraries for agent-framework developers. It does not need to be written in the language currently fashionable for agent orchestration.

---

## Tool Naming

### No Name Was Previously Settled

The earlier design discussion and project narrative used **“universal CLI”** and examples such as `universal trello ...` descriptively.

That was a placeholder, not a settled executable name or abbreviation.

No authoritative abbreviation was selected.

### Naming Requirements

The final name should work well in both human and agent contexts.

It should be:

- short enough to type frequently;
- easy to pronounce and dictate;
- unambiguous in shell transcripts;
- stable as a command prefix;
- broad enough to cover many services;
- not tied exclusively to AI agents;
- not confused with an existing major CLI;
- searchable as a project and package name;
- suitable for related terms such as policy files, schemas, and integration repositories.

Because the executable name becomes the first token in every approval rule, brevity matters more than it would for an ordinary application.

### Candidate Directions

#### `universal`

Pros:

- directly communicates the ambition;
- matches the language already used in the design;
- easy to understand in prose.

Cons:

- long for a frequently typed executable;
- generic and difficult to search;
- likely to collide with packages, binaries, or unrelated projects;
- can sound more absolute than the project's intentionally pragmatic scope.

Recommendation: retain **universal CLI** as a descriptive project category, not necessarily the binary name.

#### `univ`

Pros:

- recognizable abbreviation of universal;
- relatively short;
- preserves continuity with the current narrative.

Cons:

- may be read as “university”;
- not immediately descriptive;
- still generic.

#### `ucli`

Pros:

- short;
- clearly suggests “universal CLI”;
- works naturally as an executable prefix.

Cons:

- generic;
- may already be used by unrelated projects;
- says little about operations, services, or safety.

#### `svc`

Pros:

- extremely short;
- suggests services;
- produces concise commands such as `svc trello board list`.

Cons:

- highly generic;
- likely to collide;
- can be confused with service-management commands or internal abbreviations.

#### `svcctl`

Pros:

- communicates service control;
- follows familiar `kubectl` and `systemctl` conventions;
- creates readable command lines.

Cons:

- “control” may overemphasize mutation rather than observation;
- somewhat generic;
- five-plus characters are repeated in every invocation.

#### `opsctl`

Pros:

- signals operational use;
- familiar command-line form;
- appropriate for humans and agents.

Cons:

- may imply infrastructure operations only;
- Trello and other business applications do not always feel like “ops”;
- many projects already use similar names.

#### `agentctl`

Pros:

- immediately communicates agent relevance;
- easy to explain.

Cons:

- wrongly suggests that it controls agents rather than services;
- sidelines the human-use case;
- ties the product to current terminology.

#### `actl`

Pros:

- short;
- can be expanded flexibly.

Cons:

- ambiguous;
- difficult to search;
- expansion is not obvious;
- likely to collide.

#### A Distinct Brand Name

A coined project name may ultimately be stronger than a descriptive abbreviation.

Pros:

- easier to search and register;
- less likely to collide;
- can grow beyond the exact initial framing;
- can name the executable, schema, repository, and policy format consistently.

Cons:

- requires explanation;
- does not communicate purpose on first encounter;
- choosing well takes additional work.

### Provisional Naming Recommendation

Until a distinct name is selected:

- refer to the concept as the **universal service CLI** or **universal operational CLI**;
- use `<tool>` in normative command examples;
- use `ucli` only in throwaway prototypes if a concrete executable name is required;
- do not treat `universal` or `ucli` as finalized;
- perform collision, package, repository, and trademark checks before selecting the public name.

The naming decision should be made before approval-rule formats become stable, because the executable name is embedded in every generated prefix rule.

---

## Initial Integrations

### Trello

The Trello integration should validate the framework against a familiar resource model:

- workspaces;
- boards;
- lists;
- cards;
- labels;
- members;
- checklists;
- comments.

It provides useful distinctions among:

- observe operations;
- content creation;
- workflow movement;
- assignment and labeling;
- archival;
- deletion;
- singular and bulk operations.

### Tailscale

The Tailscale integration should validate administrative and security-sensitive semantics:

- devices;
- users;
- keys;
- tags;
- ACLs or grants;
- DNS configuration;
- routes;
- posture and policy settings;
- audit or activity data.

The existing `tailscale` command should remain the natural tool for controlling the local node. The new integration should focus on Tailnet-wide administration rather than creating confusing overlap.

Tailscale is especially useful for testing:

- authorization-changing operations;
- credential generation and revocation;
- policy validation versus policy application;
- local versus remote effects;
- high-risk administrative commands.

---

## Vision

The objective is not to create another universal API standard.

It is to establish a dependable operational language shared by humans and agents.

Existing services retain their existing APIs. Strong provider CLIs remain in place. The universal CLI fills gaps, wraps capabilities where useful, and provides a consistent semantic and policy layer.

The command schema becomes the source of truth for:

- human-readable commands;
- agent discovery;
- effect and safety metadata;
- runtime restrictions;
- approval-rule generation;
- documentation;
- auditing.

The project succeeds when an agent can understand that a command is safe without guessing, a human can inspect and run the same command, and a local policy can prevent an unsafe remote action even when ordinary filesystem or process sandboxing cannot.
