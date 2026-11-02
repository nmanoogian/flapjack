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
| `fj status [--json]` | Lists heads with their PR and remote state, then orphaned PRs (marked `!`) |
| `fj mark <rev> <name>` | Adds a `Flapjack-Branch: <name>` trailer to a commit, or renames an existing one |
| `fj unmark <name>` | Removes a head. Its PR is left open and shows as orphaned |
| `fj push [--dry-run] [--draft] [<name>...]` | Points PR branches at their heads, pushes them atomically with `--force-with-lease`, creates or retargets PRs |
| `fj sync` | After bottom PRs merge, rebases the WIP branch onto the base without them and retargets the next PR |

flapjack doesn't run lint, type checks or tests. Check each head (the `sha` fields in `fj status --json`) before `fj push`.
