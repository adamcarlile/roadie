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

// Tab switching.
document.querySelectorAll("nav button").forEach((b) => {
  b.onclick = () => {
    document.querySelectorAll("nav button").forEach((x) => x.classList.remove("active"));
    document.querySelectorAll(".tab").forEach((x) => x.classList.remove("active"));
    b.classList.add("active");
    $("#" + b.dataset.tab).classList.add("active");
    if (b.dataset.tab === "manifest") loadManifest();
  };
});

let collections = [];

async function loadCollections() {
  try {
    collections = await api("/api/collections");
  } catch (err) {
    $("#objects").innerHTML = `<li>Failed to load collections: ${esc(err.message)}</li>`;
    return;
  }
  const sel = $("#collection");
  sel.innerHTML = collections
    .map((c) => `<option value="${esc(c.id)}">${esc(c.label)}</option>`)
    .join("");
  sel.onchange = () => browse(sel.value, "");
  if (collections.length) browse(collections[0].id, "");
}

async function browse(id, basePath) {
  const col = collections.find((c) => c.id === id);
  const ul = $("#objects");
  if (!col) {
    ul.innerHTML = "<li>Unknown collection</li>";
    return;
  }
  $("#filter").value = ""; // a stale filter from the previous view would mislead
  let objs;
  try {
    objs = await api(`/api/collections/${id}/browse?path=${encodeURIComponent(basePath)}`);
  } catch (err) {
    ul.innerHTML = `<li>Source unavailable: ${esc(err.message)}</li>`;
    return;
  }
  ul.innerHTML = "";
  if (basePath) {
    const up = document.createElement("li");
    up.innerHTML = `<span>⬆ up</span>`;
    up.onclick = () => browse(id, basePath.split("/").slice(0, -1).join("/"));
    ul.appendChild(up);
  }
  objs.forEach((o) => {
    const full = basePath ? basePath + "/" + o.name : o.name;
    const li = document.createElement("li");
    li.dataset.name = o.name.toLowerCase();
    const drill =
      o.isDir && col.kind === "tv"
        ? `<button class="row" data-act="open">Open</button>`
        : "";
    li.innerHTML = `<span>${o.isDir ? "📁" : "🎬"} ${esc(o.name)}</span>
      <span>${drill}<button class="row" data-act="add">Add</button></span>`;
    li.querySelector('[data-act="add"]').onclick = () => addEntry(id, full);
    if (drill) li.querySelector('[data-act="open"]').onclick = () => browse(id, full);
    ul.appendChild(li);
  });
}

$("#filter").oninput = (e) => {
  const q = e.target.value.toLowerCase();
  document.querySelectorAll("#objects li[data-name]").forEach((li) => {
    li.style.display = li.dataset.name.includes(q) ? "" : "none";
  });
};

async function addEntry(collection, path) {
  try {
    await api("/api/manifest/entries", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ collection, path }),
    });
    alert(`Added ${path}`);
  } catch (err) {
    alert(`Failed to add ${path}: ${err.message}`);
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
    box.insertAdjacentHTML("beforeend", `<p>Done — ${e.copied} copied, ${e.failed} failed.</p>`);
  }
};

$("#sync-btn").onclick = () =>
  api("/api/sync", { method: "POST" }).catch((e) => alert("Sync: " + e.message));

loadCollections();
