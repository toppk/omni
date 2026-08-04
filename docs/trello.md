# Trello

Omni's Trello integration uses the Trello REST API. The authoritative API
reference is <https://developer.atlassian.com/cloud/trello/rest/>.

The checked-in [OpenAPI v3 snapshot](trello-openapi-v3-snapshot-20260724.json)
captures the provider contract used for Trello service work on 2026-07-24. It
was downloaded from Trello's published
<https://dac-static.atlassian.com/cloud/trello/swagger.v3.json?_v=1.1132.0>
endpoint. Treat it as a dated investigation input, not as a promise that a
provider endpoint will remain unchanged.

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

## Output and archive safety

Board overview returns board identity, description, short URL, and each list's
ID and name. To resolve or manage labels, use `observe trello label list
BOARD_ID`: its records include the usable label ID, color, and name. Card
observation returns the fields used for ordinary workflow work: identity, list
and position,
description, due and activity dates, members, labels, archive state, short
URL, checklist IDs, and checklist progress in `badges`. It intentionally omits
provider metadata that does not support a current Omni operation.

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

Card creation accepts repeatable `--label LABEL_ID` and `--member MEMBER_ID`
options. Omni verifies every requested ID against the destination board before
creating the card, then sends the relationships in Trello's initial card-create
request so they cannot be silently dropped.

Label operations distinguish board scope from card scope. `update trello card
label add` and `update trello card label remove` change one card's labels and
reverse each other. `delete trello label delete LABEL_ID` deletes the label
from the board itself, which removes it from every card that carried it. That
deletion is irreversible, so Omni reads the label first and reports its board,
name, and color in the response; recreating an equivalent label with `create
trello label create` yields a new ID and reattaches it to nothing. Labels are
board state rather than card state, so no archive guard applies.

`move trello card move` and other card mutations refuse archived cards. Restore
a card deliberately with `update trello card unarchive CARD_ID`; archive and
unarchive are each other's explicit reversal paths.

Card creation and list name/position changes likewise refuse archived lists.
Restore the list explicitly with `update trello list unarchive LIST_ID` before
adding cards or changing its metadata.

## MVP capabilities

The Trello MVP supports compact card enumeration and search; attachment list,
download, upload, and deletion; checklist item inspection, completion, and rename; card updates,
due-date completion, and explicit unarchive; board-description updates; list
creation, rename, archive, and unarchive; comments; board label listing,
creation, and deletion; board
members; card member and label assignment; and compact card activity. Use
`omni describe trello` for the exact action-first command forms and their
safety constraints.

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
