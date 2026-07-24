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

const usage = `Omni is a safety-oriented CLI for service APIs.

Usage:
  omni configure init
	  omni configure set trello.default-board-id BOARD_ID
	  omni configure secret set trello.api-key API_KEY
  omni configure secret set trello.api-token API_TOKEN
  omni configure trello auth --api-key API_KEY --api-token API_TOKEN
  omni configure describe
	  omni configure help SECTION.KEY
  omni describe [<effect> <service> <resource> <verb>] [--format json]
  omni policy explain <effect> <service> <resource> <verb>
  omni [--policy MODE] <effect> <service> <resource> <verb>

The effect is always the first token after "omni": observe, create, update,
move, archive, delete, execute, transfer, authorize, or administer.
Arguments and flags may identify resources or format output, but may not change
an operation's effect. Use "omni describe --format json" for the registry.
`

func Run(_ context.Context, args []string, out, errOut io.Writer) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		_, err := fmt.Fprint(out, usage)
		return err
	}
	if len(args) > 0 && args[0] == "configure" {
		return configure(args[1:], out)
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
	if err := config.Initialize(p); err != nil {
		return fmt.Errorf("initialize configuration: %w", err)
	}
	if same(args, []string{"init"}) {
		_, err = fmt.Fprintf(out, "Initialized:\n  %s\n  %s\n\nSee docs/credentials.md for service setup.\n", p.Settings, p.Credentials)
		return err
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
	return fmt.Errorf("unknown configure command; use: configure init | configure set SECTION.KEY VALUE | configure secret set SECTION.KEY VALUE | configure trello auth --api-key VALUE --api-token VALUE")
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
