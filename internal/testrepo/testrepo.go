// Package testrepo builds throwaway git repositories for tests.
package testrepo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nmanoogian/flapjack/internal/git"
)

type Repo struct {
	git.Repo
	t *testing.T
	// Remote is the bare repository the work repo pushes to as origin.
	Remote git.Repo
}

// New creates a work repo on branch "wip" with one commit on "main", plus a
// bare origin whose HEAD points at main.
func New(t *testing.T) *Repo {
	t.Helper()
	isolate(t)
	root := t.TempDir()
	remote := git.Repo{Dir: filepath.Join(root, "remote.git")}
	work := &Repo{Repo: git.Repo{Dir: filepath.Join(root, "work")}, t: t, Remote: remote}

	if err := os.Mkdir(work.Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	work.Git("init", "--quiet", "--bare", "--initial-branch=main", remote.Dir)
	work.Git("init", "--quiet", "--initial-branch=main")
	work.Git("remote", "add", "origin", remote.Dir)
	work.Commit("base", "base.txt")
	work.Git("push", "--quiet", "origin", "main")
	work.Git("remote", "set-head", "origin", "main")
	work.Git("switch", "--quiet", "--create", "wip")
	return work
}

func isolate(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	for _, kind := range []string{"AUTHOR", "COMMITTER"} {
		t.Setenv("GIT_"+kind+"_NAME", "Test")
		t.Setenv("GIT_"+kind+"_EMAIL", "test@example.com")
	}
}

// Git runs git in the work repo and fails the test on error.
func (r *Repo) Git(args ...string) string {
	r.t.Helper()
	out, err := r.Run(args...)
	if err != nil {
		r.t.Fatal(err)
	}
	return out
}

// Commit writes a file named after the message's subject and commits it
// with msg, returning the new SHA.
func (r *Repo) Commit(msg string, file string) string {
	r.t.Helper()
	subject, _, _ := strings.Cut(msg, "\n")
	if err := os.WriteFile(filepath.Join(r.Dir, file), []byte(subject+"\n"), 0o644); err != nil {
		r.t.Fatal(err)
	}
	r.Git("add", file)
	if _, err := r.RunWith(nil, msg, "commit", "--quiet", "--cleanup=verbatim", "-F", "-"); err != nil {
		r.t.Fatal(err)
	}
	return r.Git("rev-parse", "HEAD")
}
