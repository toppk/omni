// Package trello implements Omni's native Trello service integration.
package trello

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/toppk/omni/internal/command"
	"github.com/toppk/omni/internal/config"
)

const defaultBaseURL = "https://api.trello.com/1"

type Client struct {
	baseURL string
	http    *http.Client
	creds   config.TrelloCredentials
}

func NewClient(creds config.TrelloCredentials) *Client {
	return &Client{baseURL: defaultBaseURL, http: &http.Client{Timeout: 30 * time.Second}, creds: creds}
}

func (c *Client) request(method, endpoint string, payload any, result any) error {
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(encoded)
	}
	u, err := url.Parse(c.baseURL + endpoint)
	if err != nil {
		return err
	}
	q := u.Query()
	q.Set("key", c.creds.APIKey)
	q.Set("token", c.creds.Token)
	u.RawQuery = q.Encode()
	req, err := http.NewRequest(method, u.String(), body)
	if err != nil {
		return err
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("Trello request %s %s: %w", method, endpoint, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Trello request %s %s returned %s: %s", method, endpoint, resp.Status, strings.TrimSpace(string(data)))
	}
	if result != nil && len(data) > 0 {
		if err := json.Unmarshal(data, result); err != nil {
			return fmt.Errorf("decode Trello response: %w", err)
		}
	}
	return nil
}

// Execute maps only reviewed leaf commands to fixed Trello API requests.
// There is intentionally no arbitrary method/path command.
func Execute(d command.Definition, args []string, creds config.TrelloCredentials, settings config.TrelloSettings, out io.Writer) error {
	c := NewClient(creds)
	c.baseURL = strings.TrimRight(settings.APIURL, "/")
	var result any
	switch d.Name() {
	case "observe trello board list":
		if err := exact(args, 0, d.Name()); err != nil {
			return err
		}
		var boards []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}
		if err := c.request(http.MethodGet, "/members/me/boards", nil, &boards); err != nil {
			return err
		}
		result = map[string]any{"boards": boards}
	case "observe trello board overview":
		boardID, err := boardID(args, settings.DefaultBoardID, d.Name())
		if err != nil {
			return err
		}
		var board map[string]any
		var lists []map[string]any
		if err := c.request(http.MethodGet, "/boards/"+url.PathEscape(boardID), nil, &board); err != nil {
			return err
		}
		if err := c.request(http.MethodGet, "/boards/"+url.PathEscape(boardID)+"/lists", nil, &lists); err != nil {
			return err
		}
		result = map[string]any{"board": board, "lists": lists}
	case "observe trello list list":
		boardID, err := boardID(args, settings.DefaultBoardID, d.Name())
		if err != nil {
			return err
		}
		var lists []map[string]any
		if err := c.request(http.MethodGet, "/boards/"+url.PathEscape(boardID)+"/lists", nil, &lists); err != nil {
			return err
		}
		result = map[string]any{"lists": lists}
	case "observe trello card get":
		if err := exact(args, 1, d.Name()); err != nil {
			return err
		}
		var card map[string]any
		if err := c.request(http.MethodGet, "/cards/"+url.PathEscape(args[0]), nil, &card); err != nil {
			return err
		}
		result = map[string]any{"card": card}
	case "create trello card create":
		if len(args) < 2 {
			return fmt.Errorf("%s requires LIST_ID and --name NAME", d.Name())
		}
		fields, err := named(args[1:], "name", "description", "due")
		if err != nil {
			return err
		}
		if fields["name"] == "" {
			return fmt.Errorf("%s requires --name NAME", d.Name())
		}
		payload := map[string]string{"idList": args[0], "name": fields["name"]}
		if fields["description"] != "" {
			payload["desc"] = fields["description"]
		}
		if fields["due"] != "" {
			payload["due"] = fields["due"]
		}
		var card map[string]any
		if err := c.request(http.MethodPost, "/cards", payload, &card); err != nil {
			return err
		}
		result = map[string]any{"card": card}
	case "move trello card move":
		if len(args) < 2 {
			return fmt.Errorf("%s requires CARD_ID LIST_ID", d.Name())
		}
		fields, err := named(args[2:], "position")
		if err != nil {
			return err
		}
		if fields["position"] == "" {
			fields["position"] = "bottom"
		}
		var card map[string]any
		if err := c.request(http.MethodPut, "/cards/"+url.PathEscape(args[0]), map[string]string{"idList": args[1], "pos": fields["position"]}, &card); err != nil {
			return err
		}
		result = map[string]any{"card": card}
	case "archive trello card archive":
		if err := exact(args, 1, d.Name()); err != nil {
			return err
		}
		var card map[string]any
		if err := c.request(http.MethodPut, "/cards/"+url.PathEscape(args[0]), map[string]bool{"closed": true}, &card); err != nil {
			return err
		}
		result = map[string]any{"card": card}
	case "delete trello card delete":
		if err := exact(args, 1, d.Name()); err != nil {
			return err
		}
		if err := c.request(http.MethodDelete, "/cards/"+url.PathEscape(args[0]), nil, nil); err != nil {
			return err
		}
		result = map[string]string{"deleted_card_id": args[0]}
	default:
		return fmt.Errorf("%s is registered but not implemented yet", d.Name())
	}
	return json.NewEncoder(out).Encode(result)
}

func boardID(args []string, fallback, name string) (string, error) {
	if len(args) == 1 {
		return args[0], nil
	}
	if len(args) == 0 && fallback != "" {
		return fallback, nil
	}
	if len(args) > 1 {
		return "", fmt.Errorf("%s expects at most one BOARD_ID", name)
	}
	return "", fmt.Errorf("%s requires BOARD_ID or configured trello.default-board-id", name)
}

func exact(args []string, want int, name string) error {
	if len(args) != want {
		return fmt.Errorf("%s expects %d argument(s)", name, want)
	}
	return nil
}
func named(args []string, allowed ...string) (map[string]string, error) {
	result := map[string]string{}
	valid := map[string]bool{}
	for _, key := range allowed {
		valid[key] = true
	}
	for len(args) > 0 {
		if len(args) < 2 || !strings.HasPrefix(args[0], "--") {
			return nil, fmt.Errorf("expected --NAME VALUE")
		}
		key := strings.TrimPrefix(args[0], "--")
		if !valid[key] {
			return nil, fmt.Errorf("unsupported option --%s", key)
		}
		result[key] = args[1]
		args = args[2:]
	}
	return result, nil
}
