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

// Definition is the source of truth for help, discovery, and policy checks.
// Path excludes the effect because Effect is always the first command token.
type Definition struct {
	Effect       Effect      `json:"effect"`
	Path         []string    `json:"path"`
	Summary      string      `json:"summary"`
	Description  string      `json:"description"`
	Response     string      `json:"response_description"`
	Cardinality  Cardinality `json:"cardinality"`
	Reversible   bool        `json:"reversible"`
	Credentials  string      `json:"credentials"`
	UnattendedOK bool        `json:"unattended_ok"`
	Status       string      `json:"status"`
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
	if d.Description == "" || d.Response == "" {
		return fmt.Errorf("command %q needs descriptions", d.Name())
	}
	if d.Status != "implemented" && d.Status != "planned" {
		return fmt.Errorf("command %q has invalid status %q", d.Name(), d.Status)
	}
	return nil
}
