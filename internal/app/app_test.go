package app

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/toppk/omni/internal/config"
)

func TestSetupTrelloShowsSingleConfigurationCommand(t *testing.T) {
	var out bytes.Buffer
	if err := Run(context.Background(), []string{"setup", "trello"}, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	if !strings.Contains(text, "omni configure trello --default-board BOARD_ID --api-key API_KEY --api-token API_TOKEN") {
		t.Fatalf("unexpected setup output: %s", text)
	}
	if !strings.Contains(text, "developer.atlassian.com") {
		t.Fatal("missing credential URL")
	}
}

func TestConfigureTrelloHelpDoesNotInitializeFiles(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var out bytes.Buffer
	if err := Run(context.Background(), []string{"configure", "trello", "--help"}, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "--api-token") {
		t.Fatal("missing Trello options")
	}
}

func TestConfigureTrelloStoresSettingsAndSecrets(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var out bytes.Buffer
	err := Run(context.Background(), []string{"configure", "trello", "--default-board", "board-id", "--api-key", "key-value", "--api-token", "token-value"}, &out, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	paths, err := config.DefaultPaths()
	if err != nil {
		t.Fatal(err)
	}
	board, err := config.Get(paths.Settings, "trello.default-board-id")
	if err != nil {
		t.Fatal(err)
	}
	if board != "board-id" {
		t.Fatalf("board = %q", board)
	}
	credentials, err := config.LoadTrelloCredentials(paths.Credentials)
	if err != nil {
		t.Fatal(err)
	}
	if credentials.APIKey != "key-value" || credentials.Token != "token-value" {
		t.Fatalf("credentials were not stored")
	}
	if strings.Contains(out.String(), "key-value") || strings.Contains(out.String(), "token-value") {
		t.Fatal("secret appeared in output")
	}
}

func TestSetupAndConfigureTailscale(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var out bytes.Buffer
	if err := Run(context.Background(), []string{"setup", "tailscale"}, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "--client-id CLIENT_ID") || !strings.Contains(out.String(), "tailscale.com") {
		t.Fatalf("unexpected setup output: %s", out.String())
	}
	out.Reset()
	if err := Run(context.Background(), []string{"configure", "tailscale", "--tailnet", "example.com", "--client-id", "client-id", "--client-secret", "client-secret", "--api-key", "secret-token"}, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	paths, err := config.DefaultPaths()
	if err != nil {
		t.Fatal(err)
	}
	settings, err := config.LoadTailscaleSettings(paths.Settings)
	if err != nil || settings.Tailnet != "example.com" || settings.ClientID != "client-id" {
		t.Fatalf("settings = %#v, err = %v", settings, err)
	}
	credentials, err := config.LoadTailscaleCredentials(paths.Credentials)
	if err != nil || credentials.APIKey != "secret-token" || credentials.ClientSecret != "client-secret" {
		t.Fatalf("credentials = %#v, err = %v", credentials, err)
	}
	if strings.Contains(out.String(), "secret-token") || strings.Contains(out.String(), "client-secret") {
		t.Fatal("secret appeared in output")
	}
}

func TestConfigureDeletesSettingAndSecret(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var out bytes.Buffer
	if err := Run(context.Background(), []string{"configure", "tailscale", "--client-id", "client", "--client-secret", "secret"}, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if err := Run(context.Background(), []string{"configure", "delete", "tailscale.client-id"}, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if err := Run(context.Background(), []string{"configure", "secret", "delete", "tailscale.client-secret"}, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	paths, err := config.DefaultPaths()
	if err != nil {
		t.Fatal(err)
	}
	if value, err := config.Get(paths.Settings, "tailscale.client-id"); err != nil || value != "" {
		t.Fatalf("client id=%q err=%v", value, err)
	}
	if value, err := config.Get(paths.Credentials, "tailscale.client-secret"); err != nil || value != "" {
		t.Fatalf("client secret=%q err=%v", value, err)
	}
}

func TestVersionAliases(t *testing.T) {
	for _, args := range [][]string{{"version"}, {"--version"}, {"-V"}} {
		var out bytes.Buffer
		if err := Run(context.Background(), args, &out, &bytes.Buffer{}); err != nil {
			t.Fatal(err)
		}
		if got, want := out.String(), "omni "+Version+"\n"; got != want {
			t.Fatalf("%v: got %q, want %q", args, got, want)
		}
	}
}

func TestDescribeOverviewListsServiceDiscovery(t *testing.T) {
	var out bytes.Buffer
	if err := Run(context.Background(), []string{"describe"}, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	for _, service := range []string{"omni describe trello", "omni describe tailscale"} {
		if !strings.Contains(out.String(), service) {
			t.Fatalf("missing service discovery: %s", out.String())
		}
	}
}

func TestDescribeTrelloJSONIncludesOperationMetadata(t *testing.T) {
	var out bytes.Buffer
	if err := Run(context.Background(), []string{"describe", "trello", "--format=json"}, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	var result struct {
		Service    string `json:"service"`
		Operations []struct {
			OperationID string `json:"operation_id"`
			Description string `json:"description"`
			Response    string `json:"response_description"`
		} `json:"operations"`
	}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Service != "trello" || len(result.Operations) == 0 {
		t.Fatalf("unexpected catalog: %#v", result)
	}
	first := result.Operations[0]
	if first.OperationID != "omni.observe.trello.board.list" || first.Description == "" || first.Response == "" {
		t.Fatalf("incomplete operation metadata: %#v", first)
	}
}

func TestDescribeTrelloUsesServiceManualLayout(t *testing.T) {
	var out bytes.Buffer
	if err := Run(context.Background(), []string{"describe", "trello"}, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	for _, heading := range []string{"TRELLO(1)", "NAME", "SYNOPSIS", "DESCRIPTION", "COMMANDS", "CONFIGURATION"} {
		if !strings.Contains(text, heading) {
			t.Fatalf("missing %s from manual: %s", heading, text)
		}
	}
	if strings.Contains(text, "operation_id") || strings.Contains(text, "implemented") {
		t.Fatalf("internal schema leaked into human manual: %s", text)
	}
}

func TestActionServiceHelpUsesServiceManual(t *testing.T) {
	var out bytes.Buffer
	if err := Run(context.Background(), []string{"observe", "tailscale", "--help"}, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "TAILSCALE(1)") || !strings.Contains(out.String(), "acl preview") {
		t.Fatalf("unexpected contextual help: %s", out.String())
	}
}

func TestPolicyExplainNamesACLBackupReversal(t *testing.T) {
	var out bytes.Buffer
	if err := Run(context.Background(), []string{"policy", "explain", "administer", "tailscale", "acl", "set"}, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "reversible: true") || !strings.Contains(out.String(), "pre-change backup") || !strings.Contains(out.String(), "no separate revert command") {
		t.Fatalf("unexpected policy explanation: %s", out.String())
	}
}
