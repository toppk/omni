# Trello

Omni's Trello integration uses the Trello REST API. The authoritative API
reference is <https://developer.atlassian.com/cloud/trello/rest/>.

For API-key and token creation, use the official [API introduction and
credential guide](https://developer.atlassian.com/cloud/trello/guides/rest-api/api-introduction/).
That URL is also available locally through:

```bash
omni setup trello
omni configure help trello.api-key
```

Configure the supported values in one command:

```bash
omni configure trello --default-board BOARD_ID --api-key API_KEY --api-token API_TOKEN
```

`trello.default-board-id` is optional. `trello.api-url` defaults to the public
API endpoint and may be overridden for a compatible mock server.

