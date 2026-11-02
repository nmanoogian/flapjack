package cli

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/nmanoogian/flapjack/internal/desc"
	"github.com/nmanoogian/flapjack/internal/gh"
	"github.com/nmanoogian/flapjack/internal/stack"
)

const descUsage = `usage: fj desc <command>

PR titles and descriptions live in .git/flapjack/prs/<branch>.md.

Commands:
  path <name>    print the description file for a head, creating it if needed
  sync [<name>...]  sync description files with GitHub in both directions
`

func runDesc(app *App, args []string) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprint(app.Out, descUsage)
		return nil
	}
	switch args[0] {
	case "path":
		return runDescPath(app, args[1:])
	case "sync":
		return runDescSync(app, args[1:])
	}
	fmt.Fprint(os.Stderr, descUsage)
	return fmt.Errorf("unknown desc command %q", args[0])
}

func runDescPath(app *App, args []string) error {
	fs := flag.NewFlagSet("desc path", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "usage: fj desc path <name>\n\nPrints the description file for a head. If it doesn't exist, creates it from the open PR, or from the head commit's message if there's no PR yet.")
	}
	if help, err := parseFlags(fs, args); help || err != nil {
		return err
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return errors.New("desc path takes a name")
	}
	s, err := stack.Load(app.Repo)
	if err != nil {
		return err
	}
	i := s.HeadIndex(fs.Arg(0))
	if i < 0 {
		return fmt.Errorf("no head named %q", fs.Arg(0))
	}
	h := s.Heads[i]
	path, err := app.descPath(h.Branch)
	if err != nil {
		return err
	}
	existing, err := desc.Read(path)
	if err != nil {
		return err
	}
	if existing == nil {
		d, err := app.initialDesc(s, h)
		if err != nil {
			return err
		}
		if err := desc.Write(path, d); err != nil {
			return err
		}
	}
	fmt.Fprintln(app.Out, path)
	return nil
}

// initialDesc returns the open PR's description, or one built from the head
// commit's message if the head has no open PR.
func (app *App) initialDesc(s *stack.Stack, h stack.Head) (*desc.Description, error) {
	prs, err := app.loadPRs(s, []stack.Head{h})
	if err != nil {
		return nil, err
	}
	if pr := prs.byBranch[h.Branch]; pr != nil && pr.State == gh.StateOpen {
		return remoteDesc(pr), nil
	}
	msg, err := stack.Message(app.Repo, h.SHA)
	if err != nil {
		return nil, err
	}
	return &desc.Description{Title: h.Subject, Body: stack.Body(msg)}, nil
}

func remoteDesc(pr *gh.PR) *desc.Description {
	d := &desc.Description{Title: pr.Title, Body: pr.Body, PR: pr.Number}
	d.Synced = d.Hash()
	return d
}

func (app *App) descPath(branch string) (string, error) {
	dir, err := desc.Dir(app.Repo)
	if err != nil {
		return "", err
	}
	return desc.Path(dir, branch), nil
}

const (
	preferLocal  = "local"
	preferRemote = "remote"
)

func runDescSync(app *App, args []string) error {
	fs := flag.NewFlagSet("desc sync", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "usage: fj desc sync [flags] [<name>...]\n\nPushes local edits to GitHub and pulls GitHub edits into the files. When both sides changed, writes GitHub's version next to the file as <branch>.remote.md and stops.\n\nFlags:")
		fs.PrintDefaults()
	}
	prefer := fs.String("prefer", "", "resolve conflicts by keeping the `local` or `remote` version")
	dryRun := fs.Bool("dry-run", false, "print what would change and change nothing")
	if help, err := parseFlags(fs, args); help || err != nil {
		return err
	}
	if *prefer != "" && *prefer != preferLocal && *prefer != preferRemote {
		return fmt.Errorf("--prefer must be %q or %q", preferLocal, preferRemote)
	}

	s, err := stack.Load(app.Repo)
	if err != nil {
		return err
	}
	selected, err := selectHeads(s, fs.Args())
	if err != nil {
		return err
	}
	var heads []stack.Head
	for _, i := range selected {
		heads = append(heads, s.Heads[i])
	}
	prs, err := app.loadPRs(s, heads)
	if err != nil {
		return err
	}
	conflicts := 0
	for _, h := range heads {
		pr := prs.byBranch[h.Branch]
		if pr == nil || pr.State != gh.StateOpen {
			fmt.Fprintf(app.Out, "skipped %s: no open PR\n", h.Name)
			continue
		}
		conflict, err := app.syncDesc(h, pr, *prefer, *dryRun)
		if err != nil {
			return err
		}
		if conflict {
			conflicts++
		}
	}
	if conflicts > 0 {
		return fmt.Errorf("%d %s; compare each file with its .remote.md, then rerun with --prefer local or --prefer remote",
			conflicts, plural(conflicts, "conflict", "conflicts"))
	}
	return nil
}

// syncDesc does a three-way sync between the local file, the PR, and the
// version recorded at the last sync.
func (app *App) syncDesc(h stack.Head, pr *gh.PR, prefer string, dryRun bool) (conflict bool, err error) {
	path, err := app.descPath(h.Branch)
	if err != nil {
		return false, err
	}
	local, err := desc.Read(path)
	if err != nil {
		return false, err
	}
	remote := remoteDesc(pr)
	label := fmt.Sprintf("%s (#%d)", h.Name, pr.Number)

	pull := func() error {
		fmt.Fprintf(app.Out, "%s %s\n", verb(dryRun, "pull", "pulled"), label)
		if dryRun {
			return nil
		}
		return app.writeSynced(path, remote)
	}
	push := func() error {
		fmt.Fprintf(app.Out, "%s %s\n", verb(dryRun, "push", "pushed"), label)
		if dryRun {
			return nil
		}
		if err := app.GH.SetDescription(pr.Number, local.Title, local.Body); err != nil {
			return err
		}
		return app.writeSynced(path, &desc.Description{Title: local.Title, Body: local.Body, PR: pr.Number, Synced: local.Hash()})
	}

	if local == nil {
		return false, pull()
	}
	localHash, remoteHash := local.Hash(), remote.Synced
	trackedPR := local.PR == pr.Number && local.Synced != ""
	switch {
	case localHash == remoteHash:
		if dryRun || (local.PR == pr.Number && local.Synced == localHash) {
			return false, nil
		}
		return false, app.writeSynced(path, remote)
	case trackedPR && localHash == local.Synced:
		return false, pull()
	case trackedPR && remoteHash == local.Synced:
		return false, push()
	case prefer == preferLocal:
		return false, push()
	case prefer == preferRemote:
		return false, pull()
	}
	fmt.Fprintf(app.Out, "conflict %s: local and GitHub versions both changed\n", label)
	if dryRun {
		return true, nil
	}
	return true, desc.Write(desc.RemotePath(path), remote)
}

// writeSynced writes d and removes any conflict copy left by an earlier sync.
func (app *App) writeSynced(path string, d *desc.Description) error {
	if err := desc.Write(path, d); err != nil {
		return err
	}
	if err := os.Remove(desc.RemotePath(path)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func verb(dryRun bool, present, past string) string {
	if dryRun {
		return "would " + present
	}
	return past
}
