// Package command defines Omni's security-relevant command schema.
package command

import (
	"fmt"
	"strings"
)

// Effect is the only action-bearing token immediately after the omni binary.
// Adding a flag must never change this classification.
type Effect string

const (
	Observe    Effect = "observe"
	Create     Effect = "create"
	Update     Effect = "update"
	Move       Effect = "move"
	Archive    Effect = "archive"
	Delete     Effect = "delete"
	Execute    Effect = "execute"
	Transfer   Effect = "transfer"
	Authorize  Effect = "authorize"
	Administer Effect = "administer"
	Unbounded  Effect = "unbounded"
)

type Cardinality string

const (
	One  Cardinality = "one"
	Many Cardinality = "many"
)

type Argument struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Optional    bool   `json:"optional"`
	Variadic    bool   `json:"variadic,omitempty"`
}

type Option struct {
	Name        string `json:"name"`
	Value       string `json:"value"`
	Description string `json:"description"`
	Optional    bool   `json:"optional"`
}

// Definition is the source of truth for help, discovery, and policy checks.
// Path excludes the effect because Effect is always the first command token.
type Definition struct {
	Effect       Effect      `json:"effect"`
	Path         []string    `json:"path"`
	Summary      string      `json:"summary"`
	Notes        []string    `json:"notes,omitempty"`
	Response     string      `json:"response_description"`
	Arguments    []Argument  `json:"arguments"`
	Options      []Option    `json:"options"`
	Cardinality  Cardinality `json:"cardinality"`
	Reversible   bool        `json:"reversible"`
	Reversal     string      `json:"reversal,omitempty"`
	Credentials  string      `json:"credentials"`
	UnattendedOK bool        `json:"unattended_ok"`
}

func (d Definition) Tokens() []string    { return append([]string{string(d.Effect)}, d.Path...) }
func (d Definition) Name() string        { return strings.Join(d.Tokens(), " ") }
func (d Definition) OperationID() string { return "omni." + strings.Join(d.Tokens(), ".") }

func (d Definition) Validate() error {
	if d.Effect == "" || d.Effect == Unbounded {
		return fmt.Errorf("invalid or unbounded effect for %q", d.Name())
	}
	if len(d.Path) < 2 {
		return fmt.Errorf("command %q needs service and resource", d.Name())
	}
	if d.Summary == "" {
		return fmt.Errorf("command %q needs a summary", d.Name())
	}
	if d.Response == "" {
		return fmt.Errorf("command %q needs a response description", d.Name())
	}
	for _, note := range d.Notes {
		if strings.TrimSpace(note) == "" {
			return fmt.Errorf("command %q has an empty note", d.Name())
		}
	}
	if d.UnattendedOK && d.Effect != Observe {
		return fmt.Errorf("unattended command %q must be observe", d.Name())
	}
	if d.Reversible && d.Effect != Observe && d.Reversal == "" {
		return fmt.Errorf("reversible mutating command %q needs a reversal explanation", d.Name())
	}
	if !d.Reversible && d.Reversal != "" {
		return fmt.Errorf("non-reversible command %q cannot declare a reversal", d.Name())
	}
	return nil
}
