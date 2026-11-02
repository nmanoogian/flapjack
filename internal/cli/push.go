package cli

import (
	"flag"
	"fmt"
	"slices"

	"github.com/nmanoogian/flapjack/internal/desc"
	"github.com/nmanoogian/flapjack/internal/gh"
	"github.com/nmanoogian/flapjack/internal/stack"
)

type pushItem struct {
	head   stack.Head
	prBase string
	remote remoteStatus
	pr     *gh.PR
}

func (p pushItem) needsPush() bool { return p.remote.State != remoteInSync }

func (p pushItem) needsPR() bool { return p.pr == nil || p.pr.State != gh.StateOpen }

func (p pushItem) needsRetarget() bool {
	return !p.needsPR() && p.pr.BaseRefName != p.prBase
}

func runPush(app *App, args []string) error {
	fs := flag.NewFlagSet("push", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "usage: fj push [flags] [<name>...]\n\nPoints each PR branch at its head commit, pushes them all at once, then creates or retargets PRs.\n\nFlags:")
		fs.PrintDefaults()
	}
	dryRun := fs.Bool("dry-run", false, "print the plan and change nothing")
	draft := fs.Bool("draft", false, "create new PRs as drafts")
	if help, err := parseFlags(fs, args); help || err != nil {
		return err
	}

	s, err := stack.Load(app.Repo)
	if err != nil {
		return err
	}
	if len(s.Heads) == 0 {
		return fmt.Errorf("no heads in %s; mark one with `fj mark <rev> <name>`", s.WIP)
	}
	selected, err := selectHeads(s, fs.Args())
	if err != nil {
		return err
	}

	checkedOut, err := app.Repo.CheckedOutBranches()
	if err != nil {
		return err
	}
	var heads []stack.Head
	for _, i := range selected {
		h := s.Heads[i]
		if path, ok := checkedOut[h.Branch]; ok {
			return fmt.Errorf("%s is checked out in %s; switch that worktree to another branch first", h.Branch, path)
		}
		heads = append(heads, h)
	}

	prs, err := app.loadPRs(s, heads)
	if err != nil {
		return err
	}
	items := make([]pushItem, 0, len(selected))
	pushing := map[string]bool{}
	for _, i := range selected {
		h := s.Heads[i]
		pr := prs.byBranch[h.Branch]
		if pr != nil && pr.State == gh.StateMerged {
			return fmt.Errorf("PR #%d for %s is merged; run `fj sync` first", pr.Number, h.Name)
		}
		remote, err := app.remoteStatus(h)
		if err != nil {
			return err
		}
		item := pushItem{head: h, prBase: s.PRBase(i), remote: remote, pr: pr}
		items = append(items, item)
		if item.needsPush() {
			pushing[h.Branch] = true
		}
	}
	for _, item := range items {
		if item.prBase == s.Base || pushing[item.prBase] {
			continue
		}
		if _, ok, err := app.Repo.RefSHA(remoteRef(item.prBase)); err != nil {
			return err
		} else if !ok {
			return fmt.Errorf("%s's PR base %s isn't on the remote; push it too", item.head.Name, item.prBase)
		}
	}

	if *dryRun {
		printPushPlan(app, items)
		return nil
	}
	if !slices.ContainsFunc(items, func(p pushItem) bool { return p.needsPush() || p.needsPR() || p.needsRetarget() }) {
		fmt.Fprintln(app.Out, "everything is up to date")
		return nil
	}
	if err := app.pushBranches(items); err != nil {
		return err
	}
	return app.updatePRs(items, *draft)
}

// selectHeads returns the indexes of the named heads in stack order, or all
// heads when names is empty.
func selectHeads(s *stack.Stack, names []string) ([]int, error) {
	if len(names) == 0 {
		all := make([]int, len(s.Heads))
		for i := range all {
			all[i] = i
		}
		return all, nil
	}
	want := map[string]bool{}
	for _, name := range names {
		if s.HeadIndex(name) < 0 {
			return nil, fmt.Errorf("no head named %q", name)
		}
		want[name] = true
	}
	var selected []int
	for i, h := range s.Heads {
		if want[h.Name] {
			selected = append(selected, i)
		}
	}
	return selected, nil
}

func printPushPlan(app *App, items []pushItem) {
	changed := false
	for _, item := range items {
		if item.needsPush() {
			from := "new"
			if item.remote.SHA != "" {
				from = short(item.remote.SHA)
			}
			fmt.Fprintf(app.Out, "would push %s: %s → %s\n", item.head.Branch, from, short(item.head.SHA))
			changed = true
		}
		if item.needsPR() {
			fmt.Fprintf(app.Out, "would create PR %s → %s\n", item.head.Branch, item.prBase)
			changed = true
		} else if item.needsRetarget() {
			fmt.Fprintf(app.Out, "would retarget PR #%d → %s\n", item.pr.Number, item.prBase)
			changed = true
		}
	}
	if !changed {
		fmt.Fprintln(app.Out, "everything is up to date")
	}
}

// pushBranches points each local PR branch at its head and pushes them in
// one atomic push. Each branch is leased against its remote-tracking ref,
// so a commit someone else pushed is never overwritten.
func (app *App) pushBranches(items []pushItem) error {
	args := []string{"push", "--atomic", stack.Remote}
	var refspecs []string
	for _, item := range items {
		if !item.needsPush() {
			continue
		}
		branch := item.head.Branch
		if _, err := app.Repo.Run("branch", "--force", branch, item.head.SHA); err != nil {
			return err
		}
		args = append(args, "--force-with-lease=refs/heads/"+branch+":"+item.remote.SHA)
		refspecs = append(refspecs, "refs/heads/"+branch+":refs/heads/"+branch)
	}
	if len(refspecs) == 0 {
		return nil
	}
	if err := app.Repo.RunAttached(append(args, refspecs...)...); err != nil {
		return fmt.Errorf("push failed and no branches were updated: %w\nIf a PR branch moved on the remote, someone else pushed to it. Inspect it before overwriting", err)
	}
	for _, item := range items {
		if item.needsPush() {
			fmt.Fprintf(app.Out, "pushed %s (%s)\n", item.head.Branch, short(item.head.SHA))
		}
	}
	return nil
}

func (app *App) updatePRs(items []pushItem, draft bool) error {
	for _, item := range items {
		switch {
		case item.needsPR():
			if err := app.createPR(item, draft); err != nil {
				return err
			}
		case item.needsRetarget():
			if err := app.GH.SetBase(item.pr.Number, item.prBase); err != nil {
				return err
			}
			fmt.Fprintf(app.Out, "retargeted PR #%d → %s\n", item.pr.Number, item.prBase)
		}
	}
	return nil
}

// createPR opens a PR using the head's description file if there is one,
// otherwise the head commit's message, and records the file as synced.
func (app *App) createPR(item pushItem, draft bool) error {
	path, err := app.descPath(item.head.Branch)
	if err != nil {
		return err
	}
	d, err := desc.Read(path)
	if err != nil {
		return err
	}
	if d == nil {
		msg, err := stack.Message(app.Repo, item.head.SHA)
		if err != nil {
			return err
		}
		d = &desc.Description{Title: item.head.Subject, Body: stack.Body(msg)}
	}
	pr, err := app.GH.Create(item.head.Branch, item.prBase, desc.Normalize(d.Title), desc.Normalize(d.Body), draft)
	if err != nil {
		return err
	}
	fmt.Fprintf(app.Out, "created PR #%d %s → %s %s\n", pr.Number, item.head.Branch, item.prBase, pr.URL)
	return app.writeSynced(path, remoteDesc(&pr))
}
