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

func TestLabelDeleteRecordsLabelIdentityBeforeDeleting(t *testing.T) {
	requests := 0
	stubExecuteClient(t, func(r *http.Request) (*http.Response, error) {
		requests++
		switch requests {
		case 1:
			if r.Method != http.MethodGet || r.URL.Path != "/labels/label-1" {
				t.Fatalf("label read = %s %s", r.Method, r.URL.Path)
			}
			return jsonResponse(r, `{"id":"label-1","idBoard":"board-1","name":"Blocked","color":"red","uses":4}`), nil
		case 2:
			if r.Method != http.MethodGet || r.URL.Path != "/boards/board-1/cards/all" {
				t.Fatalf("carrier read = %s %s", r.Method, r.URL.Path)
			}
			return jsonResponse(r, `[{"id":"card-1","name":"Open","idList":"list-1","closed":false,"labels":[{"id":"label-1"}]}]`), nil
		case 3:
			if r.Method != http.MethodDelete || r.URL.Path != "/labels/label-1" {
				t.Fatalf("label delete = %s %s", r.Method, r.URL.Path)
			}
			return jsonResponse(r, `{}`), nil
		default:
			t.Fatalf("unexpected request %d", requests)
			return nil, nil
		}
	})
	d, _ := command.Find([]string{"delete", "trello", "label", "delete"})
	out := &bytes.Buffer{}
	if err := ExecuteWithFormat(d, []string{"label-1"}, config.TrelloCredentials{}, config.TrelloSettings{APIURL: "https://example.test"}, output.JSON, out); err != nil {
		t.Fatal(err)
	}
	if requests != 3 {
		t.Fatalf("requests = %d, want 3", requests)
	}
	var payload struct {
		DeletedLabel map[string]any `json:"deleted_label"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.DeletedLabel["id"] != "label-1" || payload.DeletedLabel["idBoard"] != "board-1" {
		t.Fatalf("deleted label identity = %#v", payload.DeletedLabel)
	}
	if payload.DeletedLabel["name"] != "Blocked" || payload.DeletedLabel["color"] != "red" {
		t.Fatalf("deleted label recreation fields = %#v", payload.DeletedLabel)
	}
	if _, ok := payload.DeletedLabel["uses"]; ok {
		t.Fatalf("deleted label kept uncompacted fields = %#v", payload.DeletedLabel)
	}
}

func TestLabelDeleteReportsColorlessLabelVisiblyAndRecreatably(t *testing.T) {
	stubExecuteClient(t, func(r *http.Request) (*http.Response, error) {
		switch {
		case r.URL.Path == "/labels/label-1" && r.Method == http.MethodGet:
			return jsonResponse(r, `{"id":"label-1","idBoard":"board-1","name":"sylvanus","color":null}`), nil
		case r.URL.Path == "/boards/board-1/cards/all":
			return jsonResponse(r, `[]`), nil
		}
		return jsonResponse(r, `{}`), nil
	})
	d, _ := command.Find([]string{"delete", "trello", "label", "delete"})
	text := &bytes.Buffer{}
	if err := ExecuteWithFormat(d, []string{"label-1"}, config.TrelloCredentials{}, config.TrelloSettings{APIURL: "https://example.test"}, output.Text, text); err != nil {
		t.Fatal(err)
	}
	// A null color must render as a value, not as a bare heading that reads like
	// a rendering failure.
	if !strings.Contains(text.String(), "COLOR: -") {
		t.Fatalf("colorless label rendered without a placeholder:\n%s", text.String())
	}
	// The color the pre-read reports has to be a value label create accepts,
	// otherwise the deleted label cannot be recreated through Omni at all.
	jsonOut := &bytes.Buffer{}
	if err := ExecuteWithFormat(d, []string{"label-1"}, config.TrelloCredentials{}, config.TrelloSettings{APIURL: "https://example.test"}, output.JSON, jsonOut); err != nil {
		t.Fatal(err)
	}
	var payload struct {
		DeletedLabel map[string]any `json:"deleted_label"`
	}
	if err := json.Unmarshal(jsonOut.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if color, ok := payload.DeletedLabel["color"]; !ok || color != nil {
		t.Fatalf("JSON dropped or altered the null color: %#v", payload.DeletedLabel)
	}
	if _, err := labelColor(colorlessLabel); err != nil {
		t.Fatalf("label create cannot accept the reported colorless value: %v", err)
	}
}

func TestLabelCreateSendsExplicitNullForColorlessLabel(t *testing.T) {
	var payload map[string]any
	stubExecuteClient(t, func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost || r.URL.Path != "/labels" {
			t.Fatalf("label create = %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		return jsonResponse(r, `{"id":"label-2","idBoard":"board-1","name":"sylvanus","color":null}`), nil
	})
	d, _ := command.Find([]string{"create", "trello", "label", "create"})
	if err := Execute(d, []string{"board-1", "--name", "sylvanus", "--color", "none"}, config.TrelloCredentials{}, config.TrelloSettings{APIURL: "https://example.test"}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	// Trello requires the color parameter to be present but allows it to be
	// null, so the key must survive with a null value rather than be omitted.
	color, present := payload["color"]
	if !present || color != nil {
		t.Fatalf("create payload = %#v, want color present and null", payload)
	}
}

func TestLabelCreateRejectsUnknownColorBeforeCallingTrello(t *testing.T) {
	stubExecuteClient(t, func(r *http.Request) (*http.Response, error) {
		t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		return nil, nil
	})
	d, _ := command.Find([]string{"create", "trello", "label", "create"})
	err := Execute(d, []string{"board-1", "--name", "Blocked", "--color", "chartreuse"}, config.TrelloCredentials{}, config.TrelloSettings{APIURL: "https://example.test"}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected an unsupported color to be rejected")
	}
	if !strings.Contains(err.Error(), "chartreuse") {
		t.Fatalf("error = %v", err)
	}
}

func TestLabelSetChangesOnlySuppliedFields(t *testing.T) {
	var payload map[string]any
	stubExecuteClient(t, func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPut || r.URL.Path != "/labels/label-1" {
			t.Fatalf("label set = %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		return jsonResponse(r, `{"id":"label-1","idBoard":"board-1","name":"current","color":"sky"}`), nil
	})
	d, _ := command.Find([]string{"update", "trello", "label", "set"})
	if err := Execute(d, []string{"label-1", "--name", "current"}, config.TrelloCredentials{}, config.TrelloSettings{APIURL: "https://example.test"}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	// A rename must not carry a color, or it would reset the label's color.
	if payload["name"] != "current" || len(payload) != 1 {
		t.Fatalf("rename payload = %#v", payload)
	}
}

func TestLabelSetNormalizesShadeAndRequiresOneField(t *testing.T) {
	var payload map[string]any
	stubExecuteClient(t, func(r *http.Request) (*http.Response, error) {
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		return jsonResponse(r, `{"id":"label-1","idBoard":"board-1","name":"current","color":"sky_dark"}`), nil
	})
	d, _ := command.Find([]string{"update", "trello", "label", "set"})
	if err := Execute(d, []string{"label-1", "--color", "sky_bold"}, config.TrelloCredentials{}, config.TrelloSettings{APIURL: "https://example.test"}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if payload["color"] != "sky_dark" || len(payload) != 1 {
		t.Fatalf("recolor payload = %#v", payload)
	}
	if err := Execute(d, []string{"label-1"}, config.TrelloCredentials{}, config.TrelloSettings{APIURL: "https://example.test"}, &bytes.Buffer{}); err == nil {
		t.Fatal("expected label set with no fields to be rejected")
	}
}

func TestCardListScopeReadsArchivedCardsThroughBoardFilter(t *testing.T) {
	paths := []string{}
	stubExecuteClient(t, func(r *http.Request) (*http.Response, error) {
		paths = append(paths, r.URL.Path)
		switch r.URL.Path {
		case "/lists/list-1":
			return jsonResponse(r, `{"id":"list-1","idBoard":"board-1","name":"Doing"}`), nil
		case "/boards/board-1/cards/closed":
			return jsonResponse(r, `[{"id":"card-1","idList":"list-1","name":"Archived","closed":true},{"id":"card-2","idList":"list-9","name":"Elsewhere","closed":true}]`), nil
		default:
			t.Fatalf("unexpected request %s", r.URL.Path)
			return nil, nil
		}
	})
	d, _ := command.Find([]string{"observe", "trello", "card", "list"})
	out := &bytes.Buffer{}
	if err := ExecuteWithFormat(d, []string{"list-1", "--scope", "archived"}, config.TrelloCredentials{}, config.TrelloSettings{APIURL: "https://example.test"}, output.JSON, out); err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Cards []map[string]any `json:"cards"`
		Scope string           `json:"scope"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	// Only this list's cards, and the scope is reported so an archived-only read
	// cannot be mistaken for the whole list.
	if len(payload.Cards) != 1 || payload.Cards[0]["id"] != "card-1" || payload.Scope != "archived" {
		t.Fatalf("payload = %#v", payload)
	}
	if len(paths) != 2 {
		t.Fatalf("requests = %v", paths)
	}
}

func TestCardListOpenScopeKeepsTheSingleListRequest(t *testing.T) {
	requests := 0
	stubExecuteClient(t, func(r *http.Request) (*http.Response, error) {
		requests++
		if r.URL.Path != "/lists/list-1/cards" {
			t.Fatalf("open card list = %s", r.URL.Path)
		}
		return jsonResponse(r, `[{"id":"card-1","idList":"list-1","name":"Open"}]`), nil
	})
	d, _ := command.Find([]string{"observe", "trello", "card", "list"})
	if err := Execute(d, []string{"list-1"}, config.TrelloCredentials{}, config.TrelloSettings{APIURL: "https://example.test"}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want 1", requests)
	}
}

func TestLabelDeleteRecordsEveryCardCarryingTheLabelIncludingArchived(t *testing.T) {
	deleted := false
	stubExecuteClient(t, func(r *http.Request) (*http.Response, error) {
		switch {
		case r.URL.Path == "/labels/label-1" && r.Method == http.MethodGet:
			return jsonResponse(r, `{"id":"label-1","idBoard":"board-1","name":"sylvanus","color":"red","uses":3}`), nil
		case r.URL.Path == "/boards/board-1/cards/all":
			if deleted {
				t.Fatal("carriers were read after the label was already deleted")
			}
			return jsonResponse(r, `[
				{"id":"card-1","name":"Open work","idList":"list-1","closed":false,"labels":[{"id":"label-1"},{"id":"label-9"}]},
				{"id":"card-2","name":"Archived work","idList":"list-2","closed":true,"labels":[{"id":"label-1"}]},
				{"id":"card-3","name":"Unlabelled","idList":"list-1","closed":false,"labels":[{"id":"label-9"}]}]`), nil
		case r.URL.Path == "/labels/label-1" && r.Method == http.MethodDelete:
			deleted = true
			return jsonResponse(r, `{}`), nil
		}
		t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		return nil, nil
	})
	d, _ := command.Find([]string{"delete", "trello", "label", "delete"})
	out := &bytes.Buffer{}
	if err := ExecuteWithFormat(d, []string{"label-1"}, config.TrelloCredentials{}, config.TrelloSettings{APIURL: "https://example.test"}, output.JSON, out); err != nil {
		t.Fatal(err)
	}
	var payload struct {
		DetachedCards []map[string]any `json:"detached_cards"`
		Count         int              `json:"detached_card_count"`
		ReportedUses  float64          `json:"reported_uses"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	// The archived carrier is the one no other read would surface, and it loses
	// the label just as the open one does.
	if payload.Count != 2 || len(payload.DetachedCards) != 2 {
		t.Fatalf("detached cards = %#v", payload.DetachedCards)
	}
	if payload.DetachedCards[1]["id"] != "card-2" || payload.DetachedCards[1]["closed"] != true {
		t.Fatalf("archived carrier not recorded: %#v", payload.DetachedCards)
	}
	// The ID needed to reattach the recreated label must be present, and must
	// survive text rendering: a record carrying idList renders as a card table,
	// which omits the ID entirely.
	if payload.DetachedCards[0]["id"] != "card-1" || payload.DetachedCards[0]["name"] != "Open work" {
		t.Fatalf("restore recipe incomplete: %#v", payload.DetachedCards[0])
	}
	if _, ok := payload.DetachedCards[0]["idList"]; ok {
		t.Fatalf("carrier record would render as a card table and hide its ID: %#v", payload.DetachedCards[0])
	}
	// Trello said 3, Omni saw 2: the discrepancy is observable exactly once.
	if payload.ReportedUses != 3 {
		t.Fatalf("reported uses = %v, want Trello's own count for comparison", payload.ReportedUses)
	}
}

func TestLabelDeleteAbortsWhenCarriersCannotBeRecorded(t *testing.T) {
	stubExecuteClient(t, func(r *http.Request) (*http.Response, error) {
		switch {
		case r.URL.Path == "/labels/label-1" && r.Method == http.MethodGet:
			return jsonResponse(r, `{"id":"label-1","idBoard":"board-1","name":"sylvanus","color":"red"}`), nil
		case r.URL.Path == "/boards/board-1/cards/all":
			return &http.Response{StatusCode: http.StatusForbidden, Status: "403 Forbidden", Body: io.NopCloser(bytes.NewBufferString("nope")), Request: r}, nil
		case r.Method == http.MethodDelete:
			t.Fatal("label was deleted without recording the cards carrying it")
		}
		return nil, nil
	})
	d, _ := command.Find([]string{"delete", "trello", "label", "delete"})
	if err := Execute(d, []string{"label-1"}, config.TrelloCredentials{}, config.TrelloSettings{APIURL: "https://example.test"}, &bytes.Buffer{}); err == nil {
		t.Fatal("expected the delete to abort when carriers cannot be read")
	}
}

func TestLabelDeleteRefusesLabelWithoutBoardIdentity(t *testing.T) {
	stubExecuteClient(t, func(r *http.Request) (*http.Response, error) {
		if r.Method == http.MethodDelete {
			t.Fatal("label was deleted without a board to enumerate carriers from")
		}
		return jsonResponse(r, `{"id":"label-1","name":"sylvanus","color":"red"}`), nil
	})
	d, _ := command.Find([]string{"delete", "trello", "label", "delete"})
	if err := Execute(d, []string{"label-1"}, config.TrelloCredentials{}, config.TrelloSettings{APIURL: "https://example.test"}, &bytes.Buffer{}); err == nil {
		t.Fatal("expected a label without idBoard to be refused")
	}
}

func TestLabelDeleteRejectsMissingLabelIDWithoutRequests(t *testing.T) {
	stubExecuteClient(t, func(r *http.Request) (*http.Response, error) {
		t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		return nil, nil
	})
	d, _ := command.Find([]string{"delete", "trello", "label", "delete"})
	if err := Execute(d, nil, config.TrelloCredentials{}, config.TrelloSettings{APIURL: "https://example.test"}, &bytes.Buffer{}); err == nil {
		t.Fatal("expected an error when LABEL_ID is missing")
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

func TestCompactListsKeepsIdentityAndArchiveState(t *testing.T) {
	lists := compactLists([]map[string]any{
		{"id": "list-1", "name": "Doing", "closed": false, "color": "blue"},
		{"id": "list-2", "name": "📋 BACKLOG - Medium Priority", "closed": true, "color": "blue"},
	})
	if got := lists[0]; len(got) != 3 || got["id"] != "list-1" || got["name"] != "Doing" || got["closed"] != false {
		t.Fatalf("open list = %#v", got)
	}
	// An archived list in a mixed result must be distinguishable from an open one.
	if got := lists[1]; got["closed"] != true {
		t.Fatalf("archived list = %#v", got)
	}
	if _, ok := lists[0]["color"]; ok {
		t.Fatalf("list retains provider noise: %#v", lists[0])
	}
}

func TestListListScopeEnumeratesArchivedListsDirectly(t *testing.T) {
	paths := []string{}
	stubExecuteClient(t, func(r *http.Request) (*http.Response, error) {
		paths = append(paths, r.URL.Path)
		return jsonResponse(r, `[{"id":"list-1","name":"Doing","closed":false},{"id":"list-2","name":"📋 BACKLOG - Medium Priority","closed":true}]`), nil
	})
	d, _ := command.Find([]string{"observe", "trello", "list", "list"})
	out := &bytes.Buffer{}
	if err := ExecuteWithFormat(d, []string{"board-1", "--scope", "all"}, config.TrelloCredentials{}, config.TrelloSettings{APIURL: "https://example.test"}, output.JSON, out); err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 || paths[0] != "/boards/board-1/lists/all" {
		t.Fatalf("requests = %v", paths)
	}
	var payload struct {
		Lists []map[string]any `json:"lists"`
		Scope string           `json:"scope"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Scope != "all" || len(payload.Lists) != 2 || payload.Lists[1]["closed"] != true {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestListListDefaultsToOpenScopeAndTakesBoardFromSettings(t *testing.T) {
	paths := []string{}
	stubExecuteClient(t, func(r *http.Request) (*http.Response, error) {
		paths = append(paths, r.URL.Path)
		return jsonResponse(r, `[{"id":"list-1","name":"Doing","closed":false}]`), nil
	})
	d, _ := command.Find([]string{"observe", "trello", "list", "list"})
	settings := config.TrelloSettings{APIURL: "https://example.test", DefaultBoardID: "board-default"}
	// The option must not be mistaken for the optional BOARD_ID argument.
	if err := Execute(d, []string{"--scope", "archived"}, config.TrelloCredentials{}, settings, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if err := Execute(d, nil, config.TrelloCredentials{}, settings, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if len(paths) != 2 || paths[0] != "/boards/board-default/lists/closed" || paths[1] != "/boards/board-default/lists/open" {
		t.Fatalf("requests = %v", paths)
	}
	if err := Execute(d, []string{"board-1", "--scope", "sideways"}, config.TrelloCredentials{}, settings, &bytes.Buffer{}); err == nil {
		t.Fatal("expected an unknown scope to be rejected")
	}
	if err := Execute(d, []string{"board-1", "board-2"}, config.TrelloCredentials{}, settings, &bytes.Buffer{}); err == nil {
		t.Fatal("expected a second positional argument to be rejected")
	}
}

func TestBoardOverviewScopeIncludesArchivedLists(t *testing.T) {
	paths := []string{}
	stubExecuteClient(t, func(r *http.Request) (*http.Response, error) {
		paths = append(paths, r.URL.Path)
		if r.URL.Path == "/boards/board-1" {
			return jsonResponse(r, `{"id":"board-1","name":"Board","desc":"","shortUrl":"https://trello.com/b/x"}`), nil
		}
		return jsonResponse(r, `[{"id":"list-2","name":"Retired","closed":true}]`), nil
	})
	d, _ := command.Find([]string{"observe", "trello", "board", "overview"})
	out := &bytes.Buffer{}
	if err := ExecuteWithFormat(d, []string{"board-1", "--scope", "archived"}, config.TrelloCredentials{}, config.TrelloSettings{APIURL: "https://example.test"}, output.JSON, out); err != nil {
		t.Fatal(err)
	}
	if len(paths) != 2 || paths[1] != "/boards/board-1/lists/closed" {
		t.Fatalf("requests = %v", paths)
	}
	if !strings.Contains(out.String(), `"scope":"archived"`) {
		t.Fatalf("overview did not report its list scope: %s", out.String())
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
		case "/boards/board-1/lists/all":
			return jsonResponse(r, `[{"id":"list-1","name":"Ideas"},{"id":"list-2","name":"Done"}]`), nil
		case "/boards/board-1/cards/open":
			return jsonResponse(r, `[{"id":"card-1","name":"Ship Omni","desc":"release","idList":"list-1"},{"id":"card-2","name":"Other","desc":"","idList":"list-1"},{"id":"card-3","name":"Other","desc":"","idList":"list-2"}]`), nil
		default:
			t.Fatalf("request = %s", r.URL.Path)
			return nil, nil
		}
	})}
	cards, searched, matched, err := c.searchCards("board-1", "omni", 1, "open")
	if err != nil {
		t.Fatal(err)
	}
	if requests != 2 || searched != 3 || matched != 1 || len(cards) != 1 || cards[0]["id"] != "card-1" {
		t.Fatalf("requests=%d searched=%d matched=%d cards=%#v", requests, searched, matched, cards)
	}
	list, ok := cards[0]["list"].(map[string]any)
	if !ok || list["id"] != "list-1" || list["name"] != "Ideas" {
		t.Fatalf("list annotation = %#v", cards[0]["list"])
	}
}

func TestSearchCardsScopeReachesArchivedCardsAndArchivedLists(t *testing.T) {
	c := NewClient(config.TrelloCredentials{})
	c.baseURL = "https://example.test"
	c.http = &http.Client{Transport: roundTripper(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/boards/board-1/lists/all":
			return jsonResponse(r, `[{"id":"list-1","name":"Doing","closed":false},{"id":"list-2","name":"Retired","closed":true}]`), nil
		case "/boards/board-1/cards/all":
			return jsonResponse(r, `[
				{"id":"card-1","name":"Open work","idList":"list-1","closed":false,"labels":[{"id":"label-1","name":"sylvanus"}]},
				{"id":"card-2","name":"Archived work","idList":"list-1","closed":true,"labels":[{"id":"label-1","name":"sylvanus"}]},
				{"id":"card-3","name":"In retired list","idList":"list-2","closed":false,"labels":[{"id":"label-1","name":"sylvanus"}]},
				{"id":"card-4","name":"Unlabelled","idList":"list-1","closed":false,"labels":[]}]`), nil
		case "/boards/board-1/labels":
			return jsonResponse(r, `[{"id":"label-1","name":"sylvanus","color":null}]`), nil
		default:
			t.Fatalf("request = %s", r.URL.Path)
			return nil, nil
		}
	})}
	cards, searched, matched, err := c.searchCards("board-1", "label:sylvanus", 20, "all")
	if err != nil {
		t.Fatal(err)
	}
	if searched != 4 || matched != 3 || len(cards) != 3 {
		t.Fatalf("searched=%d matched=%d cards=%#v", searched, matched, cards)
	}
	// The archived card is the one an open-only read misses, and the card in the
	// archived list must still name that list rather than a bare ID.
	if cards[1]["id"] != "card-2" {
		t.Fatalf("archived card missing from all-scope search: %#v", cards)
	}
	retired, ok := cards[2]["list"].(map[string]any)
	if !ok || retired["name"] != "Retired" || retired["closed"] != true {
		t.Fatalf("archived list annotation = %#v", cards[2]["list"])
	}
}

func TestSearchCardsLabelQueryKeepsTheWholeRemainderAsTheName(t *testing.T) {
	c := NewClient(config.TrelloCredentials{})
	c.baseURL = "https://example.test"
	c.http = &http.Client{Transport: roundTripper(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/boards/board-1/labels" {
			t.Fatalf("unexpected request %s before label resolution", r.URL.Path)
		}
		return jsonResponse(r, `[{"id":"label-1","name":"sylvanus"}]`), nil
	})}
	_, _, _, err := c.searchCards("board-1", "label:sylvanus is:archived", 20, "open")
	if err == nil {
		t.Fatal("expected a compound label query to be rejected")
	}
	// The message must quote the value it actually looked for, so a caller can
	// see the filter was swallowed rather than ignored.
	if !strings.Contains(err.Error(), `"sylvanus is:archived"`) || !strings.Contains(err.Error(), "--scope") {
		t.Fatalf("error = %v", err)
	}
}

func TestLabelColorAcceptsShadesAndColorlessAndRejectsOthers(t *testing.T) {
	for _, valid := range []struct{ in, want string }{
		{"green", "green"}, {"GREEN", "green"}, {"sky_subtle", "sky_light"}, {"sky_light", "sky_light"},
		{"black_bold", "black_dark"}, {"black_dark", "black_dark"}, {"lime_normal", "lime"},
	} {
		got, err := labelColor(valid.in)
		if err != nil || got != valid.want {
			t.Fatalf("labelColor(%q) = %#v, %v; want %q", valid.in, got, err, valid.want)
		}
	}
	got, err := labelColor("none")
	if err != nil || got != nil {
		t.Fatalf("colorless label = %#v, %v; want nil", got, err)
	}
	for _, invalid := range []string{"", "chartreuse", "green_vivid", "_green"} {
		if _, err := labelColor(invalid); err == nil {
			t.Fatalf("labelColor(%q) was accepted", invalid)
		}
	}
	err = func() error { _, err := labelColor("chartreuse"); return err }()
	if !strings.Contains(err.Error(), "green") || !strings.Contains(err.Error(), "none") {
		t.Fatalf("color error does not disclose the palette: %v", err)
	}
}

func TestLabelPaletteEnumeratesExactlyWhatLabelColorAccepts(t *testing.T) {
	palette := labelPalette()
	if len(palette) != 31 {
		t.Fatalf("palette has %d colors, want 31 (ten hues, three shades, plus colorless)", len(palette))
	}
	for _, entry := range palette {
		color, _ := entry["color"].(string)
		normalized, err := labelColor(color)
		if err != nil {
			t.Fatalf("palette lists %q but label create rejects it: %v", color, err)
		}
		// An enumerated color must already be in the spelling Trello stores, so
		// what the palette shows is what a label record reports back.
		if color == colorlessLabel {
			if normalized != nil {
				t.Fatalf("colorless entry normalized to %#v", normalized)
			}
			continue
		}
		if normalized != color {
			t.Fatalf("palette lists %q but it normalizes to %#v", color, normalized)
		}
		if alias, ok := entry["also_accepted"].(string); ok {
			aliased, err := labelColor(alias)
			if err != nil || aliased != color {
				t.Fatalf("alias %q = %#v, %v; want %q", alias, aliased, err, color)
			}
		}
	}
}

// snapshotLabelColors is the Color enum from the committed Trello OpenAPI
// snapshot. It is here to be proven insufficient: the capture must contain a
// color this list lacks, or it cannot detect the drift it exists to detect.
var snapshotLabelColors = []string{"yellow", "purple", "blue", "red", "green", "orange", "black", "sky", "pink", "lime"}

func TestLiveCaptureColorsAreAcceptedByTheWriteValidator(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "label-colors.json"))
	if err != nil {
		t.Fatal(err)
	}
	var capture struct {
		Labels []struct {
			Color *string `json:"color"`
			Name  string  `json:"name"`
		} `json:"labels"`
	}
	if err := json.Unmarshal(raw, &capture); err != nil {
		t.Fatal(err)
	}
	if len(capture.Labels) == 0 {
		t.Fatal("capture holds no labels")
	}
	tiers := map[string]bool{}
	beyondSnapshot := false
	for _, label := range capture.Labels {
		if label.Color == nil {
			tiers["colorless"] = true
			// A read reports null; the write path spells it none. If that stopped
			// resolving, a colorless label would be unrecreatable through Omni.
			if got, err := labelColor(colorlessLabel); err != nil || got != nil {
				t.Fatalf("colorless label %q cannot be recreated: %#v, %v", label.Name, got, err)
			}
			continue
		}
		color := *label.Color
		got, err := labelColor(color)
		if err != nil {
			t.Fatalf("Trello returned color %q on label %q, but label create rejects it: %v", color, label.Name, err)
		}
		if got != color {
			t.Fatalf("color %q from a read normalizes to %#v, so a read cannot be fed back to a write verbatim", color, got)
		}
		switch {
		case strings.HasSuffix(color, "_light"):
			tiers["subtle"] = true
		case strings.HasSuffix(color, "_dark"):
			tiers["bold"] = true
		default:
			tiers["unshaded"] = true
		}
		if !contains(snapshotLabelColors, color) {
			beyondSnapshot = true
		}
	}
	for _, tier := range []string{"unshaded", "subtle", "bold", "colorless"} {
		if !tiers[tier] {
			t.Fatalf("capture covers no %s color; a refreshed capture lost a shade tier and this pin would pass by construction (see testdata/README.md)", tier)
		}
	}
	if !beyondSnapshot {
		t.Fatal("capture contains nothing absent from the committed snapshot's Color enum, so it cannot catch a validator built from that snapshot")
	}
}

func TestLabelColorListAnswersWithoutCallingTrello(t *testing.T) {
	stubExecuteClient(t, func(r *http.Request) (*http.Response, error) {
		t.Fatalf("unexpected request %s %s for a fixed palette", r.Method, r.URL.Path)
		return nil, nil
	})
	d, _ := command.Find([]string{"observe", "trello", "label", "color", "list"})
	out := &bytes.Buffer{}
	if err := ExecuteWithFormat(d, nil, config.TrelloCredentials{}, config.TrelloSettings{APIURL: "https://example.test"}, output.JSON, out); err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Colors []map[string]any `json:"colors"`
		Count  int              `json:"count"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Count != 31 || len(payload.Colors) != 31 {
		t.Fatalf("count = %d, colors = %d", payload.Count, len(payload.Colors))
	}
}

func TestArchiveScopeMapsOperatorVocabularyOntoTrelloFilters(t *testing.T) {
	for in, want := range map[string]string{"": "open", "open": "open", "archived": "closed", "ALL": "all"} {
		got, err := archiveScope(in)
		if err != nil || got != want {
			t.Fatalf("archiveScope(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	if _, err := archiveScope("closed"); err == nil {
		t.Fatal("cardScope accepted a provider spelling instead of the operator one")
	}
	if got := scopeName("closed"); got != "archived" {
		t.Fatalf("scopeName(closed) = %q", got)
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
