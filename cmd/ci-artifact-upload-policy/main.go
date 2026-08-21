package main

import (
	"fmt"
	"os"

	"github.com/uesugitorachiyo/ao-command/internal/workflowpolicy"
)

func main() {
	if len(os.Args) == 1 {
		fmt.Fprintln(os.Stderr, "usage: ci-artifact-upload-policy WORKFLOW...")
		os.Exit(2)
	}
	if err := workflowpolicy.ValidateFiles(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
