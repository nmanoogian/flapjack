// Package stack reads a stack of PR heads from the Flapjack-Branch trailers
// on a WIP branch.
package stack

import (
	"errors"
	"fmt"
	"strings"

	"github.com/nmanoogian/flapjack/internal/git"
)

const (
	TrailerKey = "Flapjack-Branch"
	Remote     = "origin"
)

type Commit struct {
	SHA     string
	Parent  string
	Subject string
	// Names holds every Flapjack-Branch trailer value on the commit.
	Names []string
}

// IsFixup reports whether the commit will be squashed into another by
// `rebase --autosquash`. Trailers on these commits are ignored.
func (c Commit) IsFixup() bool {
	for _, prefix := range []string{"fixup! ", "squash! ", "amend! "} {
		if strings.HasPrefix(c.Subject, prefix) {
			return true
		}
	}
	return false
}

type Head struct {
	Name    string
	Branch  string
	SHA     string
	Subject string
}

type Stack struct {
	WIP string
	// Base is the branch the stack targets, without the remote prefix.
	Base string
	// BaseRef is the remote-tracking ref for Base.
	BaseRef string
	// Commits holds base..WIP, oldest first.
	Commits []Commit
	// Heads holds the commits with a Flapjack-Branch trailer, oldest first.
	Heads []Head
	// Unpublished counts the commits after the last head.
	Unpublished int
}

func BranchName(wip, name string) string {
	return wip + "_" + name
}

// Load reads the stack for the currently checked-out branch.
func Load(r git.Repo) (*Stack, error) {
	wip, err := r.Run("symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil {
		return nil, errors.New("not on a branch; check out the WIP branch (or finish the rebase in progress)")
	}
	base, baseRef, err := baseBranch(r)
	if err != nil {
		return nil, err
	}
	if wip == base {
		return nil, fmt.Errorf("on the base branch %q; check out a WIP branch", base)
	}
	commits, err := readCommits(r, baseRef)
	if err != nil {
		return nil, err
	}
	s := &Stack{WIP: wip, Base: base, BaseRef: baseRef, Commits: commits}

	seen := map[string]string{}
	lastHead := -1
	for i, c := range commits {
		if c.IsFixup() || len(c.Names) == 0 {
			continue
		}
		if len(c.Names) > 1 {
			return nil, fmt.Errorf("commit %s has more than one %s trailer (%s)", short(c.SHA), TrailerKey, strings.Join(c.Names, ", "))
		}
		name := c.Names[0]
		if err := ValidateName(r, wip, name); err != nil {
			return nil, fmt.Errorf("commit %s: %w", short(c.SHA), err)
		}
		if other, ok := seen[name]; ok {
			return nil, fmt.Errorf("%s %q is on both %s and %s", TrailerKey, name, short(other), short(c.SHA))
		}
		seen[name] = c.SHA
		s.Heads = append(s.Heads, Head{Name: name, Branch: BranchName(wip, name), SHA: c.SHA, Subject: c.Subject})
		lastHead = i
	}
	s.Unpublished = len(commits) - 1 - lastHead
	return s, nil
}

// ValidateName checks that name is non-empty and makes a valid branch name.
func ValidateName(r git.Repo, wip, name string) error {
	if name == "" {
		return errors.New("empty name")
	}
	if strings.ContainsAny(name, " \t\n") {
		return fmt.Errorf("name %q contains whitespace", name)
	}
	if _, err := r.Run("check-ref-format", "refs/heads/"+BranchName(wip, name)); err != nil {
		return fmt.Errorf("name %q doesn't make a valid branch name", name)
	}
	return nil
}

// PRBase returns the branch the PR for Heads[i] should target.
func (s *Stack) PRBase(i int) string {
	if i == 0 {
		return s.Base
	}
	return s.Heads[i-1].Branch
}

func (s *Stack) HeadIndex(name string) int {
	for i, h := range s.Heads {
		if h.Name == name {
			return i
		}
	}
	return -1
}

func (s *Stack) CommitIndex(sha string) int {
	for i, c := range s.Commits {
		if c.SHA == sha {
			return i
		}
	}
	return -1
}

func baseBranch(r git.Repo) (base, baseRef string, err error) {
	headRef := "refs/remotes/" + Remote + "/HEAD"
	target, err := r.Run("symbolic-ref", headRef)
	if err != nil {
		return "", "", fmt.Errorf("%s/HEAD is not set; run `git remote set-head %s --auto`", Remote, Remote)
	}
	prefix := "refs/remotes/" + Remote + "/"
	base, ok := strings.CutPrefix(target, prefix)
	if !ok {
		return "", "", fmt.Errorf("%s points outside %s: %s", headRef, prefix, target)
	}
	return base, target, nil
}

const (
	fieldSep   = "\x1f"
	trailerSep = "\x1d"
)

func readCommits(r git.Repo, baseRef string) ([]Commit, error) {
	format := strings.Join([]string{"%H", "%P", "%s", "%(trailers:key=" + TrailerKey + ",valueonly,separator=%x1d)"}, "%x1f")
	out, err := r.Run("log", "-z", "--reverse", "--format="+format, baseRef+"..HEAD")
	if err != nil {
		return nil, err
	}
	var commits []Commit
	for record := range strings.SplitSeq(out, "\x00") {
		if record == "" {
			continue
		}
		fields := strings.Split(record, fieldSep)
		if len(fields) != 4 {
			return nil, fmt.Errorf("unexpected git log output: %q", record)
		}
		parents := strings.Fields(fields[1])
		if len(parents) != 1 {
			return nil, fmt.Errorf("commit %s is a merge commit; flapjack needs a linear WIP branch", short(fields[0]))
		}
		c := Commit{SHA: fields[0], Parent: parents[0], Subject: fields[2]}
		for name := range strings.SplitSeq(fields[3], trailerSep) {
			if name = strings.TrimSpace(name); name != "" {
				c.Names = append(c.Names, name)
			}
		}
		commits = append(commits, c)
	}
	return commits, nil
}

func short(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}
