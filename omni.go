// Package omni provides Omni's in-process command and capability-discovery API.
//
// It is intended for programs that want Omni's action-first command contract,
// policy enforcement, configuration conventions, and service integrations
// without starting a separate omni process. The command-line binary is a thin
// wrapper around Run.
package omni

import (
	"context"
	"io"

	"github.com/toppk/omni/internal/app"
	"github.com/toppk/omni/internal/command"
)

// Argument describes a positional operation argument.
type Argument struct {
	Name        string
	Description string
	Optional    bool
	Variadic    bool
}

// Option describes an operation option.
type Option struct {
	Name        string
	Value       string
	Description string
	Optional    bool
}

// Operation is Omni's stable, provider-neutral operation description.
// Path excludes Effect; use Tokens when the action-first command path is
// needed.
type Operation struct {
	ID           string
	Effect       string
	Path         []string
	Summary      string
	Notes        []string
	Response     string
	Arguments    []Argument
	Options      []Option
	Cardinality  string
	Reversible   bool
	Reversal     string
	Credentials  string
	UnattendedOK bool
}

// Tokens returns a new action-first command path, suitable for passing to Run.
func (o Operation) Tokens() []string {
	return append([]string{o.Effect}, o.Path...)
}

// Operations returns a copy of every runnable Omni operation. The result is
// safe for callers to inspect or modify.
func Operations() []Operation {
	operations := make([]Operation, len(command.Registry))
	for i, definition := range command.Registry {
		operations[i] = operationFromDefinition(definition)
	}
	return operations
}

// FindOperation returns the operation whose action-first path is tokens.
func FindOperation(tokens []string) (Operation, error) {
	definition, err := command.Find(tokens)
	if err != nil {
		return Operation{}, err
	}
	return operationFromDefinition(definition), nil
}

// Run executes the same command contract as the omni binary. args excludes
// the binary name. Output is written to out and diagnostics to errOut.
func Run(ctx context.Context, args []string, out, errOut io.Writer) error {
	return app.Run(ctx, args, out, errOut)
}

func operationFromDefinition(definition command.Definition) Operation {
	operation := Operation{
		ID:           definition.OperationID(),
		Effect:       string(definition.Effect),
		Path:         append([]string(nil), definition.Path...),
		Summary:      definition.Summary,
		Notes:        append([]string(nil), definition.Notes...),
		Response:     definition.Response,
		Cardinality:  string(definition.Cardinality),
		Reversible:   definition.Reversible,
		Reversal:     definition.Reversal,
		Credentials:  definition.Credentials,
		UnattendedOK: definition.UnattendedOK,
		Arguments:    make([]Argument, len(definition.Arguments)),
		Options:      make([]Option, len(definition.Options)),
	}
	for i, argument := range definition.Arguments {
		operation.Arguments[i] = Argument{Name: argument.Name, Description: argument.Description, Optional: argument.Optional, Variadic: argument.Variadic}
	}
	for i, option := range definition.Options {
		operation.Options[i] = Option{Name: option.Name, Value: option.Value, Description: option.Description, Optional: option.Optional}
	}
	return operation
}
