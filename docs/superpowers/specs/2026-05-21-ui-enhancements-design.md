# roadie UI enhancements — Design Spec

**Date:** 2026-05-21
**Status:** Approved design — ready for implementation planning

---

## Purpose

Three improvements to the roadie web UI, none of which change the sync engine or
the manifest model:

1. **Browse page** — show each object's sync state, make the table easier to
   read on a large screen, and replace the page-swapping tv drill-in with an
   inline expanding tree.
2. **Sync screen** — replace the sparse "occasional name" output with a full job
   checklist and clear overall progress.

The `reconcile` engine, the manifest store, the scanner, and the API surface are
unchanged. The only backend change is one extra field on the sync `Event`.

---

## Enhancement 1 — Browse page

### Row states

Every browse row carries one of four states, shown as a leading icon:

| State | Icon | Meaning |
|-------|------|---------|
| `synced` | ✓ green | Picked, and already on the carnet drive. |
| `pending` | ↓ amber | Picked, will be copied on the next sync. |
| `partial` | ◐ amber | A tv folder with *some* of its seasons picked individually (the folder itself is not a manifest entry). |
| *(none)* | — | Not picked. |

A fifth, derived presentation — **covered** — applies to a row that is itself not
a manifest entry but sits *inside* a whole-folder pick (e.g. a season inside a
show that was added wholesale). A covered row shows its parent entry's icon
(`synced`/`pending`), dimmed, and has **no action button** — it cannot be removed
on its own; it is managed through the parent row.

### Status data flow (Approach B — client-side decoration)

The browse endpoint (`GET /api/collections/{id}/browse`) is **unchanged** — it
remains a pure disk lister returning `{name, isDir}` objects.

`GET /api/manifest` already returns the full reconciled plan: every picked entry
with its `synced`/`pending`/`blocked` status, computed by the pure `reconcile`
engine. The Browse tab fetches that plan and decorates rows client-side:

- On initial load, on switching to the Browse tab, and after every Add/Remove,
  the client fetches `/api/manifest` and keeps `plan.entries`.
- For a rendered row at full path `P` in collection `C`, against the entries
  filtered to collection `C`:
  - **picked** — an entry's path equals `P`. State = that entry's status.
  - **covered** — no exact entry, but some entry's path `A` is an ancestor of `P`
    (`P` starts with `A + "/"`). Icon = ancestor entry's status; no button.
  - **partial** — no exact entry and no ancestor entry, but some entry's path is
    a descendant of `P` (it starts with `P + "/"`). State = `partial`.
  - **none** — otherwise.
  - Precedence: picked > covered > partial > none.

`reconcile` stays the single source of truth for `synced` vs `pending`; the
client only adds the `partial`/`covered` *relationships*, which are derived from
the same entry list with the same ancestor test the engine uses.

### Action button

The per-row button reflects state, replacing today's blind "Add" + `alert()`:

| State | Button | Action |
|-------|--------|--------|
| none | **Add** | `POST /api/manifest/entries` for this path. |
| partial | **Add all** | `POST` the folder path — adds the whole show. Overlaps the existing season picks; this is harmless and the Manifest tab already flags it as redundant. |
| picked | **Remove** | `DELETE /api/manifest/entries` for this path. |
| covered | *(no button)* | Managed via the parent row. |

After a successful Add or Remove, the client refetches the plan and re-runs
decoration over all rendered rows, so the clicked row's icon and button update
immediately — no `alert()`, no page reload.

### Inline tree

The tv drill-in stops swapping the whole list. Instead:

- Top-level objects for the selected collection render as before.
- In a **tv** collection, every directory row gets a **caret** (`▸` collapsed /
  `▾` expanded). Clicking it fetches that directory's children
  (`GET /api/collections/{id}/browse?path=<full path>`) and renders them as a
  nested subtree directly below the row; clicking again collapses it.
- A subtree is a `<ul>` nested inside the parent `<li>`, indented one step and
  fenced by a vertical guide line with a slightly darker background, so its
  scope is unambiguous. A deeper level (a season expanded into its episode
  files) steps in again.
- Multiple rows may be expanded at once. Expansions and scroll position survive,
  because nothing is re-rendered — newly fetched rows are simply inserted.
- The `⬆ up` row is removed; there is no page change, so no "back".
- **Movie** collections stay a flat list — no carets, no expansion.
- Switching collection via the `<select>` resets to that collection's top level.

### Filter

The client-side fuzzy filter is kept. While a filter string is active, it
matches **top-level rows only**; expanded subtrees are hidden for the duration
and restored when the filter is cleared. Finding a *show* is the common case and
shows are always top-level; this avoids orphaned child rows under a
filtered-out parent. A deliberate v1 simplification.

### Zebra & hover

Pure CSS in `style.css`:

- **Zebra** — alternating row background shading, for legibility on a wide
  screen.
- **Hover** — the hovered row lightens and gains a blue inset left edge.

---

## Enhancement 2 — Sync screen

### Full job checklist

The sync screen lists **every pending entry upfront** and moves each through its
lifecycle, under an overall progress header.

**Overall header** — a progress bar, an "*X of N*" count, and a running
`copied · failed` tally. The bar advances smoothly: completed entries plus the
current entry's own percentage, i.e. `(doneEntries + currentPercent/100) / N`.

**Per-entry rows** — each entry is one row moving through:

- `· queued` (grey)
- `▶ copying` (blue) — with its own inline progress bar, transfer rate, and ETA
- `✓ done` (green) **or** `✗ failed` (red) — a failed row shows rsync's error
  tail inline

**After the run** — the checklist stays on screen with every entry's final
mark; the header reads `Done — C copied, F failed`.

**Nothing to do** — a run with no pending entries shows
"Everything in the manifest is already on the drive — nothing to copy."

**Reload-safe** — the SSE hub already replays the current run's events to a new
subscriber, so reloading mid-sync rebuilds the checklist exactly where it is.

The **Sync now** button is disabled while a run is in progress (driven by
`run-start` / `run-done`) and re-enabled when it finishes.

### Backend change — `run-start` carries the job list

Today `run-start` carries only `TotalEntries` (a count). To list every entry
upfront, the event must also carry the jobs. This is the *only* backend change.

**`internal/syncer/event.go`** — add a job-reference type and an `Event` field:

```go
// EvJob names one job in a run by collection and path. The run-start event
// carries the full list so the UI can show every entry before it starts.
type EvJob struct {
    Collection string `json:"collection"`
    Path       string `json:"path"`
}
```

`Event` gains `Jobs []EvJob \`json:"jobs,omitempty"\``. `Source`/`Dest` are
server-internal and deliberately excluded — `EvJob` is the public projection of
`Job`, mirroring how `Collection` already excludes its `Source`/`Dest` from JSON.

**`internal/syncer/runner.go`** — `Run` builds the `EvJob` slice from its
`jobs` argument and includes it on the `EvRunStart` event. No other event
changes.

### Front-end event handling

`app.js` rewrites the `EventSource` handler:

- **`run-start`** — clear the panel, render the overall header and one checklist
  row per `e.jobs` entry (all `queued`), keep a `collection\0path → row` map,
  disable the Sync button. Empty `jobs` → show the "nothing to do" message.
- **`entry-start`** — find the row by `collection`+`path`; set it `copying`, add
  its inline bar.
- **`entry-progress`** — update the current row's bar, rate, and ETA; update the
  overall bar.
- **`entry-done`** — set the row `✓ done` or `✗ failed` (with `e.err` inline);
  update the `X of N` count, the tally, and the overall bar.
- **`run-done`** — header → `Done — C copied, F failed`; re-enable the Sync
  button. A `run-done` carrying `e.err` (e.g. the pre-flight "not enough space"
  abort, which is published without a preceding `run-start`) shows that error
  message.

---

## Components touched

| File | Change |
|------|--------|
| `internal/syncer/event.go` | Add `EvJob` type; add `Jobs []EvJob` to `Event`. |
| `internal/syncer/runner.go` | Populate `Jobs` on the `run-start` event. |
| `internal/syncer/runner_test.go` | Assert `run-start` carries the job list. |
| `internal/server/assets/app.js` | Browse: plan fetch + row decoration, inline tree, toggle button; Sync: checklist rendering. |
| `internal/server/assets/style.css` | Status icons, zebra, hover, tree indentation/guide line, checklist row states, overall progress bar. |
| `internal/server/assets/index.html` | Minor structural tweaks if the new markup needs them. |

No change to `reconcile`, `manifest`, `scan`, `config`, the HTTP handlers, or
the API surface. The root `web/` directory is a stale duplicate of
`internal/server/assets/` and is **not** the embedded copy; editing it has no
effect — out of scope to reconcile here.

---

## Testing

roadie's convention: table-driven Go tests; no JS test tooling.

- **`runner_test.go`** — extend to assert the `run-start` event's `Jobs` slice
  matches the jobs passed to `Run` (collection + path, in order). Existing runner
  tests must still pass.
- **No new Go tests elsewhere** — `reconcile`, the handlers, and the API are
  unchanged, so their existing tests cover them as-is.
- **Front-end** — verified manually against a running server: browse-row states
  and the toggle button, tree expand/collapse with indentation, zebra/hover, and
  a sync run showing the checklist progressing (including a forced failure and
  an empty run).

---

## Out of scope

- Any change to the sync engine, rsync invocation, manifest model, or reconcile
  logic.
- Server-side decoration of the browse endpoint (Approach A) — rejected to keep
  `reconcile` the single source of truth for status.
- Filtering *into* expanded subtrees — the filter matches top-level rows only.
- A build step or front-end framework — the UI stays plain embedded HTML/CSS/JS.
- Reconciling the stale root `web/` directory against `internal/server/assets/`.

---

## Key Decisions

| Decision | Choice | Reasoning |
|----------|--------|-----------|
| Browse status source | **Client decorates from `/api/manifest`** | The reconcile engine already computes `synced`/`pending`; reusing its output keeps one source of truth instead of duplicating classification in the browse handler. |
| Partial tv folders | **Distinct `partial` state** | A show with some seasons picked is neither synced nor pending; rolling it up to either would mislead. A fourth state is honest. |
| Covered rows | **Icon only, no button** | A season inside a whole-show pick has no manifest entry of its own; offering "Remove" on it would be a lie. It is managed via the parent. |
| Browse row button | **Toggles Add ⇄ Remove** | Makes the browse page a full pick/un-pick surface and gives the immediate click feedback the current `alert()` lacks. |
| tv drill-in | **Inline expanding tree** | Page-swapping loses scroll position and location; inline expansion preserves both and shows context. |
| Sync screen | **Full job checklist** | Every entry visible upfront with per-entry state and progress replaces the sparse, statusless output. |
| `run-start` payload | **Carries the job list** | The checklist needs every entry's name before the run starts; a bare count cannot provide it. One small, isolated event change. |
