package main

import (
	"os"

	"github.com/nirhuri/dev-agent/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}
