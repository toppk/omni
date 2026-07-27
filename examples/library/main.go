// library demonstrates embedding Omni into another Go program. It performs no
// network requests and does not require Omni credentials.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/toppk/omni"
)

func main() {
	operation, err := omni.FindOperation([]string{"observe", "trello", "board", "list"})
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	fmt.Printf("%s is a %s operation: %s\n\n", operation.ID, operation.Effect, operation.Summary)

	if err := omni.Run(context.Background(), []string{"describe", "trello"}, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
