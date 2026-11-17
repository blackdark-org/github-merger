# github-merger

GitHub App poller that merges labelled pull requests after their checks finish.

## Verify

- `mise run check`: gofmt, vet, and race-enabled tests.
- `mise run zizmor` after changing workflows.
- Build the Docker image after changing the container or startup configuration; CI also checks non-root startup and required configuration failure.

CI uses one required `check` gate plus actionlint and strict Renovate validation.

## Stack

Follows https://github.com/BlackDark/agent-presets/blob/main/stack/core.md.

Deviations:
- GoReleaser and the existing signed image release remain unchanged.
- Renovate does not platform-automerge here: this service is the merge driver and acts on the `automerge` label instead.

## Git

- Conventional Commits, one topic per PR.
- Squash unless commits need separate review.
- Force-push only with `--force-with-lease` on own branches.

## Boundaries

- Do not create tags, publish images, or merge dependency PRs from a local session.
- Tools come from `mise.toml`; no global installs.
