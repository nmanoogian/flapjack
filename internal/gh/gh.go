// Package gh reads and updates pull requests through the gh CLI.
package gh

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

const (
	StateOpen   = "OPEN"
	StateClosed = "CLOSED"
	StateMerged = "MERGED"
)

type PR struct {
	Number      int    `json:"number"`
	State       string `json:"state"`
	IsDraft     bool   `json:"isDraft"`
	HeadRefName string `json:"headRefName"`
	BaseRefName string `json:"baseRefName"`
	URL         string `json:"url"`
	Title       string `json:"title"`
	Additions   int    `json:"additions"`
	Deletions   int    `json:"deletions"`
	Body        string `json:"body"`
}

type Client interface {
	// ListOpen returns the open PRs authored by the current user.
	ListOpen() ([]PR, error)
	// ForBranch returns the most recent PR in any state with head branch,
	// or nil if there is none.
	ForBranch(branch string) (*PR, error)
	Create(head, base, title, body string, draft bool) (PR, error)
	SetBase(number int, base string) error
	SetDescription(number int, title, body string) error
}

type CLI struct {
	Dir string
}

const prFields = "number,state,isDraft,headRefName,baseRefName,url,title,body,additions,deletions"

func (c CLI) ListOpen() ([]PR, error) {
	var prs []PR
	err := c.json(&prs, "pr", "list", "--state", "open", "--author", "@me", "--limit", "1000", "--json", prFields)
	return prs, err
}

func (c CLI) ForBranch(branch string) (*PR, error) {
	var prs []PR
	if err := c.json(&prs, "pr", "list", "--state", "all", "--head", branch, "--limit", "1", "--json", prFields); err != nil {
		return nil, err
	}
	if len(prs) == 0 {
		return nil, nil
	}
	return &prs[0], nil
}

func (c CLI) Create(head, base, title, body string, draft bool) (PR, error) {
	args := []string{"pr", "create", "--head", head, "--base", base, "--title", title, "--body", body}
	if draft {
		args = append(args, "--draft")
	}
	out, err := c.run(args...)
	if err != nil {
		return PR{}, err
	}
	url := strings.TrimSpace(out)
	if i := strings.LastIndex(url, "\n"); i >= 0 {
		url = url[i+1:]
	}
	number, err := strconv.Atoi(url[strings.LastIndex(url, "/")+1:])
	if err != nil {
		return PR{}, fmt.Errorf("gh pr create: unexpected output %q", out)
	}
	return PR{Number: number, State: StateOpen, IsDraft: draft, HeadRefName: head, BaseRefName: base, URL: url, Title: title, Body: body}, nil
}

func (c CLI) SetBase(number int, base string) error {
	_, err := c.run("pr", "edit", strconv.Itoa(number), "--base", base)
	return err
}

func (c CLI) SetDescription(number int, title, body string) error {
	_, err := c.runInput(body, "pr", "edit", strconv.Itoa(number), "--title", title, "--body-file", "-")
	return err
}

func (c CLI) json(v any, args ...string) error {
	out, err := c.run(args...)
	if err != nil {
		return err
	}
	if err := json.Unmarshal([]byte(out), v); err != nil {
		return fmt.Errorf("gh %s: %w", strings.Join(args, " "), err)
	}
	return nil
}

func (c CLI) run(args ...string) (string, error) {
	return c.runInput("", args...)
}

func (c CLI) runInput(stdin string, args ...string) (string, error) {
	cmd := exec.Command("gh", args...)
	cmd.Dir = c.Dir
	cmd.Stdin = strings.NewReader(stdin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = err.Error()
		}
		return "", fmt.Errorf("gh %s: %s", strings.Join(args[:2], " "), detail)
	}
	return stdout.String(), nil
}
