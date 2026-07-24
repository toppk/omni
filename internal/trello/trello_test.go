package trello

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/toppk/omni/internal/config"
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

type roundTripper func(*http.Request) (*http.Response, error)

func (f roundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
