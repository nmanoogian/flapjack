package stack

import (
	"fmt"
	"strings"

	"github.com/nmanoogian/flapjack/internal/git"
)

// Reword gives commit target a new message and recreates every commit after
// it on the WIP branch. Only messages change, so trees, the index and the
// worktree are untouched, and there is nothing to conflict.
func Reword(r git.Repo, s *Stack, target, msg string) error {
	idx := s.CommitIndex(target)
	if idx < 0 {
		return fmt.Errorf("commit %s is not in %s..%s", short(target), s.Base, s.WIP)
	}
	parent := s.Commits[idx].Parent
	for i := idx; i < len(s.Commits); i++ {
		c := s.Commits[i]
		m := msg
		if i != idx {
			var err error
			if m, err = Message(r, c.SHA); err != nil {
				return err
			}
		}
		var err error
		if parent, err = recommit(r, c.SHA, parent, m); err != nil {
			return err
		}
	}
	oldTip := s.Commits[len(s.Commits)-1].SHA
	_, err := r.Run("update-ref", "-m", "fj: reword "+short(target), "refs/heads/"+s.WIP, parent, oldTip)
	return err
}

// recommit creates a copy of sha with a new parent and message, keeping its
// tree and author. The committer is the current user, as with rebase.
func recommit(r git.Repo, sha, parent, msg string) (string, error) {
	info, err := r.Run("log", "-1", "--date=raw", "--format=%an%x00%ae%x00%ad", sha)
	if err != nil {
		return "", err
	}
	author := strings.Split(info, "\x00")
	if len(author) != 3 {
		return "", fmt.Errorf("unexpected author info for %s: %q", short(sha), info)
	}
	env := []string{
		"GIT_AUTHOR_NAME=" + author[0],
		"GIT_AUTHOR_EMAIL=" + author[1],
		"GIT_AUTHOR_DATE=" + author[2],
	}
	out, err := r.RunWith(env, msg, "commit-tree", sha+"^{tree}", "-p", parent, "-F", "-")
	return strings.TrimSpace(out), err
}
