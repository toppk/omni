// Package config owns the intentionally small, inspectable on-disk layout.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Paths struct{ Root, Settings, Credentials string }
type EphemeralPaths struct{ Root, Credentials string }

func DefaultPaths() (Paths, error) {
	root, err := os.UserConfigDir()
	if err != nil {
		return Paths{}, fmt.Errorf("find configuration directory: %w", err)
	}
	root = filepath.Join(root, "omni")
	return Paths{root, filepath.Join(root, "settings.toml"), filepath.Join(root, "credentials", "credentials.toml")}, nil
}

func DefaultEphemeralPaths() (EphemeralPaths, error) {
	root := os.Getenv("XDG_DATA_HOME")
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return EphemeralPaths{}, fmt.Errorf("find home directory: %w", err)
		}
		root = filepath.Join(home, ".local", "share")
	}
	root = filepath.Join(root, "omni", "ephemeral")
	return EphemeralPaths{Root: root, Credentials: filepath.Join(root, "credentials.toml")}, nil
}

type TrelloCredentials struct {
	APIKey string
	Token  string
}

type TailscaleCredentials struct {
	APIKey       string
	ClientSecret string
}

// Entry describes a key in Omni's configuration registry. Entries can have a
// code default, be optional, or be required secrets.
type Entry struct {
	Key         string
	Secret      bool
	Required    bool
	Default     string
	Description string
	SetupURL    string
	Ephemeral   bool
}

var Registry = []Entry{
	{Key: "tailscale.api-url", Default: "https://api.tailscale.com/api/v2", Description: "Tailscale API base URL; override for a compatible mock server."},
	{Key: "tailscale.tailnet", Default: "-", Description: "Tailnet ID; - selects the tailnet that owns the access token."},
	{Key: "tailscale.client-id", Description: "OAuth client ID used to issue access tokens."},
	{Key: "tailscale.client-secret", Secret: true, Description: "OAuth client secret used to issue access tokens.", SetupURL: "https://tailscale.com/docs/features/oauth-clients"},
	{Key: "tailscale.api-key", Secret: true, Description: "Optional user-provided Tailscale API access-token override.", SetupURL: "https://tailscale.com/docs/reference/tailscale-api"},
	{Key: "tailscale.generated-api-key", Secret: true, Ephemeral: true, Description: "OAuth-generated access token cached until expiry in XDG data storage."},
	{Key: "trello.api-url", Default: "https://api.trello.com/1", Description: "Trello REST API base URL; override for a compatible mock server."},
	{Key: "trello.default-board-id", Description: "Board ID used when a board command omits BOARD_ID."},
	{Key: "trello.api-key", Secret: true, Required: true, Description: "Trello API key.", SetupURL: "https://developer.atlassian.com/cloud/trello/guides/rest-api/api-introduction/"},
	{Key: "trello.api-token", Secret: true, Required: true, Description: "Trello user token.", SetupURL: "https://developer.atlassian.com/cloud/trello/guides/rest-api/api-introduction/"},
}

func LoadTailscaleCredentials(path string) (TailscaleCredentials, error) {
	key, err := Get(path, "tailscale.api-key")
	if err != nil {
		return TailscaleCredentials{}, err
	}
	clientSecret, err := Get(path, "tailscale.client-secret")
	if err != nil {
		return TailscaleCredentials{}, err
	}
	return TailscaleCredentials{APIKey: key, ClientSecret: clientSecret}, nil
}

func LoadEphemeralTailscaleToken(path string, now time.Time) (string, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return "", nil
	} else if err != nil {
		return "", err
	}
	token, err := Get(path, "tailscale.generated-api-key")
	if err != nil {
		return "", err
	}
	expires, err := Get(path, "tailscale.generated-api-key-expires-at")
	if err != nil || token == "" || expires == "" {
		return "", err
	}
	expiresAt, err := time.Parse(time.RFC3339, expires)
	if err != nil || !expiresAt.After(now.Add(5*time.Minute)) {
		return "", nil
	}
	return token, nil
}

func StoreEphemeralTailscaleToken(path, token string, expiresAt time.Time) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	if err := os.Chmod(filepath.Dir(path), 0700); err != nil {
		return err
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := os.WriteFile(path, []byte("# Generated short-lived secrets. Do not edit.\n"), 0600); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	if err := Set(path, "tailscale.generated-api-key", token); err != nil {
		return err
	}
	if err := Set(path, "tailscale.generated-api-key-expires-at", expiresAt.UTC().Format(time.RFC3339)); err != nil {
		return err
	}
	return os.Chmod(path, 0600)
}

func Lookup(key string) (Entry, bool) {
	for _, entry := range Registry {
		if entry.Key == key {
			return entry, true
		}
	}
	return Entry{}, false
}

// LoadTrelloCredentials reads the deliberately narrow TOML subset used by the
// credentials file: a [trello] section with quoted api_key and token strings.
// Keeping this local avoids adding a TOML dependency just for two secrets.
func LoadTrelloCredentials(path string) (TrelloCredentials, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return TrelloCredentials{}, fmt.Errorf("read credentials: %w", err)
	}
	var credentials TrelloCredentials
	inTrello := false
	for lineNo, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if line == "[trello]" {
			inTrello = true
			continue
		}
		if strings.HasPrefix(line, "[") {
			inTrello = false
			continue
		}
		if !inTrello {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return TrelloCredentials{}, fmt.Errorf("credentials line %d: expected key = value", lineNo+1)
		}
		parsed, err := strconv.Unquote(strings.TrimSpace(strings.SplitN(value, "#", 2)[0]))
		if err != nil {
			return TrelloCredentials{}, fmt.Errorf("credentials line %d: values must be quoted strings", lineNo+1)
		}
		switch strings.TrimSpace(key) {
		case "api-key", "api_key":
			credentials.APIKey = parsed
		case "api-token", "token":
			credentials.Token = parsed
		}
	}
	if credentials.APIKey == "" || credentials.Token == "" {
		return TrelloCredentials{}, fmt.Errorf("Trello credentials require [trello] api-key and api-token in %s", path)
	}
	return credentials, nil
}

func Initialize(p Paths) error {
	if err := os.MkdirAll(filepath.Dir(p.Credentials), 0700); err != nil {
		return err
	}
	// MkdirAll preserves an existing directory's mode, so explicitly correct it.
	// Credential directories must never be searchable or writable by group/other.
	if err := os.Chmod(filepath.Dir(p.Credentials), 0700); err != nil {
		return fmt.Errorf("secure credentials directory: %w", err)
	}
	if err := writeIfMissing(p.Settings, []byte("# Omni settings. Restrictions only narrow what commands may do.\npolicy = \"default\"\n"), 0600); err != nil {
		return err
	}
	if err := writeIfMissing(p.Credentials, []byte("# Credentials are local secrets. Keep this file mode 0600.\n# [tailscale]\n# api-key = \"tskey-api-...\"\n# client-secret = \"...\"\n#\n# [trello]\n# api-key = \"...\"\n# api-token = \"...\"\n"), 0600); err != nil {
		return err
	}
	// WriteFile also retains an existing file's mode. Repair it on every init.
	if err := os.Chmod(p.Credentials, 0600); err != nil {
		return fmt.Errorf("secure credentials file: %w", err)
	}
	return nil
}

// Set stores a lower-case registry key such as trello.default-board-id. It
// updates only that TOML assignment and preserves unrelated configuration.
func Set(path, registryKey, value string) error {
	section, key, ok := strings.Cut(registryKey, ".")
	if !ok || section == "" || key == "" || strings.Contains(key, ".") {
		return fmt.Errorf("configuration key must have the form section.key")
	}
	if registryKey != strings.ToLower(registryKey) {
		return fmt.Errorf("configuration keys must be lower-case")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read configuration: %w", err)
	}
	lines := strings.Split(string(b), "\n")
	header := "[" + section + "]"
	start, end := -1, len(lines)
	for i, line := range lines {
		if strings.TrimSpace(line) == header {
			start = i
			continue
		}
		if start >= 0 && strings.HasPrefix(strings.TrimSpace(line), "[") {
			end = i
			break
		}
	}
	assignment := key + " = " + strconv.Quote(value)
	if start >= 0 {
		if end == len(lines) {
			for end > start+1 && strings.TrimSpace(lines[end-1]) == "" {
				end--
			}
		}
		for i := start + 1; i < end; i++ {
			current, _, found := strings.Cut(strings.TrimSpace(lines[i]), "=")
			if found && strings.TrimSpace(current) == key {
				lines[i] = assignment
				return os.WriteFile(path, toml(lines), 0600)
			}
		}
		lines = append(lines[:end], append([]string{assignment}, lines[end:]...)...)
	} else {
		if len(lines) > 0 && lines[len(lines)-1] != "" {
			lines = append(lines, "")
		}
		lines = append(lines, header, assignment)
	}
	return os.WriteFile(path, toml(lines), 0600)
}

// Delete removes one registry assignment while preserving unrelated sections,
// comments, and settings. Its boolean result reports whether a value existed.
func Delete(path, registryKey string) (bool, error) {
	section, key, ok := strings.Cut(registryKey, ".")
	if !ok || section == "" || key == "" || strings.Contains(key, ".") {
		return false, fmt.Errorf("configuration key must have the form section.key")
	}
	if registryKey != strings.ToLower(registryKey) {
		return false, fmt.Errorf("configuration keys must be lower-case")
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return false, fmt.Errorf("read configuration: %w", err)
	}
	lines := strings.Split(string(b), "\n")
	inSection := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "["+section+"]" {
			inSection = true
			continue
		}
		if strings.HasPrefix(trimmed, "[") {
			inSection = false
			continue
		}
		if !inSection || trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		current, _, found := strings.Cut(trimmed, "=")
		if found && strings.TrimSpace(current) == key {
			lines = append(lines[:i], lines[i+1:]...)
			return true, os.WriteFile(path, toml(lines), 0600)
		}
	}
	return false, nil
}

func toml(lines []string) []byte {
	return []byte(strings.TrimRight(strings.Join(lines, "\n"), "\n") + "\n")
}

// Get reads one quoted string from the same small TOML registry layout used by Set.
func Get(path, registryKey string) (string, error) {
	section, key, ok := strings.Cut(registryKey, ".")
	if !ok || section == "" || key == "" || strings.Contains(key, ".") {
		return "", fmt.Errorf("configuration key must have the form section.key")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read configuration: %w", err)
	}
	inSection := false
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "["+section+"]" {
			inSection = true
			continue
		}
		if strings.HasPrefix(line, "[") {
			inSection = false
			continue
		}
		if !inSection || line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		current, raw, found := strings.Cut(line, "=")
		if !found || strings.TrimSpace(current) != key {
			continue
		}
		value, err := strconv.Unquote(strings.TrimSpace(strings.SplitN(raw, "#", 2)[0]))
		if err != nil {
			return "", err
		}
		return value, nil
	}
	return "", nil
}

type TrelloSettings struct {
	APIURL         string
	DefaultBoardID string
}

type TailscaleSettings struct {
	APIURL   string
	Tailnet  string
	ClientID string
}

func LoadTailscaleSettings(path string) (TailscaleSettings, error) {
	apiURL, err := Get(path, "tailscale.api-url")
	if err != nil {
		return TailscaleSettings{}, err
	}
	if apiURL == "" {
		apiURL = "https://api.tailscale.com/api/v2"
	}
	tailnet, err := Get(path, "tailscale.tailnet")
	if err != nil {
		return TailscaleSettings{}, err
	}
	if tailnet == "" {
		tailnet = "-"
	}
	clientID, err := Get(path, "tailscale.client-id")
	if err != nil {
		return TailscaleSettings{}, err
	}
	return TailscaleSettings{APIURL: apiURL, Tailnet: tailnet, ClientID: clientID}, nil
}

func LoadTrelloSettings(path string) (TrelloSettings, error) {
	apiURL, err := Get(path, "trello.api-url")
	if err != nil {
		return TrelloSettings{}, err
	}
	if apiURL == "" {
		apiURL = "https://api.trello.com/1"
	}
	boardID, err := Get(path, "trello.default-board-id")
	if err != nil {
		return TrelloSettings{}, err
	}
	return TrelloSettings{APIURL: apiURL, DefaultBoardID: boardID}, nil
}

func writeIfMissing(path string, data []byte, mode os.FileMode) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.WriteFile(path, data, mode)
}
