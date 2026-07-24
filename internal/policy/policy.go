package policy

import (
	"fmt"

	"github.com/toppk/omni/internal/command"
)

// Allows implements restrictive modes. Unknown modes fail closed.
func Allows(mode string, d command.Definition) error {
	switch mode {
	case "", "default":
		return nil
	case "read-only":
		if d.Effect != command.Observe {
			return fmt.Errorf("%s is blocked by read-only policy", d.Name())
		}
	case "no-delete":
		if d.Effect == command.Delete {
			return fmt.Errorf("%s is blocked by no-delete policy", d.Name())
		}
	case "unattended-safe":
		if !d.UnattendedOK {
			return fmt.Errorf("%s requires interactive approval", d.Name())
		}
	default:
		return fmt.Errorf("unknown policy %q", mode)
	}
	return nil
}
