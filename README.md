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

## Build & deploy

```bash
GOOS=linux GOARCH=amd64 go build -o roadie ./...
scp roadie adam@carnet.home.adamcarlile.com:/tmp/roadie
scp collections.toml.example adam@carnet.home.adamcarlile.com:/tmp/
scp deploy/roadie.service adam@carnet.home.adamcarlile.com:/tmp/
```

On the carnet box:
```bash
sudo install -m755 /tmp/roadie /usr/local/bin/roadie
sudo mkdir -p /etc/roadie /var/lib/roadie
sudo cp /tmp/collections.toml.example /etc/roadie/collections.toml
sudo cp /tmp/roadie.service /etc/systemd/system/roadie.service
sudo systemctl enable --now roadie
```

The UI is then at `http://carnet.home.adamcarlile.com:8473`.
