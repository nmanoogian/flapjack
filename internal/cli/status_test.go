package cli

import (
	"testing"

	"github.com/nmanoogian/flapjack/internal/gh"
	"github.com/nmanoogian/flapjack/internal/testrepo"
)

func TestStatus(t *testing.T) {
	r := testrepo.New(t)
	models := r.Commit("Add models\n\nFlapjack-Branch: models\n", "a.txt")
	api := r.Commit("Add API\n\nFlapjack-Branch: api\n", "b.txt")
	r.Commit("WIP", "c.txt")
	// models is pushed and current; api's remote branch is behind.
	r.Git("push", "--quiet", "origin", models+":refs/heads/wip_models", models+":refs/heads/wip_api")
	fake := &fakeGH{prs: []gh.PR{
		{Number: 1, State: gh.StateOpen, HeadRefName: "wip_models", BaseRefName: "main"},
		{Number: 2, State: gh.StateOpen, IsDraft: true, HeadRefName: "wip_api", BaseRefName: "wip_models"},
		{Number: 3, State: gh.StateOpen, HeadRefName: "wip_old-name", BaseRefName: "main"},
		{Number: 4, State: gh.StateMerged, HeadRefName: "wip_merged", BaseRefName: "main"},
		{Number: 5, State: gh.StateOpen, HeadRefName: "other_x", BaseRefName: "main"},
	}}

	got := mustRun(t, r, fake, "status")
	want := "wip → main\n" +
		"  models    " + models[:7] + "  #1 open   in sync\n" +
		"  api       " + api[:7] + "  #2 draft  stale (remote at " + models[:7] + ")\n" +
		"  (1 unpublished commit)\n" +
		"! old-name  -        #3 open   orphaned\n"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}
