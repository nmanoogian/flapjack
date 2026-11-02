// Package cli implements the fj subcommands.
package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/nmanoogian/flapjack/internal/gh"
	"github.com/nmanoogian/flapjack/internal/git"
)

const usage = `fj manages stacked PRs built from a single WIP branch.

Usage:
  fj <command> [flags]

Commands:
`

type App struct {
	Repo git.Repo
	GH   gh.Client
	Out  io.Writer
}

type command struct {
	name    string
	summary string
	run     func(app *App, args []string) error
}

var commands = []command{
	{"status", "show the stack's heads, PRs and orphaned PRs", runStatus},
	{"mark", "make a commit the head of a PR, or rename a head", runMark},
	{"unmark", "remove a head from the stack", runUnmark},
	{"push", "update PR branches from their head commits and create or retarget PRs", runPush},
}

func Main(args []string) error {
	dir, err := os.Getwd()
	if err != nil {
		return err
	}
	app := &App{Repo: git.Repo{Dir: dir}, GH: gh.CLI{Dir: dir}, Out: os.Stdout}
	return app.Run(args)
}

func (app *App) Run(args []string) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		printUsage(app.Out)
		return nil
	}
	for _, c := range commands {
		if c.name == args[0] {
			return c.run(app, args[1:])
		}
	}
	printUsage(os.Stderr)
	return fmt.Errorf("unknown command %q", args[0])
}

func printUsage(w io.Writer) {
	fmt.Fprint(w, usage)
	for _, c := range commands {
		fmt.Fprintf(w, "  %-8s %s\n", c.name, c.summary)
	}
}

// parseFlags parses a subcommand's flags, treating -h as success.
func parseFlags(fs *flag.FlagSet, args []string) (help bool, err error) {
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return true, nil
		}
		return false, err
	}
	return false, nil
}
