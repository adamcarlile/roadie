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

// Live sync — one SSE connection for the page lifetime.
const events = new EventSource("/api/sync/stream");
events.onmessage = (m) => {
  const e = JSON.parse(m.data);
  const box = $("#progress");
  if (e.type === "run-start") {
    box.innerHTML = `<p>Starting ${e.totalEntries} job(s)…</p>`;
  } else if (e.type === "entry-start") {
    box.insertAdjacentHTML(
      "beforeend",
      `<p>${esc(e.path)} <span class="bar"><div style="width:0%"></div></span></p>`,
    );
  } else if (e.type === "entry-progress") {
    const bars = document.querySelectorAll(".bar > div");
    if (bars.length) bars[bars.length - 1].style.width = e.percent + "%";
  } else if (e.type === "run-done") {
    const msg = e.err
      ? `Sync aborted: ${esc(e.err)}`
      : `Done — ${e.copied} copied, ${e.failed} failed.`;
    box.insertAdjacentHTML("beforeend", `<p>${msg}</p>`);
  }
};

$("#sync-btn").onclick = () =>
  api("/api/sync", { method: "POST" }).catch((e) => alert("Sync: " + e.message));

loadCollections();
