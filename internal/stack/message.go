package stack

import (
	"strings"

	"github.com/nmanoogian/flapjack/internal/git"
)

// SetTrailer returns msg with its Flapjack-Branch trailer set to name,
// replacing any existing one.
func SetTrailer(r git.Repo, msg, name string) (string, error) {
	return r.RunWith(nil, msg, "interpret-trailers", "--if-exists", "replace", "--trailer", TrailerKey+": "+name)
}

// RemoveTrailer returns msg without Flapjack-Branch trailers. If that
// empties the trailer block, the blank line before it is removed too.
func RemoveTrailer(msg string) string {
	lines := strings.Split(strings.TrimRight(msg, "\n"), "\n")
	start := len(lines)
	for start > 0 && strings.TrimSpace(lines[start-1]) != "" {
		start--
	}
	if start == 0 {
		// A message with a single paragraph has no trailer block.
		return msg
	}
	kept := make([]string, 0, len(lines))
	kept = append(kept, lines[:start]...)
	for _, line := range lines[start:] {
		if !isOwnTrailer(line) {
			kept = append(kept, line)
		}
	}
	for len(kept) > 0 && strings.TrimSpace(kept[len(kept)-1]) == "" {
		kept = kept[:len(kept)-1]
	}
	return strings.Join(kept, "\n") + "\n"
}

// Body returns the message without its subject line and Flapjack-Branch
// trailer, for use as a PR description.
func Body(msg string) string {
	msg = RemoveTrailer(msg)
	_, body, _ := strings.Cut(msg, "\n")
	return strings.TrimSpace(body)
}

func isOwnTrailer(line string) bool {
	key, _, ok := strings.Cut(line, ":")
	return ok && strings.EqualFold(strings.TrimSpace(key), TrailerKey)
}

// Message returns a commit's full message exactly as stored.
func Message(r git.Repo, sha string) (string, error) {
	raw, err := r.RunWith(nil, "", "cat-file", "commit", sha)
	if err != nil {
		return "", err
	}
	_, msg, _ := strings.Cut(raw, "\n\n")
	return msg, nil
}
