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

Design approved. See [`docs/superpowers/specs/2026-05-20-roadie-design.md`](docs/superpowers/specs/2026-05-20-roadie-design.md).
Implementation not yet started.
