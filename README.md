# github-merger

Polls GitHub for open pull requests and merges the ones that are ready. It uses a GitHub App installation token and the pull request merge API.

A pull request merges only when it has every required label, none of the blocked labels, is not a draft, and its head branch is in the same repository. `mergeable` must be true. Every check suite that is in progress, or queued with at least one check run, must be completed. A queued suite with no runs does not block. The newest suite must be at least `settle` old, and the highest-id run of each check name must be `success` or `skipped`. A `squash` label selects a squash merge. Otherwise the merge method is `default_merge_method` (`merge` by default).

## GitHub App

Create the app on the account that owns the repositories. Leave the webhook disabled. Install it on selected repositories, not all repositories.

Repository permissions:

- Contents: Read and write. Write is required. `PUT /pulls/{n}/merge` is granted by Contents, not by Pull requests.
- Pull requests: Read. The poller only lists and loads pull requests.
- Checks: Read
- Commit statuses: Read
- Metadata: Read. GitHub adds this to every app.

No other permissions. No webhook. Pull requests write, Actions, and Administration are unused.

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

## Docker

The image is `ghcr.io/blackdark-org/github-merger`. Use `0.1.2` for this release, or `0.1` to follow the 0.1 line. The process runs as UID 65532 and does not write to disk, so the mounted PEM and config must be readable by that user.

```sh
chown 65532:65532 app.pem config.yaml
chmod 400 app.pem
```

```sh
docker run -d --name github-merger --restart unless-stopped \
  --read-only \
  -e GITHUB_APP_ID=123456 \
  -e GITHUB_APP_INSTALLATION_ID=12345678 \
  -e GITHUB_APP_PRIVATE_KEY=/secrets/app.pem \
  -e GITHUB_MERGER_CONFIG=/config/config.yaml \
  -v "$PWD/app.pem:/secrets/app.pem:ro" \
  -v "$PWD/config.yaml:/config/config.yaml:ro" \
  ghcr.io/blackdark-org/github-merger:0.1.2
```

```yaml
services:
  github-merger:
    image: ghcr.io/blackdark-org/github-merger:0.1.2
    restart: unless-stopped
    read_only: true
    environment:
      GITHUB_APP_ID: "123456"
      GITHUB_APP_INSTALLATION_ID: "12345678"
      GITHUB_APP_PRIVATE_KEY: /secrets/app.pem
      GITHUB_MERGER_CONFIG: /config/config.yaml
    volumes:
      - ./app.pem:/secrets/app.pem:ro
      - ./config.yaml:/config/config.yaml:ro
```

`config.yaml` is `config.example.yaml` with your `owner/name` repos. The container logs `start` and then skips or merges each open pull request. It exits immediately if `GITHUB_MERGER_CONFIG` is unset.
