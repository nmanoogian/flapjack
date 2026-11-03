# flapjack

`fj` manages stacked PRs from a single WIP branch. Each PR branch points at a real commit on the WIP branch; nothing is rebuilt.

## Install

```sh
go install github.com/nmanoogian/flapjack/cmd/fj@latest
```

Needs `git` and an authenticated `gh`. The repo's `origin/HEAD` must be set (`git remote set-head origin --auto`).

## Workflow

```sh
git switch -c nicm/billing-v3 origin/main
# ...commit...
fj mark HEAD~3 new-models   # HEAD~3 is the last commit of the first PR
fj mark HEAD api            # HEAD is the last commit of the second PR
fj status                   # heads, PRs, which remote branches are stale
fj push                     # pushes nicm/billing-v3_new-models and nicm/billing-v3_api, opens PRs

# address review feedback on the WIP branch
git commit --fixup <sha>
git rebase -i --autosquash origin/main
fj push

# after the bottom PR merges
fj sync
fj push
```

| Command | What it does |
|---|---|
| `fj status [--json \| --md]` | Lists heads with their PR and remote state, then orphaned PRs (marked `!`). `--json` includes each head's `descPath`. `--md` prints one `[PR 812](url) - Title (repo +a/-d)` line per PR, bottom to top |
| `fj mark <rev> <name>` | Adds a `Flapjack-Branch: <name>` trailer to a commit, or renames an existing one |
| `fj unmark <name>` | Removes a head. Its PR is left open and shows as orphaned |
| `fj push [--dry-run] [--draft] [<name>...]` | Points PR branches at their heads, pushes them atomically with `--force-with-lease`, creates or retargets PRs |
| `fj sync [--restack]` | After bottom PRs merge, rebases the WIP branch onto the base without them and retargets the next PR. `--restack` rebases onto `origin/<base>` even if nothing merged |
| `fj desc path [<name>...]` | Prints description files for all heads (bottom to top) or the named ones, creating missing files from the open PR or the head commit |
| `fj desc sync [--dry-run] [--prefer local\|remote] [<name>...]` | Syncs description files with GitHub in both directions |

flapjack doesn't run lint, type checks or tests. Check each head (the `sha` fields in `fj status --json`) before `fj push`.

## PR descriptions

PR titles and descriptions can be edited as files in `.git/flapjack/prs/<branch>.md`, shared by all worktrees:

```markdown
---
title: Add billing v3 data models
pr: 812
synced: 3f2a9c1e5b7d0a4c
---

Introduces Plan, Price and Subscription models.
```

`pr` and `synced` are managed by flapjack; edit the title and body.

```sh
$EDITOR "$(fj desc path new-models)"   # write or edit one description
$EDITOR $(fj desc path)                 # open every description in the stack
fj desc sync                            # push local edits, pull GitHub edits
```

- `fj push` creates new PRs from the description file if one exists, otherwise from the head commit's message, and writes the file either way.
- `fj desc sync` compares each file with its PR and with the version from the last sync. Local edits are pushed, GitHub edits are pulled. If both changed, it writes GitHub's version to `<branch>.remote.md` and stops; rerun with `--prefer local` or `--prefer remote` once you've merged them.
- `fj push` never updates existing descriptions; that's only `fj desc sync`.
- Renaming a head with `fj mark` moves its file to the new branch name. `fj sync` deletes files for merged PRs.
