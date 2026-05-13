# Git safety rules

## Protected branch

Do not work directly on `main`.

Every change must happen in a dedicated branch with a clear name.

## Restricted actions

The following actions require explicit approval from the project owner:

- creating commits
- pushing branches
- pulling remote changes
- merging branches
- rebasing remote branches
- force-updating refs
- opening pull requests
- changing repository settings

## Safe actions

The following actions are acceptable during analysis and implementation:

- reading files
- searching code
- proposing diffs
- creating local changes
- running local tests
- documenting pending work

## Handoff

At the end of work, document what changed, what was not completed and what needs owner review.
