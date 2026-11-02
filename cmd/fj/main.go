// Command fj manages stacked PRs built from a single WIP branch.
package main

import (
	"fmt"
	"os"

	"github.com/nmanoogian/flapjack/internal/cli"
)

func main() {
	if err := cli.Main(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "fj:", err)
		os.Exit(1)
	}
}
