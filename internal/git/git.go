// Package git runs git commands in a repository.
package git

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type Repo struct {
	Dir string
}

type Error struct {
	Args   []string
	Stderr string
	Err    error
}

func (e *Error) Error() string {
	detail := strings.TrimSpace(e.Stderr)
	if detail == "" {
		detail = e.Err.Error()
	}
	return fmt.Sprintf("git %s: %s", strings.Join(e.Args, " "), detail)
}

func (e *Error) Unwrap() error { return e.Err }

// Run runs git and returns stdout without the trailing newline.
func (r Repo) Run(args ...string) (string, error) {
	out, err := r.RunWith(nil, "", args...)
	return strings.TrimRight(out, "\n"), err
}

// RunWith runs git with extra environment variables and stdin, returning stdout as-is.
func (r Repo) RunWith(env []string, stdin string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = r.Dir
	if env != nil {
		cmd.Env = append(os.Environ(), env...)
	}
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return stdout.String(), &Error{Args: args, Stderr: stderr.String(), Err: err}
	}
	return stdout.String(), nil
}

// RunAttached runs git with its output connected to the terminal, for
// commands like push and rebase whose progress and conflict messages the
// user needs to see.
func (r Repo) RunAttached(args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = r.Dir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return &Error{Args: args, Err: err}
	}
	return nil
}

// RefSHA resolves ref to a commit SHA. ok is false when the ref doesn't exist.
func (r Repo) RefSHA(ref string) (sha string, ok bool, err error) {
	sha, err = r.Run("rev-parse", "--verify", "--quiet", ref+"^{commit}")
	if err == nil {
		return sha, true, nil
	}
	var gitErr *Error
	var exitErr *exec.ExitError
	if errors.As(err, &gitErr) && errors.As(err, &exitErr) && exitErr.ExitCode() == 1 && strings.TrimSpace(gitErr.Stderr) == "" {
		return "", false, nil
	}
	return "", false, err
}

// InProgressOperation names an unfinished rebase, merge, cherry-pick or
// revert, or returns "" if there is none.
func (r Repo) InProgressOperation() (string, error) {
	markers := []struct{ path, name string }{
		{"rebase-merge", "rebase"},
		{"rebase-apply", "rebase"},
		{"MERGE_HEAD", "merge"},
		{"CHERRY_PICK_HEAD", "cherry-pick"},
		{"REVERT_HEAD", "revert"},
	}
	for _, m := range markers {
		path, err := r.Run("rev-parse", "--path-format=absolute", "--git-path", m.path)
		if err != nil {
			return "", err
		}
		if _, err := os.Stat(path); err == nil {
			return m.name, nil
		}
	}
	return "", nil
}

// CheckedOutBranches maps each branch checked out in any worktree to that
// worktree's path.
func (r Repo) CheckedOutBranches() (map[string]string, error) {
	out, err := r.Run("worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	branches := map[string]string{}
	var path string
	for line := range strings.SplitSeq(out, "\n") {
		if p, ok := strings.CutPrefix(line, "worktree "); ok {
			path = p
		} else if ref, ok := strings.CutPrefix(line, "branch refs/heads/"); ok {
			branches[ref] = path
		}
	}
	return branches, nil
}
