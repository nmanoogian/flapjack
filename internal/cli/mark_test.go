package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nmanoogian/flapjack/internal/testrepo"
)

func TestMarkRenameUnmark(t *testing.T) {
	r := testrepo.New(t)
	r.Commit("One", "a.txt")
	r.Git("commit", "--quiet", "--amend", "--no-edit", "--author", "Someone Else <else@example.com>", "--date", "2001-02-03T04:05:06Z")
	r.Commit("Two\n\nBody of two.\n", "b.txt")
	r.Commit("Three\n\nCo-authored-by: A <a@example.com>\n", "c.txt")
	trees := r.Git("log", "--format=%T", "main..")
	// Uncommitted work must survive the rewrite.
	dirty := filepath.Join(r.Dir, "c.txt")
	if err := os.WriteFile(dirty, []byte("uncommitted\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	mustRun(t, r, &fakeGH{}, "mark", "HEAD~2", "first")
	mustRun(t, r, &fakeGH{}, "mark", "HEAD", "third")
	messages := r.Git("log", "--format=%B%x00", "main..")
	wantMessages := "Three\n\nCo-authored-by: A <a@example.com>\nFlapjack-Branch: third\n\x00\n" +
		"Two\n\nBody of two.\n\x00\n" +
		"One\n\nFlapjack-Branch: first\n\x00"
	if messages != wantMessages {
		t.Errorf("messages = %q, want %q", messages, wantMessages)
	}
	if got := r.Git("log", "--format=%T", "main.."); got != trees {
		t.Errorf("trees changed:\n%s\nwant:\n%s", got, trees)
	}
	if got := r.Git("log", "-1", "--format=%an <%ae> %aI", "HEAD~2"); got != "Someone Else <else@example.com> 2001-02-03T04:05:06Z" {
		t.Errorf("author = %q", got)
	}
	if b, _ := os.ReadFile(dirty); string(b) != "uncommitted\n" {
		t.Errorf("worktree change lost: %q", b)
	}

	mustRun(t, r, &fakeGH{}, "mark", "HEAD~2", "renamed")
	if got := r.Git("log", "-1", "--format=%B", "HEAD~2"); got != "One\n\nFlapjack-Branch: renamed" {
		t.Errorf("after rename: %q", got)
	}

	mustRun(t, r, &fakeGH{}, "unmark", "renamed")
	if got := r.Git("log", "-1", "--format=%B", "HEAD~2"); got != "One" {
		t.Errorf("after unmark: %q", got)
	}
}

func TestMarkRejects(t *testing.T) {
	r := testrepo.New(t)
	r.Commit("One\n\nFlapjack-Branch: taken\n", "a.txt")
	r.Commit("fixup! One", "b.txt")
	r.Commit("Three", "c.txt")

	tests := map[string]struct {
		args    []string
		wantErr string
	}{
		"fixup commit":       {[]string{"mark", "HEAD~1", "x"}, "fixup commit"},
		"duplicate name":     {[]string{"mark", "HEAD", "taken"}, `"taken" already marks`},
		"commit on base":     {[]string{"mark", "main", "x"}, "not in main..wip"},
		"unknown head":       {[]string{"unmark", "nope"}, `no head named "nope"`},
		"invalid ref name":   {[]string{"mark", "HEAD", "a..b"}, "valid branch name"},
		"whitespace in name": {[]string{"mark", "HEAD", "a b"}, "whitespace"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := run(t, r, &fakeGH{}, tt.args...)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}
