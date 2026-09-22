# github-merger

Polls GitHub for open pull requests and merges the ones that are ready. It uses a GitHub App installation token and the pull request merge API.

A pull request merges only when it has every required label, none of the blocked labels, is not a draft, and its head branch is in the same repository. `mergeable` must be true. Every check suite on the head SHA must be completed, the newest suite must be at least `settle` old, and the latest run of each check name must be `success` or `skipped`. A `squash` label selects a squash merge. Otherwise the merge method is `default_merge_method` (`merge` by default).

## GitHub App

Create the app on the account that owns the repositories. Leave the webhook disabled. Install it on selected repositories, not all repositories.

Permissions:

- Contents: Read and write
- Pull requests: Read and write
- Checks: Read
- Commit statuses: Read
- Metadata: Read

Put the private key outside the git checkout.

## Configuration

Copy `config.example.yaml`. `repos` entries are `owner/name`.

Environment:

- `GITHUB_MERGER_CONFIG`: path to the YAML file
- `GITHUB_APP_ID`
- `GITHUB_APP_INSTALLATION_ID`
- `GITHUB_APP_PRIVATE_KEY`: path to the PEM file

The app id and installation id are not secrets. The PEM is.

## Run

```sh
mise exec -- go run ./cmd/github-merger
```

`github-merger.service` is a systemd unit for the same process. Install the binary at `/usr/local/bin/github-merger` and point the unit at the config and PEM.
