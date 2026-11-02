package stack_test

import (
	"strings"
	"testing"

	"github.com/nmanoogian/flapjack/internal/stack"
	"github.com/nmanoogian/flapjack/internal/testrepo"
)

func TestLoadFindsHeadsInOrder(t *testing.T) {
	r := testrepo.New(t)
	r.Commit("c1", "c1.txt")
	models := r.Commit("Add models\n\nFlapjack-Branch: new-models\n", "c2.txt")
	r.Commit("c3", "c3.txt")
	api := r.Commit("Add API\n\nSome detail.\n\nCo-authored-by: A <a@example.com>\nflapjack-branch: api\n", "c4.txt")
	r.Commit("c5", "c5.txt")
	r.Commit("c6", "c6.txt")

	s, err := stack.Load(r.Repo)
	if err != nil {
		t.Fatal(err)
	}
	if s.WIP != "wip" || s.Base != "main" || s.BaseRef != "refs/remotes/origin/main" {
		t.Fatalf("wip/base = %q/%q/%q", s.WIP, s.Base, s.BaseRef)
	}
	want := []stack.Head{
		{Name: "new-models", Branch: "wip_new-models", SHA: models, Subject: "Add models"},
		{Name: "api", Branch: "wip_api", SHA: api, Subject: "Add API"},
	}
	if len(s.Heads) != len(want) {
		t.Fatalf("heads = %+v", s.Heads)
	}
	for i := range want {
		if s.Heads[i] != want[i] {
			t.Errorf("head %d = %+v, want %+v", i, s.Heads[i], want[i])
		}
	}
	if s.Unpublished != 2 {
		t.Errorf("unpublished = %d, want 2", s.Unpublished)
	}
	if s.PRBase(0) != "main" || s.PRBase(1) != "wip_new-models" {
		t.Errorf("PR bases = %q, %q", s.PRBase(0), s.PRBase(1))
	}
}

func TestLoadIgnoresTrailersOnFixups(t *testing.T) {
	r := testrepo.New(t)
	r.Commit("Add models\n\nFlapjack-Branch: models\n", "c1.txt")
	r.Commit("fixup! Add models\n\nFlapjack-Branch: models\n", "c2.txt")

	s, err := stack.Load(r.Repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Heads) != 1 || s.Unpublished != 1 {
		t.Fatalf("heads = %+v, unpublished = %d", s.Heads, s.Unpublished)
	}
}

func TestLoadRejectsInvalidStacks(t *testing.T) {
	tests := map[string]struct {
		messages []string
		wantErr  string
	}{
		"duplicate name": {
			messages: []string{"a\n\nFlapjack-Branch: x\n", "b\n\nFlapjack-Branch: x\n"},
			wantErr:  `"x" is on both`,
		},
		"two names on one commit": {
			messages: []string{"a\n\nFlapjack-Branch: x\nFlapjack-Branch: y\n"},
			wantErr:  "more than one",
		},
		"invalid ref name": {
			messages: []string{"a\n\nFlapjack-Branch: x..y\n"},
			wantErr:  "valid branch name",
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			r := testrepo.New(t)
			for i, msg := range tt.messages {
				r.Commit(msg, string(rune('a'+i))+".txt")
			}
			_, err := stack.Load(r.Repo)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestLoadRequiresOriginHead(t *testing.T) {
	r := testrepo.New(t)
	r.Git("remote", "set-head", "origin", "--delete")
	_, err := stack.Load(r.Repo)
	if err == nil || !strings.Contains(err.Error(), "git remote set-head origin --auto") {
		t.Fatalf("err = %v", err)
	}
}
