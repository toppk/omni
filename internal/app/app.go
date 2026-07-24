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
	"github.com/toppk/omni/internal/policy"
	"github.com/toppk/omni/internal/trello"
)

// Version is set by the release workflow with Go linker flags. Local builds
// intentionally retain the development value rather than a checked-in version.
var Version = "dev"

const usage = `Omni is a safety-oriented CLI for service APIs.

Usage:
  omni setup SERVICE
  omni configure SERVICE [OPTIONS]
  omni configure describe
  omni configure set SECTION.KEY VALUE
  omni configure secret set SECTION.KEY VALUE
  omni version
  omni describe [<effect> <service> <resource> <verb>] [--format json]
  omni policy explain <effect> <service> <resource> <verb>
  omni [--policy MODE] <effect> <service> <resource> <verb>

The effect is always the first token after "omni": observe, create, update,
move, archive, delete, execute, transfer, authorize, or administer.
Arguments and flags may identify resources or format output, but may not change
an operation's effect. Use "omni describe --format json" for the registry.

Run "omni setup SERVICE" for service-specific credentials and configuration.
`

const trelloConfigUsage = `Trello configuration

Usage:
  omni configure trello [--default-board BOARD_ID] [--api-key API_KEY] [--api-token API_TOKEN] [--api-url URL]

All supplied values are stored at once. --default-board and --api-url are
ordinary settings; --api-key and --api-token are secrets. Secret values are
never printed by Omni, but command-line values can remain in shell history.
`

const configureUsage = `Configure Omni

Usage:
  omni configure SERVICE [OPTIONS]
  omni configure describe
  omni configure help SECTION.KEY

For service-specific options, run "omni configure SERVICE --help".
`

func Run(_ context.Context, args []string, out, errOut io.Writer) error {
	if len(args) == 1 && (args[0] == "version" || args[0] == "--version" || args[0] == "-V") {
		_, err := fmt.Fprintf(out, "omni %s\n", Version)
		return err
	}
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		_, err := fmt.Fprint(out, usage)
		return err
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
		return trello.Execute(d, operands, credentials, settings, out)
	}
	return fmt.Errorf("%s is registered but not implemented yet", d.Name())
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
	if same(args, []string{"trello", "--help"}) || same(args, []string{"trello", "help"}) {
		_, err = fmt.Fprint(out, trelloConfigUsage)
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
	if len(args) == 4 && args[0] == "secret" && args[1] == "set" {
		entry, ok := config.Lookup(args[2])
		if !ok {
			return fmt.Errorf("unknown configuration key %q", args[2])
		}
		if !entry.Secret {
			return fmt.Errorf("%s is not a secret; use configure set", args[2])
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
	if len(args) != 1 || args[0] != "trello" {
		return fmt.Errorf("unknown service; available setup: trello")
	}
	key, _ := config.Lookup("trello.api-key")
	_, err := fmt.Fprintf(out, "Trello setup\n\n1. Get an API key and user token from:\n   %s\n\n2. Configure Omni in one command:\n   omni configure trello --default-board BOARD_ID --api-key API_KEY --api-token API_TOKEN\n\nThe board ID is optional; omit --default-board if you prefer to specify it per command.\n", key.SetupURL)
	return err
}

func describe(args []string, out io.Writer) error {
	jsonFormat := false
	filtered := make([]string, 0, len(args))
	for _, a := range args {
		if a == "--format=json" {
			jsonFormat = true
		} else if a == "--format" {
			return fmt.Errorf("use --format=json")
		} else {
			filtered = append(filtered, a)
		}
	}
	if jsonFormat {
		var value any = command.Registry
		if len(filtered) > 0 {
			d, err := command.Find(filtered)
			if err != nil {
				return err
			}
			value = d
		}
		return json.NewEncoder(out).Encode(value)
	}
	if len(filtered) > 0 {
		d, err := command.Find(filtered)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(out, "%s\n  effect: %s\n  %s\n", d.Name(), d.Effect, d.Summary)
		return err
	}
	for _, d := range command.Registry {
		if _, err := fmt.Fprintf(out, "%-43s %s\n", d.Name(), d.Summary); err != nil {
			return err
		}
	}
	return nil
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
	_, err = fmt.Fprintf(out, "%s\neffect: %s\ncardinality: %s\nreversible: %t\ncredentials: %s\napproval: %s\n", d.Name(), d.Effect, d.Cardinality, d.Reversible, d.Credentials, approval)
	return err
}

func same(a, b []string) bool { return strings.Join(a, "\x00") == strings.Join(b, "\x00") }
