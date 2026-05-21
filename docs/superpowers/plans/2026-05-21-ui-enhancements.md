# roadie UI Enhancements Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add sync-state icons and an inline tree to the browse page, and a full job checklist to the sync screen.

**Architecture:** One small backend change — the `run-start` sync event carries the job list. Everything else is front-end: `app.js` and `style.css` in the embedded UI. Browse-row status is decorated client-side from the existing `/api/manifest` plan, so the `reconcile` engine stays the single source of truth for `synced`/`pending`.

**Tech Stack:** Go 1.23 (`internal/syncer`), plain embedded HTML/CSS/JS (`internal/server/assets/`). No build step — `go run .` re-embeds assets on each start.

**Spec:** `docs/superpowers/specs/2026-05-21-ui-enhancements-design.md`

---

## Critical notes for the implementer

- **Edit `internal/server/assets/`, NOT the root `web/` directory.** The root `web/` is a stale byte-identical duplicate that is *not* embedded; editing it has no effect.
- **`index.html` needs no changes** in this plan — only `app.js` and `style.css`.
- The JS edits below are anchored by **content markers** (comment lines), not line numbers, because line numbers shift between tasks.
- roadie has no JS test tooling; front-end tasks are verified manually against a running server, exactly as the project's existing UI was.

---

## File structure

| File | Responsibility | Change |
|------|----------------|--------|
| `internal/syncer/event.go` | Sync event types | Add `EvJob` type; add `Jobs []EvJob` field to `Event`. |
| `internal/syncer/runner.go` | rsync execution + event emission | Populate `Jobs` on the `run-start` event. |
| `internal/syncer/runner_test.go` | Runner tests | Add a test asserting `run-start` carries the job list. |
| `internal/server/assets/app.js` | UI behaviour | Rewrite the browse section and the live-sync section. |
| `internal/server/assets/style.css` | UI styling | Append browse-row and sync-run rules. |

---

## Local verification setup (used by Tasks 2 and 3)

Run this **once** before Task 2 to build a scratch environment:

```bash
mkdir -p "/tmp/roadie-dev/src/tv/Bluey (2018)/Series 1" \
         "/tmp/roadie-dev/src/tv/Hey Duggee (2014)/Series 1" \
         "/tmp/roadie-dev/src/tv/Hey Duggee (2014)/Series 2" \
         "/tmp/roadie-dev/src/tv/Hey Duggee (2014)/Series 3" \
         "/tmp/roadie-dev/src/tv/Octonauts (2010)/Series 1" \
         "/tmp/roadie-dev/src/movies" \
         "/tmp/roadie-dev/dest/tv" "/tmp/roadie-dev/dest/movies"
for f in \
  "src/tv/Bluey (2018)/Series 1/ep1.mkv" \
  "src/tv/Hey Duggee (2014)/Series 1/ep1.mkv" \
  "src/tv/Hey Duggee (2014)/Series 2/ep1.mkv" \
  "src/tv/Hey Duggee (2014)/Series 3/ep1.mkv" \
  "src/tv/Octonauts (2010)/Series 1/ep1.mkv" \
  "src/movies/Up (2009).mkv" ; do
  head -c 1048576 /dev/zero > "/tmp/roadie-dev/$f"
done
cat > /tmp/roadie-dev/collections.toml <<'EOF'
[[collection]]
id = "tv"
label = "TV"
source = "/tmp/roadie-dev/src/tv"
dest = "/tmp/roadie-dev/dest/tv"
kind = "tv"

[[collection]]
id = "movies"
label = "Movies"
source = "/tmp/roadie-dev/src/movies"
dest = "/tmp/roadie-dev/dest/movies"
kind = "movie"
EOF
```

Start the server (re-run after each asset edit — `Ctrl-C` then up-arrow):

```bash
go run . serve -config /tmp/roadie-dev/collections.toml \
  -manifest /tmp/roadie-dev/manifest.json -addr 127.0.0.1:8473
```

Open `http://127.0.0.1:8473`.

---

## Task 1: `run-start` event carries the job list

**Files:**
- Modify: `internal/syncer/event.go`
- Modify: `internal/syncer/runner.go:38-40`
- Test: `internal/syncer/runner_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/syncer/runner_test.go` (the file already imports `context`, `os`, `path/filepath`, `testing` and has a `write` helper):

```go
func TestRunStartCarriesJobList(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src")
	dst := filepath.Join(root, "dst")
	for _, d := range []string{src, dst} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write(t, filepath.Join(src, "Cars (2006).mkv"), "movie")
	write(t, filepath.Join(src, "Up (2009).mkv"), "movie")

	jobs := []Job{
		{Collection: "movies", Path: "Cars (2006).mkv", Source: src, Dest: dst},
		{Collection: "movies", Path: "Up (2009).mkv", Source: src, Dest: dst},
	}
	var start Event
	NewRunner().Run(context.Background(), jobs, func(e Event) {
		if e.Type == EvRunStart {
			start = e
		}
	})

	want := []EvJob{
		{Collection: "movies", Path: "Cars (2006).mkv"},
		{Collection: "movies", Path: "Up (2009).mkv"},
	}
	if len(start.Jobs) != len(want) {
		t.Fatalf("run-start Jobs = %+v, want %+v", start.Jobs, want)
	}
	for i := range want {
		if start.Jobs[i] != want[i] {
			t.Errorf("Jobs[%d] = %+v, want %+v", i, start.Jobs[i], want[i])
		}
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/syncer/ -run TestRunStartCarriesJobList`
Expected: FAIL — compile error, `undefined: EvJob` and `start.Jobs undefined`.

- [ ] **Step 3: Add the `EvJob` type and `Event.Jobs` field**

In `internal/syncer/event.go`, add the `EvJob` type immediately after the `EventType` constants block (after the `)` that closes the `const` block):

```go
// EvJob names one job in a run by collection and path. The run-start event
// carries the full list so the UI can show every entry before it starts.
// It is the public projection of Job, omitting the server-internal Source/Dest.
type EvJob struct {
	Collection string `json:"collection"`
	Path       string `json:"path"`
}
```

Then add this field to the `Event` struct, immediately after the `Err string` field:

```go
	Jobs []EvJob `json:"jobs,omitempty"` // populated on run-start only
```

- [ ] **Step 4: Populate `Jobs` on the run-start event**

In `internal/syncer/runner.go`, replace the first line of the `Run` method body — currently:

```go
	emit(Event{Type: EvRunStart, TotalEntries: len(jobs)})
```

with:

```go
	refs := make([]EvJob, len(jobs))
	for i, j := range jobs {
		refs[i] = EvJob{Collection: j.Collection, Path: j.Path}
	}
	emit(Event{Type: EvRunStart, TotalEntries: len(jobs), Jobs: refs})
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./... && go vet ./...`
Expected: all packages PASS, vet clean. (`Jobs` is `omitempty`, so events without it serialise unchanged — existing server/SSE tests are unaffected.)

- [ ] **Step 6: Commit**

```bash
git add internal/syncer/event.go internal/syncer/runner.go internal/syncer/runner_test.go
git commit -m "Carry the job list on the run-start sync event"
```

---

## Task 2: Browse page — inline tree, sync-state icons, zebra & hover

**Files:**
- Modify: `internal/server/assets/style.css` (append)
- Modify: `internal/server/assets/app.js` (replace the browse section)

- [ ] **Step 1: Append browse styling to `style.css`**

Add these rules to the **end** of `internal/server/assets/style.css` (leave all existing rules in place):

```css

/* --- Browse rows -------------------------------------------------------- */
#objects li.row { display: block; padding: 0; }
.rowline { display: flex; align-items: center; gap: .5rem; padding: .5rem .8rem; }
#objects > li.row:nth-child(even) > .rowline { background: #181c22; }
.subtree > li.row:nth-child(even) > .rowline { background: #15191f; }
/* hover is declared after zebra; equal specificity means source order wins */
#objects li.row > .rowline:hover { background: #222834; box-shadow: inset 3px 0 0 #3b82f6; }
.caret { width: 1rem; color: #7a8290; user-select: none; }
.caret.active { cursor: pointer; color: #e6e6e6; }
.state { width: 1.2rem; text-align: center; }
.name { flex: 1; }
.actions { display: flex; gap: .4rem; }
li.row.covered > .rowline { opacity: .55; }
.ic.synced { color: #4ade80; }
.ic.pending, .ic.partial { color: #fbbf24; }
.ic.blocked { color: #f87171; }
.subtree { list-style: none; margin: 0 0 0 1.6rem; padding: 0;
           border-left: 2px solid #3a3f48; background: #101317; }
.row-btn { background: #3b82f6; color: #fff; border: 0; padding: .25rem .6rem;
           cursor: pointer; border-radius: 3px; }
.row-btn.remove { background: #3a3f48; color: #e6e6e6; }
```

- [ ] **Step 2: Replace the browse section of `app.js`**

In `internal/server/assets/app.js`, replace everything **from** the line `// Tab switching.` **through** the end of the `addEntry` function (the closing `}` before the `loadManifest` function) with the block below. The helpers `$`, `api`, `esc` above it and `loadManifest` / the sync code below it are left untouched.

```js
// --- Tab switching -------------------------------------------------------
document.querySelectorAll("nav button").forEach((b) => {
  b.onclick = () => {
    document.querySelectorAll("nav button").forEach((x) => x.classList.remove("active"));
    document.querySelectorAll(".tab").forEach((x) => x.classList.remove("active"));
    b.classList.add("active");
    $("#" + b.dataset.tab).classList.add("active");
    if (b.dataset.tab === "manifest") loadManifest();
    if (b.dataset.tab === "browse") refreshDecoration();
  };
});

// --- Browse --------------------------------------------------------------
let collections = [];
let planEntries = []; // picked manifest entries with computed status

async function loadCollections() {
  try {
    collections = await api("/api/collections");
  } catch (err) {
    $("#objects").innerHTML = `<li class="row">Failed to load collections: ${esc(err.message)}</li>`;
    return;
  }
  const sel = $("#collection");
  sel.innerHTML = collections
    .map((c) => `<option value="${esc(c.id)}">${esc(c.label)}</option>`)
    .join("");
  sel.onchange = () => openCollection(sel.value);
  if (collections.length) openCollection(collections[0].id);
}

// loadPlan refreshes the picked-entry list used to decorate browse rows.
async function loadPlan() {
  try {
    const v = await api("/api/manifest");
    planEntries = v.plan.entries;
  } catch {
    planEntries = []; // decoration is best-effort; browsing still works
  }
}

// covers reports whether path a equals, or is an ancestor directory of, b.
function covers(a, b) {
  return a === b || b.startsWith(a + "/");
}

// statusFor classifies a browse object against the picked manifest entries.
function statusFor(collection, path) {
  const here = planEntries.filter((e) => e.entry.collection === collection);
  const exact = here.find((e) => e.entry.path === path);
  if (exact) return { kind: "picked", status: exact.status };
  const ancestor = here.find((e) => covers(e.entry.path, path));
  if (ancestor) return { kind: "covered", status: ancestor.status };
  if (here.some((e) => covers(path, e.entry.path))) return { kind: "partial" };
  return { kind: "none" };
}

const STATE_ICONS = {
  synced: '<span class="ic synced" title="On the drive">✓</span>',
  pending: '<span class="ic pending" title="Queued to copy">↓</span>',
  blocked: '<span class="ic blocked" title="Source unavailable">✗</span>',
  partial: '<span class="ic partial" title="Some seasons picked">◐</span>',
};

// decorateRow sets one row's status icon and action button from the plan.
function decorateRow(li) {
  const state = li.querySelector(":scope > .rowline > .state");
  const actions = li.querySelector(":scope > .rowline > .actions");
  if (!state || !actions) return; // placeholder/error row — nothing to decorate
  const st = statusFor(li.dataset.collection, li.dataset.path);
  li.classList.toggle("covered", st.kind === "covered");
  if (st.kind === "picked") {
    state.innerHTML = STATE_ICONS[st.status] || "";
    actions.innerHTML = `<button class="row-btn remove">Remove</button>`;
  } else if (st.kind === "covered") {
    state.innerHTML = STATE_ICONS[st.status] || "";
    actions.innerHTML = ""; // managed via the parent row
  } else if (st.kind === "partial") {
    state.innerHTML = STATE_ICONS.partial;
    actions.innerHTML = `<button class="row-btn">Add all</button>`;
  } else {
    state.innerHTML = "";
    actions.innerHTML = `<button class="row-btn">Add</button>`;
  }
  const btn = actions.querySelector("button");
  if (btn) {
    const remove = st.kind === "picked";
    btn.onclick = (e) => {
      e.stopPropagation();
      (remove ? removeEntry : addEntry)(li.dataset.collection, li.dataset.path);
    };
  }
}

// refreshDecoration reloads the plan and re-decorates every rendered row.
async function refreshDecoration() {
  await loadPlan();
  document.querySelectorAll("#objects li.row").forEach(decorateRow);
}

// makeRow builds one <li> browse row for a pickable object.
function makeRow(col, basePath, obj) {
  const full = basePath ? basePath + "/" + obj.name : obj.name;
  const li = document.createElement("li");
  li.className = "row";
  li.dataset.name = obj.name.toLowerCase();
  li.dataset.collection = col.id;
  li.dataset.path = full;
  const expandable = obj.isDir && col.kind === "tv";
  const glyph = obj.isDir ? "📁" : "🎬";
  li.innerHTML =
    `<div class="rowline">` +
      `<span class="caret">${expandable ? "▸" : ""}</span>` +
      `<span class="state"></span>` +
      `<span class="name">${glyph} ${esc(obj.name)}</span>` +
      `<span class="actions"></span>` +
    `</div>`;
  if (expandable) {
    const caret = li.querySelector(":scope > .rowline > .caret");
    caret.classList.add("active");
    caret.onclick = (e) => {
      e.stopPropagation();
      toggleExpand(li, col);
    };
  }
  decorateRow(li);
  return li;
}

// toggleExpand opens or closes a tv folder's subtree inline.
async function toggleExpand(li, col) {
  const caret = li.querySelector(":scope > .rowline > .caret");
  const open = li.querySelector(":scope > .subtree");
  if (open) {
    open.remove();
    caret.textContent = "▸";
    return;
  }
  caret.textContent = "▾";
  const sub = document.createElement("ul");
  sub.className = "subtree";
  li.appendChild(sub);
  let objs;
  try {
    objs = await api(
      `/api/collections/${col.id}/browse?path=${encodeURIComponent(li.dataset.path)}`,
    );
  } catch (err) {
    sub.innerHTML =
      `<li class="row"><div class="rowline"><span class="name">Unavailable: ${esc(err.message)}</span></div></li>`;
    return;
  }
  objs.forEach((o) => sub.appendChild(makeRow(col, li.dataset.path, o)));
  applyFilter($("#filter").value);
}

// openCollection renders a collection's top level into #objects.
async function openCollection(id) {
  const col = collections.find((c) => c.id === id);
  const root = $("#objects");
  $("#filter").value = ""; // a stale filter from the previous collection would mislead
  if (!col) {
    root.innerHTML =
      `<li class="row"><div class="rowline"><span class="name">Unknown collection</span></div></li>`;
    return;
  }
  root.innerHTML = "";
  await loadPlan();
  let objs;
  try {
    objs = await api(`/api/collections/${id}/browse?path=`);
  } catch (err) {
    root.innerHTML =
      `<li class="row"><div class="rowline"><span class="name">Source unavailable: ${esc(err.message)}</span></div></li>`;
    return;
  }
  objs.forEach((o) => root.appendChild(makeRow(col, "", o)));
}

// applyFilter shows/hides top-level rows by name; subtrees hide while filtering.
function applyFilter(q) {
  q = q.toLowerCase();
  document.querySelectorAll("#objects > li.row").forEach((li) => {
    li.style.display = li.dataset.name && li.dataset.name.includes(q) ? "" : "none";
  });
  document.querySelectorAll("#objects .subtree").forEach((ul) => {
    ul.style.display = q ? "none" : "";
  });
}

$("#filter").oninput = (e) => applyFilter(e.target.value);

async function addEntry(collection, path) {
  try {
    await api("/api/manifest/entries", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ collection, path }),
    });
    await refreshDecoration();
  } catch (err) {
    alert(`Failed to add ${path}: ${err.message}`);
  }
}

async function removeEntry(collection, path) {
  try {
    await api(
      `/api/manifest/entries?collection=${encodeURIComponent(collection)}&path=${encodeURIComponent(path)}`,
      { method: "DELETE" },
    );
    await refreshDecoration();
  } catch (err) {
    alert(`Failed to remove ${path}: ${err.message}`);
  }
}
```

- [ ] **Step 3: Run the server and verify the browse page**

If the scratch environment from "Local verification setup" does not exist yet, create it now. Then start the server and open `http://127.0.0.1:8473`.

Verify on the **Browse** tab:
- The TV collection lists `Bluey`, `Hey Duggee`, `Octonauts`, each with a `▸` caret and an **Add** button. Rows alternate shading (zebra); hovering a row highlights it with a blue left edge.
- Clicking a caret expands the show's seasons **inline**, indented with a guide line; the caret turns to `▾`. Clicking again collapses it. Expanding a second show leaves the first expanded.
- Clicking **Add** on `Series 1` of `Hey Duggee` flips that row to a `↓` pending icon and a **Remove** button — no `alert()`. The `Hey Duggee` show row gains a `◐` partial icon and its button reads **Add all**.
- Clicking **Add** on the `Bluey` show row flips it to pending; expanding it shows its seasons with the pending icon, dimmed, and **no button** (covered).
- Switching the collection `<select>` to Movies shows a flat list (`Up (2009).mkv`) with no carets.
- Typing in the filter narrows the top-level list; clearing it restores the rows and any expanded subtrees.

- [ ] **Step 4: Commit**

```bash
git add internal/server/assets/app.js internal/server/assets/style.css
git commit -m "Add sync-state icons and an inline tree to the browse page"
```

---

## Task 3: Sync screen — full job checklist

**Files:**
- Modify: `internal/server/assets/style.css` (append)
- Modify: `internal/server/assets/app.js` (replace the live-sync section)

- [ ] **Step 1: Append sync-run styling to `style.css`**

Add these rules to the **end** of `internal/server/assets/style.css`:

```css

/* --- Sync run ----------------------------------------------------------- */
.run-head { padding: .2rem 0 1rem; }
.run-line { display: flex; justify-content: space-between; margin-bottom: .35rem; }
#run-count { color: #7a8290; }
.run-line .ok { color: #4ade80; }
.run-line .warn { color: #fbbf24; }
.run-line .err { color: #f87171; }
.bar.big { height: .55rem; }
#run-bar { width: 0; transition: width .2s; }
#run-bar.ok { background: #4ade80; }
#run-bar.warn { background: #fbbf24; }
#job-list { padding: 0; }
li.job { display: block; padding: .5rem .8rem; border-bottom: 1px solid #2a2f38; }
li.job:nth-child(even) { background: #181c22; }
li.job.copying { background: #222834; box-shadow: inset 3px 0 0 #3b82f6; }
.job-line { display: flex; align-items: center; gap: .6rem; }
.job-ic { width: 1.2rem; text-align: center; }
.job.queued .job-ic, .job.queued .job-name { color: #7a8290; }
.job.copying .job-ic { color: #3b82f6; }
.job.done .job-ic { color: #4ade80; }
.job.failed .job-ic { color: #f87171; }
.job-name { flex: 1; }
.job-meta { font-size: .75rem; color: #7a8290; }
.job-err { font-size: .72rem; color: #f87171; margin: .15rem 0 0 1.8rem; }
.bar.job-bar { height: .4rem; margin: .35rem 0 0 1.8rem; }
.sync-idle { color: #7a8290; }
.sync-err { color: #f87171; }
```

- [ ] **Step 2: Replace the live-sync section of `app.js`**

In `internal/server/assets/app.js`, replace everything **from** the line `// Live sync — one SSE connection for the page lifetime.` **through** the `$("#sync-btn").onclick = ...` statement (the last statement before the final `loadCollections();` call) with the block below. The final `loadCollections();` line stays.

```js
// --- Live sync -----------------------------------------------------------
let jobRows = {}; // "collection\0path" -> <li> for the current run
let runState = { total: 0, done: 0, copied: 0, failed: 0 };

function jobKey(collection, path) {
  return collection + " " + path;
}

// setSyncing reflects run state on the Sync button.
function setSyncing(on) {
  const btn = $("#sync-btn");
  btn.disabled = on;
  btn.textContent = on ? "Syncing…" : "Sync now";
}

// updateRunHead repaints the overall progress bar and count. curPercent is
// the in-flight entry's percent, so the bar advances smoothly within a job.
function updateRunHead(curPercent) {
  const { total, done, copied, failed } = runState;
  const frac = total ? (done + (curPercent || 0) / 100) / total : 0;
  $("#run-bar").style.width = Math.min(100, frac * 100) + "%";
  $("#run-count").textContent =
    `${done} of ${total}` +
    (copied || failed ? ` · ${copied} copied · ${failed} failed` : "");
}

// renderRunStart builds the checklist — one row per job — from run-start.
function renderRunStart(e) {
  jobRows = {};
  runState = { total: (e.jobs || []).length, done: 0, copied: 0, failed: 0 };
  const box = $("#progress");
  if (!runState.total) {
    box.innerHTML =
      `<p class="sync-idle">Everything in the manifest is already on the drive — nothing to copy.</p>`;
    setSyncing(false);
    return;
  }
  setSyncing(true);
  box.innerHTML =
    `<div class="run-head">` +
      `<div class="run-line"><strong id="run-label">Syncing</strong><span id="run-count"></span></div>` +
      `<div class="bar big"><div id="run-bar"></div></div>` +
    `</div><ul id="job-list"></ul>`;
  const list = $("#job-list");
  e.jobs.forEach((j) => {
    const li = document.createElement("li");
    li.className = "job queued";
    li.innerHTML =
      `<div class="job-line">` +
        `<span class="job-ic">·</span>` +
        `<span class="job-name">${esc(j.path)}</span>` +
        `<span class="job-meta">queued</span>` +
      `</div>`;
    jobRows[jobKey(j.collection, j.path)] = li;
    list.appendChild(li);
  });
  updateRunHead(0);
}

function onEntryStart(e) {
  const li = jobRows[jobKey(e.collection, e.path)];
  if (!li) return;
  li.className = "job copying";
  li.querySelector(".job-ic").textContent = "▶";
  li.querySelector(".job-meta").textContent = "0%";
  const bar = document.createElement("div");
  bar.className = "bar job-bar";
  bar.innerHTML = "<div></div>";
  li.appendChild(bar);
}

function onEntryProgress(e) {
  const li = jobRows[jobKey(e.collection, e.path)];
  if (!li) return;
  const fill = li.querySelector(".job-bar > div");
  if (fill) fill.style.width = e.percent + "%";
  li.querySelector(".job-meta").textContent =
    `${e.percent}%` + (e.rate ? ` · ${e.rate}` : "") + (e.eta ? ` · ETA ${e.eta}` : "");
  updateRunHead(e.percent);
}

function onEntryDone(e) {
  runState.done++;
  if (e.err) runState.failed++;
  else runState.copied++;
  const li = jobRows[jobKey(e.collection, e.path)];
  if (li) {
    const bar = li.querySelector(".job-bar");
    if (bar) bar.remove();
    li.querySelector(".job-ic").textContent = e.err ? "✗" : "✓";
    li.querySelector(".job-meta").textContent = e.err ? "failed" : "done";
    li.className = e.err ? "job failed" : "job done";
    if (e.err) {
      const err = document.createElement("div");
      err.className = "job-err";
      err.textContent = e.err; // textContent — rsync stderr is not trusted markup
      li.appendChild(err);
    }
  }
  updateRunHead(0);
}

function onRunDone(e) {
  setSyncing(false);
  if (e.err && !runState.total) {
    // Pre-flight abort published without a preceding run-start (e.g. no space).
    $("#progress").innerHTML = `<p class="sync-err">Sync aborted: ${esc(e.err)}</p>`;
    return;
  }
  const label = $("#run-label");
  if (!label) return; // empty run already showed its own message
  if (e.err) {
    label.textContent = `Sync aborted: ${e.err}`;
    label.className = "err";
  } else {
    label.textContent = `Done — ${e.copied} copied, ${e.failed} failed`;
    label.className = e.failed ? "warn" : "ok";
  }
  const bar = $("#run-bar");
  if (bar) bar.className = e.err || e.failed ? "warn" : "ok";
}

// One SSE connection for the page lifetime; the hub replays the current run
// on connect, so a mid-sync reload rebuilds the checklist where it left off.
const events = new EventSource("/api/sync/stream");
events.onmessage = (m) => {
  const e = JSON.parse(m.data);
  if (e.type === "run-start") renderRunStart(e);
  else if (e.type === "entry-start") onEntryStart(e);
  else if (e.type === "entry-progress") onEntryProgress(e);
  else if (e.type === "entry-done") onEntryDone(e);
  else if (e.type === "run-done") onRunDone(e);
};

$("#sync-btn").onclick = () => {
  setSyncing(true);
  api("/api/sync", { method: "POST" }).catch((err) => {
    const running = err.message === "409";
    if (!running) setSyncing(false); // 409 means a run is genuinely in progress
    alert("Sync: " + (running ? "a sync is already running" : err.message));
  });
};
```

- [ ] **Step 3: Make the destination a mounted drive, then run the server**

`handleSync`'s pre-flight refuses to sync unless the destination is on a drive mounted separately from `/`. For local verification, mount a tmpfs at the dest root (requires sudo):

```bash
sudo mount -t tmpfs tmpfs /tmp/roadie-dev/dest
mkdir -p /tmp/roadie-dev/dest/tv /tmp/roadie-dev/dest/movies
```

Start the server (see "Local verification setup") and open `http://127.0.0.1:8473`.

- [ ] **Step 4: Verify the sync screen**

On the **Browse** tab, Add a few items (e.g. `Bluey`, `Up (2009).mkv`, two `Hey Duggee` seasons). Then on the **Sync** tab click **Sync now** and verify:
- The button reads "Syncing…" and is disabled during the run.
- An overall header appears — a progress bar plus "X of N" — and every picked entry is listed: `·` queued → `▶` copying (with its own bar, percent, rate, ETA) → `✓` done.
- When the run finishes, the header reads `Done — N copied, 0 failed`, the overall bar turns green, and the button re-enables.
- **Failed entry:** remove a source file mid-test (`rm "/tmp/roadie-dev/src/movies/Up (2009).mkv"`), Add it again, and sync — its row shows `✗ failed` with rsync's error text inline, and the run continues to the next entry.
- **Empty run:** with everything already synced, click **Sync now** again — the screen shows "Everything in the manifest is already on the drive — nothing to copy."
- **Reload-safe:** start a sync and reload the page mid-run — the checklist rebuilds at its current position.

Unmount the tmpfs when done: `sudo umount /tmp/roadie-dev/dest`.

- [ ] **Step 5: Confirm nothing regressed and commit**

Run: `go test ./... && go vet ./...`
Expected: all PASS, vet clean.

```bash
git add internal/server/assets/app.js internal/server/assets/style.css
git commit -m "Show a full job checklist on the sync screen"
```

---

## Self-review

**Spec coverage:**
- Browse four states (none/pending/synced/partial) + covered — Task 2 (`statusFor`, `decorateRow`, `STATE_ICONS`). ✓
- Client-side decoration from `/api/manifest` (Approach B) — Task 2 (`loadPlan`, `planEntries`). ✓
- Toggle Add ⇄ Remove with instant feedback, no `alert()` on success — Task 2 (`decorateRow`, `addEntry`/`removeEntry` → `refreshDecoration`). ✓
- Inline tree, no page change, multiple expansions, no "up" — Task 2 (`toggleExpand`, `makeRow`). ✓
- Movie collections flat — Task 2 (`expandable = obj.isDir && col.kind === "tv"`). ✓
- Filter top-level only, subtrees hidden while filtering — Task 2 (`applyFilter`). ✓
- Zebra + hover — Task 2 (CSS). ✓
- Sync full job checklist with overall header, per-entry states, rate/ETA, failure tail — Task 3. ✓
- Empty run and pre-flight-abort messages — Task 3 (`renderRunStart`, `onRunDone`). ✓
- Reload-safe via SSE replay — works unchanged; verified in Task 3 Step 4. ✓
- `run-start` carries the job list (only backend change) — Task 1. ✓
- Browse tab re-decorates after Manifest-tab edits — Task 2 (tab-switch handler). ✓

**Placeholder scan:** No TBD/TODO; every code step shows complete code; every command has expected output. ✓

**Type consistency:** `EvJob{Collection, Path}` (Task 1) matches the `e.jobs` array of `{collection, path}` consumed in `renderRunStart` (Task 3). `planEntries` items use `e.entry.collection`/`e.entry.path`/`e.status`, matching `reconcile.EntryStatus`'s JSON (`entry`, `status`). `decorateRow`/`makeRow`/`toggleExpand`/`refreshDecoration`/`applyFilter`/`statusFor`/`covers` names are used consistently across Task 2. ✓
