package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/toppk/omni/internal/command"
	"github.com/toppk/omni/internal/config"
	"github.com/toppk/omni/internal/output"
	"github.com/toppk/omni/internal/policy"
	"github.com/toppk/omni/internal/tailscale"
	"github.com/toppk/omni/internal/trello"
)

// Version is set by the release workflow with Go linker flags. Local builds
// intentionally retain the development value rather than a checked-in version.
var Version = "dev"

const usage = `Omni is a safety-oriented CLI for service APIs.

Usage:
  omni describe [SERVICE] [--format text|json]
  omni describe <effect> <service> <resource> <verb>
  omni setup SERVICE
  omni configure SERVICE [OPTIONS]
  omni [--policy MODE] <effect> <service> <resource> <verb> [--format text|json]

The effect is always the first token after "omni": observe, create, update,
move, archive, delete, execute, transfer, authorize, or administer.
Arguments and flags may identify resources or format output, but may not change
an operation's effect.

Service commands render text by default. Set OMNI_OUTPUT=json for a
machine-readable default, or use --format text|json after the effect.

Use "omni describe" to discover services and their operations. Use
"omni configure --help" for low-level registry commands, or "omni version"
for the installed version.
`

const trelloConfigUsage = `Trello configuration

Usage:
  omni configure trello [--default-board BOARD_ID] [--api-key API_KEY] [--api-token API_TOKEN] [--api-url URL]

All supplied values are stored at once. --default-board and --api-url are
ordinary settings; --api-key and --api-token are secrets. Secret values are
never printed by Omni, but command-line values can remain in shell history.

For this local agent workflow, generate a user token manually from the API-key
page. A Power-Up is required to obtain an API key, but Omni does not require
the Power-Up app secret or implement Trello's OAuth flow.

--default-board is optional. If you do not know a board ID, configure the API
key and token first, then list visible boards:
  omni observe trello board list
  omni observe trello board list --format json | jq -r '.boards[] | "\(.id)\t\(.name)"'

Choose an ID from that output and store it later:
  omni configure set trello.default-board-id BOARD_ID
`

const tailscaleConfigUsage = `Tailscale configuration

Usage:
  omni configure tailscale [--tailnet TAILNET_ID] [--api-key ACCESS_TOKEN]
                           [--client-id CLIENT_ID] [--client-secret CLIENT_SECRET] [--api-url URL]

All supplied values are stored at once. --tailnet, --client-id, and --api-url
are ordinary settings; --api-key and --client-secret are secrets. Omni uses
the client credentials to issue a short-lived access token for API calls;
--api-key is an optional explicit token override. Secret values are never
printed by Omni, but command-line values can remain in shell history.
`

const configureUsage = `Configure Omni

Usage:
  omni configure SERVICE [OPTIONS]
  omni configure describe
  omni configure help SECTION.KEY
  omni configure set SECTION.KEY VALUE
  omni configure delete SECTION.KEY
  omni configure secret set SECTION.KEY VALUE
  omni configure secret delete SECTION.KEY

For service-specific options, run "omni configure SERVICE --help".
`

func Run(_ context.Context, args []string, out, errOut io.Writer) error {
	if len(args) == 1 && (args[0] == "version" || args[0] == "--version" || args[0] == "-V") {
		_, err := fmt.Fprintf(out, "omni %s\n", Version)
		return err
	}
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		return rootHelp(out)
	}
	if len(args) >= 2 && args[0] != "configure" && args[0] != "setup" && (args[len(args)-1] == "--help" || args[len(args)-1] == "-h") {
		return contextualHelp(args[:len(args)-1], out)
	}
	if len(args) > 0 && args[0] == "configure" {
		return configure(args[1:], out)
	}
	if len(args) > 0 && args[0] == "setup" {
		return setup(args[1:], out)
	}
	if args[0] == "describe" {
		return describe(args[1:], out)
	}
	if len(args) >= 2 && args[0] == "policy" && args[1] == "explain" {
		return explain(args[2:], out)
	}

	mode := os.Getenv("OMNI_POLICY")
	if len(args) >= 2 && args[0] == "--policy" {
		mode, args = args[1], args[2:]
	}
	args, format, err := operationOutput(args)
	if err != nil {
		return err
	}
	d, operands, e := command.Match(args)
	if e != nil {
		return fmt.Errorf("%w\nrun 'omni describe' to see supported operations", e)
	}
	if e := policy.Allows(mode, d); e != nil {
		return e
	}
	if len(d.Path) > 0 && d.Path[0] == "trello" {
		paths, err := config.DefaultPaths()
		if err != nil {
			return err
		}
		credentials, err := config.LoadTrelloCredentials(paths.Credentials)
		if err != nil {
			return err
		}
		settings, err := config.LoadTrelloSettings(paths.Settings)
		if err != nil {
			return err
		}
		return trello.ExecuteWithFormat(d, operands, credentials, settings, format, out)
	}
	if len(d.Path) > 0 && d.Path[0] == "tailscale" {
		paths, err := config.DefaultPaths()
		if err != nil {
			return err
		}
		credentials, err := config.LoadTailscaleCredentials(paths.Credentials)
		if err != nil {
			return err
		}
		settings, err := config.LoadTailscaleSettings(paths.Settings)
		if err != nil {
			return err
		}
		ephemeral, err := config.DefaultEphemeralPaths()
		if err != nil {
			return err
		}
		return tailscale.ExecuteWithFormat(d, operands, credentials, settings, ephemeral.Credentials, format, out)
	}
	return fmt.Errorf("%s is registered but not implemented yet", d.Name())
}

func rootHelp(out io.Writer) error {
	if _, err := fmt.Fprint(out, usage); err != nil {
		return err
	}
	paths, err := config.DefaultPaths()
	if err != nil {
		return err
	}
	trelloStatus := "needs setup"
	if _, err := config.LoadTrelloCredentials(paths.Credentials); err == nil {
		trelloStatus = "configured"
	}
	tailscaleStatus := "needs setup"
	tailscaleCredentials, credentialsErr := config.LoadTailscaleCredentials(paths.Credentials)
	tailscaleSettings, settingsErr := config.LoadTailscaleSettings(paths.Settings)
	if credentialsErr == nil && (tailscaleCredentials.APIKey != "" || (tailscaleCredentials.ClientSecret != "" && settingsErr == nil && tailscaleSettings.ClientID != "")) {
		tailscaleStatus = "configured"
	}
	_, err = fmt.Fprintf(out, "\nServices:\n  trello     %-12s omni describe trello\n  tailscale  %-12s omni describe tailscale\n\nSet up a service with:  omni setup SERVICE\n", trelloStatus, tailscaleStatus)
	return err
}

func contextualHelp(args []string, out io.Writer) error {
	if len(args) > 0 && isEffect(args[0]) {
		var err error
		args, _, err = operationOutput(args)
		if err != nil {
			return err
		}
	}
	if len(args) == 2 && isEffect(args[0]) {
		if len(serviceOperations(args[1])) == 0 {
			return fmt.Errorf("unknown service %q", args[1])
		}
		return describeService(args[1], false, out)
	}
	d, operands, err := command.Match(args)
	if err != nil || len(operands) != 0 {
		return fmt.Errorf("unknown command help; run 'omni describe' to see supported operations")
	}
	return describeOperation(d, out)
}

func isEffect(value string) bool {
	switch command.Effect(value) {
	case command.Observe, command.Create, command.Update, command.Move, command.Archive, command.Delete, command.Execute, command.Transfer, command.Authorize, command.Administer:
		return true
	}
	return false
}

func configure(args []string, out io.Writer) error {
	p, err := config.DefaultPaths()
	if err != nil {
		return err
	}
	if len(args) == 0 || same(args, []string{"--help"}) || same(args, []string{"help"}) {
		_, err = fmt.Fprint(out, configureUsage)
		return err
	}
	if len(args) == 1 && args[0] == "describe" {
		for _, entry := range config.Registry {
			kind := "setting"
			if entry.Secret {
				kind = "secret"
			}
			if entry.Ephemeral {
				kind = "ephemeral secret"
			}
			requirement := "optional"
			if entry.Required {
				requirement = "required"
			}
			if entry.Default != "" {
				requirement += ", default=" + entry.Default
			}
			if _, err := fmt.Fprintf(out, "%s\t%s\t%s\t%s\n", entry.Key, kind, requirement, entry.Description); err != nil {
				return err
			}
		}
		return nil
	}
	if len(args) == 2 && args[0] == "help" {
		entry, ok := config.Lookup(args[1])
		if !ok {
			return fmt.Errorf("unknown configuration key %q", args[1])
		}
		if entry.SetupURL == "" {
			return fmt.Errorf("%s has no setup URL", args[1])
		}
		_, err = fmt.Fprintf(out, "%s\n", entry.SetupURL)
		return err
	}
	if same(args, []string{"trello"}) || same(args, []string{"trello", "--help"}) || same(args, []string{"trello", "help"}) {
		_, err = fmt.Fprint(out, trelloConfigUsage)
		return err
	}
	if same(args, []string{"tailscale", "--help"}) || same(args, []string{"tailscale", "help"}) {
		_, err = fmt.Fprint(out, tailscaleConfigUsage)
		return err
	}
	if err := config.Initialize(p); err != nil {
		return fmt.Errorf("initialize configuration: %w", err)
	}
	if same(args, []string{"init"}) {
		_, err = fmt.Fprintf(out, "Initialized:\n  %s\n  %s\n\nSee docs/credentials.md for service setup.\n", p.Settings, p.Credentials)
		return err
	}
	if len(args) > 0 && args[0] == "trello" {
		return configureTrello(p, args[1:], out)
	}
	if len(args) > 0 && args[0] == "tailscale" {
		return configureTailscale(p, args[1:], out)
	}
	if len(args) == 3 && args[0] == "set" {
		entry, ok := config.Lookup(args[1])
		if !ok {
			return fmt.Errorf("unknown configuration key %q", args[1])
		}
		if entry.Secret {
			return fmt.Errorf("%s is a secret; use configure secret set", args[1])
		}
		if err := config.Set(p.Settings, args[1], args[2]); err != nil {
			return err
		}
		_, err = fmt.Fprintf(out, "Set %s in %s\n", args[1], p.Settings)
		return err
	}
	if len(args) == 2 && args[0] == "delete" {
		entry, ok := config.Lookup(args[1])
		if !ok {
			return fmt.Errorf("unknown configuration key %q", args[1])
		}
		if entry.Secret {
			return fmt.Errorf("%s is a secret; use configure secret delete", args[1])
		}
		deleted, err := config.Delete(p.Settings, args[1])
		if err != nil {
			return err
		}
		if deleted {
			_, err = fmt.Fprintf(out, "Deleted %s from %s\n", args[1], p.Settings)
		} else {
			_, err = fmt.Fprintf(out, "%s was already absent from %s\n", args[1], p.Settings)
		}
		return err
	}
	if len(args) == 4 && args[0] == "secret" && args[1] == "set" {
		entry, ok := config.Lookup(args[2])
		if !ok {
			return fmt.Errorf("unknown configuration key %q", args[2])
		}
		if !entry.Secret {
			return fmt.Errorf("%s is not a secret; use configure set", args[2])
		}
		if entry.Ephemeral {
			return fmt.Errorf("%s is managed automatically and cannot be set", args[2])
		}
		if err := config.Set(p.Credentials, args[2], args[3]); err != nil {
			return err
		}
		if err := config.Initialize(p); err != nil {
			return err
		}
		_, err = fmt.Fprintf(out, "Set secret %s in %s\n", args[2], p.Credentials)
		return err
	}
	if len(args) == 3 && args[0] == "secret" && args[1] == "delete" {
		entry, ok := config.Lookup(args[2])
		if !ok {
			return fmt.Errorf("unknown configuration key %q", args[2])
		}
		if !entry.Secret {
			return fmt.Errorf("%s is not a secret; use configure delete", args[2])
		}
		path := p.Credentials
		if entry.Ephemeral {
			ephemeral, err := config.DefaultEphemeralPaths()
			if err != nil {
				return err
			}
			path = ephemeral.Credentials
		}
		deleted, err := config.Delete(path, args[2])
		if err != nil {
			return err
		}
		if deleted {
			_, err = fmt.Fprintf(out, "Deleted secret %s from %s\n", args[2], path)
		} else {
			_, err = fmt.Fprintf(out, "%s was already absent from %s\n", args[2], path)
		}
		return err
	}
	if len(args) == 6 && same(args[:2], []string{"trello", "auth"}) && args[2] == "--api-key" && args[4] == "--api-token" {
		if err := config.Set(p.Credentials, "trello.api-key", args[3]); err != nil {
			return err
		}
		if err := config.Set(p.Credentials, "trello.api-token", args[5]); err != nil {
			return err
		}
		if err := config.Initialize(p); err != nil {
			return err
		}
		_, err = fmt.Fprintf(out, "Stored Trello credentials in %s\n", p.Credentials)
		return err
	}
	return fmt.Errorf("unknown configure command; use 'omni configure --help'")
}

func configureTailscale(p config.Paths, args []string, out io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("no Tailscale values supplied; run 'omni configure tailscale --help'")
	}
	settings, secrets, seen := map[string]string{}, map[string]string{}, map[string]bool{}
	for len(args) > 0 {
		if len(args) < 2 || !strings.HasPrefix(args[0], "--") {
			return fmt.Errorf("expected --OPTION VALUE; run 'omni configure tailscale --help'")
		}
		option, value := args[0], args[1]
		if seen[option] {
			return fmt.Errorf("%s specified more than once", option)
		}
		seen[option] = true
		switch option {
		case "--tailnet":
			settings["tailscale.tailnet"] = value
		case "--client-id":
			settings["tailscale.client-id"] = value
		case "--api-url":
			settings["tailscale.api-url"] = value
		case "--api-key":
			secrets["tailscale.api-key"] = value
		case "--client-secret":
			secrets["tailscale.client-secret"] = value
		default:
			return fmt.Errorf("unknown Tailscale option %s; run 'omni configure tailscale --help'", option)
		}
		args = args[2:]
	}
	for key, value := range settings {
		if err := config.Set(p.Settings, key, value); err != nil {
			return err
		}
	}
	for key, value := range secrets {
		if err := config.Set(p.Credentials, key, value); err != nil {
			return err
		}
	}
	if len(secrets) > 0 {
		if err := config.Initialize(p); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintln(out, "Stored Tailscale configuration. Secret values were not displayed.")
	return err
}

func configureTrello(p config.Paths, args []string, out io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("no Trello values supplied; run 'omni configure trello --help'")
	}
	settings := map[string]string{}
	secrets := map[string]string{}
	seen := map[string]bool{}
	for len(args) > 0 {
		if len(args) < 2 || !strings.HasPrefix(args[0], "--") {
			return fmt.Errorf("expected --OPTION VALUE; run 'omni configure trello --help'")
		}
		option, value := args[0], args[1]
		switch option {
		case "--default-board", "--default-board-id":
			if seen["default-board"] {
				return fmt.Errorf("%s specified more than once", option)
			}
			seen["default-board"] = true
			settings["trello.default-board-id"] = value
		case "--api-url":
			if seen[option] {
				return fmt.Errorf("%s specified more than once", option)
			}
			seen[option] = true
			settings["trello.api-url"] = value
		case "--api-key":
			if seen[option] {
				return fmt.Errorf("%s specified more than once", option)
			}
			seen[option] = true
			secrets["trello.api-key"] = value
		case "--api-token":
			if seen[option] {
				return fmt.Errorf("%s specified more than once", option)
			}
			seen[option] = true
			secrets["trello.api-token"] = value
		default:
			return fmt.Errorf("unknown Trello option %s; run 'omni configure trello --help'", option)
		}
		args = args[2:]
	}
	for key, value := range settings {
		if err := config.Set(p.Settings, key, value); err != nil {
			return err
		}
	}
	for key, value := range secrets {
		if err := config.Set(p.Credentials, key, value); err != nil {
			return err
		}
	}
	if len(secrets) > 0 {
		if err := config.Initialize(p); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintln(out, "Stored Trello configuration. Secret values were not displayed.")
	return err
}

func setup(args []string, out io.Writer) error {
	if len(args) != 1 {
		return fmt.Errorf("unknown service; available setup: tailscale, trello")
	}
	if args[0] == "tailscale" {
		oauth, _ := config.Lookup("tailscale.client-secret")
		_, err := fmt.Fprintf(out, "Tailscale setup\n\nChoose one authentication method:\n\n1. API access token — simple, broad tailnet access, expires in 1–90 days:\n   https://tailscale.com/docs/reference/tailscale-api\n   omni configure tailscale --api-key ACCESS_TOKEN\n\n2. OAuth client — scoped, short-lived access tokens minted automatically:\n   %s\n   omni configure tailscale --client-id CLIENT_ID --client-secret CLIENT_SECRET\n\nIf both are configured, Omni uses the explicit API access token. OAuth-minted tokens are cached in secured XDG data storage until shortly before expiry. The tailnet defaults to - (the credential's owning tailnet).\n\nStart with an observe command. Replacing tags and applying policy are separate authorization and administration operations.\n", oauth.SetupURL)
		return err
	}
	if args[0] != "trello" {
		return fmt.Errorf("unknown service; available setup: tailscale, trello")
	}
	key, _ := config.Lookup("trello.api-key")
	_, err := fmt.Fprintf(out, "Trello setup\n\nStart with the local configuration guide:\n   omni configure trello\n\n1. Read the API overview:\n   %s\n\n2. Follow Trello's app-management walkthrough to create the minimal Power-Up required for an API key:\n   https://developer.atlassian.com/cloud/trello/guides/power-ups/managing-apps/\n\n3. Open the Trello app-management page directly:\n   https://trello.com/apps/admin\n\n4. Open the app's API Key tab and click Generate a Token. Approve access, copy the user token, then configure Omni:\n   omni configure trello --default-board BOARD_ID --api-key API_KEY --api-token API_TOKEN\n\nThis is Omni's current agent-oriented setup.\nThe Power-Up only supplies the API key; its app secret is not needed.\nThe board ID is optional; omit --default-board if you prefer to specify it per command.\n", key.SetupURL)
	return err
}

func describe(args []string, out io.Writer) error {
	filtered, format, err := extractOutput(args, 0)
	if err != nil {
		return err
	}
	jsonFormat := format == output.JSON
	if len(filtered) == 0 {
		return describeOverview(jsonFormat, out)
	}
	if len(filtered) == 1 && hasService(filtered[0]) {
		return describeService(filtered[0], jsonFormat, out)
	}
	if len(filtered) > 0 {
		d, err := command.Find(filtered)
		if err != nil {
			return err
		}
		if jsonFormat {
			return json.NewEncoder(out).Encode(operationInfoFor(d))
		}
		return describeOperation(d, out)
	}
	return fmt.Errorf("unknown service or command %q", strings.Join(filtered, " "))
}

func operationOutput(args []string) ([]string, output.Format, error) {
	if len(args) == 0 {
		return nil, "", fmt.Errorf("missing effect")
	}
	return extractOutput(args, 1)
}

func extractOutput(args []string, start int) ([]string, output.Format, error) {
	format, err := defaultOutput()
	if err != nil {
		return nil, "", err
	}
	filtered := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		value := args[i]
		if i >= start && value == "--format" {
			if i+1 == len(args) {
				return nil, "", fmt.Errorf("--format requires text or json")
			}
			i++
			format, err = output.Parse(args[i])
			if err != nil {
				return nil, "", err
			}
			continue
		}
		if i >= start && strings.HasPrefix(value, "--format=") {
			format, err = output.Parse(strings.TrimPrefix(value, "--format="))
			if err != nil {
				return nil, "", err
			}
			continue
		}
		if i >= start && value == "--json" {
			return nil, "", fmt.Errorf("use --format json")
		}
		filtered = append(filtered, value)
	}
	return filtered, format, nil
}

func defaultOutput() (output.Format, error) {
	value := os.Getenv("OMNI_OUTPUT")
	if value == "" {
		return output.Text, nil
	}
	return output.Parse(value)
}

type operationInfo struct {
	OperationID         string              `json:"operation_id"`
	Command             []string            `json:"command"`
	Effect              command.Effect      `json:"effect"`
	Summary             string              `json:"summary"`
	Notes               []string            `json:"notes,omitempty"`
	ResponseDescription string              `json:"response_description"`
	Cardinality         command.Cardinality `json:"cardinality"`
	Reversible          bool                `json:"reversible"`
	Reversal            string              `json:"reversal,omitempty"`
	Credentials         string              `json:"credentials"`
	UnattendedOK        bool                `json:"unattended_ok"`
	Arguments           []command.Argument  `json:"arguments"`
	Options             []command.Option    `json:"options"`
}

func operationInfoFor(d command.Definition) operationInfo {
	arguments := d.Arguments
	if arguments == nil {
		arguments = []command.Argument{}
	}
	options := d.Options
	if options == nil {
		options = []command.Option{}
	}
	return operationInfo{OperationID: d.OperationID(), Command: d.Tokens(), Effect: d.Effect, Summary: d.Summary, Notes: d.Notes, ResponseDescription: d.Response, Cardinality: d.Cardinality, Reversible: d.Reversible, Reversal: d.Reversal, Credentials: d.Credentials, UnattendedOK: d.UnattendedOK, Arguments: arguments, Options: options}
}

type serviceDescription struct {
	Service    string          `json:"service"`
	Summary    string          `json:"summary"`
	Operations []operationInfo `json:"operations"`
}

func describeOverview(jsonFormat bool, out io.Writer) error {
	services := serviceNames()
	if jsonFormat {
		items := make([]map[string]any, 0, len(services))
		for _, service := range services {
			items = append(items, map[string]any{"service": service, "describe": []string{"describe", service}, "operations": len(serviceOperations(service))})
		}
		return json.NewEncoder(out).Encode(map[string]any{"services": items, "discovery": []string{"omni describe SERVICE", "omni describe EFFECT SERVICE RESOURCE VERB"}})
	}
	if _, err := fmt.Fprintln(out, "Omni capability discovery\n\nServices:"); err != nil {
		return err
	}
	for _, service := range services {
		if _, err := fmt.Fprintf(out, "  %-12s %d operations  → omni describe %s\n", service, len(serviceOperations(service)), service); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintln(out, "\nDiscover a service:  omni describe SERVICE\nInspect one operation: omni describe EFFECT SERVICE RESOURCE VERB\nMachine-readable output: omni describe [SERVICE] --format=json")
	return err
}

func describeService(service string, jsonFormat bool, out io.Writer) error {
	operations := serviceOperations(service)
	detail := serviceDescription{Service: service, Summary: serviceSummary(service), Operations: make([]operationInfo, 0, len(operations))}
	for _, d := range operations {
		detail.Operations = append(detail.Operations, operationInfoFor(d))
	}
	if jsonFormat {
		return json.NewEncoder(out).Encode(detail)
	}
	upper := strings.ToUpper(service)
	if _, err := fmt.Fprintf(out, "%s(1)                         Omni Manual                        %s(1)\n\nNAME\n       omni-%s - %s\n\nSYNOPSIS\n", upper, upper, service, serviceSummary(service)); err != nil {
		return err
	}
	for _, d := range operations {
		if _, err := fmt.Fprintf(out, "       omni %s\n", synopsis(d)); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(out, "\nDESCRIPTION\n       %s Omni command paths are action-first: the first token after\n       omni identifies the operation effect, and arguments or options never\n       change that effect.\n\nCOMMANDS\n", serviceSummary(service)); err != nil {
		return err
	}
	for _, d := range operations {
		if err := writeCommandDetails(d, out); err != nil {
			return err
		}
	}
	if service == "trello" {
		_, err := fmt.Fprint(out, "CONFIGURATION\n       Get Trello credentials and configuration guidance:\n\n               omni setup trello\n\n       Configure settings and credentials in one command:\n\n               omni configure trello [--default-board BOARD_ID] [--api-key API_KEY]\n                       [--api-token API_TOKEN] [--api-url URL]\n\n       The API key and token are stored as local secrets; the default board and\n       API URL are ordinary local settings.\n")
		return err
	}
	if service == "tailscale" {
		_, err := fmt.Fprint(out, "CONFIGURATION\n       See both authentication choices:\n\n               omni setup tailscale\n\n       API-token path (simple, broad):\n\n               omni configure tailscale --api-key ACCESS_TOKEN\n\n       OAuth-client path (scoped):\n\n               omni configure tailscale --client-id CLIENT_ID --client-secret CLIENT_SECRET\n\n       If both are configured, the API token wins. OAuth-minted access tokens\n       are cached in secured XDG data storage until shortly before expiry. Client\n       ID, tailnet, and API URL are ordinary settings; the client secret and API\n       token are local secrets.\n")
		return err
	}
	return nil
}

func describeOperation(d command.Definition, out io.Writer) error {
	title := strings.ToUpper(strings.Join(d.Tokens(), "-"))
	if _, err := fmt.Fprintf(out, "%s(1)                         Omni Manual                        %s(1)\n\nNAME\n       omni-%s - %s\n\nSYNOPSIS\n       omni %s\n\n", title, title, strings.Join(d.Tokens(), "-"), d.Summary, synopsis(d)); err != nil {
		return err
	}
	if len(d.Notes) > 0 {
		if _, err := fmt.Fprintln(out, "NOTES"); err != nil {
			return err
		}
		for _, note := range d.Notes {
			if _, err := fmt.Fprintf(out, "       %s\n", note); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintln(out); err != nil {
			return err
		}
	}
	if err := writeArgumentsOptions(d, out); err != nil {
		return err
	}
	_, err := fmt.Fprintf(out, "RETURNS\n       %s\n", d.Response)
	return err
}

func writeCommandDetails(d command.Definition, out io.Writer) error {
	if _, err := fmt.Fprintf(out, "       omni %s\n           %s\n", synopsis(d), d.Summary); err != nil {
		return err
	}
	if len(d.Notes) > 0 {
		if _, err := fmt.Fprintln(out, "           Notes:"); err != nil {
			return err
		}
		for _, note := range d.Notes {
			if _, err := fmt.Fprintf(out, "               %s\n", note); err != nil {
				return err
			}
		}
	}
	if len(d.Arguments) > 0 {
		if _, err := fmt.Fprintln(out, "           Arguments:"); err != nil {
			return err
		}
		for _, arg := range d.Arguments {
			if _, err := fmt.Fprintf(out, "               %s\n                   %s\n", arg.Name, arg.Description); err != nil {
				return err
			}
		}
	}
	if len(d.Options) > 0 {
		if _, err := fmt.Fprintln(out, "           Options:"); err != nil {
			return err
		}
		for _, option := range d.Options {
			name := option.Name
			if option.Value != "" {
				name += " " + option.Value
			}
			if _, err := fmt.Fprintf(out, "               %s\n                   %s\n", name, option.Description); err != nil {
				return err
			}
		}
	}
	_, err := fmt.Fprintf(out, "           Returns: %s\n\n", d.Response)
	return err
}

func writeArgumentsOptions(d command.Definition, out io.Writer) error {
	if len(d.Arguments) > 0 {
		if _, err := fmt.Fprintln(out, "ARGUMENTS"); err != nil {
			return err
		}
		for _, arg := range d.Arguments {
			if _, err := fmt.Fprintf(out, "       %s\n           %s\n", arg.Name, arg.Description); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintln(out); err != nil {
			return err
		}
	}
	if len(d.Options) > 0 {
		if _, err := fmt.Fprintln(out, "OPTIONS"); err != nil {
			return err
		}
		for _, option := range d.Options {
			name := option.Name
			if option.Value != "" {
				name += " " + option.Value
			}
			if _, err := fmt.Fprintf(out, "       %s\n           %s\n", name, option.Description); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintln(out); err != nil {
			return err
		}
	}
	return nil
}

func synopsis(d command.Definition) string {
	parts := append([]string{}, d.Tokens()...)
	for _, arg := range d.Arguments {
		value := arg.Name
		if arg.Variadic {
			value += "..."
		}
		if arg.Optional {
			parts = append(parts, "["+value+"]")
		} else {
			parts = append(parts, value)
		}
	}
	for _, option := range d.Options {
		value := option.Name
		if option.Value != "" {
			value += " " + option.Value
		}
		if option.Optional {
			value = "[" + value + "]"
		}
		parts = append(parts, value)
	}
	return strings.Join(parts, " ")
}

func serviceNames() []string {
	seen := map[string]bool{}
	var names []string
	for _, d := range command.Registry {
		if !seen[d.Path[0]] {
			seen[d.Path[0]] = true
			names = append(names, d.Path[0])
		}
	}
	return names
}
func hasService(service string) bool {
	for _, name := range serviceNames() {
		if name == service {
			return true
		}
	}
	return false
}
func serviceOperations(service string) []command.Definition {
	var operations []command.Definition
	for _, d := range command.Registry {
		if d.Path[0] == service {
			operations = append(operations, d)
		}
	}
	return operations
}
func serviceSummary(service string) string {
	if service == "trello" {
		return "List and change Trello boards, lists, and cards."
	}
	if service == "tailscale" {
		return "Inspect and deliberately administer a Tailscale tailnet."
	}
	return ""
}

func explain(args []string, out io.Writer) error {
	d, err := command.Find(args)
	if err != nil {
		return err
	}
	approval := "requires interactive approval"
	if d.UnattendedOK {
		approval = "eligible for safe-unattended approval"
	}
	reversal := "not automatically reversible"
	if d.Reversible {
		reversal = d.Reversal
		if reversal == "" {
			reversal = "the service operation has a compensating or non-destructive path"
		}
	}
	_, err = fmt.Fprintf(out, "%s\neffect: %s\ncardinality: %s\nreversible: %t\nreversal: %s\ncredentials: %s\napproval: %s\n", d.Name(), d.Effect, d.Cardinality, d.Reversible, reversal, d.Credentials, approval)
	return err
}

func same(a, b []string) bool { return strings.Join(a, "\x00") == strings.Join(b, "\x00") }
