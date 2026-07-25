package trello

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/toppk/omni/internal/command"
	"github.com/toppk/omni/internal/config"
	"github.com/toppk/omni/internal/output"
)

func TestRequestAddsCredentialsAndJSONPayload(t *testing.T) {
	c := NewClient(config.TrelloCredentials{APIKey: "key-value", Token: "token-value"})
	c.baseURL = "https://example.test"
	c.http = &http.Client{Transport: roundTripper(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost || r.URL.Path != "/cards" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if got := r.URL.Query().Get("key"); got != "key-value" {
			t.Fatalf("key = %q", got)
		}
		if got := r.URL.Query().Get("token"); got != "token-value" {
			t.Fatalf("token = %q", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("content type = %q", got)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(bytes.NewBufferString(`{"id":"card-1","name":"New card"}`)),
			Request:    r,
		}, nil
	})}
	var card map[string]string
	if err := c.request(http.MethodPost, "/cards", map[string]string{"name": "New card"}, &card); err != nil {
		t.Fatal(err)
	}
	if card["id"] != "card-1" {
		t.Fatalf("card = %#v", card)
	}
}

func TestNetworkErrorDoesNotExposeCredentials(t *testing.T) {
	c := NewClient(config.TrelloCredentials{APIKey: "private-key", Token: "private-token"})
	c.baseURL = "https://example.test"
	c.http = &http.Client{Transport: roundTripper(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("request failed for key=private-key&token=private-token")
	})}
	err := c.request(http.MethodGet, "/boards", nil, nil)
	if err == nil {
		t.Fatal("expected network error")
	}
	if text := err.Error(); strings.Contains(text, "private-key") || strings.Contains(text, "private-token") {
		t.Fatalf("error exposes credentials: %s", text)
	}
}

func TestRequestWithQueryAddsProviderAndCredentialParameters(t *testing.T) {
	c := NewClient(config.TrelloCredentials{APIKey: "key-value", Token: "token-value"})
	c.baseURL = "https://example.test"
	c.http = &http.Client{Transport: roundTripper(func(r *http.Request) (*http.Response, error) {
		if got := r.URL.Query().Get("filter"); got != "commentCard" {
			t.Fatalf("filter = %q", got)
		}
		if got := r.URL.Query().Get("limit"); got != "20" {
			t.Fatalf("limit = %q", got)
		}
		if got := r.URL.Query().Get("reactions"); got != "true" {
			t.Fatalf("reactions = %q", got)
		}
		if got := r.URL.Query().Get("key"); got != "key-value" {
			t.Fatalf("key = %q", got)
		}
		if got := r.URL.Query().Get("token"); got != "token-value" {
			t.Fatalf("token = %q", got)
		}
		return jsonResponse(r, `[]`), nil
	})}
	var actions []map[string]any
	if err := c.requestWithQuery(http.MethodGet, "/cards/card-1/actions", map[string]string{"filter": "commentCard", "limit": "20", "reactions": "true"}, nil, &actions); err != nil {
		t.Fatal(err)
	}
}

func TestMoveCardRefusesArchivedCardWithoutMutation(t *testing.T) {
	requests := 0
	c := NewClient(config.TrelloCredentials{})
	c.baseURL = "https://example.test"
	c.http = &http.Client{Transport: roundTripper(func(r *http.Request) (*http.Response, error) {
		requests++
		if r.Method != http.MethodGet || r.URL.Path != "/cards/card-1" {
			t.Fatalf("unexpected request = %s %s", r.Method, r.URL.Path)
		}
		return jsonResponse(r, `{"id":"card-1","closed":true}`), nil
	})}

	_, err := c.moveCard("card-1", "list-2", "bottom")
	if err == nil || err.Error() != "cannot change archived Trello card card-1; unarchive it first" {
		t.Fatalf("error = %v", err)
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want only the archive-state read", requests)
	}
}

func TestMoveCardReadsOpenStateThenUpdates(t *testing.T) {
	requests := 0
	c := NewClient(config.TrelloCredentials{})
	c.baseURL = "https://example.test"
	c.http = &http.Client{Transport: roundTripper(func(r *http.Request) (*http.Response, error) {
		requests++
		switch requests {
		case 1:
			if r.Method != http.MethodGet || r.URL.Path != "/cards/card-1" {
				t.Fatalf("read request = %s %s", r.Method, r.URL.Path)
			}
			return jsonResponse(r, `{"id":"card-1","closed":false}`), nil
		case 2:
			if r.Method != http.MethodGet || r.URL.Path != "/lists/list-2" {
				t.Fatalf("list read request = %s %s", r.Method, r.URL.Path)
			}
			return jsonResponse(r, `{"id":"list-2","closed":false}`), nil
		case 3:
			if r.Method != http.MethodPut || r.URL.Path != "/cards/card-1" {
				t.Fatalf("update request = %s %s", r.Method, r.URL.Path)
			}
			return jsonResponse(r, `{"id":"card-1","idList":"list-2","closed":false}`), nil
		default:
			t.Fatalf("unexpected request %d", requests)
			return nil, nil
		}
	})}

	card, err := c.moveCard("card-1", "list-2", "bottom")
	if err != nil {
		t.Fatal(err)
	}
	if got := card["idList"]; got != "list-2" {
		t.Fatalf("idList = %v", got)
	}
}

func TestRequireOpenListRefusesArchivedList(t *testing.T) {
	c := NewClient(config.TrelloCredentials{})
	c.baseURL = "https://example.test"
	c.http = &http.Client{Transport: roundTripper(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodGet || r.URL.Path != "/lists/list-1" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		return jsonResponse(r, `{"id":"list-1","closed":true}`), nil
	})}
	if err := c.requireOpenList("list-1"); err == nil || err.Error() != "cannot change archived Trello list list-1; unarchive it first" {
		t.Fatalf("error = %v", err)
	}
}

func TestArchiveListCardsReturnsIDsNeededForReversal(t *testing.T) {
	archived := map[string]bool{}
	c := NewClient(config.TrelloCredentials{})
	c.baseURL = "https://example.test"
	c.http = &http.Client{Transport: roundTripper(func(r *http.Request) (*http.Response, error) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/lists/list-1/cards":
			return jsonResponse(r, `[{"id":"card-1"},{"id":"card-2"}]`), nil
		case r.Method == http.MethodPut && r.URL.Path == "/cards/card-1":
			archived["card-1"] = true
			return jsonResponse(r, `{}`), nil
		case r.Method == http.MethodPut && r.URL.Path == "/cards/card-2":
			archived["card-2"] = true
			return jsonResponse(r, `{}`), nil
		default:
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
			return nil, nil
		}
	})}

	ids, err := c.archiveListCards("list-1")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(ids, ",") != "card-1,card-2" || !archived["card-1"] || !archived["card-2"] {
		t.Fatalf("ids=%#v archived=%#v", ids, archived)
	}
}

func TestAttachmentListRequestsAndReturnsCompactAttachments(t *testing.T) {
	stubExecuteClient(t, func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodGet || r.URL.Path != "/cards/card-1/attachments" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		return jsonResponse(r, `[{"id":"attachment-1","name":"chart.png","url":"https://example.test/chart.png","mimeType":"image/png","bytes":42,"date":"2026-07-25","isUpload":true,"previews":[{}]}]`), nil
	})
	d, err := command.Find([]string{"observe", "trello", "attachment", "list"})
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := Execute(d, []string{"card-1"}, config.TrelloCredentials{}, config.TrelloSettings{APIURL: "https://example.test"}, &out); err != nil {
		t.Fatal(err)
	}
	var result struct {
		CardID      string           `json:"card_id"`
		Attachments []map[string]any `json:"attachments"`
	}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.CardID != "card-1" || len(result.Attachments) != 1 || result.Attachments[0]["id"] != "attachment-1" || result.Attachments[0]["name"] != "chart.png" || result.Attachments[0]["previews"] != nil {
		t.Fatalf("attachments = %#v", result)
	}
}

func TestAttachmentListRendersTextWhenRequested(t *testing.T) {
	stubExecuteClient(t, func(r *http.Request) (*http.Response, error) {
		return jsonResponse(r, `[{"id":"attachment-1","name":"chart.png","mimeType":"image/png","bytes":42}]`), nil
	})
	d, err := command.Find([]string{"observe", "trello", "attachment", "list"})
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := ExecuteWithFormat(d, []string{"card-1"}, config.TrelloCredentials{}, config.TrelloSettings{APIURL: "https://example.test"}, output.Text, &out); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"CARD ID: card-1", "ATTACHMENTS (1)", "attachment-1", "chart.png"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("text output missing %q:\n%s", want, out.String())
		}
	}
}

func TestDownloadAttachmentUsesOAuthHeaderWithoutQueryCredentials(t *testing.T) {
	output := filepath.Join(t.TempDir(), "image.png")
	c := NewClient(config.TrelloCredentials{APIKey: "key-value", Token: "token-value"})
	c.baseURL = "https://example.test"
	c.http = &http.Client{Transport: roundTripper(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Host {
		case "example.test":
			return jsonResponse(r, `[{"id":"attachment-1","isUpload":true,"url":"https://trello.com/download/image.png","mimeType":"image/png"}]`), nil
		case "trello.com":
			if got, want := r.Header.Get("Authorization"), `OAuth oauth_consumer_key="key-value", oauth_token="token-value"`; got != want {
				t.Fatalf("authorization = %q, want %q", got, want)
			}
			if r.URL.RawQuery != "" {
				t.Fatalf("download URL has query %q", r.URL.RawQuery)
			}
			return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Body: io.NopCloser(strings.NewReader("image")), Header: make(http.Header)}, nil
		default:
			t.Fatalf("host = %s", r.URL.Host)
			return nil, nil
		}
	})}
	result, err := c.downloadAttachment("card-1", "attachment-1", output)
	if err != nil {
		t.Fatal(err)
	}
	if result["bytes"] != 5 {
		t.Fatalf("result = %#v", result)
	}
	data, err := os.ReadFile(output)
	if err != nil || string(data) != "image" {
		t.Fatalf("file = %q, err = %v", data, err)
	}
}

func TestBoardDescriptionAndDueCompletionUseGuardedUpdates(t *testing.T) {
	requests := 0
	stubExecuteClient(t, func(r *http.Request) (*http.Response, error) {
		requests++
		switch requests {
		case 1:
			if r.Method != http.MethodPut || r.URL.Path != "/boards/board-1" {
				t.Fatalf("board request = %s %s", r.Method, r.URL.Path)
			}
			var payload map[string]string
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || payload["desc"] != "Guide" {
				t.Fatalf("board payload = %#v, err = %v", payload, err)
			}
			return jsonResponse(r, `{"id":"board-1","name":"Board","desc":"Guide"}`), nil
		case 2:
			if r.Method != http.MethodGet || r.URL.Path != "/cards/card-1" {
				t.Fatalf("card guard = %s %s", r.Method, r.URL.Path)
			}
			return jsonResponse(r, `{"id":"card-1","closed":false}`), nil
		case 3:
			if r.Method != http.MethodPut || r.URL.Path != "/cards/card-1" {
				t.Fatalf("due update = %s %s", r.Method, r.URL.Path)
			}
			var payload map[string]bool
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || !payload["dueComplete"] {
				t.Fatalf("due payload = %#v, err = %v", payload, err)
			}
			return jsonResponse(r, `{"id":"card-1","badges":{"dueComplete":true}}`), nil
		default:
			t.Fatalf("unexpected request %d", requests)
			return nil, nil
		}
	})
	board, _ := command.Find([]string{"update", "trello", "board", "set"})
	if err := Execute(board, []string{"board-1", "--description", "Guide"}, config.TrelloCredentials{}, config.TrelloSettings{APIURL: "https://example.test"}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	due, _ := command.Find([]string{"update", "trello", "card", "due", "complete", "set"})
	if err := Execute(due, []string{"card-1", "--state", "complete"}, config.TrelloCredentials{}, config.TrelloSettings{APIURL: "https://example.test"}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
}

func TestChecklistItemRenameUsesChecklistIdentityAndOpenCardGuard(t *testing.T) {
	requests := 0
	stubExecuteClient(t, func(r *http.Request) (*http.Response, error) {
		requests++
		switch requests {
		case 1:
			if r.Method != http.MethodGet || r.URL.Path != "/checklists/checklist-1" {
				t.Fatalf("checklist request = %s %s", r.Method, r.URL.Path)
			}
			return jsonResponse(r, `{"id":"checklist-1","idCard":"card-1"}`), nil
		case 2:
			if r.Method != http.MethodGet || r.URL.Path != "/cards/card-1" {
				t.Fatalf("card guard = %s %s", r.Method, r.URL.Path)
			}
			return jsonResponse(r, `{"id":"card-1","closed":false}`), nil
		case 3:
			if r.Method != http.MethodPut || r.URL.Path != "/cards/card-1/checkItem/item-1" {
				t.Fatalf("rename request = %s %s", r.Method, r.URL.Path)
			}
			var payload map[string]string
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || payload["name"] != "Corrected" {
				t.Fatalf("rename payload = %#v, err = %v", payload, err)
			}
			return jsonResponse(r, `{"id":"item-1","idChecklist":"checklist-1","name":"Corrected","state":"complete"}`), nil
		default:
			t.Fatalf("unexpected request %d", requests)
			return nil, nil
		}
	})
	d, _ := command.Find([]string{"update", "trello", "checklist", "item", "name", "set"})
	if err := Execute(d, []string{"checklist-1", "item-1", "--name", "Corrected"}, config.TrelloCredentials{}, config.TrelloSettings{APIURL: "https://example.test"}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
}

func stubExecuteClient(t *testing.T, handler roundTripper) {
	t.Helper()
	previous := makeClient
	makeClient = func(creds config.TrelloCredentials) *Client {
		client := NewClient(creds)
		client.baseURL = "https://example.test"
		client.http = &http.Client{Transport: handler}
		return client
	}
	t.Cleanup(func() { makeClient = previous })
}

func TestCompactCardKeepsWorkflowAndChecklistProgress(t *testing.T) {
	card := compactCard(map[string]any{
		"id": "card-1", "name": "Plan", "idList": "list-1", "closed": false,
		"idChecklists": []any{"checklist-1"}, "url": "unneeded", "cover": map[string]any{},
		"badges": map[string]any{
			"checkItems": 4, "checkItemsChecked": 2, "comments": 1,
			"lastUpdatedByAi": true, "attachmentsByType": map[string]any{},
		},
	})
	if got := card["idChecklists"]; got == nil {
		t.Fatal("idChecklists was dropped")
	}
	badges, ok := card["badges"].(map[string]any)
	if !ok || badges["checkItems"] != 4 || badges["checkItemsChecked"] != 2 || badges["comments"] != 1 {
		t.Fatalf("badges = %#v", card["badges"])
	}
	if _, ok := badges["lastUpdatedByAi"]; ok {
		t.Fatalf("badges retains provider noise: %#v", badges)
	}
	if _, ok := card["url"]; ok {
		t.Fatalf("card retains duplicate URL: %#v", card)
	}
}

func TestCompactListsKeepsOnlyIdentity(t *testing.T) {
	lists := compactLists([]map[string]any{{"id": "list-1", "name": "Doing", "closed": false, "color": "blue"}})
	if got := lists[0]; len(got) != 2 || got["id"] != "list-1" || got["name"] != "Doing" {
		t.Fatalf("list = %#v", got)
	}
}

func TestCompactBoardOmitsLabelNamesAndPreferences(t *testing.T) {
	board := compactBoard(map[string]any{
		"id": "board-1", "name": "Project", "desc": "Plan", "shortUrl": "https://trello.com/b/x",
		"labelNames": map[string]any{"green": "review"},
		"prefs":      map[string]any{"backgroundImageScaled": []any{}},
		"url":        "https://trello.com/b/x",
		"closed":     false,
	})
	if _, ok := board["labelNames"]; ok {
		t.Fatalf("board retains redundant label names: %#v", board)
	}
	if _, ok := board["prefs"]; ok {
		t.Fatalf("board retains preferences: %#v", board)
	}
	if _, ok := board["url"]; ok {
		t.Fatalf("board retains duplicate URL: %#v", board)
	}
}

func TestCompactChecklistsKeepsItemsAndStates(t *testing.T) {
	checklists := compactChecklists([]map[string]any{{
		"id": "checklist-1", "idCard": "card-1", "name": "Ship", "pos": 1,
		"checkItems": []any{map[string]any{"id": "item-1", "name": "Test", "state": "complete", "pos": 2, "nameData": map[string]any{}}},
	}})
	items, ok := checklists[0]["items"].([]map[string]any)
	if !ok || len(items) != 1 || items[0]["state"] != "complete" || items[0]["name"] != "Test" {
		t.Fatalf("checklists = %#v", checklists)
	}
	if _, ok := items[0]["nameData"]; ok {
		t.Fatalf("item retains provider noise: %#v", items[0])
	}
}

func TestCompactCommentKeepsReactionIdentity(t *testing.T) {
	comment := compactComment(map[string]any{
		"id": "comment-1", "data": map[string]any{"text": "Ship it"},
		"reactions": []any{map[string]any{"id": "reaction-1", "idMember": "member-1", "emoji": map[string]any{"native": "✅"}, "noise": true}},
	})
	reactions, ok := comment["reactions"].([]map[string]any)
	if !ok || len(reactions) != 1 || reactions[0]["id"] != "reaction-1" || reactions[0]["idMember"] != "member-1" || reactions[0]["emoji"] == nil || len(reactions[0]) != 3 {
		t.Fatalf("reactions = %#v", comment["reactions"])
	}
}

func TestSearchCardsStopsAtLimitAndAnnotatesList(t *testing.T) {
	requests := 0
	c := NewClient(config.TrelloCredentials{})
	c.baseURL = "https://example.test"
	c.http = &http.Client{Transport: roundTripper(func(r *http.Request) (*http.Response, error) {
		requests++
		switch r.URL.Path {
		case "/boards/board-1/lists":
			return jsonResponse(r, `[{"id":"list-1","name":"Ideas"},{"id":"list-2","name":"Done"}]`), nil
		case "/lists/list-1/cards":
			return jsonResponse(r, `[{"id":"card-1","name":"Ship Omni","desc":"release"},{"id":"card-2","name":"Other","desc":""}]`), nil
		case "/lists/list-2/cards":
			t.Fatal("search should stop after reaching its limit")
			return nil, nil
		default:
			t.Fatalf("request = %s", r.URL.Path)
			return nil, nil
		}
	})}
	cards, searched, err := c.searchCards("board-1", "omni", 1)
	if err != nil {
		t.Fatal(err)
	}
	if requests != 2 || searched != 2 || len(cards) != 1 || cards[0]["id"] != "card-1" {
		t.Fatalf("requests=%d searched=%d cards=%#v", requests, searched, cards)
	}
	list, ok := cards[0]["list"].(map[string]any)
	if !ok || list["id"] != "list-1" || list["name"] != "Ideas" {
		t.Fatalf("list annotation = %#v", cards[0]["list"])
	}
}

func TestReactionPayloadUsesTrelloEmojiShape(t *testing.T) {
	payload := reactionPayload("🎉")
	if payload["shortName"] != "tada" || payload["unified"] != "1f389" || payload["native"] != "🎉" {
		t.Fatalf("payload = %#v", payload)
	}
	custom := reactionPayload("custom")
	if custom["shortName"] != "custom" || custom["native"] != "custom" {
		t.Fatalf("custom payload = %#v", custom)
	}
}

func TestPositiveIntBoundsResultOptions(t *testing.T) {
	if got, err := positiveInt("", 20, "--limit"); err != nil || got != 20 {
		t.Fatalf("default = %d, %v", got, err)
	}
	if got, err := positiveInt("5", 20, "--limit"); err != nil || got != 5 {
		t.Fatalf("explicit = %d, %v", got, err)
	}
	if _, err := positiveInt("0", 20, "--limit"); err == nil {
		t.Fatal("zero limit unexpectedly accepted")
	}
}

func TestCompactCardActionsOmitsRepeatedCardAndBoard(t *testing.T) {
	actions := compactCardActions([]map[string]any{{
		"id": "action-1", "type": "updateCard", "date": "2026-07-25", "idMemberCreator": "member-1",
		"data": map[string]any{
			"old":   map[string]any{"name": "Old"},
			"card":  map[string]any{"id": "card-1", "name": "Card", "shortLink": "x"},
			"board": map[string]any{"id": "board-1", "name": "Board"},
			"list":  map[string]any{"id": "list-1", "name": "Done", "pos": 1},
		},
	}})
	data := actions[0]["data"].(map[string]any)
	if _, ok := data["card"]; ok {
		t.Fatalf("card repeated in activity: %#v", data)
	}
	if _, ok := data["board"]; ok {
		t.Fatalf("board repeated in activity: %#v", data)
	}
	list := data["list"].(map[string]any)
	if len(list) != 2 || list["id"] != "list-1" || list["name"] != "Done" {
		t.Fatalf("list = %#v", list)
	}
}

func TestCompactBoardActionsOmitsRepeatedBoard(t *testing.T) {
	actions := compactBoardActions([]map[string]any{{
		"id": "action-1", "type": "updateCard", "date": "2026-07-25", "idMemberCreator": "member-1",
		"data": map[string]any{
			"board": map[string]any{"id": "board-1", "name": "Board", "shortLink": "x"},
			"card":  map[string]any{"id": "card-1", "name": "Card", "shortLink": "x"},
			"list":  map[string]any{"id": "list-1", "name": "Done", "pos": 1},
		},
	}})
	data := actions[0]["data"].(map[string]any)
	if _, ok := data["board"]; ok {
		t.Fatalf("board repeated in board activity: %#v", data)
	}
	for _, kind := range []string{"card", "list"} {
		value, ok := data[kind].(map[string]any)
		if !ok || len(value) != 2 || value["id"] == nil || value["name"] == nil {
			t.Fatalf("%s = %#v", kind, data[kind])
		}
	}
}

func TestCompactLabelsOmitsRepeatedBoardID(t *testing.T) {
	labels := compactLabels([]map[string]any{{"id": "label-1", "idBoard": "board-1", "name": "review", "color": "green"}})
	if _, ok := labels[0]["idBoard"]; ok {
		t.Fatalf("label repeats board ID: %#v", labels[0])
	}
}

func jsonResponse(r *http.Request, body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewBufferString(body)),
		Request:    r,
	}
}

type roundTripper func(*http.Request) (*http.Response, error)

func (f roundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
