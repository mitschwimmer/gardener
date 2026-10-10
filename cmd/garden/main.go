package main

import (
	"fmt"
	"os"

	"github.com/mitschwimmer/gardener/internal/cli"
)

func main() {
	if err := cli.Run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "garden: %v\n", err)
		os.Exit(1)
	}
}
