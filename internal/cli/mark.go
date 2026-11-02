package cli

import (
	"errors"
	"flag"
	"fmt"
	"slices"

	"github.com/nmanoogian/flapjack/internal/stack"
)

func runMark(app *App, args []string) error {
	fs := flag.NewFlagSet("mark", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "usage: fj mark <rev> <name>\n\nMakes <rev> the head commit of PR <name>. Marking a head again renames it.")
	}
	if help, err := parseFlags(fs, args); help || err != nil {
		return err
	}
	if fs.NArg() != 2 {
		fs.Usage()
		return errors.New("mark takes a revision and a name")
	}
	rev, name := fs.Arg(0), fs.Arg(1)

	s, err := app.loadForRewrite()
	if err != nil {
		return err
	}
	sha, ok, err := app.Repo.RefSHA(rev)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("unknown revision %q", rev)
	}
	idx := s.CommitIndex(sha)
	if idx < 0 {
		return fmt.Errorf("%s is not in %s..%s", rev, s.Base, s.WIP)
	}
	c := s.Commits[idx]
	if c.IsFixup() {
		return fmt.Errorf("%s is a fixup commit; mark the commit it fixes", rev)
	}
	if err := stack.ValidateName(app.Repo, s.WIP, name); err != nil {
		return err
	}
	if i := s.HeadIndex(name); i >= 0 && s.Heads[i].SHA != sha {
		return fmt.Errorf("%q already marks %s %s", name, short(s.Heads[i].SHA), s.Heads[i].Subject)
	}
	if slices.Equal(c.Names, []string{name}) {
		fmt.Fprintf(app.Out, "%s %s is already marked %s\n", short(sha), c.Subject, name)
		return nil
	}

	msg, err := stack.Message(app.Repo, sha)
	if err != nil {
		return err
	}
	newMsg, err := stack.SetTrailer(app.Repo, msg, name)
	if err != nil {
		return err
	}
	if err := stack.Reword(app.Repo, s, sha, newMsg); err != nil {
		return err
	}
	fmt.Fprintf(app.Out, "marked %s %s as %s (%s)\n", short(sha), c.Subject, name, stack.BranchName(s.WIP, name))
	return nil
}

func runUnmark(app *App, args []string) error {
	fs := flag.NewFlagSet("unmark", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "usage: fj unmark <name>\n\nRemoves the Flapjack-Branch trailer for <name>. Its PR is left alone and shows as orphaned.")
	}
	if help, err := parseFlags(fs, args); help || err != nil {
		return err
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return errors.New("unmark takes a name")
	}
	name := fs.Arg(0)

	s, err := app.loadForRewrite()
	if err != nil {
		return err
	}
	i := s.HeadIndex(name)
	if i < 0 {
		return fmt.Errorf("no head named %q", name)
	}
	h := s.Heads[i]
	msg, err := stack.Message(app.Repo, h.SHA)
	if err != nil {
		return err
	}
	if err := stack.Reword(app.Repo, s, h.SHA, stack.RemoveTrailer(msg)); err != nil {
		return err
	}
	fmt.Fprintf(app.Out, "unmarked %s %s\n", short(h.SHA), h.Subject)
	return nil
}

func (app *App) loadForRewrite() (*stack.Stack, error) {
	op, err := app.Repo.InProgressOperation()
	if err != nil {
		return nil, err
	}
	if op != "" {
		return nil, fmt.Errorf("a %s is in progress; finish or abort it first", op)
	}
	return stack.Load(app.Repo)
}
