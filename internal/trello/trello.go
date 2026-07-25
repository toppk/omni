// Package trello implements Omni's native Trello service integration.
package trello

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/toppk/omni/internal/command"
	"github.com/toppk/omni/internal/config"
	"github.com/toppk/omni/internal/output"
)

const defaultBaseURL = "https://api.trello.com/1"
const maxAttachmentBytes = 25 << 20

type Client struct {
	baseURL string
	http    *http.Client
	creds   config.TrelloCredentials
}

func NewClient(creds config.TrelloCredentials) *Client {
	return &Client{baseURL: defaultBaseURL, http: &http.Client{Timeout: 30 * time.Second}, creds: creds}
}

var makeClient = NewClient

func (c *Client) request(method, endpoint string, payload any, result any) error {
	return c.requestWithQuery(method, endpoint, nil, payload, result)
}

func (c *Client) requestWithQuery(method, endpoint string, query map[string]string, payload any, result any) error {
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
	for key, value := range query {
		q.Set(key, value)
	}
	q.Set("key", c.creds.APIKey)
	q.Set("token", c.creds.Token)
	u.RawQuery = q.Encode()
	req, err := http.NewRequest(method, u.String(), body)
	if err != nil {
		return fmt.Errorf("prepare Trello request %s %s", method, endpoint)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		// net/http errors can include the full request URL. That URL contains
		// query-string credentials, so never return the underlying error text.
		return fmt.Errorf("Trello request %s %s failed: network error", method, endpoint)
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

func (c *Client) uploadAttachment(cardID, file, name string) (map[string]any, error) {
	if file == "" {
		return nil, fmt.Errorf("create trello attachment upload requires --file FILE")
	}
	if err := c.requireOpenCard(cardID); err != nil {
		return nil, err
	}
	info, err := os.Stat(file)
	if err != nil {
		return nil, fmt.Errorf("read attachment file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("attachment file must be a regular file")
	}
	if info.Size() > maxAttachmentBytes {
		return nil, fmt.Errorf("attachment file exceeds 25 MiB limit")
	}
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	part, err := form.CreateFormFile("file", filepath.Base(file))
	if err != nil {
		return nil, err
	}
	if _, err := io.Copy(part, f); err != nil {
		return nil, err
	}
	if name != "" {
		_ = form.WriteField("name", name)
	}
	if err := form.Close(); err != nil {
		return nil, err
	}
	u, _ := url.Parse(c.baseURL + "/cards/" + url.PathEscape(cardID) + "/attachments")
	q := u.Query()
	q.Set("key", c.creds.APIKey)
	q.Set("token", c.creds.Token)
	u.RawQuery = q.Encode()
	req, err := http.NewRequest(http.MethodPost, u.String(), &body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", form.FormDataContentType())
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Trello attachment upload failed: network error")
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("Trello attachment upload returned %s: %s", resp.Status, strings.TrimSpace(string(data)))
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("decode Trello response: %w", err)
	}
	return result, nil
}

func (c *Client) downloadAttachment(cardID, attachmentID, output string) (map[string]any, error) {
	if output == "" {
		return nil, fmt.Errorf("observe trello attachment download requires --output FILE")
	}
	var attachments []map[string]any
	if err := c.request(http.MethodGet, "/cards/"+url.PathEscape(cardID)+"/attachments", nil, &attachments); err != nil {
		return nil, err
	}
	var attachment map[string]any
	for _, candidate := range attachments {
		if candidate["id"] == attachmentID {
			attachment = candidate
			break
		}
	}
	if attachment == nil || attachment["isUpload"] != true {
		return nil, fmt.Errorf("attachment %s is not a downloadable Trello upload", attachmentID)
	}
	downloadURL, _ := attachment["url"].(string)
	u, err := url.Parse(downloadURL)
	if err != nil || (u.Host != "trello.com" && !strings.HasSuffix(u.Host, ".trello.com")) {
		return nil, fmt.Errorf("attachment %s does not have a Trello-hosted download URL", attachmentID)
	}
	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", fmt.Sprintf("OAuth oauth_consumer_key=%q, oauth_token=%q", c.creds.APIKey, c.creds.Token))
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Trello attachment download failed: network error")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("Trello attachment download returned %s", resp.Status)
	}
	if resp.ContentLength > maxAttachmentBytes {
		return nil, fmt.Errorf("attachment exceeds 25 MiB limit")
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxAttachmentBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxAttachmentBytes {
		return nil, fmt.Errorf("attachment exceeds 25 MiB limit")
	}
	f, err := os.OpenFile(output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return nil, fmt.Errorf("create attachment file: %w", err)
	}
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		return nil, err
	}
	return map[string]any{"attachment_id": attachmentID, "card_id": cardID, "file": output, "bytes": len(data), "mime_type": attachment["mimeType"]}, nil
}

// Execute maps only reviewed leaf commands to fixed Trello API requests.
// There is intentionally no arbitrary method/path command.
func Execute(d command.Definition, args []string, creds config.TrelloCredentials, settings config.TrelloSettings, out io.Writer) error {
	return ExecuteWithFormat(d, args, creds, settings, output.JSON, out)
}

func ExecuteWithFormat(d command.Definition, args []string, creds config.TrelloCredentials, settings config.TrelloSettings, format output.Format, out io.Writer) error {
	c := makeClient(creds)
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
		result = map[string]any{"board": compactBoard(board), "lists": compactLists(lists)}
	case "observe trello list list":
		boardID, err := boardID(args, settings.DefaultBoardID, d.Name())
		if err != nil {
			return err
		}
		var lists []map[string]any
		if err := c.request(http.MethodGet, "/boards/"+url.PathEscape(boardID)+"/lists", nil, &lists); err != nil {
			return err
		}
		result = map[string]any{"lists": compactLists(lists)}
	case "observe trello card list":
		if err := exact(args, 1, d.Name()); err != nil {
			return err
		}
		var cards []map[string]any
		if err := c.request(http.MethodGet, "/lists/"+url.PathEscape(args[0])+"/cards", nil, &cards); err != nil {
			return err
		}
		result = map[string]any{"cards": compactCards(cards)}
	case "observe trello card get":
		if err := exact(args, 1, d.Name()); err != nil {
			return err
		}
		var card map[string]any
		if err := c.request(http.MethodGet, "/cards/"+url.PathEscape(args[0]), nil, &card); err != nil {
			return err
		}
		result = map[string]any{"card": compactCard(card)}
	case "observe trello attachment list":
		if err := exact(args, 1, d.Name()); err != nil {
			return err
		}
		var attachments []map[string]any
		if err := c.request(http.MethodGet, "/cards/"+url.PathEscape(args[0])+"/attachments", nil, &attachments); err != nil {
			return err
		}
		result = map[string]any{"card_id": args[0], "attachments": compactAttachments(attachments)}
	case "observe trello attachment download":
		if len(args) != 4 || args[2] != "--output" {
			return fmt.Errorf("%s requires CARD_ID ATTACHMENT_ID --output FILE", d.Name())
		}
		downloaded, err := c.downloadAttachment(args[0], args[1], args[3])
		if err != nil {
			return err
		}
		result = downloaded
	case "observe trello card get-many":
		if len(args) < 1 || len(args) > 10 {
			return fmt.Errorf("%s requires one to ten CARD_ID values", d.Name())
		}
		cards := make([]map[string]any, 0, len(args))
		for _, id := range args {
			var card map[string]any
			if err := c.request(http.MethodGet, "/cards/"+url.PathEscape(id), nil, &card); err != nil {
				return err
			}
			cards = append(cards, compactCard(card))
		}
		result = map[string]any{"cards": cards}
	case "observe trello card review":
		if err := exact(args, 1, d.Name()); err != nil {
			return err
		}
		var card map[string]any
		if err := c.request(http.MethodGet, "/cards/"+url.PathEscape(args[0]), nil, &card); err != nil {
			return err
		}
		var actions []map[string]any
		if err := c.requestWithQuery(http.MethodGet, "/cards/"+url.PathEscape(args[0])+"/actions", map[string]string{"filter": "commentCard", "limit": "20", "reactions": "true"}, nil, &actions); err != nil {
			return err
		}
		result = map[string]any{"card": compactCard(card), "comments": compactComments(actions)}
	case "observe trello card search":
		if len(args) < 1 {
			return fmt.Errorf("%s requires QUERY", d.Name())
		}
		fields, err := named(args[1:], "board", "limit")
		if err != nil {
			return err
		}
		boardID := fields["board"]
		if boardID == "" {
			boardID = settings.DefaultBoardID
		}
		if boardID == "" {
			return fmt.Errorf("%s requires --board BOARD_ID or configured trello.default-board-id", d.Name())
		}
		limit, err := positiveInt(fields["limit"], 20, "--limit")
		if err != nil {
			return err
		}
		cards, searched, err := c.searchCards(boardID, args[0], limit)
		if err != nil {
			return err
		}
		result = map[string]any{"cards": cards, "query": args[0], "searched": searched, "limit": limit}
	case "observe trello checklist list":
		if err := exact(args, 1, d.Name()); err != nil {
			return err
		}
		var checklists []map[string]any
		if err := c.requestWithQuery(http.MethodGet, "/cards/"+url.PathEscape(args[0])+"/checklists", map[string]string{"checkItems": "all"}, nil, &checklists); err != nil {
			return err
		}
		result = map[string]any{"checklists": compactChecklists(checklists)}
	case "observe trello checklist get":
		if err := exact(args, 1, d.Name()); err != nil {
			return err
		}
		var checklist map[string]any
		if err := c.requestWithQuery(http.MethodGet, "/checklists/"+url.PathEscape(args[0]), map[string]string{"checkItems": "all"}, nil, &checklist); err != nil {
			return err
		}
		result = map[string]any{"checklist": compactChecklists([]map[string]any{checklist})[0]}
	case "observe trello comment list":
		if len(args) < 1 {
			return fmt.Errorf("%s requires CARD_ID", d.Name())
		}
		fields, err := named(args[1:], "limit")
		if err != nil {
			return err
		}
		limit, err := positiveInt(fields["limit"], 20, "--limit")
		if err != nil {
			return err
		}
		var actions []map[string]any
		if err := c.requestWithQuery(http.MethodGet, "/cards/"+url.PathEscape(args[0])+"/actions", map[string]string{"filter": "commentCard", "limit": strconv.Itoa(limit), "reactions": "true"}, nil, &actions); err != nil {
			return err
		}
		result = map[string]any{"comments": compactComments(actions)}
	case "observe trello label list":
		boardID, err := boardID(args, settings.DefaultBoardID, d.Name())
		if err != nil {
			return err
		}
		var labels []map[string]any
		if err := c.request(http.MethodGet, "/boards/"+url.PathEscape(boardID)+"/labels", nil, &labels); err != nil {
			return err
		}
		result = map[string]any{"labels": compactLabels(labels)}
	case "observe trello member list":
		boardID, err := boardID(args, settings.DefaultBoardID, d.Name())
		if err != nil {
			return err
		}
		var members []map[string]any
		if err := c.requestWithQuery(http.MethodGet, "/boards/"+url.PathEscape(boardID)+"/members", map[string]string{"fields": "id,username,fullName,initials"}, nil, &members); err != nil {
			return err
		}
		result = map[string]any{"members": compactMembers(members)}
	case "observe trello member get":
		if err := exact(args, 1, d.Name()); err != nil {
			return err
		}
		var member map[string]any
		if err := c.requestWithQuery(http.MethodGet, "/members/"+url.PathEscape(args[0]), map[string]string{"fields": "id,username,fullName,initials"}, nil, &member); err != nil {
			return err
		}
		result = map[string]any{"member": selectFields(member, "id", "username", "fullName", "initials")}
	case "observe trello list overview":
		if err := exact(args, 1, d.Name()); err != nil {
			return err
		}
		var list map[string]any
		if err := c.request(http.MethodGet, "/lists/"+url.PathEscape(args[0]), nil, &list); err != nil {
			return err
		}
		var cards []map[string]any
		if err := c.request(http.MethodGet, "/lists/"+url.PathEscape(args[0])+"/cards", nil, &cards); err != nil {
			return err
		}
		result = map[string]any{"list": compactList(list), "cards": compactCards(cards)}
	case "observe trello board activity list":
		if len(args) > 3 {
			return fmt.Errorf("%s expects at most BOARD_ID and --limit COUNT", d.Name())
		}
		boardArgs, optionArgs := args, []string(nil)
		if len(args) > 0 && strings.HasPrefix(args[0], "--") {
			boardArgs, optionArgs = nil, args
		} else if len(args) > 1 {
			boardArgs, optionArgs = args[:1], args[1:]
		}
		boardID, err := boardID(boardArgs, settings.DefaultBoardID, d.Name())
		if err != nil {
			return err
		}
		fields, err := named(optionArgs, "limit")
		if err != nil {
			return err
		}
		limit, err := positiveInt(fields["limit"], 20, "--limit")
		if err != nil {
			return err
		}
		var actions []map[string]any
		if err := c.requestWithQuery(http.MethodGet, "/boards/"+url.PathEscape(boardID)+"/actions", map[string]string{"filter": "all", "limit": strconv.Itoa(limit)}, nil, &actions); err != nil {
			return err
		}
		result = map[string]any{"board_id": boardID, "activity": compactBoardActions(actions)}
	case "observe trello card activity list":
		if len(args) < 1 {
			return fmt.Errorf("%s requires CARD_ID", d.Name())
		}
		fields, err := named(args[1:], "limit")
		if err != nil {
			return err
		}
		limit, err := positiveInt(fields["limit"], 20, "--limit")
		if err != nil {
			return err
		}
		var actions []map[string]any
		if err := c.requestWithQuery(http.MethodGet, "/cards/"+url.PathEscape(args[0])+"/actions", map[string]string{"filter": "all", "limit": strconv.Itoa(limit)}, nil, &actions); err != nil {
			return err
		}
		result = map[string]any{"card_id": args[0], "activity": compactCardActions(actions)}
	case "create trello card create":
		if len(args) < 2 {
			return fmt.Errorf("%s requires LIST_ID and --name NAME", d.Name())
		}
		fields, labels, members, err := cardCreateOptions(args[1:])
		if err != nil {
			return err
		}
		if fields["name"] == "" {
			return fmt.Errorf("%s requires --name NAME", d.Name())
		}
		boardID, err := c.openListBoard(args[0])
		if err != nil {
			return err
		}
		if boardID == "" && (len(labels) > 0 || len(members) > 0) {
			return fmt.Errorf("Trello list %s did not include its board ID for label or member validation", args[0])
		}
		if err := c.validateCardAssignments(boardID, labels, members); err != nil {
			return err
		}
		payload := map[string]string{"idList": args[0], "name": fields["name"]}
		if fields["description"] != "" {
			payload["desc"] = fields["description"]
		}
		if fields["due"] != "" {
			payload["due"] = fields["due"]
		}
		if len(labels) > 0 {
			payload["idLabels"] = strings.Join(labels, ",")
		}
		if len(members) > 0 {
			payload["idMembers"] = strings.Join(members, ",")
		}
		var card map[string]any
		if err := c.request(http.MethodPost, "/cards", payload, &card); err != nil {
			return err
		}
		result = map[string]any{"card": compactCard(card)}
	case "create trello attachment upload":
		if len(args) < 3 {
			return fmt.Errorf("%s requires CARD_ID --file FILE", d.Name())
		}
		fields, err := named(args[1:], "file", "name")
		if err != nil {
			return err
		}
		attachment, err := c.uploadAttachment(args[0], fields["file"], fields["name"])
		if err != nil {
			return err
		}
		result = map[string]any{"attachment": compactAttachments([]map[string]any{attachment})[0]}
	case "create trello list create":
		if len(args) < 2 {
			return fmt.Errorf("%s requires BOARD_ID and --name NAME", d.Name())
		}
		fields, err := named(args[1:], "name", "position")
		if err != nil {
			return err
		}
		if fields["name"] == "" {
			return fmt.Errorf("%s requires --name NAME", d.Name())
		}
		if fields["position"] == "" {
			fields["position"] = "bottom"
		}
		var list map[string]any
		if err := c.request(http.MethodPost, "/lists", map[string]string{"idBoard": args[0], "name": fields["name"], "pos": fields["position"]}, &list); err != nil {
			return err
		}
		result = map[string]any{"list": compactList(list)}
	case "create trello comment create":
		if len(args) < 2 {
			return fmt.Errorf("%s requires CARD_ID and --text TEXT", d.Name())
		}
		fields, err := named(args[1:], "text")
		if err != nil {
			return err
		}
		if fields["text"] == "" {
			return fmt.Errorf("%s requires --text TEXT", d.Name())
		}
		if err := c.requireOpenCard(args[0]); err != nil {
			return err
		}
		var comment map[string]any
		if err := c.request(http.MethodPost, "/cards/"+url.PathEscape(args[0])+"/actions/comments", map[string]string{"text": fields["text"]}, &comment); err != nil {
			return err
		}
		result = map[string]any{"comment": compactComment(comment)}
	case "create trello label create":
		if len(args) < 2 {
			return fmt.Errorf("%s requires BOARD_ID, --name NAME, and --color COLOR", d.Name())
		}
		fields, err := named(args[1:], "name", "color")
		if err != nil {
			return err
		}
		if fields["name"] == "" || fields["color"] == "" {
			return fmt.Errorf("%s requires --name NAME and --color COLOR", d.Name())
		}
		var label map[string]any
		if err := c.request(http.MethodPost, "/labels", map[string]string{"idBoard": args[0], "name": fields["name"], "color": fields["color"]}, &label); err != nil {
			return err
		}
		result = map[string]any{"label": compactLabel(label)}
	case "create trello checklist create":
		if len(args) < 2 {
			return fmt.Errorf("%s requires CARD_ID and --name NAME", d.Name())
		}
		fields, err := named(args[1:], "name", "position")
		if err != nil {
			return err
		}
		if fields["name"] == "" {
			return fmt.Errorf("%s requires --name NAME", d.Name())
		}
		if fields["position"] == "" {
			fields["position"] = "bottom"
		}
		if err := c.requireOpenCard(args[0]); err != nil {
			return err
		}
		var checklist map[string]any
		if err := c.request(http.MethodPost, "/cards/"+url.PathEscape(args[0])+"/checklists", map[string]string{"name": fields["name"], "pos": fields["position"]}, &checklist); err != nil {
			return err
		}
		result = map[string]any{"checklist": compactChecklists([]map[string]any{checklist})[0]}
	case "create trello checklist item create":
		if len(args) < 2 {
			return fmt.Errorf("%s requires CHECKLIST_ID and --name NAME", d.Name())
		}
		fields, err := named(args[1:], "name", "position")
		if err != nil {
			return err
		}
		if fields["name"] == "" {
			return fmt.Errorf("%s requires --name NAME", d.Name())
		}
		if fields["position"] == "" {
			fields["position"] = "bottom"
		}
		var item map[string]any
		if err := c.request(http.MethodPost, "/checklists/"+url.PathEscape(args[0])+"/checkItems", map[string]string{"name": fields["name"], "pos": fields["position"]}, &item); err != nil {
			return err
		}
		result = map[string]any{"item": compactChecklistItem(item)}
	case "create trello comment reaction create":
		if len(args) < 2 {
			return fmt.Errorf("%s requires COMMENT_ID and --emoji EMOJI", d.Name())
		}
		fields, err := named(args[1:], "emoji")
		if err != nil {
			return err
		}
		if fields["emoji"] == "" {
			return fmt.Errorf("%s requires --emoji EMOJI", d.Name())
		}
		var reaction map[string]any
		if err := c.request(http.MethodPost, "/actions/"+url.PathEscape(args[0])+"/reactions", reactionPayload(fields["emoji"]), &reaction); err != nil {
			return err
		}
		result = map[string]any{"comment_id": args[0], "reaction": selectFields(reaction, "id", "idMember", "emoji")}
	case "update trello card set":
		if len(args) < 2 {
			return fmt.Errorf("%s requires CARD_ID and at least one update option", d.Name())
		}
		fields, err := named(args[1:], "name", "description", "due")
		if err != nil {
			return err
		}
		payload := map[string]string{}
		for option, field := range map[string]string{"name": "name", "description": "desc", "due": "due"} {
			if value, ok := fields[option]; ok {
				payload[field] = value
			}
		}
		if len(payload) == 0 {
			return fmt.Errorf("%s requires at least one update option", d.Name())
		}
		if err := c.requireOpenCard(args[0]); err != nil {
			return err
		}
		var card map[string]any
		if err := c.request(http.MethodPut, "/cards/"+url.PathEscape(args[0]), payload, &card); err != nil {
			return err
		}
		result = map[string]any{"card": compactCard(card)}
	case "update trello board set":
		boardArgs, optionArgs := args, []string(nil)
		if len(args) > 0 && strings.HasPrefix(args[0], "--") {
			boardArgs, optionArgs = nil, args
		} else if len(args) > 0 {
			boardArgs, optionArgs = args[:1], args[1:]
		}
		boardID, err := boardID(boardArgs, settings.DefaultBoardID, d.Name())
		if err != nil {
			return err
		}
		fields, err := named(optionArgs, "description")
		if err != nil {
			return err
		}
		description, ok := fields["description"]
		if !ok {
			return fmt.Errorf("%s requires --description TEXT", d.Name())
		}
		var board map[string]any
		if err := c.request(http.MethodPut, "/boards/"+url.PathEscape(boardID), map[string]string{"desc": description}, &board); err != nil {
			return err
		}
		result = map[string]any{"board": compactBoard(board)}
	case "update trello card due complete set":
		if len(args) < 3 {
			return fmt.Errorf("%s requires CARD_ID and --state STATE", d.Name())
		}
		fields, err := named(args[1:], "state")
		if err != nil {
			return err
		}
		if fields["state"] != "complete" && fields["state"] != "incomplete" {
			return fmt.Errorf("%s requires --state complete or --state incomplete", d.Name())
		}
		if err := c.requireOpenCard(args[0]); err != nil {
			return err
		}
		var card map[string]any
		if err := c.request(http.MethodPut, "/cards/"+url.PathEscape(args[0]), map[string]bool{"dueComplete": fields["state"] == "complete"}, &card); err != nil {
			return err
		}
		result = map[string]any{"card": compactCard(card)}
	case "update trello card unarchive":
		if err := exact(args, 1, d.Name()); err != nil {
			return err
		}
		var card map[string]any
		if err := c.request(http.MethodPut, "/cards/"+url.PathEscape(args[0]), map[string]bool{"closed": false}, &card); err != nil {
			return err
		}
		result = map[string]any{"card": compactCard(card)}
	case "update trello list name set":
		if len(args) < 2 {
			return fmt.Errorf("%s requires LIST_ID and --name NAME", d.Name())
		}
		fields, err := named(args[1:], "name")
		if err != nil {
			return err
		}
		if fields["name"] == "" {
			return fmt.Errorf("%s requires --name NAME", d.Name())
		}
		if err := c.requireOpenList(args[0]); err != nil {
			return err
		}
		var list map[string]any
		if err := c.request(http.MethodPut, "/lists/"+url.PathEscape(args[0]), map[string]string{"name": fields["name"]}, &list); err != nil {
			return err
		}
		result = map[string]any{"list": compactList(list)}
	case "update trello list unarchive":
		if err := exact(args, 1, d.Name()); err != nil {
			return err
		}
		var list map[string]any
		if err := c.request(http.MethodPut, "/lists/"+url.PathEscape(args[0]), map[string]bool{"closed": false}, &list); err != nil {
			return err
		}
		result = map[string]any{"list": compactList(list)}
	case "move trello list move":
		if len(args) < 2 {
			return fmt.Errorf("%s requires LIST_ID and --position POSITION", d.Name())
		}
		fields, err := named(args[1:], "position")
		if err != nil {
			return err
		}
		if fields["position"] == "" {
			return fmt.Errorf("%s requires --position POSITION", d.Name())
		}
		if err := c.requireOpenList(args[0]); err != nil {
			return err
		}
		var list map[string]any
		if err := c.request(http.MethodPut, "/lists/"+url.PathEscape(args[0]), map[string]string{"pos": fields["position"]}, &list); err != nil {
			return err
		}
		result = map[string]any{"list": compactList(list)}
	case "update trello checklist item set":
		if len(args) < 3 {
			return fmt.Errorf("%s requires CHECKLIST_ID ITEM_ID and --state STATE", d.Name())
		}
		fields, err := named(args[2:], "state")
		if err != nil {
			return err
		}
		if fields["state"] != "complete" && fields["state"] != "incomplete" {
			return fmt.Errorf("%s requires --state complete or --state incomplete", d.Name())
		}
		var checklist map[string]any
		if err := c.request(http.MethodGet, "/checklists/"+url.PathEscape(args[0]), nil, &checklist); err != nil {
			return err
		}
		cardID, _ := checklist["idCard"].(string)
		if cardID == "" {
			return fmt.Errorf("Trello returned a checklist without a card ID")
		}
		if err := c.requireOpenCard(cardID); err != nil {
			return err
		}
		var item map[string]any
		if err := c.request(http.MethodPut, "/cards/"+url.PathEscape(cardID)+"/checkItem/"+url.PathEscape(args[1]), map[string]string{"state": fields["state"]}, &item); err != nil {
			return err
		}
		result = map[string]any{"item": compactChecklistItem(item)}
	case "update trello checklist item name set":
		if len(args) < 3 {
			return fmt.Errorf("%s requires CHECKLIST_ID ITEM_ID and --name NAME", d.Name())
		}
		fields, err := named(args[2:], "name")
		if err != nil {
			return err
		}
		if fields["name"] == "" {
			return fmt.Errorf("%s requires --name NAME", d.Name())
		}
		var checklist map[string]any
		if err := c.request(http.MethodGet, "/checklists/"+url.PathEscape(args[0]), nil, &checklist); err != nil {
			return err
		}
		cardID, _ := checklist["idCard"].(string)
		if cardID == "" {
			return fmt.Errorf("Trello returned a checklist without a card ID")
		}
		if err := c.requireOpenCard(cardID); err != nil {
			return err
		}
		var item map[string]any
		if err := c.request(http.MethodPut, "/cards/"+url.PathEscape(cardID)+"/checkItem/"+url.PathEscape(args[1]), map[string]string{"name": fields["name"]}, &item); err != nil {
			return err
		}
		result = map[string]any{"item": compactChecklistItem(item)}
	case "update trello card label add", "update trello card label remove", "update trello card member add", "update trello card member remove":
		if err := exact(args, 2, d.Name()); err != nil {
			return err
		}
		if err := c.requireOpenCard(args[0]); err != nil {
			return err
		}
		kind, action := "label", "add"
		if strings.Contains(d.Name(), "member") {
			kind = "member"
		}
		if strings.HasSuffix(d.Name(), "remove") {
			action = "remove"
		}
		resource := "Labels"
		if kind == "member" {
			resource = "Members"
		}
		endpoint := "/cards/" + url.PathEscape(args[0]) + "/id" + resource
		if action == "add" {
			if err := c.request(http.MethodPost, endpoint, map[string]string{"value": args[1]}, nil); err != nil {
				return err
			}
		} else if err := c.request(http.MethodDelete, endpoint+"/"+url.PathEscape(args[1]), nil, nil); err != nil {
			return err
		}
		verb := "added"
		if action == "remove" {
			verb = "removed"
		}
		result = map[string]string{verb + "_" + kind + "_id": args[1], "card_id": args[0]}
	case "update trello card member add-many":
		if len(args) < 2 {
			return fmt.Errorf("%s requires CARD_ID and one or more MEMBER_ID values", d.Name())
		}
		if err := c.requireOpenCard(args[0]); err != nil {
			return err
		}
		for _, memberID := range args[1:] {
			if err := c.request(http.MethodPost, "/cards/"+url.PathEscape(args[0])+"/idMembers", map[string]string{"value": memberID}, nil); err != nil {
				return err
			}
		}
		result = map[string]any{"card_id": args[0], "added_member_ids": args[1:]}
	case "update trello comment set":
		if len(args) < 2 {
			return fmt.Errorf("%s requires COMMENT_ID and --text TEXT", d.Name())
		}
		fields, err := named(args[1:], "text")
		if err != nil {
			return err
		}
		if fields["text"] == "" {
			return fmt.Errorf("%s requires --text TEXT", d.Name())
		}
		var comment map[string]any
		if err := c.request(http.MethodPut, "/actions/"+url.PathEscape(args[0]), map[string]string{"text": fields["text"]}, &comment); err != nil {
			return err
		}
		result = map[string]any{"comment": compactComment(comment)}
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
		card, err := c.moveCard(args[0], args[1], fields["position"])
		if err != nil {
			return err
		}
		result = map[string]any{"card": compactCard(card)}
	case "archive trello card archive":
		if err := exact(args, 1, d.Name()); err != nil {
			return err
		}
		var card map[string]any
		if err := c.request(http.MethodPut, "/cards/"+url.PathEscape(args[0]), map[string]bool{"closed": true}, &card); err != nil {
			return err
		}
		result = map[string]any{"card": compactCard(card)}
	case "archive trello list archive":
		if err := exact(args, 1, d.Name()); err != nil {
			return err
		}
		var list map[string]any
		if err := c.request(http.MethodPut, "/lists/"+url.PathEscape(args[0]), map[string]bool{"closed": true}, &list); err != nil {
			return err
		}
		result = map[string]any{"list": compactList(list)}
	case "archive trello list cards archive":
		if err := exact(args, 1, d.Name()); err != nil {
			return err
		}
		archivedIDs, err := c.archiveListCards(args[0])
		if err != nil {
			return err
		}
		result = map[string]any{"list_id": args[0], "archived_cards": len(archivedIDs), "archived_card_ids": archivedIDs}
	case "delete trello checklist item delete":
		if err := exact(args, 2, d.Name()); err != nil {
			return err
		}
		if err := c.request(http.MethodDelete, "/checklists/"+url.PathEscape(args[0])+"/checkItems/"+url.PathEscape(args[1]), nil, nil); err != nil {
			return err
		}
		result = map[string]string{"deleted_checklist_item_id": args[1]}
	case "delete trello attachment delete":
		if err := exact(args, 2, d.Name()); err != nil {
			return err
		}
		if err := c.requireOpenCard(args[0]); err != nil {
			return err
		}
		if err := c.request(http.MethodDelete, "/cards/"+url.PathEscape(args[0])+"/attachments/"+url.PathEscape(args[1]), nil, nil); err != nil {
			return err
		}
		result = map[string]string{"card_id": args[0], "deleted_attachment_id": args[1]}
	case "delete trello checklist delete":
		if err := exact(args, 1, d.Name()); err != nil {
			return err
		}
		if err := c.request(http.MethodDelete, "/checklists/"+url.PathEscape(args[0]), nil, nil); err != nil {
			return err
		}
		result = map[string]string{"deleted_checklist_id": args[0]}
	case "delete trello comment reaction delete":
		if err := exact(args, 2, d.Name()); err != nil {
			return err
		}
		if err := c.request(http.MethodDelete, "/actions/"+url.PathEscape(args[0])+"/reactions/"+url.PathEscape(args[1]), nil, nil); err != nil {
			return err
		}
		result = map[string]string{"deleted_reaction_id": args[1], "comment_id": args[0]}
	case "delete trello comment delete":
		if err := exact(args, 1, d.Name()); err != nil {
			return err
		}
		if err := c.request(http.MethodDelete, "/actions/"+url.PathEscape(args[0]), nil, nil); err != nil {
			return err
		}
		result = map[string]string{"deleted_comment_id": args[0]}
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
	return output.Encode(out, format, result)
}

// moveCard rejects archived cards before sending Trello a mutation. They must
// be explicitly unarchived before their workflow placement can change.
func (c *Client) moveCard(cardID, listID, position string) (map[string]any, error) {
	if err := c.requireOpenCard(cardID); err != nil {
		return nil, err
	}
	if err := c.requireOpenList(listID); err != nil {
		return nil, err
	}
	var card map[string]any
	if err := c.request(http.MethodPut, "/cards/"+url.PathEscape(cardID), map[string]string{"idList": listID, "pos": position}, &card); err != nil {
		return nil, err
	}
	return card, nil
}

func (c *Client) archiveListCards(listID string) ([]string, error) {
	var cards []map[string]any
	if err := c.request(http.MethodGet, "/lists/"+url.PathEscape(listID)+"/cards", nil, &cards); err != nil {
		return nil, err
	}
	archivedIDs := make([]string, 0, len(cards))
	for _, card := range cards {
		id, _ := card["id"].(string)
		if id == "" {
			return nil, fmt.Errorf("Trello returned a card without an ID")
		}
		if err := c.request(http.MethodPut, "/cards/"+url.PathEscape(id), map[string]bool{"closed": true}, nil); err != nil {
			return nil, err
		}
		archivedIDs = append(archivedIDs, id)
	}
	return archivedIDs, nil
}

func (c *Client) requireOpenCard(cardID string) error {
	var current map[string]any
	if err := c.request(http.MethodGet, "/cards/"+url.PathEscape(cardID), nil, &current); err != nil {
		return err
	}
	if closed, _ := current["closed"].(bool); closed {
		return fmt.Errorf("cannot change archived Trello card %s; unarchive it first", cardID)
	}
	return nil
}

func (c *Client) requireOpenList(listID string) error {
	_, err := c.openListBoard(listID)
	return err
}

func (c *Client) openListBoard(listID string) (string, error) {
	var list map[string]any
	if err := c.request(http.MethodGet, "/lists/"+url.PathEscape(listID), nil, &list); err != nil {
		return "", err
	}
	if closed, _ := list["closed"].(bool); closed {
		return "", fmt.Errorf("cannot change archived Trello list %s; unarchive it first", listID)
	}
	boardID, _ := list["idBoard"].(string)
	return boardID, nil
}

func (c *Client) validateCardAssignments(boardID string, labels, members []string) error {
	if len(labels) > 0 {
		var records []map[string]any
		if err := c.request(http.MethodGet, "/boards/"+url.PathEscape(boardID)+"/labels", nil, &records); err != nil {
			return err
		}
		known := map[string]bool{}
		for _, record := range records {
			if id, _ := record["id"].(string); id != "" {
				known[id] = true
			}
		}
		for _, id := range labels {
			if !known[id] {
				return fmt.Errorf("Trello label %s is not on board %s", id, boardID)
			}
		}
	}
	if len(members) > 0 {
		var records []map[string]any
		if err := c.request(http.MethodGet, "/boards/"+url.PathEscape(boardID)+"/members", nil, &records); err != nil {
			return err
		}
		known := map[string]bool{}
		for _, record := range records {
			if id, _ := record["id"].(string); id != "" {
				known[id] = true
			}
		}
		for _, id := range members {
			if !known[id] {
				return fmt.Errorf("Trello member %s is not on board %s", id, boardID)
			}
		}
	}
	return nil
}

func compactLists(lists []map[string]any) []map[string]any {
	compact := make([]map[string]any, len(lists))
	for i, list := range lists {
		compact[i] = selectFields(list, "id", "name")
	}
	return compact
}

func compactList(list map[string]any) map[string]any {
	return selectFields(list, "id", "name", "pos", "closed")
}

func compactCards(cards []map[string]any) []map[string]any {
	compact := make([]map[string]any, len(cards))
	for i, card := range cards {
		compact[i] = compactCard(card)
	}
	return compact
}

func compactBoard(board map[string]any) map[string]any {
	return selectFields(board, "id", "name", "desc", "shortUrl")
}

func compactCard(card map[string]any) map[string]any {
	compact := selectFields(card,
		"id", "name", "idList", "desc", "due", "dateLastActivity",
		"labels", "idMembers", "closed", "pos", "shortUrl", "idChecklists",
	)
	if badges, ok := card["badges"].(map[string]any); ok {
		compact["badges"] = selectFields(badges,
			"attachments", "checkItems", "checkItemsChecked", "comments",
			"description", "due", "dueComplete", "start", "subscribed", "votes",
		)
	}
	return compact
}

func compactChecklists(checklists []map[string]any) []map[string]any {
	compact := make([]map[string]any, len(checklists))
	for i, checklist := range checklists {
		entry := selectFields(checklist, "id", "idCard", "name", "pos")
		if items, ok := checklist["checkItems"].([]any); ok {
			compactItems := make([]map[string]any, 0, len(items))
			for _, item := range items {
				if itemMap, ok := item.(map[string]any); ok {
					compactItems = append(compactItems, compactChecklistItem(itemMap))
				}
			}
			entry["items"] = compactItems
		}
		compact[i] = entry
	}
	return compact
}

func compactChecklistItem(item map[string]any) map[string]any {
	return selectFields(item, "id", "idChecklist", "name", "state", "pos")
}

func compactLabels(labels []map[string]any) []map[string]any {
	compact := make([]map[string]any, len(labels))
	for i, label := range labels {
		compact[i] = selectFields(label, "id", "name", "color")
	}
	return compact
}

func compactLabel(label map[string]any) map[string]any {
	return selectFields(label, "id", "idBoard", "name", "color")
}

func compactMembers(members []map[string]any) []map[string]any {
	compact := make([]map[string]any, len(members))
	for i, member := range members {
		compact[i] = selectFields(member, "id", "username", "fullName", "initials")
	}
	return compact
}

func compactComments(actions []map[string]any) []map[string]any {
	compact := make([]map[string]any, len(actions))
	for i, action := range actions {
		compact[i] = compactComment(action)
	}
	return compact
}

func compactComment(comment map[string]any) map[string]any {
	compact := selectFields(comment, "id", "date", "idMemberCreator")
	if data, ok := comment["data"].(map[string]any); ok {
		if text, ok := data["text"]; ok {
			compact["text"] = text
		}
	}
	if text, ok := comment["text"]; ok {
		compact["text"] = text
	}
	if reactions, ok := comment["reactions"].([]any); ok {
		compact["reactions"] = compactReactions(reactions)
	}
	return compact
}

func compactAttachments(attachments []map[string]any) []map[string]any {
	compact := make([]map[string]any, len(attachments))
	for i, attachment := range attachments {
		compact[i] = selectFields(attachment, "id", "name", "url", "mimeType", "bytes", "date", "isUpload")
	}
	return compact
}

func compactReactions(reactions []any) []map[string]any {
	compact := make([]map[string]any, 0, len(reactions))
	for _, reaction := range reactions {
		if record, ok := reaction.(map[string]any); ok {
			compact = append(compact, selectFields(record, "id", "idMember", "emoji"))
		}
	}
	return compact
}

func compactBoardActions(actions []map[string]any) []map[string]any {
	compact := make([]map[string]any, len(actions))
	for i, action := range actions {
		entry := selectFields(action, "id", "type", "date", "idMemberCreator")
		if data, ok := action["data"].(map[string]any); ok {
			compactData := selectFields(data, "text", "old")
			if card, ok := data["card"].(map[string]any); ok {
				compactData["card"] = selectFields(card, "id", "name")
			}
			if list, ok := data["list"].(map[string]any); ok {
				compactData["list"] = selectFields(list, "id", "name")
			}
			entry["data"] = compactData
		}
		compact[i] = entry
	}
	return compact
}

func compactCardActions(actions []map[string]any) []map[string]any {
	compact := make([]map[string]any, len(actions))
	for i, action := range actions {
		entry := selectFields(action, "id", "type", "date", "idMemberCreator")
		if data, ok := action["data"].(map[string]any); ok {
			compactData := selectFields(data, "text", "old")
			if list, ok := data["list"].(map[string]any); ok {
				compactData["list"] = selectFields(list, "id", "name")
			}
			entry["data"] = compactData
		}
		compact[i] = entry
	}
	return compact
}

func (c *Client) searchCards(boardID, query string, limit int) ([]map[string]any, int, error) {
	needle := strings.ToLower(query)
	labelID := ""
	if strings.HasPrefix(needle, "label:") {
		labelName := strings.TrimSpace(strings.TrimPrefix(needle, "label:"))
		var labels []map[string]any
		if err := c.request(http.MethodGet, "/boards/"+url.PathEscape(boardID)+"/labels", nil, &labels); err != nil {
			return nil, 0, err
		}
		for _, label := range labels {
			if name, _ := label["name"].(string); strings.EqualFold(name, labelName) {
				labelID, _ = label["id"].(string)
				break
			}
		}
		if labelID == "" {
			return []map[string]any{}, 0, nil
		}
		needle = ""
	}
	var lists []map[string]any
	if err := c.request(http.MethodGet, "/boards/"+url.PathEscape(boardID)+"/lists", nil, &lists); err != nil {
		return nil, 0, err
	}
	results := make([]map[string]any, 0, limit)
	searched := 0
	for _, list := range lists {
		if len(results) >= limit {
			break
		}
		listID, _ := list["id"].(string)
		var cards []map[string]any
		if err := c.request(http.MethodGet, "/lists/"+url.PathEscape(listID)+"/cards", nil, &cards); err != nil {
			return nil, searched, err
		}
		searched += len(cards)
		for _, card := range cards {
			if len(results) >= limit {
				break
			}
			name, _ := card["name"].(string)
			description, _ := card["desc"].(string)
			if labelID != "" && !hasLabel(card, labelID) {
				continue
			}
			if needle != "" && !strings.Contains(strings.ToLower(name+" "+description), needle) {
				continue
			}
			entry := compactCard(card)
			entry["list"] = compactList(list)
			results = append(results, entry)
		}
	}
	return results, searched, nil
}

func hasLabel(card map[string]any, labelID string) bool {
	labels, _ := card["labels"].([]any)
	for _, value := range labels {
		if label, ok := value.(map[string]any); ok {
			if id, _ := label["id"].(string); id == labelID {
				return true
			}
		}
	}
	return false
}

func positiveInt(value string, fallback int, option string) (int, error) {
	if value == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(value)
	if err != nil || n < 1 {
		return 0, fmt.Errorf("%s must be a positive integer", option)
	}
	return n, nil
}

func reactionPayload(emoji string) map[string]string {
	known := map[string][2]string{
		"👍": {"+1", "1f44d"}, "👎": {"-1", "1f44e"}, "❤️": {"heart", "2764-fe0f"},
		"😄": {"smile", "1f604"}, "😮": {"open_mouth", "1f62e"}, "😕": {"confused", "1f615"}, "🎉": {"tada", "1f389"},
	}
	if value, ok := known[emoji]; ok {
		return map[string]string{"shortName": value[0], "unified": value[1], "native": emoji}
	}
	return map[string]string{"shortName": emoji, "native": emoji}
}

func selectFields(record map[string]any, names ...string) map[string]any {
	selected := make(map[string]any, len(names))
	for _, name := range names {
		if value, ok := record[name]; ok {
			selected[name] = value
		}
	}
	return selected
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

func cardCreateOptions(args []string) (map[string]string, []string, []string, error) {
	fields := map[string]string{}
	var labels, members []string
	for len(args) > 0 {
		if len(args) < 2 || !strings.HasPrefix(args[0], "--") {
			return nil, nil, nil, fmt.Errorf("expected --NAME VALUE")
		}
		key, value := strings.TrimPrefix(args[0], "--"), args[1]
		switch key {
		case "name", "description", "due":
			if _, exists := fields[key]; exists {
				return nil, nil, nil, fmt.Errorf("--%s specified more than once", key)
			}
			fields[key] = value
		case "label":
			labels = append(labels, value)
		case "member":
			members = append(members, value)
		default:
			return nil, nil, nil, fmt.Errorf("unsupported option --%s", key)
		}
		args = args[2:]
	}
	return fields, labels, members, nil
}
