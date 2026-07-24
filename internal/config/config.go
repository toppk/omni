// Package config owns the intentionally small, inspectable on-disk layout.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Paths struct{ Root, Settings, Credentials string }

func DefaultPaths() (Paths, error) {
	root, err := os.UserConfigDir()
	if err != nil {
		return Paths{}, fmt.Errorf("find configuration directory: %w", err)
	}
	root = filepath.Join(root, "omni")
	return Paths{root, filepath.Join(root, "settings.toml"), filepath.Join(root, "credentials", "credentials.toml")}, nil
}

type TrelloCredentials struct {
	APIKey string
	Token  string
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
}

var Registry = []Entry{
	{Key: "trello.api-url", Default: "https://api.trello.com/1", Description: "Trello REST API base URL; override for a compatible mock server."},
	{Key: "trello.default-board-id", Description: "Board ID used when a board command omits BOARD_ID."},
	{Key: "trello.api-key", Secret: true, Required: true, Description: "Trello API key.", SetupURL: "https://developer.atlassian.com/cloud/trello/guides/rest-api/api-introduction/"},
	{Key: "trello.api-token", Secret: true, Required: true, Description: "Trello user token.", SetupURL: "https://developer.atlassian.com/cloud/trello/guides/rest-api/api-introduction/"},
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
	if err := writeIfMissing(p.Credentials, []byte("# Credentials are local secrets. Keep this file mode 0600.\n# [tailscale]\n# api-key = \"tskey-api-...\"\n#\n# [trello]\n# api-key = \"...\"\n# api-token = \"...\"\n"), 0600); err != nil {
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
