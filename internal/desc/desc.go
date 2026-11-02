// Package desc stores PR titles and descriptions as Markdown files with
// YAML frontmatter under .git/flapjack/prs.
package desc

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/nmanoogian/flapjack/internal/git"
)

type Description struct {
	Title string
	Body  string
	// PR is the number of the PR this file was last synced with, or 0.
	PR int
	// Synced is the Hash of the title and body at the last sync, or "".
	Synced string
}

type frontmatter struct {
	Title  string `yaml:"title"`
	PR     int    `yaml:"pr,omitempty"`
	Synced string `yaml:"synced,omitempty"`
}

// Dir returns the directory holding description files. It lives in git's
// common directory, so every worktree shares it.
func Dir(r git.Repo) (string, error) {
	common, err := r.Run("rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return "", err
	}
	return filepath.Join(common, "flapjack", "prs"), nil
}

// Path returns the file for a PR branch. Slashes in the branch name become
// subdirectories.
func Path(dir, branch string) string {
	return filepath.Join(dir, filepath.FromSlash(branch)+".md")
}

// RemotePath returns where the remote version is written when the local and
// remote descriptions conflict.
func RemotePath(path string) string {
	return strings.TrimSuffix(path, ".md") + ".remote.md"
}

// Normalize converts GitHub's CRLF line endings and trims surrounding
// whitespace, so formatting-only differences don't count as edits.
func Normalize(s string) string {
	return strings.TrimSpace(strings.ReplaceAll(s, "\r\n", "\n"))
}

func Hash(title, body string) string {
	sum := sha256.Sum256([]byte(Normalize(title) + "\x00" + Normalize(body)))
	return hex.EncodeToString(sum[:8])
}

func (d *Description) Hash() string { return Hash(d.Title, d.Body) }

// Read parses a description file. It returns nil, nil if the file doesn't exist.
func Read(path string) (*Description, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	rest, ok := strings.CutPrefix(text, "---\n")
	if !ok {
		return nil, fmt.Errorf("%s: missing frontmatter (the file must start with ---)", path)
	}
	header, body, ok := strings.Cut(rest, "\n---\n")
	if !ok {
		header, ok = strings.CutSuffix(rest, "\n---")
		if !ok {
			return nil, fmt.Errorf("%s: unterminated frontmatter", path)
		}
	}
	var fm frontmatter
	if err := yaml.Unmarshal([]byte(header), &fm); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if strings.TrimSpace(fm.Title) == "" {
		return nil, fmt.Errorf("%s: title is empty", path)
	}
	return &Description{Title: strings.TrimSpace(fm.Title), Body: Normalize(body), PR: fm.PR, Synced: fm.Synced}, nil
}

func Write(path string, d *Description) error {
	header, err := yaml.Marshal(frontmatter{Title: Normalize(d.Title), PR: d.PR, Synced: d.Synced})
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	buf.WriteString("---\n")
	buf.Write(header)
	buf.WriteString("---\n")
	if body := Normalize(d.Body); body != "" {
		buf.WriteString("\n" + body + "\n")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}
