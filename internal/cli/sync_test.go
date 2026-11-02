package cli

import (
	"slices"
	"testing"

	"github.com/nmanoogian/flapjack/internal/gh"
	"github.com/nmanoogian/flapjack/internal/testrepo"
)

func TestSyncAfterSquashMerge(t *testing.T) {
	r := testrepo.New(t)
	r.Commit("A1", "a1.txt")
	r.Commit("A2\n\nFlapjack-Branch: a\n", "a2.txt")
	r.Commit("B\n\nFlapjack-Branch: b\n", "b.txt")
	r.Commit("Unpublished", "c.txt")
	fake := &fakeGH{}
	mustRun(t, r, fake, "push")

	// GitHub squash-merges PR a into main.
	other := testrepo.Clone(t, r)
	other.Git("merge", "--quiet", "--squash", "origin/wip_a")
	other.Git("commit", "--quiet", "--message", "A (#101)")
	other.Git("push", "--quiet", "origin", "main")
	fake.prs[0].State = gh.StateMerged
	fake.calls = nil

	mustRun(t, r, fake, "sync")
	if got := r.Git("log", "--format=%s", "origin/main..wip"); got != "Unpublished\nB" {
		t.Errorf("WIP commits after sync:\n%s", got)
	}
	if got := r.Git("rev-parse", "wip~2"); got != r.Git("rev-parse", "origin/main") {
		t.Errorf("WIP is not on top of origin/main")
	}
	if _, ok, _ := r.RefSHA("refs/heads/wip_a"); ok {
		t.Errorf("local wip_a still exists")
	}
	if d, _ := readDesc(t, r, "wip_a"); d != nil {
		t.Errorf("description file for merged wip_a still exists")
	}
	if d, _ := readDesc(t, r, "wip_b"); d == nil {
		t.Errorf("description file for open wip_b was removed")
	}
	if want := []string{"set-base #102 -> main"}; !slices.Equal(fake.calls, want) {
		t.Errorf("gh calls = %q, want %q", fake.calls, want)
	}

	// The rebased head can then be pushed.
	fake.calls = nil
	mustRun(t, r, fake, "push")
	if got := remoteSHA(t, r, "wip_b"); got != r.Git("rev-parse", "wip~1") {
		t.Errorf("wip_b on remote = %q", got)
	}
}

func TestSyncWithNothingMerged(t *testing.T) {
	r := testrepo.New(t)
	before := r.Commit("A\n\nFlapjack-Branch: a\n", "a.txt")
	fake := &fakeGH{}
	mustRun(t, r, fake, "push")
	fake.calls = nil

	out := mustRun(t, r, fake, "sync")
	if out != "no merged PRs at the bottom of the stack\n" || fake.calls != nil {
		t.Errorf("out = %q, calls = %q", out, fake.calls)
	}
	if got := r.Git("rev-parse", "wip"); got != before {
		t.Errorf("WIP branch moved")
	}
}
