// Package main starts the mivia-agent CLI.
// Plan: WS0. PRD: §1, §4, §9.
package main

import (
	"fmt"
	"os"

	"github.com/MiviaLabs/mivia-agentkit/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
