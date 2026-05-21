const $ = (s) => document.querySelector(s);
const api = (p, opts) =>
  fetch(p, opts).then((r) => {
    if (!r.ok) throw new Error(r.status);
    return r.headers.get("content-type")?.includes("json") ? r.json() : r.text();
  });

// esc HTML-escapes a string before it is interpolated into innerHTML, so a
// filesystem name containing & < > " cannot break or inject markup.
const esc = (s) =>
  String(s).replace(/[&<>"]/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" })[c]);

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
    if (li.dataset.name === undefined) {
      li.style.display = ""; // placeholder/error row — never filtered out
      return;
    }
    li.style.display = li.dataset.name.includes(q) ? "" : "none";
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

async function loadManifest() {
  let v;
  try {
    v = await api("/api/manifest");
  } catch (err) {
    $("#space").textContent = `Failed to load manifest: ${err.message}`;
    return;
  }
  $("#space").textContent = `Pending: ${v.pendingMB} MB · Free: ${v.freeMB} MB`;
  const ul = $("#entries");
  ul.innerHTML = "";
  v.plan.entries.forEach((es) => {
    const li = document.createElement("li");
    const ov = es.overlap
      ? ` <span class="overlap">(redundant — covered by another entry)</span>`
      : "";
    li.innerHTML = `<span class="status-${esc(es.status)}">[${esc(es.status)}] ${esc(es.entry.collection)} / ${esc(es.entry.path)}${ov}</span>
      <button class="row">Remove</button>`;
    li.querySelector("button").onclick = async () => {
      try {
        await api(
          `/api/manifest/entries?collection=${encodeURIComponent(es.entry.collection)}&path=${encodeURIComponent(es.entry.path)}`,
          { method: "DELETE" },
        );
        loadManifest();
      } catch (err) {
        alert(`Failed to remove: ${err.message}`);
      }
    };
    ul.appendChild(li);
  });
  const d = $("#drift");
  d.innerHTML = "";
  if (!v.plan.drift.length) {
    d.innerHTML = "<li>None</li>";
    return;
  }
  v.plan.drift.forEach((x) => {
    const li = document.createElement("li");
    li.innerHTML = `<span>${esc(x.collection)} / ${esc(x.path)}</span><button class="row">Remove</button>`;
    li.querySelector("button").onclick = async () => {
      if (!confirm(`Delete ${x.collection} / ${x.path} from the carnet drive?`)) return;
      try {
        await api("/api/prune", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ collection: x.collection, path: x.path }),
        });
        await loadManifest();
      } catch (err) {
        alert(`Failed to remove ${x.path}: ${err.message}`);
      }
    };
    d.appendChild(li);
  });
}

// --- Live sync -----------------------------------------------------------
let jobRows = {}; // "collection\0path" -> <li> for the current run
let runState = { total: 0, done: 0, copied: 0, failed: 0 };

function jobKey(collection, path) {
  return collection + "\u0000" + path;
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

loadCollections();
