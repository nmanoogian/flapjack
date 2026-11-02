package cli

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/nmanoogian/flapjack/internal/gh"
	"github.com/nmanoogian/flapjack/internal/testrepo"
)

func remoteSHA(t *testing.T, r *testrepo.Repo, branch string) string {
	t.Helper()
	sha, ok, err := r.Remote.RefSHA("refs/heads/" + branch)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		return ""
	}
	return sha
}

func TestPushCreatesBranchesAndPRs(t *testing.T) {
	r := testrepo.New(t)
	models := r.Commit("Add models\n\nModel details.\n\nFlapjack-Branch: models\n", "a.txt")
	r.Commit("Middle", "b.txt")
	api := r.Commit("Add API\n\nFlapjack-Branch: api\n", "c.txt")
	r.Commit("Unpublished", "d.txt")
	fake := &fakeGH{}

	mustRun(t, r, fake, "push")
	if got := remoteSHA(t, r, "wip_models"); got != models {
		t.Errorf("wip_models on remote = %q, want %q", got, models)
	}
	if got := remoteSHA(t, r, "wip_api"); got != api {
		t.Errorf("wip_api on remote = %q, want %q", got, api)
	}
	wantCalls := []string{
		"create wip_models -> main: Add models | Model details.",
		"create wip_api -> wip_models: Add API | ",
	}
	if !slices.Equal(fake.calls, wantCalls) {
		t.Errorf("gh calls = %q, want %q", fake.calls, wantCalls)
	}

	fake.calls = nil
	if out := mustRun(t, r, fake, "push"); out != "everything is up to date\n" || fake.calls != nil {
		t.Errorf("second push changed something: %q, calls %q", out, fake.calls)
	}
}

func TestPushRetargetsAfterReorder(t *testing.T) {
	r := testrepo.New(t)
	r.Commit("A\n\nFlapjack-Branch: a\n", "a.txt")
	r.Commit("B\n\nFlapjack-Branch: b\n", "b.txt")
	fake := &fakeGH{}
	mustRun(t, r, fake, "push")

	// Swap the two commits so b is now at the bottom.
	r.Git("switch", "--quiet", "--create", "reordered", "main")
	r.Git("cherry-pick", "wip", "wip~1")
	r.Git("branch", "--force", "wip", "reordered")
	r.Git("switch", "--quiet", "wip")

	fake.calls = nil
	mustRun(t, r, fake, "push")
	wantCalls := []string{"set-base #102 -> main", "set-base #101 -> wip_b"}
	if !slices.Equal(fake.calls, wantCalls) {
		t.Errorf("gh calls = %q, want %q", fake.calls, wantCalls)
	}
}

func TestPushLeaseProtectsRemoteChanges(t *testing.T) {
	r := testrepo.New(t)
	r.Commit("A\n\nFlapjack-Branch: a\n", "a.txt")
	r.Commit("B\n\nFlapjack-Branch: b\n", "b.txt")
	fake := &fakeGH{}
	mustRun(t, r, fake, "push")
	pushedA := remoteSHA(t, r, "wip_a")

	// Someone else pushes a commit to wip_b without us fetching it.
	other := testrepo.Clone(t, r)
	other.Git("switch", "--quiet", "wip_b")
	other.Commit("Reviewer suggestion", "suggestion.txt")
	other.Git("push", "--quiet", "origin", "wip_b")
	theirs := remoteSHA(t, r, "wip_b")

	// Rewrite both heads so both branches need a push.
	r.Git("reset", "--quiet", "--hard", "main")
	r.Commit("A v2\n\nFlapjack-Branch: a\n", "a.txt")
	r.Commit("B v2\n\nFlapjack-Branch: b\n", "b.txt")

	_, err := run(t, r, fake, "push")
	if err == nil || !strings.Contains(err.Error(), "no branches were updated") {
		t.Fatalf("err = %v", err)
	}
	if got := remoteSHA(t, r, "wip_b"); got != theirs {
		t.Errorf("wip_b was overwritten: %q, want %q", got, theirs)
	}
	if got := remoteSHA(t, r, "wip_a"); got != pushedA {
		t.Errorf("wip_a changed despite atomic push: %q, want %q", got, pushedA)
	}
}

func TestPushRefuses(t *testing.T) {
	t.Run("branch checked out in another worktree", func(t *testing.T) {
		r := testrepo.New(t)
		head := r.Commit("A\n\nFlapjack-Branch: a\n", "a.txt")
		r.Git("branch", "wip_a", head)
		wt := filepath.Join(t.TempDir(), "wt")
		r.Git("worktree", "add", "--quiet", wt, "wip_a")
		_, err := run(t, r, &fakeGH{}, "push")
		if err == nil || !strings.Contains(err.Error(), "wip_a is checked out in") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("merged PR", func(t *testing.T) {
		r := testrepo.New(t)
		r.Commit("A\n\nFlapjack-Branch: a\n", "a.txt")
		fake := &fakeGH{prs: []gh.PR{{Number: 7, State: gh.StateMerged, HeadRefName: "wip_a", BaseRefName: "main"}}}
		_, err := run(t, r, fake, "push")
		if err == nil || !strings.Contains(err.Error(), "run `fj sync`") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("PR base not on remote", func(t *testing.T) {
		r := testrepo.New(t)
		r.Commit("A\n\nFlapjack-Branch: a\n", "a.txt")
		r.Commit("B\n\nFlapjack-Branch: b\n", "b.txt")
		_, err := run(t, r, &fakeGH{}, "push", "b")
		if err == nil || !strings.Contains(err.Error(), "PR base wip_a isn't on the remote") {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestPushDryRun(t *testing.T) {
	r := testrepo.New(t)
	sha := r.Commit("A\n\nFlapjack-Branch: a\n", "a.txt")
	fake := &fakeGH{}
	out := mustRun(t, r, fake, "push", "--dry-run")
	want := "would push wip_a: new → " + sha[:7] + "\nwould create PR wip_a → main\n"
	if out != want || fake.calls != nil || remoteSHA(t, r, "wip_a") != "" {
		t.Errorf("out = %q, calls = %q", out, fake.calls)
	}
}
