package cli

import (
	"sort"
	"strconv"
	"strings"

	"github.com/nmanoogian/flapjack/internal/gh"
	"github.com/nmanoogian/flapjack/internal/stack"
)

type stackPRs struct {
	// byBranch holds the PR for each head branch that has one. Open PRs win;
	// otherwise it's the most recent merged or closed PR.
	byBranch map[string]*gh.PR
	// orphans are open PRs whose branch starts with "<wip>_" but matches
	// no current head.
	orphans []gh.PR
}

// loadPRs looks up PRs for the given heads, plus orphaned PRs for the stack.
// Heads without an open PR cost one extra gh call each.
func (app *App) loadPRs(s *stack.Stack, heads []stack.Head) (*stackPRs, error) {
	open, err := app.GH.ListOpen()
	if err != nil {
		return nil, err
	}
	current := map[string]bool{}
	for _, h := range s.Heads {
		current[h.Branch] = true
	}
	result := &stackPRs{byBranch: map[string]*gh.PR{}}
	prefix := stack.BranchName(s.WIP, "")
	for i := range open {
		pr := &open[i]
		if current[pr.HeadRefName] {
			result.byBranch[pr.HeadRefName] = pr
		} else if strings.HasPrefix(pr.HeadRefName, prefix) {
			result.orphans = append(result.orphans, *pr)
		}
	}
	sort.Slice(result.orphans, func(i, j int) bool { return result.orphans[i].Number < result.orphans[j].Number })

	for _, h := range heads {
		if result.byBranch[h.Branch] != nil {
			continue
		}
		pr, err := app.GH.ForBranch(h.Branch)
		if err != nil {
			return nil, err
		}
		if pr != nil {
			result.byBranch[h.Branch] = pr
		}
	}
	return result, nil
}

func prLabel(pr *gh.PR) string {
	if pr == nil {
		return "no PR"
	}
	state := strings.ToLower(pr.State)
	if pr.State == gh.StateOpen && pr.IsDraft {
		state = "draft"
	}
	return "#" + strconv.Itoa(pr.Number) + " " + state
}
