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

func runSync(app *App, args []string) error {
	fs := flag.NewFlagSet("sync", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "usage: fj sync [flags]\n\nAfter the bottom PRs merge, rebases the WIP branch onto the base without them and retargets the next PR.\n\nFlags:")
		fs.PrintDefaults()
	}
	restack := fs.Bool("restack", false, "rebase onto origin/<base> even if no PRs merged")
	if help, err := parseFlags(fs, args); help || err != nil {
		return err
	}
	s, err := app.loadForRewrite()
	if err != nil {
		return err
	}
	// Fetch only the base so PR branches' remote-tracking refs, which back
	// the push leases, keep their last-known values.
	if _, err := app.Repo.Run("fetch", "--quiet", stack.Remote, s.Base); err != nil {
		return err
	}
	if s, err = stack.Load(app.Repo); err != nil {
		return err
	}

	merged, err := app.mergedHeads(s)
	if err != nil {
		return err
	}
	switch {
	case len(merged) > 0:
		if err := app.cleanUpMerged(merged); err != nil {
			return err
		}
		top := merged[len(merged)-1]
		if err := app.rebase(s.BaseRef, top.SHA, s.WIP); err != nil {
			return err
		}
		for _, h := range merged {
			fmt.Fprintf(app.Out, "dropped merged %s (%s)\n", h.Name, h.Subject)
		}
	case *restack:
		if err := app.rebase(s.BaseRef, s.BaseRef, s.WIP); err != nil {
			return err
		}
		fmt.Fprintf(app.Out, "rebased %s onto %s/%s\n", s.WIP, stack.Remote, s.Base)
	default:
		fmt.Fprintln(app.Out, "no merged PRs at the bottom of the stack")
	}
	if len(merged) > 0 || *restack {
		if s, err = stack.Load(app.Repo); err != nil {
			return err
		}
	}

	if err := app.retargetBottom(s); err != nil {
		return err
	}
	if (len(merged) > 0 || *restack) && len(s.Heads) > 0 {
		fmt.Fprintln(app.Out, "run `fj push` to update the PR branches")
	}
	return nil
}

// rebase replays upstream..branch onto onto. On conflicts, git stops the
// rebase and the user finishes it.
func (app *App) rebase(onto, upstream, branch string) error {
	if err := app.Repo.RunAttached("rebase", "--onto", onto, upstream, branch); err != nil {
		return fmt.Errorf("%w\nResolve the conflicts, run `git rebase --continue`, then run `fj sync` again to retarget PRs", err)
	}
	return nil
}

// mergedHeads returns the heads at the bottom of the stack whose PRs are
// merged, stopping at the first one that isn't.
func (app *App) mergedHeads(s *stack.Stack) ([]stack.Head, error) {
	var merged []stack.Head
	for _, h := range s.Heads {
		pr, err := app.GH.ForBranch(h.Branch)
		if err != nil {
			return nil, err
		}
		if pr == nil || pr.State != gh.StateMerged {
			break
		}
		merged = append(merged, h)
	}
	return merged, nil
}

// cleanUpMerged deletes the local branches and description files of
// merged heads.
func (app *App) cleanUpMerged(heads []stack.Head) error {
	checkedOut, err := app.Repo.CheckedOutBranches()
	if err != nil {
		return err
	}
	for _, h := range heads {
		path, err := app.descPath(h.Branch)
		if err != nil {
			return err
		}
		for _, p := range []string{path, desc.RemotePath(path)} {
			if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
		if path, ok := checkedOut[h.Branch]; ok {
			fmt.Fprintf(app.Out, "kept %s: checked out in %s\n", h.Branch, path)
			continue
		}
		if _, ok, err := app.Repo.RefSHA("refs/heads/" + h.Branch); err != nil {
			return err
		} else if !ok {
			continue
		}
		if _, err := app.Repo.Run("branch", "--delete", "--force", h.Branch); err != nil {
			return err
		}
	}
	return nil
}

// retargetBottom points the bottom head's open PR at the base, which it
// needs once the PRs below it have merged.
func (app *App) retargetBottom(s *stack.Stack) error {
	if len(s.Heads) == 0 {
		return nil
	}
	bottom := s.Heads[0]
	pr, err := app.GH.ForBranch(bottom.Branch)
	if err != nil || pr == nil || pr.State != gh.StateOpen || pr.BaseRefName == s.Base {
		return err
	}
	if err := app.GH.SetBase(pr.Number, s.Base); err != nil {
		return err
	}
	fmt.Fprintf(app.Out, "retargeted PR #%d → %s\n", pr.Number, s.Base)
	return nil
}
