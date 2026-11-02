package cli

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/nmanoogian/flapjack/internal/gh"
	"github.com/nmanoogian/flapjack/internal/testrepo"
)

// fakeGH stores PRs in memory. Merged and closed PRs are only returned by
// ForBranch, matching the real ListOpen.
type fakeGH struct {
	prs  []gh.PR
	next int
	// calls records Create and SetBase calls.
	calls []string
}

func (f *fakeGH) ListOpen() ([]gh.PR, error) {
	var open []gh.PR
	for _, pr := range f.prs {
		if pr.State == gh.StateOpen {
			open = append(open, pr)
		}
	}
	return open, nil
}

func (f *fakeGH) ForBranch(branch string) (*gh.PR, error) {
	for i := len(f.prs) - 1; i >= 0; i-- {
		if f.prs[i].HeadRefName == branch {
			pr := f.prs[i]
			return &pr, nil
		}
	}
	return nil, nil
}

func (f *fakeGH) Create(head, base, title, body string, draft bool) (gh.PR, error) {
	f.next++
	pr := gh.PR{Number: 100 + f.next, State: gh.StateOpen, IsDraft: draft, HeadRefName: head, BaseRefName: base, Title: title, Body: body}
	f.prs = append(f.prs, pr)
	f.calls = append(f.calls, fmt.Sprintf("create %s -> %s: %s | %s", head, base, title, body))
	return pr, nil
}

func (f *fakeGH) SetBase(number int, base string) error {
	for i := range f.prs {
		if f.prs[i].Number == number {
			f.prs[i].BaseRefName = base
		}
	}
	f.calls = append(f.calls, fmt.Sprintf("set-base #%d -> %s", number, base))
	return nil
}

func (f *fakeGH) SetDescription(number int, title, body string) error {
	for i := range f.prs {
		if f.prs[i].Number == number {
			f.prs[i].Title, f.prs[i].Body = title, body
		}
	}
	f.calls = append(f.calls, fmt.Sprintf("set-desc #%d: %s | %s", number, title, body))
	return nil
}

func run(t *testing.T, r *testrepo.Repo, fake *fakeGH, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	app := &App{Repo: r.Repo, GH: fake, Out: &out}
	err := app.Run(args)
	return out.String(), err
}

func mustRun(t *testing.T, r *testrepo.Repo, fake *fakeGH, args ...string) string {
	t.Helper()
	out, err := run(t, r, fake, args...)
	if err != nil {
		t.Fatalf("fj %s: %v", strings.Join(args, " "), err)
	}
	return out
}
