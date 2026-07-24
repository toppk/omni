package tailscale

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/toppk/omni/internal/command"
	"github.com/toppk/omni/internal/config"
)

func definition(t *testing.T, tokens ...string) command.Definition {
	t.Helper()
	d, err := command.Find(tokens)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestDeviceListUsesScopedEndpointAndCompactOutput(t *testing.T) {
	c := testClient(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/tailnet/-/devices" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		user, pass, ok := r.BasicAuth()
		if !ok || user != "tskey-api-secret" || pass != "" {
			t.Fatal("missing expected API basic auth")
		}
		return response(`{"devices":[{"id":"dev1","name":"workstation","hostname":"workstation.tailnet.ts.net","addresses":["100.64.0.1"],"tags":["tag:dev"],"unwanted":"large"}]}`, nil), nil
	})
	var out bytes.Buffer
	err := c.execute(definition(t, "observe", "tailscale", "device", "list"), nil, &out)
	if err != nil {
		t.Fatal(err)
	}
	text := out.String()
	if !strings.Contains(text, `"hostname":"workstation.tailnet.ts.net"`) || strings.Contains(text, "unwanted") || strings.Contains(text, "addresses") {
		t.Fatalf("unexpected compact output: %s", text)
	}
}

func TestDeviceListDetailsAddsSelectionContext(t *testing.T) {
	c := testClient(t, func(*http.Request) (*http.Response, error) {
		return response(`{"devices":[{"id":"dev1","hostname":"host","os":"linux","lastSeen":"now","addresses":["100.64.0.1"],"clientVersion":"1.2.3","unwanted":"large"}]}`, nil), nil
	})
	var out bytes.Buffer
	if err := c.execute(definition(t, "observe", "tailscale", "device", "list"), []string{"--details"}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "addresses") || !strings.Contains(out.String(), "clientVersion") || strings.Contains(out.String(), "unwanted") {
		t.Fatalf("details output: %s", out.String())
	}
}

func TestDeviceListFiltersByTagWithoutChangingAPIRequest(t *testing.T) {
	c := testClient(t, func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodGet || r.URL.Path != "/tailnet/-/devices" {
			t.Fatalf("%s %s", r.Method, r.URL.Path)
		}
		return response(`{"devices":[{"id":"runner","hostname":"runner","tags":["tag:github"]},{"id":"server","hostname":"server","tags":["tag:server"]}]}`, nil), nil
	})
	var out bytes.Buffer
	if err := c.execute(definition(t, "observe", "tailscale", "device", "list"), []string{"--tag", "tag:github", "--details"}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "runner") || strings.Contains(out.String(), "server") {
		t.Fatalf("filtered output: %s", out.String())
	}
}

func TestSetTagsIsAuthorizeCommandAndReplacesTagList(t *testing.T) {
	c := testClient(t, func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost || r.URL.Path != "/device/dev1/tags" {
			t.Fatalf("%s %s", r.Method, r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != `{"tags":["tag:prod","tag:database"]}` {
			t.Fatalf("body = %s", body)
		}
		return response("", nil), nil
	})
	var out bytes.Buffer
	err := c.execute(definition(t, "authorize", "tailscale", "device", "tag", "set"), []string{"dev1", "tag:prod", "tag:database"}, &out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"tag:prod"`) {
		t.Fatalf("output = %s", out.String())
	}
}

func TestOAuthClientCredentialsMintAndUseAccessToken(t *testing.T) {
	calls := 0
	cachePath := filepath.Join(t.TempDir(), "ephemeral", "credentials.toml")
	c := NewClientWithCache(config.TailscaleCredentials{ClientSecret: "client-secret"}, config.TailscaleSettings{APIURL: "https://example.test", Tailnet: "-", ClientID: "client-id"}, cachePath)
	c.http = &http.Client{Transport: roundTripper(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Path == "/oauth/token" {
			body, _ := io.ReadAll(r.Body)
			if r.Method != http.MethodPost || string(body) != "client_id=client-id&client_secret=client-secret" {
				t.Fatalf("token request = %s %s", r.Method, body)
			}
			return response(`{"access_token":"issued-token","expires_in":3600}`, nil), nil
		}
		if r.URL.Path != "/tailnet/-/devices" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		user, _, ok := r.BasicAuth()
		if !ok || user != "issued-token" {
			t.Fatal("issued token was not used")
		}
		return response(`{"devices":[]}`, nil), nil
	})}
	var out bytes.Buffer
	if err := c.execute(definition(t, "observe", "tailscale", "device", "list"), nil, &out); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want token + API", calls)
	}
	if cached, err := config.LoadEphemeralTailscaleToken(cachePath, time.Now()); err != nil || cached != "issued-token" {
		t.Fatalf("cached=%q err=%v", cached, err)
	}
	// A new CLI invocation gets a fresh client but must reuse the persisted
	// short-lived token rather than requesting another audited token.
	reused := NewClientWithCache(config.TailscaleCredentials{ClientSecret: "client-secret"}, config.TailscaleSettings{APIURL: "https://example.test", Tailnet: "-", ClientID: "client-id"}, cachePath)
	reused.http = &http.Client{Transport: roundTripper(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/oauth/token" {
			t.Fatal("cached token should avoid OAuth exchange")
		}
		user, _, ok := r.BasicAuth()
		if !ok || user != "issued-token" {
			t.Fatal("cached token was not used")
		}
		return response(`{"devices":[]}`, nil), nil
	})}
	if err := reused.execute(definition(t, "observe", "tailscale", "device", "list"), nil, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
}

func TestACLGetWritesNewPrivateFile(t *testing.T) {
	c := testClient(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/tailnet/example.com/acl" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		return response("// policy\n{}\n", http.Header{"Etag": []string{`"version-1"`}}), nil
	})
	path := filepath.Join(t.TempDir(), "acl.hujson")
	var out bytes.Buffer
	c.tailnet = "example.com"
	err := c.execute(definition(t, "observe", "tailscale", "acl", "get"), []string{"--output", path}, &out)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil || string(b) != "// policy\n{}\n" {
		t.Fatalf("policy = %q, err = %v", b, err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("mode = %v, err = %v", info.Mode(), err)
	}
	if !strings.Contains(out.String(), "version-1") {
		t.Fatalf("output = %s", out.String())
	}
}

func TestACLValidatePostsWithoutApplying(t *testing.T) {
	acl := filepath.Join(t.TempDir(), "acl.hujson")
	if err := os.WriteFile(acl, []byte("{\"acls\":[]}"), 0600); err != nil {
		t.Fatal(err)
	}
	c := testClient(t, func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost || r.URL.Path != "/tailnet/-/acl/validate" {
			t.Fatalf("%s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Content-Type"); got != "application/hujson" {
			t.Fatalf("content type = %q", got)
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != `{"acls":[]}` {
			t.Fatalf("body = %s", body)
		}
		return response(`{"errors":[],"warnings":[]}`, nil), nil
	})
	var out bytes.Buffer
	if err := c.execute(definition(t, "observe", "tailscale", "acl", "validate"), []string{acl}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"status":"valid"`) || !strings.Contains(out.String(), acl) {
		t.Fatalf("validation output = %s", out.String())
	}
}

func TestACLValidateFailsWithStableInvalidStatus(t *testing.T) {
	acl := filepath.Join(t.TempDir(), "invalid.hujson")
	if err := os.WriteFile(acl, []byte("{bad"), 0600); err != nil {
		t.Fatal(err)
	}
	c := testClient(t, func(*http.Request) (*http.Response, error) {
		return response(`{"message":"line 1: unexpected EOF"}`, nil), nil
	})
	var out bytes.Buffer
	err := c.execute(definition(t, "observe", "tailscale", "acl", "validate"), []string{acl}, &out)
	if err == nil || !strings.Contains(err.Error(), "validation failed") {
		t.Fatalf("error = %v", err)
	}
	if !strings.Contains(out.String(), `"status":"invalid"`) || !strings.Contains(out.String(), "unexpected EOF") {
		t.Fatalf("validation output = %s", out.String())
	}
}

func TestACLPreviewUsesFixedPreviewParameters(t *testing.T) {
	c := testClient(t, func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodGet && r.Method != http.MethodPost {
			t.Fatalf("method = %s", r.Method)
		}
		if r.Method == http.MethodGet {
			if r.URL.Path != "/tailnet/-/acl" {
				t.Fatalf("path = %s", r.URL.Path)
			}
			return response("{}", nil), nil
		}
		if r.URL.Path != "/tailnet/-/acl/preview" || r.URL.Query().Get("type") != "user" || r.URL.Query().Get("previewFor") != "tag:github" {
			t.Fatalf("preview = %s?%s", r.URL.Path, r.URL.RawQuery)
		}
		return response(`{"matches":[]}`, nil), nil
	})
	var out bytes.Buffer
	if err := c.execute(definition(t, "observe", "tailscale", "acl", "preview"), []string{"--for", "tag:github"}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "tag:github") || !strings.Contains(out.String(), "matches") || strings.Contains(out.String(), "acl_file") {
		t.Fatalf("preview output = %s", out.String())
	}
}

func TestACLSetValidatesBacksUpAndUsesETag(t *testing.T) {
	dir := t.TempDir()
	candidate := filepath.Join(dir, "candidate.hujson")
	backup := filepath.Join(dir, "before.hujson")
	if err := os.WriteFile(candidate, []byte(`{"acls":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	calls := 0
	c := testClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		switch calls {
		case 1:
			if r.Method != http.MethodPost || r.URL.Path != "/tailnet/-/acl/validate" {
				t.Fatalf("validation = %s %s", r.Method, r.URL.Path)
			}
			return response(`{}`, nil), nil
		case 2:
			if r.Method != http.MethodGet || r.URL.Path != "/tailnet/-/acl" {
				t.Fatalf("snapshot = %s %s", r.Method, r.URL.Path)
			}
			return response("// previous ACL\n{}", http.Header{"Etag": []string{`"current"`}}), nil
		case 3:
			if r.Method != http.MethodPost || r.URL.Path != "/tailnet/-/acl" || r.Header.Get("If-Match") != `"current"` {
				t.Fatalf("set = %s %s If-Match=%q", r.Method, r.URL.Path, r.Header.Get("If-Match"))
			}
			return response(`{}`, nil), nil
		default:
			t.Fatalf("unexpected request %d", calls)
			return nil, nil
		}
	})
	var out bytes.Buffer
	if err := c.execute(definition(t, "administer", "tailscale", "acl", "set"), []string{candidate, "--backup", backup}, &out); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(backup)
	if err != nil || string(data) != "// previous ACL\n{}" {
		t.Fatalf("backup = %q err=%v", data, err)
	}
	info, err := os.Stat(backup)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("backup mode=%v err=%v", info.Mode(), err)
	}
	if !strings.Contains(out.String(), "backup_file") || !strings.Contains(out.String(), "current") {
		t.Fatalf("set output = %s", out.String())
	}
}

func TestACLSetRejectsInvalidCandidateBeforeBackup(t *testing.T) {
	dir := t.TempDir()
	candidate := filepath.Join(dir, "invalid.hujson")
	backup := filepath.Join(dir, "before.hujson")
	if err := os.WriteFile(candidate, []byte(`{bad`), 0600); err != nil {
		t.Fatal(err)
	}
	c := testClient(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/tailnet/-/acl/validate" {
			t.Fatalf("unexpected request: %s", r.URL.Path)
		}
		return response(`{"message":"unexpected EOF"}`, nil), nil
	})
	err := c.execute(definition(t, "administer", "tailscale", "acl", "set"), []string{candidate, "--backup", backup}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "validation failed") {
		t.Fatalf("error = %v", err)
	}
	if _, err := os.Stat(backup); !os.IsNotExist(err) {
		t.Fatalf("backup was created after failed validation: %v", err)
	}
}

func TestACLSetAcceptsBackupFlagBeforeCandidate(t *testing.T) {
	acl, backup, err := setACLOptions([]string{"--backup", "before.hujson", "candidate.hujson"})
	if err != nil || acl != "candidate.hujson" || backup != "before.hujson" {
		t.Fatalf("acl=%q backup=%q err=%v", acl, backup, err)
	}
}

func TestACLSetBackupCollisionExplainsRecovery(t *testing.T) {
	dir := t.TempDir()
	candidate := filepath.Join(dir, "candidate.hujson")
	backup := filepath.Join(dir, "before.hujson")
	if err := os.WriteFile(candidate, []byte(`{"acls":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(backup, []byte("existing"), 0600); err != nil {
		t.Fatal(err)
	}
	calls := 0
	c := testClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return response(`{}`, nil), nil
		}
		if calls == 2 {
			return response(`{}`, http.Header{"Etag": []string{`"current"`}}), nil
		}
		t.Fatalf("replacement must not run after backup collision")
		return nil, nil
	})
	err := c.execute(definition(t, "administer", "tailscale", "acl", "set"), []string{candidate, "--backup", backup}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "already exists") || !strings.Contains(err.Error(), "different --backup") {
		t.Fatalf("error = %v", err)
	}
}

func TestKeyListReturnsMetadataWithoutSecret(t *testing.T) {
	c := testClient(t, func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodGet || r.URL.Path != "/tailnet/-/keys" {
			t.Fatalf("%s %s", r.Method, r.URL.Path)
		}
		return response(`{"keys":[{"id":"key-1","keyType":"client","created":"now","expires":"later","scopes":["oauth_keys:read"],"tags":["tag:github"],"description":"CI client","userId":"creator-1","secret":"must-not-print","capabilities":{"devices":{"create":{"ephemeral":true,"tags":["tag:github"]}}}}]}`, nil), nil
	})
	var out bytes.Buffer
	if err := c.execute(definition(t, "observe", "tailscale", "key", "list"), nil, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "tag:github") || !strings.Contains(out.String(), "keyType") || !strings.Contains(out.String(), "creator_id") || strings.Contains(out.String(), "must-not-print") {
		t.Fatalf("key output = %s", out.String())
	}
}

func TestKeyListAllRequestsAllVisibleKeyTypes(t *testing.T) {
	c := testClient(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/tailnet/-/keys" || r.URL.Query().Get("all") != "true" {
			t.Fatalf("key list request = %s?%s", r.URL.Path, r.URL.RawQuery)
		}
		return response(`{"keys":[{"id":"revoked","keyType":"client","invalid":true,"revoked":"now"}]}`, nil), nil
	})
	var out bytes.Buffer
	if err := c.execute(definition(t, "observe", "tailscale", "key", "list"), []string{"--all"}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"invalid":true`) || !strings.Contains(out.String(), "revoked") {
		t.Fatalf("all key output = %s", out.String())
	}
}

func TestKeyGetPreservesRevocationStateWithoutSecret(t *testing.T) {
	c := testClient(t, func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodGet || r.URL.Path != "/tailnet/-/keys/k123CNTRL" {
			t.Fatalf("key get = %s %s", r.Method, r.URL.Path)
		}
		return response(`{"id":"k123CNTRL","keyType":"client","invalid":true,"revoked":"now","secret":"must-not-print"}`, nil), nil
	})
	var out bytes.Buffer
	if err := c.execute(definition(t, "observe", "tailscale", "key", "get"), []string{"k123CNTRL"}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"invalid":true`) || !strings.Contains(out.String(), "revoked") || strings.Contains(out.String(), "must-not-print") {
		t.Fatalf("key get output = %s", out.String())
	}
}

func TestDNSGetReadsAllFixedEndpoints(t *testing.T) {
	seen := map[string]bool{}
	c := testClient(t, func(r *http.Request) (*http.Response, error) {
		seen[r.URL.Path] = true
		return response(`{"value":true}`, nil), nil
	})
	var out bytes.Buffer
	if err := c.execute(definition(t, "observe", "tailscale", "dns", "get"), nil, &out); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/tailnet/-/dns/nameservers", "/tailnet/-/dns/preferences", "/tailnet/-/dns/searchpaths", "/tailnet/-/dns/split-dns"} {
		if !seen[path] {
			t.Fatalf("DNS endpoint not read: %s", path)
		}
	}
	if !strings.Contains(out.String(), "splitDNS") {
		t.Fatalf("DNS output = %s", out.String())
	}
}

func TestRoutePermissionErrorNamesRequiredScope(t *testing.T) {
	c := testClient(t, func(*http.Request) (*http.Response, error) {
		return responseStatus(http.StatusForbidden, "403 Forbidden", `{"message":"calling actor does not have enough permissions"}`), nil
	})
	err := c.execute(definition(t, "observe", "tailscale", "device", "route", "list"), []string{"dev1"}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "devices:routes:read") {
		t.Fatalf("error = %v", err)
	}
}

func TestNetworkErrorDoesNotExposeAPIToken(t *testing.T) {
	c := NewClient(config.TailscaleCredentials{APIKey: "do-not-leak"}, config.TailscaleSettings{APIURL: "http://example.invalid", Tailnet: "-"})
	c.http.Transport = roundTripper(func(*http.Request) (*http.Response, error) { return nil, &urlError{} })
	_, _, err := c.request(http.MethodGet, "/anything", "", nil)
	if err == nil || strings.Contains(err.Error(), "do-not-leak") {
		t.Fatalf("credential leaked in error: %v", err)
	}
}

func testClient(t *testing.T, handler roundTripper) *Client {
	t.Helper()
	c := NewClient(config.TailscaleCredentials{APIKey: "tskey-api-secret"}, config.TailscaleSettings{APIURL: "https://example.test", Tailnet: "-"})
	c.http = &http.Client{Transport: handler}
	return c
}

func response(body string, header http.Header) *http.Response {
	if header == nil {
		header = make(http.Header)
	}
	return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Header: header, Body: io.NopCloser(strings.NewReader(body))}
}

func responseStatus(code int, status, body string) *http.Response {
	return &http.Response{StatusCode: code, Status: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}

type roundTripper func(*http.Request) (*http.Response, error)

func (f roundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type urlError struct{}

func (*urlError) Error() string { return "request failed" }
