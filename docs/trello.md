# Trello

Omni's Trello integration uses the Trello REST API. The authoritative API
reference is <https://developer.atlassian.com/cloud/trello/rest/>.

The checked-in [OpenAPI v3 snapshot](trello-openapi-v3-snapshot-20260724.json)
captures the provider contract used for Trello service work on 2026-07-24. It
was downloaded from Trello's published
<https://dac-static.atlassian.com/cloud/trello/swagger.v3.json?_v=1.1132.0>
endpoint. Treat it as a dated investigation input, not as a promise that a
provider endpoint will remain unchanged.

It is explicitly not the authority for values Omni validates before a write. Its
`Color` enum, for one, predates Trello's subtle and bold label shades, so a
validator built from it would reject colors Trello itself reports. Label colors
are instead pinned to a live capture in
[internal/trello/testdata](../internal/trello/testdata/README.md).

For API-key and token creation, use these official Trello resources in order:

1. [API introduction and credential overview](https://developer.atlassian.com/cloud/trello/guides/rest-api/api-introduction/)
2. [App-management walkthrough](https://developer.atlassian.com/cloud/trello/guides/power-ups/managing-apps/)
3. [Trello app-management page](https://trello.com/apps/admin), the direct location for creating or managing an API key

`omni setup trello` starts by directing you to the local configuration guide:

```bash
omni setup trello
omni configure trello
```

Configure the supported values in one command:

```bash
omni configure trello --default-board BOARD_ID --api-key API_KEY --api-token API_TOKEN
```

For Omni's current agent-oriented setup, create only the minimal Power-Up
needed to obtain an API key, then open its API Key tab and click **Generate a
Token**. Approve access and copy the resulting user token into Omni. Omni does
not need the Power-Up app secret.
Trello documents a separate OAuth flow, but that flow is designed for an
application-mediated user authorization experience and is not Omni's current
setup path.

`trello.default-board-id` is optional. `trello.api-url` defaults to the public
API endpoint and may be overridden for a compatible mock server.

If you do not know the board ID, configure the API key and token first without
`--default-board`, then list the boards visible to that Trello user:

```bash
omni observe trello board list
# Optional concise ID and name view when jq is available:
omni observe trello board list --format json | jq -r '.boards[] | "\(.id)\t\(.name)"'
omni configure set trello.default-board-id BOARD_ID
```

## Output and compact records

Board overview returns board identity, description, short URL, and each list's
ID, name, and archive state. To resolve or manage labels, use `observe trello
label list BOARD_ID`: its records include the usable label ID, color, and name,
and `observe trello label color list` enumerates every color a label operation
accepts without calling Trello. Card observation returns the fields used for
ordinary workflow work: identity, list and position, description, due and
activity dates, members, labels, archive state, short URL, checklist IDs, and
checklist progress in `badges`. It intentionally omits provider metadata that
does not support a current Omni operation.

The compact `badges` fields are workflow signals, not incidental provider
noise: `checkItems` and `checkItemsChecked` are checklist progress,
`attachments` tells a caller to run `observe trello attachment list CARD_ID`,
and `dueComplete` is the current due-date completion state. Attachment listing
returns compact metadata and the provider URL. Download an uploaded attachment
only to an explicit new file with `observe trello attachment download CARD_ID
ATTACHMENT_ID --output FILE`; uploads and deletion use their distinct create
and delete commands. Omni limits attachment transfer to 25 MiB and never
writes attachment bytes to stdout.

Card search scans the selected board before applying its output limit. Its
response includes `matched` and `truncated`, so a bounded result cannot look
complete. Use `label:NAME` to search by one board label name; an unknown label
name is rejected rather than treated as an empty result.

A `label:` query is a whole query, not one term among several. Everything after
`label:` is the label name, because Trello label names may themselves contain
spaces and colons, so `label:sylvanus is:archived` searches for a label named
`sylvanus is:archived` and is rejected as unknown. Omni recognizes that shape and
says so in the error rather than reporting a bare unknown label. Archive
filtering is a separate option, not a query term.

Card creation accepts repeatable `--label LABEL_ID` and `--member MEMBER_ID`
options. Omni verifies every requested ID against the destination board before
creating the card, then sends the relationships in Trello's initial card-create
request so they cannot be silently dropped.

## Archived visibility

Board reads are open-only by default, which understates the truth whenever
something is being retired. Four reads accept `--scope open|archived|all` and
each reports the scope it used, so an open-only result cannot be mistaken for a
complete one:

```bash
omni observe trello card search "label:sylvanus" --scope all   # cards, by text or label
omni observe trello card list LIST_ID --scope archived         # cards in one list
omni observe trello list list --scope all                      # lists on the board
omni observe trello board overview --scope all                 # board and its lists
```

Archived scopes read Trello's documented board-level filtered routes. For cards
that also reaches cards sitting in archived lists; search annotates those with
the archived list's own record, so a card is never attributed to a bare list ID.
Every list record carries `closed`, so an archived list stays distinguishable
from an open one inside a mixed `--scope all` result.

Card and list scopes are independent, and both directions matter. Without list
scope an archived list is only discoverable indirectly, as the annotation on a
card that happens to match a search, and a board's archived lists cannot be
enumerated at all. Without card scope an archived card is invisible even in a
list you already know about.

This matters before any board-level removal. Trello's `uses` count on a label
record includes cards an open-only read never shows, so an open-only search
cannot reconcile against it. Use `observe trello card search label:NAME --scope
all` to see what a label actually covers before deciding to remove it, and treat
`uses` as a hint rather than an inventory. `delete trello label delete` records
the same evidence itself, so the audit is not left to memory; see
[Labels](#labels).

## Labels

Label operations distinguish board scope from card scope. `update trello card
label add` and `update trello card label remove` change one card's labels and
reverse each other. Labels themselves are board state rather than card state, so
the archive guards below do not apply to them: a label can be changed or deleted
while the cards carrying it are archived.

`update trello label set LABEL_ID [--name NAME] [--color COLOR]` renames or
recolors a label in place. Prefer it over delete-and-recreate whenever the
label's meaning survives the change: the label keeps its ID, so every card
carrying it keeps carrying it, and no card has to be re-tagged. Only the options
supplied change; an omitted `--name` or `--color` is left alone.

`delete trello label delete LABEL_ID` deletes the label from the board itself,
which removes it from every card that carried it, archived cards included. That
deletion is irreversible, so Omni reads the label *and every card carrying it*
first, and reports both.

Recording the cards is not a convenience. Trello writes no activity entry when a
label is attached to a card, so the only evidence a label was ever used is the
cards' own label arrays and Trello's `uses` count — and the delete destroys the
first while the second disappears with the label. Nothing anywhere can
reconstruct the attachment set afterwards, from any source. The response is
therefore a complete restore recipe: recreate the label with `create trello label
create`, then reattach it with `update trello card label add` using the reported
card IDs. The new label has a new ID and starts attached to nothing.

Trello's `uses` count is reported next to the recorded cards so the two can be
compared. If they disagree, something carried the label that Omni could not see,
and that is the last moment the discrepancy is observable.

Label colors are a fixed palette: the hues `green`, `yellow`, `orange`, `red`,
`purple`, `blue`, `sky`, `lime`, `pink`, and `black`, each available unshaded or
as `HUE_subtle` or `HUE_bold`, plus `none` for a colorless label — 31 values.
Trello's API spells the shades `HUE_light` and `HUE_dark` and Omni accepts those
spellings too, reporting whatever Trello stores. An unsupported color is
rejected locally with the full palette in the error, before any request is sent.

Colorless labels are legitimate and Omni round-trips them: `--color none` sends
the explicit null Trello requires, and `delete` reports a colorless label as a
null color in JSON and as `-` in text. `--color` stays required on `label
create` rather than defaulting to colorless, so that an omitted flag cannot
silently choose the one value that renders as no color at all.

## Archive guards

`move trello card move` and other card mutations refuse archived cards. Restore
a card deliberately with `update trello card unarchive CARD_ID`; archive and
unarchive are each other's explicit reversal paths.

Card creation and list name/position changes likewise refuse archived lists.
Restore the list explicitly with `update trello list unarchive LIST_ID` before
adding cards or changing its metadata.

Renaming is the exception, because Trello itself accepts a rename on a closed
list. `update trello list name set LIST_ID --name NAME --archived-list` relabels
an archived list in place. Without the flag the open-list guard still applies, so
a rename that did not expect to touch retired state fails closed; with it, an
*open* list is refused instead. The flag declares which state the caller means,
and a mismatch in either direction is reported rather than obeyed.

The alternative is three mutations — unarchive, rename, archive — which leave the
list live on the board in between, and leave it unarchived entirely if the rename
fails. Prefixing retired lists for audit hygiene is the motivating case. Position
changes get no such option: position on an archived list means nothing.

## MVP capabilities

The Trello MVP supports compact card enumeration and search, both able to read
archived cards; attachment list, download, upload, and deletion; checklist item
inspection, completion, and rename; card updates, due-date completion, and
explicit unarchive; board-description updates; list enumeration including
archived lists; list creation, archive, unarchive, and rename, in place even when
archived; comments; the full board label lifecycle of listing, color
enumeration, creation, rename, recolor, and deletion; board members; card member
and label assignment; and compact card activity. Use `omni describe trello` for
the exact action-first command forms and their safety constraints.

The previous t3m MCP routes are represented by explicit CLI operations. Omni
does not carry t3m's `full_details` switches: those alter response size rather
than capability and would encourage unbounded agent context. `card get-many`
has a fixed ten-card limit; search, comment, and activity reads are bounded by
default; and `card review` combines one compact card with its bounded comments.
The t3m `archive_all_cards` option is a separate `archive trello list cards
archive` operation because archiving a list and archiving every card in it are
materially different targets.

`card review` intentionally does not embed checklists: checklist records can be
substantially larger and remain available through `observe trello checklist
list CARD_ID`. If live use shows the combined card, comment, and checklist view
is worth its additional bounded output, add it as an explicit operation rather
than silently widening `card review`. General board mutation beyond board
description remains deferred until a demonstrated workflow needs it.
