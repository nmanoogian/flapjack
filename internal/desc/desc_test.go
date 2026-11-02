package desc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteReadRoundTrip(t *testing.T) {
	path := Path(t.TempDir(), "nicm/billing-v3_models")
	in := &Description{Title: `Add models: "v3" #1`, Body: "Line one.\n\n- item", PR: 812, Synced: "abc"}
	if err := Write(path, in); err != nil {
		t.Fatal(err)
	}
	out, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if *out != *in {
		t.Errorf("got %+v, want %+v", out, in)
	}
	if !strings.HasSuffix(filepath.ToSlash(path), "nicm/billing-v3_models.md") {
		t.Errorf("path = %s", path)
	}
}

func TestReadHandEditedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.md")
	content := "---\r\ntitle: Hand written\r\n---\r\n\r\n\r\nBody with a --- rule\r\n---\r\nafter\r\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	d, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if d.Title != "Hand written" || d.Body != "Body with a --- rule\n---\nafter" || d.PR != 0 || d.Synced != "" {
		t.Errorf("got %+v", d)
	}
}

func TestReadErrors(t *testing.T) {
	tests := map[string]struct{ content, wantErr string }{
		"no frontmatter": {"Just text\n", "missing frontmatter"},
		"unterminated":   {"---\ntitle: x\n", "unterminated"},
		"empty title":    {"---\npr: 3\n---\nbody\n", "title is empty"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "x.md")
			if err := os.WriteFile(path, []byte(tt.content), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := Read(path); err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("err = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestHashIgnoresLineEndingsAndOuterWhitespace(t *testing.T) {
	if Hash("T", "a\r\nb\n\n") != Hash(" T", "a\nb") {
		t.Error("formatting-only differences changed the hash")
	}
	if Hash("T", "a") == Hash("T", "b") || Hash("T", "a") == Hash("U", "a") {
		t.Error("content changes didn't change the hash")
	}
}

func TestReadMissingFile(t *testing.T) {
	d, err := Read(filepath.Join(t.TempDir(), "nope.md"))
	if d != nil || err != nil {
		t.Errorf("got %v, %v", d, err)
	}
}
