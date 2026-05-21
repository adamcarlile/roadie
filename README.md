# roadie

A small web app that runs on the in-car media server ("carnet") and manages syncing
media from the home NAS onto the carnet's local drive — replacing manual `rsync` over
NFS paths.

*A roadie loads and manages the gear for the road.*

## How it works

1. **Plan** — browse collections in the browser and pick titles/seasons. Picks are
   written to a **manifest** (the declared "what should be on the carnet").
2. **Sync** — press Sync; the engine diffs the manifest against the drive and copies
   what's missing, with live progress.

Sync is additive — it only copies. Removals are surfaced as drift and deleted only
on explicit confirmation.

## Status

Implemented. See the [design spec](docs/superpowers/specs/2026-05-20-roadie-design.md)
and the [implementation plan](docs/superpowers/plans/2026-05-20-roadie.md).

## Install

On the carnet box, one command:

```bash
curl -sSL https://raw.githubusercontent.com/adamcarlile/roadie/main/install.sh | sudo sh
```

It downloads the latest release, verifies its checksum, and runs a guided setup
that installs the binary, writes a default config, and enables the service. The
UI is then at `http://carnet.home.adamcarlile.com:8473`.

## Update

```bash
sudo roadie update
```

Fetches the latest release, verifies it, swaps the binary, and restarts the
service. `roadie version` prints the running version.

## Releasing

Releases are cut from git tags. Pushing a tag builds and publishes a GitHub
Release:

```bash
git tag v1.2.0
git push origin v1.2.0
```

## Build from source

```bash
go build -o roadie .
```
