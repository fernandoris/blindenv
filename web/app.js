"use strict";

const token = new URLSearchParams(location.search).get("token") || "";
let state = { projects: [] };
let selected = null;
let revealed = {};
let audit = [];

function api(path, options = {}) {
  const headers = Object.assign(
    { "Content-Type": "application/json" },
    options.token ? { Authorization: "Bearer " + options.token } : { Authorization: "Bearer " + token }
  );
  return fetch(path, Object.assign({}, options, { headers })).then(async (res) => {
    const text = await res.text();
    const data = text ? JSON.parse(text) : {};
    if (!res.ok) throw new Error(data.error || res.statusText);
    return data;
  });
}

function el(html) {
  const t = document.createElement("template");
  t.innerHTML = html.trim();
  return t.content.firstElementChild;
}

function esc(s) {
  return String(s).replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));
}

async function refresh() {
  state = await api("/api/state");
  if (selected && !state.projects.find((p) => p.slug === selected)) selected = null;
  if (!selected && state.projects.length) selected = state.projects[0].slug;
  try {
    audit = (await api("/api/audit")).entries || [];
  } catch (e) {
    audit = [];
  }
  render();
}

function currentProject() {
  return state.projects.find((p) => p.slug === selected);
}

async function action(fn) {
  try {
    await fn();
    await refresh();
  } catch (err) {
    alert(err.message);
  }
}

function render() {
  const app = document.getElementById("app");
  app.innerHTML = "";
  app.appendChild(projectSidebar());
  app.appendChild(projectDetail());
  app.appendChild(auditPanel());
}

function projectSidebar() {
  const box = el(`<section class="mb-8">
    <div class="mb-3 flex items-center justify-between">
      <h2 class="text-sm font-semibold uppercase tracking-wide text-slate-400">Projects</h2>
    </div>
    <div class="flex flex-wrap gap-2" id="project-list"></div>
    <form id="add-project" class="mt-4 flex gap-2">
      <input name="slug" placeholder="new-project" required
        class="rounded bg-slate-800 px-3 py-2 text-sm outline-none ring-1 ring-slate-700 focus:ring-sky-500" />
      <button class="rounded bg-sky-600 px-3 py-2 text-sm font-medium hover:bg-sky-500">Add project</button>
    </form>
  </section>`);

  const list = box.querySelector("#project-list");
  if (!state.projects.length) {
    list.appendChild(el(`<p class="text-sm text-slate-400">No projects yet.</p>`));
  }
  state.projects.forEach((p) => {
    const active = p.slug === selected;
    const chip = el(`<button class="${
      active ? "bg-sky-600" : "bg-slate-800 hover:bg-slate-700"
    } rounded px-3 py-1 text-sm">${esc(p.slug)}${p.allow_execute ? " *" : ""}</button>`);
    chip.onclick = () => {
      selected = p.slug;
      revealed = {};
      render();
    };
    list.appendChild(chip);
  });

  box.querySelector("#add-project").onsubmit = (e) => {
    e.preventDefault();
    const slug = new FormData(e.target).get("slug");
    action(() => api("/api/projects", { method: "POST", body: JSON.stringify({ slug }) }));
  };
  return box;
}

function projectDetail() {
  const p = currentProject();
  if (!p) return el(`<p class="text-slate-400">Create a project to begin.</p>`);

  const box = el(`<section class="mb-8 rounded-lg border border-slate-700 p-4">
    <div class="mb-4 flex items-center justify-between">
      <h2 class="text-base font-semibold">${esc(p.slug)}</h2>
      <label class="flex items-center gap-2 text-sm text-slate-300">
        <input type="checkbox" id="allow-execute" ${p.allow_execute ? "checked" : ""} />
        allow execute
      </label>
    </div>
    <div id="scopes"></div>
    <form id="add-environment" class="mt-4 flex gap-2">
      <input name="name" placeholder="environment (e.g. staging)" required
        class="rounded bg-slate-800 px-3 py-2 text-sm outline-none ring-1 ring-slate-700 focus:ring-sky-500" />
      <button class="rounded bg-slate-700 px-3 py-2 text-sm hover:bg-slate-600">Add environment</button>
    </form>
  </section>`);

  box.querySelector("#allow-execute").onchange = (e) => {
    action(() => api(`/api/projects/${encodeURIComponent(p.slug)}/allow-execute`, {
      method: "POST",
      body: JSON.stringify({ allow: e.target.checked }),
    }));
  };
  box.querySelector("#add-environment").onsubmit = (e) => {
    e.preventDefault();
    const name = new FormData(e.target).get("name");
    action(() => api(`/api/projects/${encodeURIComponent(p.slug)}/environments`, {
      method: "POST",
      body: JSON.stringify({ name }),
    }));
  };

  const scopes = box.querySelector("#scopes");
  scopes.appendChild(scopeBlock(p.slug, "Global", "", p.globals));
  p.environments.forEach((env) => scopes.appendChild(scopeBlock(p.slug, env.name, env.name, env.secrets)));
  return box;
}

function scopeBlock(slug, title, environment, secrets) {
  const block = el(`<div class="mb-4 rounded border border-slate-800 p-3">
    <h3 class="mb-2 text-sm font-semibold text-slate-300">${esc(title)}</h3>
    <table class="w-full text-sm">
      <tbody></tbody>
    </table>
    <form class="mt-3 flex flex-wrap gap-2">
      <input name="key" placeholder="KEY" required class="rounded bg-slate-800 px-2 py-1 text-sm ring-1 ring-slate-700" />
      <input name="value" placeholder="value" required class="flex-1 rounded bg-slate-800 px-2 py-1 text-sm ring-1 ring-slate-700" />
      <button class="rounded bg-slate-700 px-3 py-1 text-sm hover:bg-slate-600">Save</button>
    </form>
  </div>`);

  const tbody = block.querySelector("tbody");
  if (!secrets.length) {
    tbody.appendChild(el(`<tr><td class="py-1 text-slate-500">empty</td></tr>`));
  }
  secrets.forEach((s) => {
    const rid = slug + "|" + environment + "|" + s.key;
    const isRevealed = revealed[rid] !== undefined;
    const badge = s.overrides
      ? `<span class="ml-2 rounded bg-amber-800 px-1 text-xs text-amber-100">override</span>`
      : "";
    const tr = el(`<tr class="border-t border-slate-800">
      <td class="py-1 pr-4 font-mono">${esc(s.key)}${badge}</td>
      <td class="py-1 pr-4 font-mono text-slate-300">${isRevealed ? esc(revealed[rid]) : "••••••"}</td>
      <td class="py-1 text-right"></td>
    </tr>`);
    const actions = tr.querySelector("td:last-child");
    const reveal = el(`<button class="rounded px-2 py-0.5 text-xs bg-slate-700 hover:bg-slate-600">reveal</button>`);
    reveal.onclick = () =>
      action(() => {
        if (isRevealed) {
          delete revealed[rid];
          return Promise.resolve();
        }
        return api(`/api/projects/${encodeURIComponent(slug)}/secrets/reveal`, {
          method: "POST",
          body: JSON.stringify({ environment, key: s.key }),
        }).then((r) => {
          revealed[rid] = r.value;
        });
      });
    actions.appendChild(reveal);
    const del = el(`<button class="ml-2 rounded px-2 py-0.5 text-xs bg-rose-800 hover:bg-rose-700">delete</button>`);
    del.onclick = () =>
      action(() =>
        api(`/api/projects/${encodeURIComponent(slug)}/secrets?environment=${encodeURIComponent(environment)}&key=${encodeURIComponent(s.key)}`, {
          method: "DELETE",
        })
      );
    actions.appendChild(del);
    tbody.appendChild(tr);
  });

  block.querySelector("form").onsubmit = (e) => {
    e.preventDefault();
    const fd = new FormData(e.target);
    action(() =>
      api(`/api/projects/${encodeURIComponent(slug)}/secrets`, {
        method: "POST",
        body: JSON.stringify({ environment, key: fd.get("key"), value: fd.get("value") }),
      })
    );
  };
  return block;
}

function auditPanel() {
  const box = el(`<section class="rounded-lg border border-slate-700 p-4">
    <h2 class="mb-3 text-sm font-semibold uppercase tracking-wide text-slate-400">Recent activity</h2>
    <div class="max-h-64 overflow-auto text-xs">
      <table class="w-full">
        <thead class="text-left text-slate-500"><tr>
          <th class="py-1">time</th><th>tool</th><th>project/env</th><th>keys</th><th>exit</th><th>redactions</th>
        </tr></thead>
        <tbody></tbody>
      </table>
    </div>
  </section>`);
  const tbody = box.querySelector("tbody");
  if (!audit.length) {
    tbody.appendChild(el(`<tr><td colspan="6" class="py-2 text-slate-500">no activity</td></tr>`));
  }
  audit.forEach((a) => {
    tbody.appendChild(el(`<tr class="border-t border-slate-800">
      <td class="py-1 text-slate-400">${esc(new Date(a.timestamp).toLocaleString())}</td>
      <td>${esc(a.tool)}</td>
      <td>${esc(a.project)}${a.environment ? "/" + esc(a.environment) : ""}</td>
      <td class="font-mono">${esc((a.key_names || []).join(", "))}</td>
      <td>${a.exit_code === null || a.exit_code === undefined ? "" : a.exit_code}</td>
      <td>${a.redactions || 0}</td>
    </tr>`));
  });
  return box;
}

refresh().catch((err) => {
  document.getElementById("app").innerHTML =
    `<p class="rounded bg-rose-900 p-4">Failed to load: ${esc(err.message)}</p>`;
});
