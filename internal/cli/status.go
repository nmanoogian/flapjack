package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"slices"
	"strings"

	"github.com/nmanoogian/flapjack/internal/gh"
	"github.com/nmanoogian/flapjack/internal/stack"
)

const (
	remoteInSync    = "in-sync"
	remoteStale     = "stale"
	remoteNotPushed = "not-pushed"
)

type remoteStatus struct {
	// SHA is the last-known remote PR branch commit, from the
	// remote-tracking ref. flapjack never fetches PR branches.
	SHA   string `json:"sha,omitempty"`
	State string `json:"state"`
}

type headStatus struct {
	Name    string       `json:"name"`
	Branch  string       `json:"branch"`
	SHA     string       `json:"sha"`
	Subject string       `json:"subject"`
	PRBase  string       `json:"prBase"`
	PR      *gh.PR       `json:"pr"`
	Remote  remoteStatus `json:"remote"`
}

type statusReport struct {
	WIP         string       `json:"wip"`
	Base        string       `json:"base"`
	Heads       []headStatus `json:"heads"`
	Unpublished int          `json:"unpublished"`
	Orphans     []gh.PR      `json:"orphans"`
}

func runStatus(app *App, args []string) error {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "print the stack as JSON")
	if help, err := parseFlags(fs, args); help || err != nil {
		return err
	}
	s, err := stack.Load(app.Repo)
	if err != nil {
		return err
	}
	prs, err := app.loadPRs(s, s.Heads)
	if err != nil {
		return err
	}
	report := statusReport{WIP: s.WIP, Base: s.Base, Unpublished: s.Unpublished, Heads: []headStatus{}, Orphans: prs.orphans}
	if report.Orphans == nil {
		report.Orphans = []gh.PR{}
	}
	for i, h := range s.Heads {
		remote, err := app.remoteStatus(h)
		if err != nil {
			return err
		}
		report.Heads = append(report.Heads, headStatus{
			Name: h.Name, Branch: h.Branch, SHA: h.SHA, Subject: h.Subject,
			PRBase: s.PRBase(i), PR: prs.byBranch[h.Branch], Remote: remote,
		})
	}
	if *asJSON {
		enc := json.NewEncoder(app.Out)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	}
	printStatus(app, report)
	return nil
}

func (app *App) remoteStatus(h stack.Head) (remoteStatus, error) {
	sha, ok, err := app.Repo.RefSHA(remoteRef(h.Branch))
	switch {
	case err != nil:
		return remoteStatus{}, err
	case !ok:
		return remoteStatus{State: remoteNotPushed}, nil
	case sha == h.SHA:
		return remoteStatus{SHA: sha, State: remoteInSync}, nil
	default:
		return remoteStatus{SHA: sha, State: remoteStale}, nil
	}
}

func remoteRef(branch string) string {
	return "refs/remotes/" + stack.Remote + "/" + branch
}

func printStatus(app *App, r statusReport) {
	fmt.Fprintf(app.Out, "%s → %s\n", r.WIP, r.Base)
	if len(r.Heads) == 0 {
		fmt.Fprintln(app.Out, "  no heads yet; mark one with `fj mark <rev> <name>`")
	}
	type row struct{ marker, name, sha, pr, remote string }
	var heads, orphans []row
	for _, h := range r.Heads {
		remote := "not pushed"
		switch h.Remote.State {
		case remoteInSync:
			remote = "in sync"
		case remoteStale:
			remote = "stale (remote at " + short(h.Remote.SHA) + ")"
		}
		heads = append(heads, row{" ", h.Name, short(h.SHA), prLabel(h.PR), remote})
	}
	prefix := stack.BranchName(r.WIP, "")
	for _, pr := range r.Orphans {
		orphans = append(orphans, row{"!", strings.TrimPrefix(pr.HeadRefName, prefix), "-", prLabel(&pr), "orphaned"})
	}
	var nameW, shaW, prW int
	for _, rw := range append(slices.Clone(heads), orphans...) {
		nameW, shaW, prW = max(nameW, len(rw.name)), max(shaW, len(rw.sha)), max(prW, len(rw.pr))
	}
	printRow := func(rw row) {
		fmt.Fprintf(app.Out, "%s %-*s  %-*s  %-*s  %s\n", rw.marker, nameW, rw.name, shaW, rw.sha, prW, rw.pr, rw.remote)
	}
	for _, rw := range heads {
		printRow(rw)
	}
	if r.Unpublished > 0 {
		fmt.Fprintf(app.Out, "  (%d unpublished %s)\n", r.Unpublished, plural(r.Unpublished, "commit", "commits"))
	}
	for _, rw := range orphans {
		printRow(rw)
	}
}

func short(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
