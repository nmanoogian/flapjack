package stack_test

import (
	"testing"

	"github.com/nmanoogian/flapjack/internal/stack"
	"github.com/nmanoogian/flapjack/internal/testrepo"
)

func TestRemoveTrailer(t *testing.T) {
	tests := map[string]struct{ in, want string }{
		"only trailer": {
			in:   "Subject\n\nBody.\n\nFlapjack-Branch: x\n",
			want: "Subject\n\nBody.\n",
		},
		"keeps other trailers": {
			in:   "Subject\n\nCo-authored-by: A <a@example.com>\nFlapjack-Branch: x\n",
			want: "Subject\n\nCo-authored-by: A <a@example.com>\n",
		},
		"single paragraph is never a trailer block": {
			in:   "Flapjack-Branch: x\n",
			want: "Flapjack-Branch: x\n",
		},
		"no trailer": {
			in:   "Subject\n\nBody.\n",
			want: "Subject\n\nBody.\n",
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := stack.RemoveTrailer(tt.in); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSetTrailerReplacesExisting(t *testing.T) {
	r := testrepo.New(t)
	got, err := stack.SetTrailer(r.Repo, "Subject\n\nFlapjack-Branch: old\n", "new")
	if err != nil {
		t.Fatal(err)
	}
	if want := "Subject\n\nFlapjack-Branch: new\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBody(t *testing.T) {
	got := stack.Body("Subject\n\nFirst line.\nSecond line.\n\nFlapjack-Branch: x\n")
	if want := "First line.\nSecond line."; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
