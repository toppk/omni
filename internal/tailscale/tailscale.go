// Package tailscale implements Omni's deliberately narrow Tailscale API client.
package tailscale

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

type Client struct {
	baseURL     string
	http        *http.Client
	creds       config.TailscaleCredentials
	tailnet     string
	clientID    string
	accessToken string
	cachePath   string
}

func NewClient(creds config.TailscaleCredentials, settings config.TailscaleSettings) *Client {
	return &Client{baseURL: strings.TrimRight(settings.APIURL, "/"), http: &http.Client{Timeout: 30 * time.Second}, creds: creds, tailnet: settings.Tailnet, clientID: settings.ClientID}
}

func NewClientWithCache(creds config.TailscaleCredentials, settings config.TailscaleSettings, cachePath string) *Client {
	c := NewClient(creds, settings)
	c.cachePath = cachePath
	return c
}

func (c *Client) request(method, endpoint, contentType string, payload io.Reader) ([]byte, http.Header, error) {
	return c.requestWithHeaders(method, endpoint, contentType, payload, nil)
}

func (c *Client) requestWithHeaders(method, endpoint, contentType string, payload io.Reader, headers http.Header) ([]byte, http.Header, error) {
	req, err := http.NewRequest(method, c.baseURL+endpoint, payload)
	if err != nil {
		return nil, nil, fmt.Errorf("prepare Tailscale request %s %s", method, endpoint)
	}
	token, err := c.token()
	if err != nil {
		return nil, nil, err
	}
	req.SetBasicAuth(token, "")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	for key, values := range headers {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	resp, err := c.http.Do(req)
	if err != nil {
		// Basic-auth requests do not put the token in a URL, but avoid returning
		// transport text anyway: it can include request-specific details.
		return nil, nil, fmt.Errorf("Tailscale request %s %s failed: network error", method, endpoint)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message := fmt.Sprintf("Tailscale request %s %s returned %s: %s", method, endpoint, resp.Status, strings.TrimSpace(string(data)))
		if resp.StatusCode == http.StatusForbidden {
			if scope := requiredScope(method, endpoint); scope != "" {
				message += "; configure the Tailscale OAuth client with scope " + scope
			}
		}
		if resp.StatusCode == http.StatusPreconditionFailed {
			message += "; the active ACL changed after the backup was created, so no replacement was made"
		}
		return nil, nil, fmt.Errorf("%s", message)
	}
	return data, resp.Header, nil
}

// token returns an explicit API-token override when configured. Otherwise it
// exchanges the stored OAuth client credentials for a short-lived API token.
func (c *Client) token() (string, error) {
	if c.creds.APIKey != "" {
		return c.creds.APIKey, nil
	}
	if c.accessToken != "" {
		return c.accessToken, nil
	}
	if c.cachePath != "" {
		cached, err := config.LoadEphemeralTailscaleToken(c.cachePath, time.Now())
		if err != nil {
			return "", fmt.Errorf("read cached Tailscale token: %w", err)
		}
		if cached != "" {
			c.accessToken = cached
			return cached, nil
		}
	}
	if c.clientID == "" || c.creds.ClientSecret == "" {
		return "", fmt.Errorf("Tailscale credentials require tailscale.client-id and tailscale.client-secret, or an optional tailscale.api-key override")
	}
	form := url.Values{"client_id": {c.clientID}, "client_secret": {c.creds.ClientSecret}}
	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/oauth/token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("prepare Tailscale OAuth token request")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("Tailscale OAuth token request failed: network error")
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("Tailscale OAuth token request returned %s: %s", resp.Status, strings.TrimSpace(string(data)))
	}
	var result struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(data, &result); err != nil || result.AccessToken == "" {
		return "", fmt.Errorf("decode Tailscale OAuth token response")
	}
	c.accessToken = result.AccessToken
	if c.cachePath != "" {
		expiresIn := result.ExpiresIn
		if expiresIn <= 0 {
			expiresIn = 3600
		}
		if err := config.StoreEphemeralTailscaleToken(c.cachePath, c.accessToken, time.Now().Add(time.Duration(expiresIn)*time.Second)); err != nil {
			return "", fmt.Errorf("cache generated Tailscale token: %w", err)
		}
	}
	return c.accessToken, nil
}

func (c *Client) tailnetPath(suffix string) string {
	return "/tailnet/" + url.PathEscape(c.tailnet) + suffix
}

// Execute maps fixed command definitions to fixed API endpoints. It provides
// no arbitrary endpoint, method, or request-body escape hatch.
func Execute(d command.Definition, args []string, creds config.TailscaleCredentials, settings config.TailscaleSettings, cachePath string, out io.Writer) error {
	return ExecuteWithFormat(d, args, creds, settings, cachePath, output.JSON, out)
}

func ExecuteWithFormat(d command.Definition, args []string, creds config.TailscaleCredentials, settings config.TailscaleSettings, cachePath string, format output.Format, out io.Writer) error {
	c := NewClientWithCache(creds, settings, cachePath)
	return c.executeWithFormat(d, args, format, out)
}

func (c *Client) execute(d command.Definition, args []string, out io.Writer) error {
	return c.executeWithFormat(d, args, output.JSON, out)
}

func (c *Client) executeWithFormat(d command.Definition, args []string, format output.Format, out io.Writer) error {
	var result any
	switch d.Name() {
	case "observe tailscale device list":
		tag, details, err := deviceListOptions(args)
		if err != nil {
			return err
		}
		data, _, err := c.request(http.MethodGet, c.tailnetPath("/devices"), "", nil)
		if err != nil {
			return err
		}
		var response struct {
			Devices []map[string]any `json:"devices"`
		}
		if err := json.Unmarshal(data, &response); err != nil {
			return decode(err)
		}
		devices := make([]map[string]any, 0, len(response.Devices))
		for _, device := range response.Devices {
			if tag != "" && !hasTag(device, tag) {
				continue
			}
			devices = append(devices, compactDeviceList(device, details))
		}
		result = map[string]any{"devices": devices}
	case "observe tailscale device get":
		if len(args) < 1 {
			return fmt.Errorf("%s requires DEVICE_ID", d.Name())
		}
		details, err := details(args[1:], d.Name())
		if err != nil {
			return err
		}
		data, _, err := c.request(http.MethodGet, "/device/"+url.PathEscape(args[0]), "", nil)
		if err != nil {
			return err
		}
		var device map[string]any
		if err := json.Unmarshal(data, &device); err != nil {
			return decode(err)
		}
		result = map[string]any{"device": compactDeviceGet(device, details)}
	case "observe tailscale device route list":
		if err := exact(args, 1, d.Name()); err != nil {
			return err
		}
		data, _, err := c.request(http.MethodGet, "/device/"+url.PathEscape(args[0])+"/routes", "", nil)
		if err != nil {
			return err
		}
		var routes any
		if err := json.Unmarshal(data, &routes); err != nil {
			return decode(err)
		}
		result = map[string]any{"routes": routes}
	case "observe tailscale device retirement preflight":
		if err := exact(args, 1, d.Name()); err != nil {
			return err
		}
		preflight, complete, err := c.deviceRetirementPreflight(args[0])
		if err != nil {
			return err
		}
		result = map[string]any{"preflight": preflight}
		if !complete {
			if err := output.Encode(out, format, result); err != nil {
				return err
			}
			return fmt.Errorf("Tailscale device retirement preflight is incomplete; review missing_evidence before deletion")
		}
	case "update tailscale device name set":
		if err := exact(args, 2, d.Name()); err != nil {
			return err
		}
		if _, _, err := c.json(http.MethodPost, "/device/"+url.PathEscape(args[0])+"/name", map[string]string{"name": args[1]}); err != nil {
			return err
		}
		result = map[string]string{"device_id": args[0], "name": args[1]}
	case "authorize tailscale device tag set":
		if len(args) < 2 {
			return fmt.Errorf("%s requires DEVICE_ID and at least one TAG", d.Name())
		}
		if _, _, err := c.json(http.MethodPost, "/device/"+url.PathEscape(args[0])+"/tags", map[string][]string{"tags": args[1:]}); err != nil {
			return err
		}
		result = map[string]any{"device_id": args[0], "tags": args[1:]}
	case "authorize tailscale device authorization set":
		deviceID, state, err := deviceStateOption(args, "--state", "authorized", "unauthorized", d.Name())
		if err != nil {
			return err
		}
		if _, _, err := c.json(http.MethodPost, "/device/"+url.PathEscape(deviceID)+"/authorized", map[string]bool{"authorized": state == "authorized"}); err != nil {
			return err
		}
		result = map[string]any{"device_id": deviceID, "authorized": state == "authorized"}
	case "update tailscale device key expiry set":
		deviceID, state, err := deviceStateOption(args, "--state", "enabled", "disabled", d.Name())
		if err != nil {
			return err
		}
		if _, _, err := c.json(http.MethodPost, "/device/"+url.PathEscape(deviceID)+"/key", map[string]bool{"keyExpiryDisabled": state == "disabled"}); err != nil {
			return err
		}
		result = map[string]any{"device_id": deviceID, "key_expiry": state}
	case "administer tailscale device key expire":
		if err := exact(args, 1, d.Name()); err != nil {
			return err
		}
		if _, _, err := c.request(http.MethodPost, "/device/"+url.PathEscape(args[0])+"/expire", "", nil); err != nil {
			return err
		}
		result = map[string]any{"device_id": args[0], "status": "expired"}
	case "delete tailscale device delete":
		if err := exact(args, 1, d.Name()); err != nil {
			return err
		}
		if _, _, err := c.request(http.MethodDelete, "/device/"+url.PathEscape(args[0]), "", nil); err != nil {
			return err
		}
		result = map[string]any{"device_id": args[0], "status": "deleted"}
	case "observe tailscale acl get":
		output, err := outputPath(args)
		if err != nil {
			return err
		}
		data, header, err := c.request(http.MethodGet, c.tailnetPath("/acl"), "", nil)
		if err != nil {
			return err
		}
		if err := writeNew(output, data); err != nil {
			return err
		}
		result = map[string]string{"acl_file": output, "etag": header.Get("Etag")}
	case "observe tailscale acl validate":
		if err := exact(args, 1, d.Name()); err != nil {
			return err
		}
		data, err := os.ReadFile(args[0])
		if err != nil {
			return fmt.Errorf("read ACL file: %w", err)
		}
		data, _, err = c.request(http.MethodPost, c.tailnetPath("/acl/validate"), "application/hujson", bytes.NewReader(data))
		if err != nil {
			return err
		}
		if len(bytes.TrimSpace(data)) == 0 {
			result = map[string]string{"acl_file": args[0], "status": "valid"}
			break
		}
		var validation any
		if err := json.Unmarshal(data, &validation); err != nil {
			return decode(err)
		}
		result = map[string]any{"acl_file": args[0], "status": validationStatus(validation), "validation": validation}
		if validationStatus(validation) == "invalid" {
			if err := output.Encode(out, format, result); err != nil {
				return err
			}
			return fmt.Errorf("Tailscale ACL validation failed: %s", validationMessage(validation))
		}
	case "observe tailscale acl preview":
		source, file, err := previewOptions(args)
		if err != nil {
			return err
		}
		var acl []byte
		if file != "" {
			acl, err = os.ReadFile(file)
			if err != nil {
				return fmt.Errorf("read ACL file: %w", err)
			}
		} else {
			acl, _, err = c.request(http.MethodGet, c.tailnetPath("/acl"), "", nil)
			if err != nil {
				return err
			}
		}
		query := url.Values{"type": {"user"}, "previewFor": {source}}
		data, _, err := c.request(http.MethodPost, c.tailnetPath("/acl/preview")+"?"+query.Encode(), "application/hujson", bytes.NewReader(acl))
		if err != nil {
			return err
		}
		var preview any
		if err := json.Unmarshal(data, &preview); err != nil {
			return decode(err)
		}
		result = map[string]any{"source": source, "preview": preview}
		if file != "" {
			result.(map[string]any)["acl_file"] = file
		}
	case "administer tailscale acl set":
		aclFile, backupFile, err := setACLOptions(args)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(aclFile)
		if err != nil {
			return fmt.Errorf("read ACL file: %w", err)
		}
		validation, err := c.validateACL(data)
		if err != nil {
			return err
		}
		if validationStatus(validation) == "invalid" {
			return fmt.Errorf("Tailscale ACL validation failed: %s", validationMessage(validation))
		}
		current, header, err := c.request(http.MethodGet, c.tailnetPath("/acl"), "", nil)
		if err != nil {
			return err
		}
		if err := writeNew(backupFile, current); err != nil {
			if errors.Is(err, os.ErrExist) {
				return fmt.Errorf("write ACL backup: %s already exists; choose a different --backup path", backupFile)
			}
			return fmt.Errorf("write ACL backup: %w", err)
		}
		etag := header.Get("Etag")
		if etag == "" {
			return fmt.Errorf("Tailscale ACL response did not include an ETag; backup saved at %s and no replacement was made", backupFile)
		}
		if _, _, err := c.requestWithHeaders(http.MethodPost, c.tailnetPath("/acl"), "application/hujson", bytes.NewReader(data), http.Header{"If-Match": {etag}}); err != nil {
			return err
		}
		result = map[string]string{"acl_file": aclFile, "backup_file": backupFile, "etag": etag, "status": "accepted"}
	case "observe tailscale key list":
		all, err := keyListOptions(args)
		if err != nil {
			return err
		}
		endpoint := c.tailnetPath("/keys")
		if all {
			endpoint += "?all=true"
		}
		data, _, err := c.request(http.MethodGet, endpoint, "", nil)
		if err != nil {
			return err
		}
		var response struct {
			Keys []map[string]any `json:"keys"`
		}
		if err := json.Unmarshal(data, &response); err != nil {
			return decode(err)
		}
		keys := make([]map[string]any, 0, len(response.Keys))
		for _, key := range response.Keys {
			keys = append(keys, compactKey(key))
		}
		result = map[string]any{"keys": keys}
	case "observe tailscale key get":
		if err := exact(args, 1, d.Name()); err != nil {
			return err
		}
		data, _, err := c.request(http.MethodGet, c.tailnetPath("/keys/")+url.PathEscape(args[0]), "", nil)
		if err != nil {
			return err
		}
		var key map[string]any
		if err := json.Unmarshal(data, &key); err != nil {
			return decode(err)
		}
		result = map[string]any{"key": compactKey(key)}
	case "observe tailscale credential get":
		if err := exact(args, 0, d.Name()); err != nil {
			return err
		}
		result = map[string]any{"credential": c.credentialReport()}
	case "create tailscale key auth create":
		options, err := authKeyCreateOptions(args, d.Name())
		if err != nil {
			return err
		}
		secretFile, err := os.OpenFile(options.Output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return fmt.Errorf("create auth-key output: %w", err)
		}
		keepFile := false
		defer func() {
			_ = secretFile.Close()
			if !keepFile {
				_ = os.Remove(options.Output)
			}
		}()
		data, _, err := c.json(http.MethodPost, c.tailnetPath("/keys"), options.Request())
		if err != nil {
			return err
		}
		var key map[string]any
		if err := json.Unmarshal(data, &key); err != nil {
			return decode(err)
		}
		secret, ok := key["key"].(string)
		if !ok || secret == "" {
			return fmt.Errorf("Tailscale auth-key response did not include one-time key material")
		}
		if _, err := io.WriteString(secretFile, secret+"\n"); err != nil {
			return fmt.Errorf("write auth-key output: %w", err)
		}
		if err := secretFile.Close(); err != nil {
			return fmt.Errorf("close auth-key output: %w", err)
		}
		keepFile = true
		delete(key, "key")
		result = map[string]any{"key": compactKey(key), "key_file": options.Output}
	case "delete tailscale key revoke":
		if err := exact(args, 1, d.Name()); err != nil {
			return err
		}
		if _, _, err := c.request(http.MethodDelete, c.tailnetPath("/keys/")+url.PathEscape(args[0]), "", nil); err != nil {
			return err
		}
		result = map[string]any{"key_id": args[0], "status": "revoked"}
	case "observe tailscale dns get":
		dns := make(map[string]any)
		for name, endpoint := range map[string]string{"nameservers": "/dns/nameservers", "preferences": "/dns/preferences", "searchPaths": "/dns/searchpaths", "splitDNS": "/dns/split-dns"} {
			data, _, err := c.request(http.MethodGet, c.tailnetPath(endpoint), "", nil)
			if err != nil {
				return err
			}
			var value any
			if err := json.Unmarshal(data, &value); err != nil {
				return decode(err)
			}
			dns[name] = value
		}
		result = map[string]any{"dns": dns}
	case "observe tailscale user list":
		if err := exact(args, 0, d.Name()); err != nil {
			return err
		}
		data, _, err := c.request(http.MethodGet, c.tailnetPath("/users"), "", nil)
		if err != nil {
			return err
		}
		var response struct {
			Users []map[string]any `json:"users"`
		}
		if err := json.Unmarshal(data, &response); err != nil {
			return decode(err)
		}
		users := make([]map[string]any, 0, len(response.Users))
		for _, user := range response.Users {
			users = append(users, compactUser(user))
		}
		result = map[string]any{"users": users}
	case "observe tailscale user get":
		if err := exact(args, 1, d.Name()); err != nil {
			return err
		}
		data, _, err := c.request(http.MethodGet, "/user/"+url.PathEscape(args[0]), "", nil)
		if err != nil {
			return err
		}
		var user map[string]any
		if err := json.Unmarshal(data, &user); err != nil {
			return decode(err)
		}
		result = map[string]any{"user": compactUser(user)}
	default:
		return fmt.Errorf("%s is registered but not implemented", d.Name())
	}
	return output.Encode(out, format, result)
}

func (c *Client) json(method, endpoint string, value any) ([]byte, http.Header, error) {
	b, err := json.Marshal(value)
	if err != nil {
		return nil, nil, err
	}
	return c.request(method, endpoint, "application/json", bytes.NewReader(b))
}

func (c *Client) credentialReport() map[string]any {
	report := map[string]any{
		"api_url": c.baseURL,
		"tailnet": c.tailnet,
	}
	if c.creds.APIKey != "" {
		report["auth_method"] = "api_access_token"
		report["configuration_sources"] = []string{"tailscale.api-key", "tailscale.tailnet", "tailscale.api-url"}
		report["scope_status"] = "not_enumerable"
		report["scope_explanation"] = "API access-token permissions follow the owning user; Omni does not inspect or expose the token secret to derive its key identity."
		return report
	}
	report["auth_method"] = "oauth_client"
	report["client_id"] = c.clientID
	report["configuration_sources"] = []string{"tailscale.client-id", "tailscale.client-secret", "tailscale.tailnet", "tailscale.api-url"}
	if c.clientID == "" || c.creds.ClientSecret == "" {
		report["configuration_status"] = "incomplete"
		report["configuration_error"] = "tailscale.client-id and tailscale.client-secret are both required"
		return report
	}
	report["configuration_status"] = "configured"
	data, _, err := c.request(http.MethodGet, c.tailnetPath("/keys/")+url.PathEscape(c.clientID), "", nil)
	if err != nil {
		report["scope_status"] = "unavailable"
		report["scope_error"] = "OAuth client metadata requires oauth_keys:read: " + c.redactCredentialError(err.Error())
	} else {
		var key map[string]any
		if err := json.Unmarshal(data, &key); err != nil {
			report["scope_status"] = "unavailable"
			report["scope_error"] = decode(err).Error()
		} else {
			report["scope_status"] = "available"
			report["client"] = compactKey(key)
		}
	}
	if c.cachePath != "" {
		metadata, err := config.LoadEphemeralTailscaleTokenMetadata(c.cachePath, time.Now())
		if err != nil {
			report["token_cache"] = map[string]any{"status": "unavailable", "error": err.Error()}
		} else {
			cache := map[string]any{"cached": metadata.Cached, "valid": metadata.Valid, "source": "tailscale.generated-api-key"}
			if !metadata.ExpiresAt.IsZero() {
				cache["expires_at"] = metadata.ExpiresAt.UTC().Format(time.RFC3339)
			}
			report["token_cache"] = cache
		}
	}
	return report
}

func (c *Client) redactCredentialError(message string) string {
	for _, secret := range []string{c.creds.APIKey, c.creds.ClientSecret, c.accessToken} {
		if secret != "" {
			message = strings.ReplaceAll(message, secret, "[REDACTED]")
		}
	}
	return message
}

func (c *Client) deviceRetirementPreflight(deviceID string) (map[string]any, bool, error) {
	data, _, err := c.request(http.MethodGet, "/device/"+url.PathEscape(deviceID), "", nil)
	if err != nil {
		return nil, false, err
	}
	var device map[string]any
	if err := json.Unmarshal(data, &device); err != nil {
		return nil, false, decode(err)
	}
	deviceEvidence := pick(device, "id", "nodeId", "hostname", "name", "user", "os", "lastSeen", "authorized", "isExternal", "isEphemeral", "tags", "addresses", "clientVersion", "expires", "keyExpiryDisabled")
	preflight := map[string]any{
		"device":                           deviceEvidence,
		"deletion_supported":               device["isExternal"] != true,
		"evidence_complete":                true,
		"missing_evidence":                 []map[string]string{},
		"tag_policy":                       []map[string]any{},
		"operator_acknowledgement_reasons": []map[string]string{},
		"consequences": []string{
			"The device will be removed from the tailnet and active connections will end.",
			"Re-enrollment requires host-side tailscale up with a valid credential.",
			"Deleting the device does not remove ACL rules, tag owners, auth keys, or references that outlive it.",
		},
		"limitations": []string{"Policy references through host aliases, IP literals, users, groups, posture, or indirect tag ownership are not resolved semantically by this preflight."},
	}
	missing := preflight["missing_evidence"].([]map[string]string)
	acknowledgements := preflight["operator_acknowledgement_reasons"].([]map[string]string)
	if preflight["deletion_supported"] != true {
		acknowledgements = append(acknowledgements, map[string]string{"code": "shared_device_not_deletable", "area": "device", "severity": "critical", "message": "Tailscale does not support deleting a device shared into the tailnet."})
	}
	routeData, _, routeErr := c.request(http.MethodGet, "/device/"+url.PathEscape(deviceID)+"/routes", "", nil)
	if routeErr != nil {
		missing = append(missing, map[string]string{"area": "routes", "error": routeErr.Error()})
	} else {
		var routes any
		if err := json.Unmarshal(routeData, &routes); err != nil {
			missing = append(missing, map[string]string{"area": "routes", "error": decode(err).Error()})
		} else {
			preflight["routes"] = routes
			if routeObject, ok := routes.(map[string]any); ok && len(stringValues(routeObject["enabledRoutes"])) > 0 {
				consequences := preflight["consequences"].([]string)
				preflight["consequences"] = append(consequences, "Enabled subnet or exit-node routes will stop being served by this device.")
				acknowledgements = append(acknowledgements, map[string]string{"code": "enabled_routes", "area": "enabled_routes", "severity": "critical", "message": "Active subnet or exit-node routes will stop being served; confirm replacement routing before deletion."})
			}
		}
	}
	tags := stringValues(device["tags"])
	if len(tags) > 0 {
		acl, _, aclErr := c.request(http.MethodGet, c.tailnetPath("/acl"), "", nil)
		if aclErr != nil {
			missing = append(missing, map[string]string{"area": "tag_policy", "error": aclErr.Error()})
		} else {
			dependencies := make([]map[string]any, 0, len(tags))
			for _, tag := range tags {
				query := url.Values{"type": {"user"}, "previewFor": {tag}}
				previewData, _, previewErr := c.request(http.MethodPost, c.tailnetPath("/acl/preview")+"?"+query.Encode(), "application/hujson", bytes.NewReader(acl))
				if previewErr != nil {
					missing = append(missing, map[string]string{"area": "tag_policy:" + tag, "error": previewErr.Error()})
					continue
				}
				var preview any
				if err := json.Unmarshal(previewData, &preview); err != nil {
					missing = append(missing, map[string]string{"area": "tag_policy:" + tag, "error": decode(err).Error()})
					continue
				}
				dependencies = append(dependencies, map[string]any{"tag": tag, "preview": preview})
				if previewHasMatches(preview) {
					acknowledgements = append(acknowledgements, map[string]string{"code": "matching_acl_policy", "area": "tag_policy:" + tag, "severity": "high", "message": "Active ACL rules match this device tag; review the preview before deletion."})
				}
			}
			preflight["tag_policy"] = dependencies
		}
	}
	preflight["missing_evidence"] = missing
	preflight["evidence_complete"] = len(missing) == 0
	preflight["operator_acknowledgement_reasons"] = acknowledgements
	preflight["requires_operator_acknowledgement"] = len(acknowledgements) > 0
	return preflight, len(missing) == 0, nil
}

func previewHasMatches(value any) bool {
	preview, ok := value.(map[string]any)
	if !ok {
		return false
	}
	matches, ok := preview["matches"].([]any)
	return ok && len(matches) > 0
}

func stringValues(value any) []string {
	values, ok := value.([]any)
	if !ok {
		return nil
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if text, ok := value.(string); ok {
			result = append(result, text)
		}
	}
	return result
}

func compactDeviceList(value map[string]any, details bool) map[string]any {
	keys := []string{"id", "hostname", "os", "lastSeen"}
	if details {
		keys = append(keys, "name", "user", "addresses", "tags", "authorized", "clientVersion", "expires", "keyExpiryDisabled")
	}
	return pick(value, keys...)
}
func compactDeviceGet(value map[string]any, details bool) map[string]any {
	keys := []string{"id", "hostname", "name", "user", "os", "lastSeen", "authorized", "tags"}
	if details {
		keys = append(keys, "addresses", "clientVersion", "expires", "keyExpiryDisabled", "nodeId")
	}
	return pick(value, keys...)
}
func compactUser(value map[string]any) map[string]any {
	return pick(value, "id", "loginName", "displayName", "profilePicUrl", "type", "role", "status", "created")
}
func compactKey(value map[string]any) map[string]any {
	result := pick(value, "id", "keyType", "created", "updated", "expires", "revoked", "invalid", "scopes", "tags", "description")
	if creator, ok := value["userId"]; ok {
		result["creator_id"] = creator
	}
	if capabilities, ok := value["capabilities"].(map[string]any); ok {
		if devices, ok := capabilities["devices"].(map[string]any); ok {
			if create, ok := devices["create"].(map[string]any); ok {
				result["deviceCreate"] = pick(create, "tags", "ephemeral", "reusable", "preauthorized")
			}
		}
	}
	return result
}
func pick(value map[string]any, keys ...string) map[string]any {
	result := make(map[string]any)
	for _, key := range keys {
		if v, ok := value[key]; ok {
			result[key] = v
		}
	}
	return result
}
func exact(args []string, want int, name string) error {
	if len(args) != want {
		return fmt.Errorf("%s expects %d argument(s)", name, want)
	}
	return nil
}
func details(args []string, name string) (bool, error) {
	if len(args) == 0 {
		return false, nil
	}
	if len(args) == 1 && args[0] == "--details" {
		return true, nil
	}
	return false, fmt.Errorf("%s accepts only --details", name)
}
func deviceListOptions(args []string) (tag string, showDetails bool, err error) {
	for len(args) > 0 {
		switch args[0] {
		case "--details":
			if showDetails {
				return "", false, fmt.Errorf("observe tailscale device list accepts --details only once")
			}
			showDetails = true
			args = args[1:]
		case "--tag":
			if tag != "" || len(args) < 2 || args[1] == "" {
				return "", false, fmt.Errorf("observe tailscale device list accepts --tag TAG once")
			}
			tag = args[1]
			args = args[2:]
		default:
			return "", false, fmt.Errorf("observe tailscale device list accepts only --tag TAG and --details")
		}
	}
	return tag, showDetails, nil
}
func previewOptions(args []string) (source, file string, err error) {
	for len(args) > 0 {
		if len(args) < 2 || args[1] == "" {
			return "", "", fmt.Errorf("observe tailscale acl preview requires --for SOURCE and accepts optional --file ACL_FILE")
		}
		switch args[0] {
		case "--for":
			if source != "" {
				return "", "", fmt.Errorf("observe tailscale acl preview accepts --for SOURCE once")
			}
			source = args[1]
		case "--file":
			if file != "" {
				return "", "", fmt.Errorf("observe tailscale acl preview accepts --file ACL_FILE once")
			}
			file = args[1]
		default:
			return "", "", fmt.Errorf("observe tailscale acl preview requires --for SOURCE and accepts optional --file ACL_FILE")
		}
		args = args[2:]
	}
	if source == "" {
		return "", "", fmt.Errorf("observe tailscale acl preview requires --for SOURCE")
	}
	return source, file, nil
}
func keyListOptions(args []string) (bool, error) {
	if len(args) == 0 {
		return false, nil
	}
	if len(args) == 1 && args[0] == "--all" {
		return true, nil
	}
	return false, fmt.Errorf("observe tailscale key list accepts only --all")
}
func deviceStateOption(args []string, option, first, second, name string) (deviceID, state string, err error) {
	if len(args) != 3 || args[1] != option {
		return "", "", fmt.Errorf("%s requires DEVICE_ID %s %s|%s", name, option, first, second)
	}
	if args[2] != first && args[2] != second {
		return "", "", fmt.Errorf("%s expects %s %s or %s", name, option, first, second)
	}
	return args[0], args[2], nil
}

type authKeyOptions struct {
	Output        string
	Description   string
	ExpirySeconds int64
	Tags          []string
	Reusable      bool
	Ephemeral     bool
	Preauthorized bool
}

func authKeyCreateOptions(args []string, name string) (authKeyOptions, error) {
	var options authKeyOptions
	for len(args) > 0 {
		switch args[0] {
		case "--output", "--description", "--expiry-seconds", "--tag":
			if len(args) < 2 || args[1] == "" {
				return options, fmt.Errorf("%s requires a value for %s", name, args[0])
			}
			option, value := args[0], args[1]
			switch option {
			case "--output":
				if options.Output != "" {
					return options, fmt.Errorf("%s accepts --output only once", name)
				}
				options.Output = value
			case "--description":
				if options.Description != "" {
					return options, fmt.Errorf("%s accepts --description only once", name)
				}
				options.Description = value
			case "--expiry-seconds":
				expiry, parseErr := strconv.ParseInt(value, 10, 64)
				if parseErr != nil || expiry <= 0 {
					return options, fmt.Errorf("%s expects --expiry-seconds to be a positive integer", name)
				}
				options.ExpirySeconds = expiry
			case "--tag":
				if !strings.HasPrefix(value, "tag:") || len(value) == len("tag:") {
					return options, fmt.Errorf("%s expects --tag values in tag:NAME form", name)
				}
				options.Tags = append(options.Tags, value)
			}
			args = args[2:]
		case "--reusable":
			options.Reusable = true
			args = args[1:]
		case "--ephemeral":
			options.Ephemeral = true
			args = args[1:]
		case "--preauthorized":
			options.Preauthorized = true
			args = args[1:]
		default:
			return options, fmt.Errorf("unknown option %s for %s", args[0], name)
		}
	}
	if options.Output == "" {
		return options, fmt.Errorf("%s requires --output FILE", name)
	}
	return options, nil
}

func (o authKeyOptions) Request() map[string]any {
	create := map[string]any{"reusable": o.Reusable, "ephemeral": o.Ephemeral, "preauthorized": o.Preauthorized}
	if len(o.Tags) > 0 {
		create["tags"] = o.Tags
	}
	request := map[string]any{"keyType": "auth", "capabilities": map[string]any{"devices": map[string]any{"create": create}}}
	if o.Description != "" {
		request["description"] = o.Description
	}
	if o.ExpirySeconds != 0 {
		request["expirySeconds"] = o.ExpirySeconds
	}
	return request
}
func setACLOptions(args []string) (aclFile, backupFile string, err error) {
	if len(args) != 3 {
		return "", "", fmt.Errorf("administer tailscale acl set requires exactly one ACL_FILE and --backup BACKUP_FILE; the backup path must be new")
	}
	if args[0] == "--backup" && args[1] != "" && args[2] != "" {
		return args[2], args[1], nil
	}
	if args[0] != "" && args[1] == "--backup" && args[2] != "" {
		return args[0], args[2], nil
	}
	return "", "", fmt.Errorf("administer tailscale acl set requires exactly one ACL_FILE and --backup BACKUP_FILE; the backup path must be new")
}
func (c *Client) validateACL(acl []byte) (any, error) {
	data, _, err := c.request(http.MethodPost, c.tailnetPath("/acl/validate"), "application/hujson", bytes.NewReader(acl))
	if err != nil {
		return nil, err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, nil
	}
	var validation any
	if err := json.Unmarshal(data, &validation); err != nil {
		return nil, decode(err)
	}
	return validation, nil
}
func validationStatus(validation any) string {
	if _, failed := validationFailureMessage(validation); failed {
		return "invalid"
	}
	return "valid"
}
func validationMessage(validation any) string {
	if message, failed := validationFailureMessage(validation); failed {
		return message
	}
	return "validation rejected the ACL"
}
func validationFailureMessage(validation any) (string, bool) {
	if object, ok := validation.(map[string]any); ok {
		if message, ok := object["message"].(string); ok && message != "" {
			return message, true
		}
	}
	return "", false
}
func requiredScope(method, endpoint string) string {
	path := strings.SplitN(endpoint, "?", 2)[0]
	switch {
	case strings.Contains(path, "/acl"):
		return "acl:read (or policy_file:read)"
	case strings.Contains(path, "/dns/"):
		return "dns:read"
	case strings.HasSuffix(path, "/routes"):
		return "devices:routes:read"
	case strings.Contains(path, "/keys"):
		if method == http.MethodGet {
			return "oauth_keys:read (OAuth clients), auth_keys:read (auth keys), and/or api_access_tokens:read (API access tokens)"
		}
		return "auth_keys (auth keys), oauth_keys (OAuth clients), and/or api_access_tokens (API access tokens)"
	case strings.Contains(path, "/users") || strings.HasPrefix(path, "/user/"):
		return "users:read"
	case strings.Contains(path, "/devices") || strings.HasPrefix(path, "/device/"):
		if method == http.MethodGet {
			return "devices:core:read"
		}
		return "devices:core"
	}
	return ""
}
func hasTag(device map[string]any, tag string) bool {
	tags, ok := device["tags"].([]any)
	if !ok {
		return false
	}
	for _, value := range tags {
		if value == tag {
			return true
		}
	}
	return false
}
func decode(err error) error { return fmt.Errorf("decode Tailscale response: %w", err) }
func outputPath(args []string) (string, error) {
	if len(args) == 0 {
		return "tailscale-acl-" + time.Now().UTC().Format("20060102T150405Z") + ".hujson", nil
	}
	if len(args) == 2 && args[0] == "--output" && args[1] != "" {
		return args[1], nil
	}
	return "", fmt.Errorf("observe tailscale acl get accepts only --output PATH")
}
func writeNew(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil && filepath.Dir(path) != "." {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("create private file: %w", err)
	}
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		return err
	}
	return f.Close()
}
