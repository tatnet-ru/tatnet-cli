// Regenerate the CLI contract without losing raw-HTTP operations not yet
// released in the pinned SDK. Invoked by ../sync-contract.sh from the repo root.
package main

import (
	"fmt"
	"github.com/tatnet-ru/tatnet-cli/internal/contract"
	"os"
)

func main() {
	if len(os.Args) != 2 {
		fail(fmt.Errorf("usage: sync-contract <sdk-openapi.json>"))
	}
	sdk, err := os.ReadFile(os.Args[1])
	if err != nil {
		fail(err)
	}
	extension, err := os.ReadFile("internal/contract/supabase-extension.json")
	if err != nil {
		fail(err)
	}
	combined, err := contract.Compose(sdk, extension)
	if err != nil {
		fail(err)
	}
	if err := os.WriteFile("internal/contract/v1.json", combined, 0644); err != nil {
		fail(err)
	}
}
func fail(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
