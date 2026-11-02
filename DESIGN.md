# flapjack (`fj`)

A CLI for managing stacked PRs where one WIP branch is the source of truth and every PR branch head is a real commit from that branch. Nothing is rebuilt or synthesized.

Written in Go. Shells out to `git` and `gh`.

## Model

```
main ── c1 ── c2 ── c3 ── c4 ── c5        nicm/billing-v3   (WIP)
              │            │
              │            └─ nicm/billing-v3_api          PR base: nicm/billing-v3_new-models
              └─ nicm/billing-v3_new-models                PR base: main
```

- **WIP branch**: the currently checked-out branch. All commits and fixups go here.
  - **Base**: the branch the stack targets. Always the remote's default branch (`origin/HEAD`).
- **Head commit**: a commit in `base..WIP` whose message has a `Flapjack-Branch: <name>` trailer.
- **PR branch**: `<wip>_<name>`. Its tip is always exactly the head commit.
- **PR base**: the previous PR branch in the stack, or the base for the first PR.
- Commits after the last head commit are unpublished and only show up in `fj status`.

The trailer is the only stored state. It survives `rebase -i`, reordering and `--autosquash` because fixups keep the target commit's message.

Trailers are never stripped. The target repos squash-merge, so GitHub builds the merged commit message from the PR and the trailer never reaches main.

### Validation (all commands)

- Duplicate names in one stack → error.
- Trailers on `fixup!`/`squash!`/`amend!` commits are ignored.
- Names must be valid in a ref name (`git check-ref-format`).

## Commands

### `fj status`

Lists each head, oldest first:

```
  new-models  a1b2c3d  #812 open      in sync
  api         d4e5f6a  #813 draft     stale (remote at 9f8e7d6)
  (2 unpublished commits)
! models      -        #810 open      orphaned
```

Columns: name, head SHA, PR number and state, and whether the remote PR branch matches the head commit. `--json` gives the same data for scripts and agents.

#### Orphaned PRs

An orphaned PR is an open PR from this stack whose branch no longer matches any trailer, for example after a trailer is renamed or removed with `fj unmark`. flapjack never closes or changes orphaned PRs. `fj status` lists them after the heads, marked with `!`.

Detection matches branch-name prefixes:

```sh
gh pr list --state open --author @me --json number,state,headRefName
```

Open PRs whose `headRefName` starts with `<wip>_`, minus the branches made from current trailers, are orphans.

Known limit: prefixes can collide. If another WIP branch is named `<wip>_<something>`, that branch and its PR branches also match and show up as orphans.

### `fj mark <rev> <name>` / `fj unmark <name>`

Adds or removes the trailer on a WIP commit. Marking a head again renames it. flapjack recreates the target commit and every commit after it with `git commit-tree`, keeping trees and authors, then moves the WIP branch with `git update-ref`. Only messages change, so there are no conflicts and uncommitted work is untouched. Refuses to run during a rebase, merge, cherry-pick or revert.

### `fj push [<name>...]`

For each head (or just the named ones):

1. Stop if the PR branch is checked out in any worktree (`git worktree list --porcelain`).
2. `git branch -f <wip>_<name> <head-sha>`.
3. Push every updated branch in one call:
   `git push --atomic origin --force-with-lease=<branch>:<last-known-remote-sha> <branch>...`
4. With `gh`: create the PR if it doesn't exist (base = PR base, title and body from the head commit only; no stack list), otherwise retarget it with `gh pr edit --base` if its base changed.

`--dry-run` prints the plan and changes nothing.

flapjack does not run lint, type check or tests. The agent's instructions handle that: check out each head from `fj status --json` before calling `fj push`.

### `fj sync`

After the bottom PR(s) merge:

1. `git fetch`.
2. Find heads whose PRs are merged, starting from the bottom.
3. `git rebase --onto <base> <top-merged-head-sha> <wip>`. This drops the merged commits even if GitHub squash-merged them.
4. Retarget the next open PR to the base. Delete local branches for merged PRs.

## Out of scope (for now)

- **GitHub stacks (`gh stack link`)**: flapjack doesn't group PRs into a GitHub stack. Link them by hand with `gh stack link <pr>...` (PR numbers, bottom to top) while testing; decide later whether `fj push` should do it. Passing PR numbers rather than branch names matters: with branch names, `gh stack link` pushes the branches itself, outside flapjack's `--force-with-lease` push.
