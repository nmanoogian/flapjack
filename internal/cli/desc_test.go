package cli

import (
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/nmanoogian/flapjack/internal/desc"
	"github.com/nmanoogian/flapjack/internal/testrepo"
)

func readDesc(t *testing.T, r *testrepo.Repo, branch string) (*desc.Description, string) {
	t.Helper()
	dir, err := desc.Dir(r.Repo)
	if err != nil {
		t.Fatal(err)
	}
	path := desc.Path(dir, branch)
	d, err := desc.Read(path)
	if err != nil {
		t.Fatal(err)
	}
	return d, path
}

func editDesc(t *testing.T, r *testrepo.Repo, branch string, edit func(*desc.Description)) {
	t.Helper()
	d, path := readDesc(t, r, branch)
	edit(d)
	if err := desc.Write(path, d); err != nil {
		t.Fatal(err)
	}
}

func TestDescSync(t *testing.T) {
	r := testrepo.New(t)
	r.Commit("A\n\nFrom the commit.\n\nFlapjack-Branch: a\n", "a.txt")
	fake := &fakeGH{}
	mustRun(t, r, fake, "push")
	pr := &fake.prs[0]

	// No file yet: pull from GitHub.
	if out := mustRun(t, r, fake, "desc", "sync"); out != "pulled a (#101)\n" {
		t.Errorf("first sync: %q", out)
	}
	d, path := readDesc(t, r, "wip_a")
	if d.Title != "A" || d.Body != "From the commit." || d.PR != 101 {
		t.Errorf("pulled file = %+v", d)
	}

	// Local edit only: push.
	editDesc(t, r, "wip_a", func(d *desc.Description) { d.Title = "A, edited"; d.Body = "Local body." })
	fake.calls = nil
	if out := mustRun(t, r, fake, "desc", "sync"); out != "pushed a (#101)\n" {
		t.Errorf("local edit sync: %q", out)
	}
	if want := []string{"set-desc #101: A, edited | Local body."}; !slices.Equal(fake.calls, want) {
		t.Errorf("gh calls = %q", fake.calls)
	}

	// Remote edit only, with GitHub's CRLF line endings: pull.
	pr.Body = "Remote body.\r\nSecond line.\r\n"
	if out := mustRun(t, r, fake, "desc", "sync"); out != "pulled a (#101)\n" {
		t.Errorf("remote edit sync: %q", out)
	}
	if d, _ = readDesc(t, r, "wip_a"); d.Body != "Remote body.\nSecond line." || d.Title != "A, edited" {
		t.Errorf("after pull = %+v", d)
	}

	// Nothing changed: no output, no gh calls.
	fake.calls = nil
	if out := mustRun(t, r, fake, "desc", "sync"); out != "" || fake.calls != nil {
		t.Errorf("no-op sync: %q, calls %q", out, fake.calls)
	}

	// Both changed: conflict, local file kept, remote copy written.
	editDesc(t, r, "wip_a", func(d *desc.Description) { d.Body = "Mine." })
	pr.Body = "Theirs."
	out, err := run(t, r, fake, "desc", "sync")
	if err == nil || !strings.Contains(err.Error(), "1 conflict") || out != "conflict a (#101): local and GitHub versions both changed\n" {
		t.Fatalf("conflict sync: %q, %v", out, err)
	}
	if d, _ = readDesc(t, r, "wip_a"); d.Body != "Mine." {
		t.Errorf("local file changed during conflict: %+v", d)
	}
	if remote, err := desc.Read(desc.RemotePath(path)); err != nil || remote == nil || remote.Body != "Theirs." {
		t.Errorf("remote copy = %+v, %v", remote, err)
	}

	mustRun(t, r, fake, "desc", "sync", "--prefer", "local")
	if pr.Body != "Mine." {
		t.Errorf("PR body after --prefer local = %q", pr.Body)
	}
	if _, err := os.Stat(desc.RemotePath(path)); !os.IsNotExist(err) {
		t.Errorf("remote copy not removed: %v", err)
	}

	// Both changed to the same thing: just record it.
	editDesc(t, r, "wip_a", func(d *desc.Description) { d.Body = "Same." })
	pr.Body = "Same."
	fake.calls = nil
	if out := mustRun(t, r, fake, "desc", "sync"); out != "" || fake.calls != nil {
		t.Errorf("converged sync: %q, calls %q", out, fake.calls)
	}
	if d, _ = readDesc(t, r, "wip_a"); d.Synced != d.Hash() {
		t.Errorf("sync state not recorded: %+v", d)
	}
}

func TestDescSyncDryRun(t *testing.T) {
	r := testrepo.New(t)
	r.Commit("A\n\nFlapjack-Branch: a\n", "a.txt")
	r.Commit("B\n\nFlapjack-Branch: b\n", "b.txt")
	fake := &fakeGH{}
	mustRun(t, r, fake, "push", "a")

	out := mustRun(t, r, fake, "desc", "sync", "--dry-run")
	if out != "would pull a (#101)\nskipped b: no open PR\n" {
		t.Errorf("out = %q", out)
	}
	if d, _ := readDesc(t, r, "wip_a"); d != nil {
		t.Errorf("dry run wrote a file: %+v", d)
	}
}
