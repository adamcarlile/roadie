# roadie — Design Spec

**Date:** 2026-05-20
**Status:** Approved design — ready for implementation planning

---

## Purpose

`roadie` is a small web application that runs on the in-car media server ("carnet")
and manages syncing media from the home NAS onto the carnet's local drive. It
replaces the current manual workflow of `cd`-ing through NFS paths and hand-typing
escaped `rsync` commands.

*The name: a roadie loads and manages the gear for the road.*

---

## Context

- **carnet** — an in-car media server (Ubuntu). Runs Jellyfin (the in-car player the
  kids use) and Plex. Provides a WiFi hotspot for iPads. Operates fully offline once
  away from home.
- **Media library** lives on a home NAS, exposed over NFS. carnet auto-mounts it
  (autofs) when on the home network:
  - `/nfs/media` — storage pool one: contains `Kids` (kids' TV), `TV`, and others.
  - `/nfs/films` — storage pool two: contains `Movies`.
- **carnet local storage** — a 1.8 TB **exfat** external drive ("Crucial X8") mounted
  at `/media/external`; media lives under `/media/external/Media/`.
- The `/nfs/films/Movies` source is **mixed**: some films sit in their own folder,
  others are bare video files directly in `Movies/`. The design handles both
  transparently (see Manifest model).
- **Current pain** — choosing media means navigating NFS directory trees and
  composing escaped `rsync` commands by hand.

---

## Model

A **web app** served by a single Go binary on the carnet. The workflow has two phases:

1. **Plan** — browse collections in the browser, pick media objects. Each pick is
   written to the **manifest** — the declared desired state ("what should be on the
   carnet"). Nothing is copied yet.
2. **Sync** — press Sync. The engine diffs the manifest against what is actually on
   the drive and runs `rsync` for everything missing, streaming live progress to the
   page.

The manifest is the single source of truth. The caller only ever *edits the
manifest*; the sync engine is the only thing that reads the disk and drives `rsync`.

**Sync is additive.** It only ever copies. Removing an entry from the manifest does
*not* delete files on the next sync. Instead, files on disk with no manifest entry
are surfaced as **drift** and deleted only on an explicit, confirmed prune. This
protects against destroying media on a mis-click.

---

## Manifest model

The central design choice: **a manifest entry is a path, not a typed object.**

An entry is `(collection, path)` — `path` is relative to the collection's source
root. The object at that path is whatever it is — a bare film file, a film folder, a
whole-show folder, a single season folder. `rsync` copies a file and a directory with
the same invocation, so no type discriminator is needed or stored; a path absorbs
every media shape, and any future shape, with no new code.

```json
{
  "version": 1,
  "entries": [
    { "collection": "movies",  "path": "Up (2009).mkv",             "added": "2026-05-20T16:30:00Z" },
    { "collection": "movies",  "path": "Cars (2006)",               "added": "2026-05-20T16:31:00Z" },
    { "collection": "kids-tv", "path": "Bluey (2018)",              "added": "2026-05-20T16:32:00Z" },
    { "collection": "kids-tv", "path": "Hey Duggee (2014)/Series 1", "added": "2026-05-20T16:33:00Z" }
  ]
}
```

Consequences:

- **Natural key:** `collection` + `path` (unique).
- **No `kind`/`layout`/`seasons` on the entry.** `kind` (`tv` vs `movie`) lives only
  on the *collection* — it is a browse concern, not a stored property.
- **Seasons are not special.** A season pick is an entry whose path points at the
  season folder; a whole-show pick points at the show folder. This makes **no
  assumption about season-folder naming** — `Season 01`, `Series 1`, anything on disk
  works as-is.
- **Mixed movie sources just work.** A foldered film and a bare film are both just
  paths.
- **Redundant/overlapping picks are harmless.** Picking a whole show *and* one of its
  seasons causes no damage — `rsync` skips already-copied files. The UI flags the
  overlap so the user knows.
- **Artwork is out of scope.** The tool transfers the picked object as-is and nothing
  else — no sidecar hunting, no artwork logic. Jellyfin regenerates whatever artwork
  it needs on the carnet side.
- Sync status is **computed** at read time by the reconcile engine, never stored —
  so the manifest cannot go stale.

---

## Collections

Media is organised into **collections**, each a `(id, label, source, dest, kind)`
tuple defined in `collections.toml`:

| id | label | source | dest | kind |
|----|-------|--------|------|------|
| `kids-tv` | Kids TV | `/nfs/media/Kids` | `/media/external/Media/Kids` | `tv` |
| `tv` | TV | `/nfs/media/TV` | `/media/external/Media/TV` | `tv` |
| `movies` | Movies | `/nfs/films/Movies` | `/media/external/Media/Movies` | `movie` |
| `kids-movies` | Kids Movies | `/nfs/films/Movies` | `/media/external/Media/Kids Movies` | `movie` |

- `kind = tv` — the browse UI drills show → seasons.
- `kind = movie` — the browse UI shows a flat list of pickable objects (a mix of
  film folders and bare film files).
- `movies` and `kids-movies` share a source but have different dests; the user
  chooses the collection at pick time.
- Adding a collection later (e.g. `Home Movies`) is one more config block. The
  config-driven model means the tool only ever looks at configured sources,
  sidestepping junk in the NFS roots (`.DS_Store`, `._.apdisk`, stray duplicates).

---

## Components

A single Go binary containing:

1. **Config loader** — reads `collections.toml`.
2. **Manifest store** — owns `manifest.json` (a list of `(collection, path)`
   entries). Stored on the carnet's root **ext4** filesystem (journaled — exfat can
   corrupt on the car's abrupt power-off). Written atomically (temp file + rename).
   Reconstructable by scanning the drive if ever lost.
3. **Source scanner** — lists the **pickable objects** under a collection: for `tv`,
   shows and, on drill-in, their season folders; for `movie`, the top-level entries
   (both per-film folders and bare video files, filtered by extension). Returns
   paths + display names, filtering junk.
4. **Reconcile engine** — pure function `(manifest, disk scan) → plan`; classifies
   each entry as `synced`, `pending`, or `blocked`, and detects `drift`. The heart of
   the tool, and the primary test target.
5. **Sync runner** — executes the plan: one `rsync` per pending entry, parses
   progress, emits events. Single run at a time (lock).
6. **HTTP server** — JSON API plus the embedded web UI.
7. **Web UI** — embedded static assets; three screens.

**Data flow:** Browse → scanner lists pickable objects → user picks → manifest store
appends a `(collection, path)` entry. Then Sync → reconcile engine diffs manifest vs
disk → sync runner executes the pending plan → SSE events → UI progress.

---

## Data Model

### `collections.toml`

TOML, an array of `[[collection]]` tables, each with `id`, `label`, `source`,
`dest`, `kind`. Deployed to `/etc/roadie/collections.toml`.

### `manifest.json`

As described under **Manifest model** above: `version` plus an array of
`{ collection, path, added }` entries. Key is `collection` + `path`.

---

## Sync Engine

### rsync invocation (per entry)

```
rsync -rtR --modify-window=1 --no-perms --no-owner --no-group \
      --partial-dir=.rsync-partial --info=progress2 \
      --exclude='.DS_Store' --exclude='._*' --exclude='@eaDir' \
      "<collectionSource>/./<path>" "<collectionDest>/"
```

- `-R` (relative), with the `/./` pivot, recreates the entry's relative `<path>` —
  parent directories and all — under the collection dest. This is uniform whether
  `<path>` is a file or a directory.
- `--no-perms/owner/group`, no `-a` — exfat cannot store Unix perms, ownership, or
  symlinks; this avoids fighting it and the resulting error noise.
- `--modify-window=1` — absorbs exfat timestamp rounding so re-runs correctly *skip*
  already-copied files.
- `--partial-dir` — power-loss insurance: an interrupted transfer is kept aside and
  resumed on the next sync rather than restarted.
- `--info=progress2` — emits a parseable running progress line.
- No `--delete` — additive only; removals go through the confirmed-prune path.

### Execution

- One `rsync` at a time, entries in order. Parallel copies to a single USB drive fed
  by one NFS mount only cause contention — sequential is simpler and no slower.
- File or directory, whole show or single season — every entry is the same operation:
  rsync the object at its path. No branching on shape.

### Live progress (SSE)

The runner parses rsync's `--info=progress2` output and emits Server-Sent Events:
`run-start` (totals) → `entry-start` → `entry-progress` (percent, rate, ETA) →
`entry-done` → `run-done` (summary).

SSE chosen over WebSocket: one-way server→client, trivial in Go (`http.Flusher`),
and the browser's `EventSource` auto-reconnects. The runner keeps live run-state in
memory and replays a snapshot on (re)connect, so reloading the page mid-sync simply
resumes showing progress. One run at a time, enforced by a lock (`POST /api/sync`
while busy → `409`).

### Pre-flight checks (before a run)

1. **Sources mounted?** autofs mounts `/nfs/media` and `/nfs/films` on access. The
   tool stats them; an unreachable source (carnet away from home, NAS down) marks
   its entries `blocked` and reports it. All sources down → "not on the home
   network".
2. **Dest mounted?** `/media/external` must be a genuinely *mounted* exfat drive, not
   the bare mountpoint directory (otherwise a sync would silently fill the root
   disk). Sync refuses otherwise.
3. **Space?** Pending entry sizes are summed and checked against free space on the
   drive — shown in the UI before Sync, and the run is refused if it will not fit.

### Reconcile & failure handling

- **Reconcile is uniform:** an entry is `synced` if its dest path exists and a check
  shows nothing left to copy, `pending` otherwise, `blocked` if its source is
  unreachable. A partially-copied entry is simply `pending` — `rsync` finishes it.
- **Drift:** media on a collection's dest not covered by any manifest entry (a folder
  entry covers everything beneath it). Surfaced in the UI; deleted only on confirmed
  prune.
- A single entry's `rsync` failing → that entry is marked `failed` with rsync's exit
  code and stderr tail, and the run **continues**. The summary lists failures;
  re-running Sync retries the failed/pending entries.
- **Power loss** mid-sync → the manifest survives (atomic write on the journaled root
  fs), the interrupted entry stays `pending`, and the next sync resumes it via
  `--partial-dir`. The system self-heals.

---

## Web UI & API

### Screens

- **Browse** — pick a collection, see its pickable objects with a client-side fuzzy
  filter; objects already in the manifest are marked. For `tv`, click into a show to
  pick individual seasons or the whole show. Picking adds a `(collection, path)`
  entry to the manifest.
- **Manifest** — the desired-state list, each entry showing computed status (synced /
  pending / blocked). Remove an entry here. Overlapping/redundant entries are flagged
  (harmless, but worth knowing). Drift (on disk, not in manifest) is listed
  separately with a "remove" that asks for confirmation. Header shows total pending
  size vs free space.
- **Sync** — the Sync button (disabled if it will not fit, or sources are offline),
  then live overall + per-entry progress, and an end-of-run summary listing any
  failures.

### API (JSON over HTTP)

```
GET    /api/collections                          list collections (id, label, kind)
GET    /api/collections/{id}/browse?path=        pickable objects at a level
                                                 (empty path = top; a path = its children, for tv drill-in)
GET    /api/manifest                             entries + computed status + drift + space
POST   /api/manifest/entries                     add an entry { collection, path }
DELETE /api/manifest/entries?collection=&path=   remove an entry
POST   /api/sync                                 start a run (409 if already running)
GET    /api/sync/stream                          SSE live progress
GET    /api/sync/status                          current/last run snapshot
POST   /api/prune                                confirm-delete drift
```

### Frontend tech

Plain HTML + CSS + vanilla JS, embedded into the binary via Go `embed`. No Node, no
build step — preserving the single-binary, zero-toolchain deploy. The fuzzy filter is
client-side over the object list; live progress is an `EventSource`.

---

## Deployment

- A single Go binary plus `collections.toml`.
- Conventional layout on the carnet:
  - binary → `/usr/local/bin/roadie`
  - config → `/etc/roadie/collections.toml`
  - manifest → `/var/lib/roadie/manifest.json` (root ext4, journaled)
  - service → `/etc/systemd/system/roadie.service`
- Runs as an always-on **systemd service**, so the UI is simply there when browsed
  to.
- Binds `0.0.0.0` on a fixed port — chosen for predictability; on a home LAN an
  exposed sync UI is not a meaningful risk.

---

## Testing

Go's standard `testing`, table-driven. Priorities:

- **Reconcile engine** — top target. Pure (`manifest + disk snapshot → plan`), so
  exhaustive table tests: synced, pending, blocked, drift, overlapping entries.
- **rsync progress parser** — fed captured `--info=progress2` output (including
  `\r`-delimited lines), asserting parsed percent / rate / ETA.
- **Source scanner** — temp dirs seeded with folders, bare files, and junk; assert
  exclude patterns, mixed movie listing, and tv drill-in.
- **Manifest store** — atomic-write round-trip and corrupt-file recovery.
- **Sync runner** — integration test against temp source/dest dirs with *real*
  `rsync` and small fake files (both a folder entry and a bare-file entry); assert
  files land at the right relative path and events fire in order.
- **HTTP handlers** — `httptest`; API behaviour, the `409` sync lock, SSE basics.

---

## v1 Scope & Deferred Work

**v1** is everything described above.

**Deferred enhancements** (clean seams exist in this design):

- **Jellyfin auto-scan** — trigger a Jellyfin library scan via its API after a
  successful sync, so new media appears in the in-car player without waiting for
  Jellyfin's scheduled scan. A post-sync hook.
- **Rich metadata** — an optional TVDB-style metadata source for a richer browse
  experience. A seam at the scanner layer. Considered a "maybe one day", not a
  requirement.

---

## Key Decisions

| Decision | Choice | Reasoning |
|----------|--------|-----------|
| Language | **Go** | Best fit for a small web service with embedded assets and subprocess streaming; single static binary; survives the planned carnet OS upgrade with no runtime dependency. |
| Manifest model | **Path-based entries `(collection, path)`** | `rsync` copies a file or directory identically, so a folder/file type discriminator branches on a distinction the transfer ignores. A path absorbs every media shape — bare file, movie folder, season, whole show — with no enum or new code. Seasons become ordinary sub-path entries, with no assumption about season-folder naming. |
| Sync semantics | **Additive + confirmed drift prune** | A full reconcile-with-delete would destroy media on a mis-click; additive + flagged drift keeps the manifest a clean desired-state document while still allowing pruning. |
| Selection source | **Filesystem browsing** | The pain is path navigation, which a fuzzy folder browser solves directly. Avoids a Plex auth token, Plex→carnet path translation, and a Plex-uptime dependency. Folder names already carry `Title (Year)`. |
| Artwork | **Out of scope** | The tool transfers only the picked media object; Jellyfin regenerates artwork on the carnet. No sidecar or artwork logic. |
| Manifest location | **Root ext4, not exfat** | Journaled; survives the car's abrupt power-off. Atomic writes. Reconstructable from a disk scan if lost. |
| Live progress | **SSE, not WebSocket** | One-way server→client; trivial in Go; `EventSource` auto-reconnect. |
