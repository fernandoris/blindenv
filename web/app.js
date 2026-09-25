"use strict";

const token = new URLSearchParams(location.search).get("token") || "";

let state = { projects: [], shared: { global: [], environments: [] }, shared_environments: [] };
let selected = null;
let audit = [];
let revealed = {};
const revealTimers = {};

const ui = {
  collapsed: new Set(),
  addOpen: new Set(),
  inheritedOpen: new Set(),
  filter: "",
  drawerOpen: false,
  auditFilter: { project: "", environment: "", tool: "" },
};

const SCOPE_LABEL = {
  project_environment: "Defined here",
  project: "Project defaults",
  environment: "Environment",
  global: "Shared (all)",
};

const SCOPE_BADGE = {
  project_environment: "bg-sky-900 text-sky-100",
  project: "bg-indigo-900 text-indigo-100",
  environment: "bg-amber-900 text-amber-100",
  global: "bg-slate-700 text-slate-200",
};

const SCOPE_RANK = { project: 0, environment: 1, global: 2, project_environment: -1 };

// --- HTTP ---

function api(path, options = {}) {
  const headers = Object.assign(
    { "Content-Type": "application/json" },
    { Authorization: "Bearer " + token }
  );
  return fetch(path, Object.assign({}, options, { headers })).then(async (res) => {
    const text = await res.text();
    const data = text ? JSON.parse(text) : {};
    if (!res.ok) throw new Error(data.error || res.statusText);
    return data;
  });
}

// --- DOM helpers ---

function el(html) {
  const t = document.createElement("template");
  t.innerHTML = html.trim();
  return t.content.firstElementChild;
}

function esc(s) {
  return String(s).replace(/[&<>"']/g, (c) =>
    ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c])
  );
}

function enc(s) {
  return encodeURIComponent(s);
}

function toast(message, kind = "info") {
  const styles = {
    info: "bg-slate-800 ring-slate-600",
    success: "bg-emerald-900 ring-emerald-700",
    warning: "bg-amber-900 ring-amber-700",
    error: "bg-rose-900 ring-rose-700",
  };
  const role = kind === "error" ? "alert" : "status";
  const box = el(
    `<div role="${role}" class="pointer-events-auto max-w-sm rounded px-4 py-2 text-sm ring-1 ${styles[kind] || styles.info}">${esc(message)}</div>`
  );
  document.getElementById("toasts").appendChild(box);
  setTimeout(() => box.remove(), 5000);
}

async function action(fn) {
  try {
    await fn();
    await refresh();
  } catch (err) {
    toast(err.message, "error");
  }
}

// --- State ---

async function refresh() {
  state = await api("/api/state");
  try {
    audit = (await api("/api/audit")).entries || [];
  } catch (e) {
    audit = [];
  }
  if (selected && !scopeExists(selected)) selected = null;
  if (!selected) selected = defaultScope();
  render();
}

function scopeExists(s) {
  if (s.kind === "global") return true;
  if (s.kind === "shared_env") return state.shared.environments.some((e) => e.name === s.environment);
  const p = state.projects.find((x) => x.slug === s.project);
  if (!p) return false;
  if (s.kind === "project") return true;
  return p.environments.some((e) => e.name === s.environment);
}

function defaultScope() {
  if (state.projects.length) {
    const p = state.projects[0];
    if (p.environments.length) return { kind: "project_env", project: p.slug, environment: p.environments[0].name };
    return { kind: "project", project: p.slug };
  }
  return { kind: "global" };
}

function scopeKey(s) {
  return `${s.kind}|${s.project || ""}|${s.environment || ""}`;
}

function currentTarget() {
  const s = selected;
  if (s.kind === "project" || s.kind === "project_env") {
    return { project: s.project, environment: s.environment || "" };
  }
  return { project: null, environment: s.environment || "" };
}

// --- Rendering ---

function render() {
  renderNav();
  renderEditor();
  renderActivity();
  document.getElementById("activity-count").textContent = String(audit.length);
}

function renderNav() {
  const nav = document.getElementById("scope-nav");
  nav.innerHTML = "";
  const q = ui.filter.toLowerCase();
  const match = (text) => !q || text.toLowerCase().includes(q);

  const shared = el(`<section class="mb-6">
    <h2 class="mb-2 text-xs font-semibold uppercase tracking-wide text-slate-400">Shared</h2>
    <ul class="space-y-1"></ul>
  </section>`);
  const sharedList = shared.querySelector("ul");

  const globalCount = state.shared.global.length;
  const globalItem = navItem("global", "Shared (all projects)", globalCount, { kind: "global" });
  if (match("shared all projects global")) sharedList.appendChild(globalItem);

  state.shared.environments.forEach((env) => {
    if (!match(env.name) && !match("environment " + env.name)) return;
    sharedList.appendChild(
      navItem("shared_env", env.name, env.secrets.length, { kind: "shared_env", environment: env.name }, env.name)
    );
  });
  nav.appendChild(shared);

  const projects = el(`<section>
    <h2 class="mb-2 text-xs font-semibold uppercase tracking-wide text-slate-400">Projects</h2>
    <ul class="space-y-1"></ul>
  </section>`);
  const projectList = projects.querySelector("ul");

  if (!state.projects.length) {
    projectList.appendChild(el(`<li class="px-2 py-1 text-slate-500">No projects yet.</li>`));
  }

  state.projects.forEach((p) => {
    const envMatch = match(p.slug) || p.environments.some((e) => match(e.name));
    const collapsed = ui.collapsed.has(p.slug) && !q;
    const group = el(`<li>
      <div class="flex items-center gap-1">
        <button type="button" class="flex-1 rounded px-2 py-1 text-left hover:bg-slate-800 ${isActiveProject(p.slug) ? "font-semibold" : ""}"
          aria-expanded="${!collapsed}">${esc(p.slug)}</button>
        <button type="button" class="rounded px-1 text-slate-400 hover:bg-slate-800" aria-label="Toggle ${esc(p.slug)}">${collapsed ? "+" : "-"}</button>
      </div>
      <ul class="ml-3 space-y-1 ${collapsed ? "hidden" : ""}"></ul>
    </li>`);
    const buttons = group.querySelectorAll("button");
    buttons[0].onclick = () => {
      selected = { kind: "project", project: p.slug };
      render();
    };
    buttons[1].onclick = () => {
      if (ui.collapsed.has(p.slug)) ui.collapsed.delete(p.slug);
      else ui.collapsed.add(p.slug);
      renderNav();
    };

    if (!envMatch) {
      projectList.appendChild(group);
      return;
    }

    const sub = group.querySelector("ul");
    if (match(p.slug)) sub.appendChild(navItem("project", "Project defaults", p.globals.length, { kind: "project", project: p.slug }));
    p.environments.forEach((e) => {
      if (!match(e.name) && !match(p.slug)) return;
      sub.appendChild(
        navItem("project_env", e.name, e.secrets.length, { kind: "project_env", project: p.slug, environment: e.name })
      );
    });
    projectList.appendChild(group);
  });
  nav.appendChild(projects);

  const addProject = el(`<form class="mt-4 flex gap-2">
    <input name="slug" placeholder="new-project" required class="min-w-0 flex-1 rounded bg-slate-800 px-2 py-1.5 text-sm ring-1 ring-slate-700 focus:ring-sky-500" />
    <button class="rounded bg-sky-600 px-3 py-1.5 text-sm font-medium hover:bg-sky-500">Add</button>
  </form>`);
  addProject.onsubmit = (e) => {
    e.preventDefault();
    const slug = new FormData(e.target).get("slug");
    action(() => api("/api/projects", { method: "POST", body: JSON.stringify({ slug }) }));
  };
  nav.appendChild(addProject);
}

function isActiveProject(slug) {
  return (selected.kind === "project" || selected.kind === "project_env") && selected.project === slug;
}

function navItem(kind, label, count, scope, sublabel) {
  const active = selected && selected.kind === kind && (kind === "project" || kind === "project_env"
    ? selected.project === (scope && scope.project) && selected.environment === (scope && scope.environment)
    : selected.environment === (scope && scope.environment));
  const li = el(`<li>
    <button type="button" class="flex w-full items-center justify-between rounded px-2 py-1 text-left ${
      active ? "bg-sky-600 text-white" : "hover:bg-slate-800 text-slate-200"
    }" ${active ? 'aria-current="page"' : ""}>
      <span class="truncate">${esc(label)}</span>
      <span class="ml-2 rounded-full bg-slate-800 px-2 text-xs text-slate-300">${count}</span>
    </button>
  </li>`);
  li.querySelector("button").onclick = () => {
    selected = scope;
    render();
  };
  return li;
}

function renderEditor() {
  const editor = document.getElementById("editor");
  editor.innerHTML = "";
  const view = describeScope(selected);

  const header = el(`<div class="mb-4 flex flex-wrap items-center gap-3 border-b border-slate-800 pb-4">
    <div>
      <h2 class="text-base font-semibold">${esc(view.title)}</h2>
      <p class="text-xs text-slate-400">${esc(view.subtitle)}</p>
    </div>
    ${view.allowExecute === null ? "" : `<label class="ml-auto flex items-center gap-2 text-sm text-slate-300">
      <input type="checkbox" id="allow-execute" ${view.allowExecute ? "checked" : ""} />
      allow execute
    </label>`}
    <button type="button" id="add-secret" class="rounded bg-sky-600 px-3 py-1.5 text-sm font-medium hover:bg-sky-500 md:ml-auto">+ Add secret</button>
  </div>`);
  editor.appendChild(header);

  if (view.allowExecute !== null) {
    header.querySelector("#allow-execute").onchange = (e) => {
      action(() =>
        api(`/api/projects/${enc(selected.project)}/allow-execute`, {
          method: "POST",
          body: JSON.stringify({ allow: e.target.checked }),
        })
      );
    };
  }
  header.querySelector("#add-secret").onclick = () => {
    const id = scopeKey(selected);
    if (ui.addOpen.has(id)) ui.addOpen.delete(id);
    else ui.addOpen.add(id);
    renderEditor();
  };

  if (selected.kind === "project" || selected.kind === "project_env") {
    const envBlock = el(`<div class="mb-4 flex flex-wrap gap-2">
      <form id="add-env" class="flex gap-2">
        <input name="name" placeholder="new environment" required class="rounded bg-slate-800 px-2 py-1.5 text-sm ring-1 ring-slate-700 focus:ring-sky-500" />
        <button class="rounded bg-slate-700 px-3 py-1.5 text-sm hover:bg-slate-600">Add environment</button>
      </form>
    </div>`);
    envBlock.querySelector("#add-env").onsubmit = (e) => {
      e.preventDefault();
      const name = new FormData(e.target).get("name");
      action(() => api(`/api/projects/${enc(selected.project)}/environments`, { method: "POST", body: JSON.stringify({ name }) }));
    };
    editor.appendChild(envBlock);
  }

  const defined = filterSecrets(view.defined);
  editor.appendChild(
    section(`Defined here (${view.defined.length})`, defined, {
      addOpen: ui.addOpen.has(scopeKey(selected)),
      target: currentTarget(),
      current: true,
    })
  );

  if (view.inherited && view.inherited.length) {
    const id = scopeKey(selected);
    const open = ui.inheritedOpen.has(id) || view.inherited.length <= 8;
    const inherited = el(`<section class="mb-6">
      <button type="button" class="mb-2 flex w-full items-center justify-between text-xs font-semibold uppercase tracking-wide text-slate-400"
        aria-expanded="${open}">
        <span>Inherited (${view.inherited.length})</span>
        <span>${open ? "hide" : "show"}</span>
      </button>
      <div class="${open ? "" : "hidden"}"></div>
    </section>`);
    inherited.querySelector("button").onclick = () => {
      if (ui.inheritedOpen.has(id)) ui.inheritedOpen.delete(id);
      else ui.inheritedOpen.add(id);
      renderEditor();
    };
    const body = inherited.querySelector("div");
    body.appendChild(inheritedTable(view.inherited));
    editor.appendChild(inherited);
  }

  if (view.effective) {
    editor.appendChild(effectiveSection(view.effective));
  }
}

function describeScope(s) {
  if (s.kind === "global") {
    return {
      title: "Shared (all projects)",
      subtitle: "Applies to every project and every environment",
      defined: state.shared.global,
      inherited: [],
      effective: null,
      allowExecute: null,
    };
  }
  if (s.kind === "shared_env") {
    const env = state.shared.environments.find((e) => e.name === s.environment);
    return {
      title: `Shared environment: ${s.environment}`,
      subtitle: "Applies to this environment in every project",
      defined: env ? env.secrets : [],
      inherited: state.shared.global.map((x) => ({ ...x, source: "global" })),
      effective: null,
      allowExecute: null,
    };
  }
  const p = state.projects.find((x) => x.slug === s.project);
  if (!p) return { title: s.project, subtitle: "", defined: [], inherited: [], effective: null, allowExecute: false };
  if (s.kind === "project") {
    return {
      title: `${p.slug} (all environments)`,
      subtitle: "Project defaults",
      defined: p.globals,
      inherited: state.shared.global.map((x) => ({ ...x, source: "global" })),
      effective: null,
      allowExecute: p.allow_execute,
    };
  }
  const env = p.environments.find((e) => e.name === s.environment);
  return {
    title: `${p.slug} / ${s.environment}`,
    subtitle: "Project + environment",
    defined: env ? env.secrets : [],
    inherited: env ? inheritedForEnv(env) : [],
    effective: env ? env.effective : [],
    allowExecute: p.allow_execute,
  };
}

function inheritedForEnv(env) {
  const definedKeys = new Set(env.secrets.map((s) => s.key));
  return env.effective
    .filter((e) => e.scope !== "project_environment" && !definedKeys.has(e.key))
    .map((e) => ({ key: e.key, scope: e.scope, source: e.scope }))
    .sort((a, b) => (SCOPE_RANK[a.scope] ?? 9) - (SCOPE_RANK[b.scope] ?? 9) || a.key.localeCompare(b.key));
}

function filterSecrets(list) {
  const q = ui.filter.toLowerCase();
  if (!q) return list;
  return list.filter((s) => s.key.toLowerCase().includes(q));
}

function section(title, secrets, opts) {
  const box = el(`<section class="mb-6">
    <h3 class="mb-2 text-xs font-semibold uppercase tracking-wide text-slate-400">${esc(title)}</h3>
    <div class="mb-2"></div>
  </section>`);
  const container = box.querySelector("div");

  if (opts.addOpen) {
    container.appendChild(addForm(opts.target));
  }

  const table = el(`<table class="w-full text-sm">
    <caption class="sr-only">${esc(title)}</caption>
    <thead class="text-left text-xs text-slate-500"><tr>
      <th scope="col" class="py-1">Key</th>
      <th scope="col">Value</th>
      <th scope="col" class="text-right">Actions</th>
    </tr></thead>
    <tbody></tbody>
  </table>`);
  const tbody = table.querySelector("tbody");
  if (!secrets.length) {
    tbody.appendChild(el(`<tr><td colspan="3" class="py-2 text-slate-500">No secrets defined here.</td></tr>`));
  }
  secrets.forEach((s) => tbody.appendChild(secretRow(s, opts.current)));
  container.appendChild(table);
  return box;
}

function addForm(target) {
  const form = el(`<form id="add-secret-form" class="mb-3 flex flex-wrap gap-2 rounded border border-slate-800 p-2">
    <input name="key" placeholder="KEY" required class="rounded bg-slate-800 px-2 py-1 text-sm ring-1 ring-slate-700 focus:ring-sky-500" />
    <input name="value" placeholder="value" required class="min-w-0 flex-1 rounded bg-slate-800 px-2 py-1 text-sm ring-1 ring-slate-700 focus:ring-sky-500" />
    <button class="rounded bg-sky-600 px-3 py-1 text-sm hover:bg-sky-500">Save</button>
  </form>`);
  form.onsubmit = (e) => {
    e.preventDefault();
    const fd = new FormData(e.target);
    const key = fd.get("key");
    const value = fd.get("value");
    action(async () => {
      const res = await createSecret(target, key, value);
      if (res && res.short) toast(`"${key}" is under 6 characters and will not be redacted from command output.`, "warning");
    });
  };
  form.querySelector("input").focus();
  return form;
}

function secretRow(s, current) {
  const target = current ? currentTarget() : null;
  const rid = scopeKey(selected) + "|" + (target ? (target.project || "") + "|" + target.environment : "") + "|" + s.key;
  const revealedNow = revealed[rid] !== undefined;
  const overrides = (s.overrides || []).map((o) => SCOPE_LABEL[o] || o);
  const shadow = overrides.length
    ? `<span class="ml-2 rounded bg-amber-800 px-1 text-xs text-amber-100" title="Shadows: ${esc(overrides.join(", "))}">overrides ${esc(overrides.join(", "))}</span>`
    : "";
  const tr = el(`<tr class="border-t border-slate-800 align-top">
    <td class="py-1.5 pr-4 font-mono">${esc(s.key)}${shadow}</td>
    <td class="py-1.5 pr-4 font-mono text-slate-300"><span class="value">${revealedNow ? esc(revealed[rid]) : "••••••"}</span></td>
    <td class="py-1.5 text-right"></td>
  </tr>`);
  const actions = tr.querySelector("td:last-child");
  if (current && target) {
    const revealBtn = el(`<button type="button" aria-pressed="${revealedNow}" class="rounded px-2 py-0.5 text-xs bg-slate-700 hover:bg-slate-600">${revealedNow ? "hide" : "reveal"}</button>`);
    revealBtn.onclick = () => toggleReveal(rid, target, s.key, tr, revealBtn);
    actions.appendChild(revealBtn);
    if (revealedNow) actions.appendChild(copyButton(revealed[rid]));
    const del = el(`<button type="button" class="ml-2 rounded px-2 py-0.5 text-xs bg-rose-800 hover:bg-rose-700">delete</button>`);
    del.onclick = () => requestDelete(s, target);
    actions.appendChild(del);
  } else if (!current && target) {
    const override = el(`<button type="button" class="rounded px-2 py-0.5 text-xs bg-slate-700 hover:bg-slate-600">override</button>`);
    override.onclick = () => {
      const id = scopeKey(selected);
      ui.addOpen.add(id);
      renderEditor();
      prefillAddForm(s.key);
    };
    actions.appendChild(override);
  }
  return tr;
}

function prefillAddForm(key) {
  const form = document.getElementById("add-secret-form");
  if (form) {
    form.querySelector("[name=key]").value = key;
    form.querySelector("[name=value]").focus();
  }
}

function copyButton(value) {
  const btn = el(`<button type="button" class="ml-2 rounded px-2 py-0.5 text-xs bg-slate-700 hover:bg-slate-600">copy</button>`);
  btn.onclick = () => {
    navigator.clipboard.writeText(value).then(() => toast("Copied to clipboard.", "success")).catch(() => toast("Copy failed.", "error"));
  };
  return btn;
}

async function toggleReveal(rid, target, key, tr, btn) {
  const cell = tr.querySelector(".value");
  if (revealed[rid] !== undefined) {
    delete revealed[rid];
    if (revealTimers[rid]) clearTimeout(revealTimers[rid]);
    cell.textContent = "••••••";
    btn.textContent = "reveal";
    btn.setAttribute("aria-pressed", "false");
    const copy = tr.querySelector("td:last-child button:nth-of-type(2)");
    if (copy && copy.textContent === "copy") copy.remove();
    return;
  }
  try {
    const res = await revealSecret(target, key);
    revealed[rid] = res.value;
    cell.textContent = res.value;
    btn.textContent = "hide";
    btn.setAttribute("aria-pressed", "true");
    const actions = tr.querySelector("td:last-child");
    actions.insertBefore(copyButton(res.value), actions.querySelectorAll("button")[1] || null);
    revealTimers[rid] = setTimeout(() => {
      delete revealed[rid];
      if (tr.isConnected) {
        cell.textContent = "••••••";
        btn.textContent = "reveal";
        btn.setAttribute("aria-pressed", "false");
      }
    }, 30000);
  } catch (err) {
    toast(err.message, "error");
  }
}

function inheritedTable(rows) {
  const table = el(`<table class="w-full text-sm">
    <caption class="sr-only">Inherited secrets</caption>
    <thead class="text-left text-xs text-slate-500"><tr>
      <th scope="col" class="py-1">Key</th>
      <th scope="col">Value</th>
      <th scope="col">Source</th>
      <th scope="col" class="text-right">Actions</th>
    </tr></thead>
    <tbody></tbody>
  </table>`);
  const tbody = table.querySelector("tbody");
  if (!rows.length) tbody.appendChild(el(`<tr><td colspan="4" class="py-2 text-slate-500">Nothing inherited.</td></tr>`));
  rows.forEach((s) => {
    const target = revealTargetFor(s.scope);
    const rid = "inherited|" + scopeKey(selected) + "|" + s.scope + "|" + s.key;
    const revealedNow = revealed[rid] !== undefined;
    const tr = el(`<tr class="border-t border-slate-800 text-slate-400 align-top">
      <td class="py-1.5 pr-4 font-mono">${esc(s.key)}</td>
      <td class="py-1.5 pr-4 font-mono"><span class="value">${revealedNow ? esc(revealed[rid]) : "••••••"}</span></td>
      <td class="py-1.5 pr-4"><span class="rounded px-1 text-xs ${SCOPE_BADGE[s.scope] || "bg-slate-700"}">${esc(SCOPE_LABEL[s.scope] || s.scope)}</span></td>
      <td class="py-1.5 text-right"></td>
    </tr>`);
    const actions = tr.querySelector("td:last-child");
    if (target) {
      const btn = el(`<button type="button" aria-pressed="${revealedNow}" class="rounded px-2 py-0.5 text-xs bg-slate-700 hover:bg-slate-600">${revealedNow ? "hide" : "reveal"}</button>`);
      btn.onclick = () => toggleReveal(rid, target, s.key, tr, btn);
      actions.appendChild(btn);
    }
    const override = el(`<button type="button" class="ml-2 rounded px-2 py-0.5 text-xs bg-slate-700 hover:bg-slate-600">override</button>`);
    override.onclick = () => {
      const id = scopeKey(selected);
      ui.addOpen.add(id);
      renderEditor();
      prefillAddForm(s.key);
    };
    actions.appendChild(override);
    tbody.appendChild(tr);
  });
  return table;
}

function revealTargetFor(scope) {
  if (scope === "global") return { project: null, environment: "" };
  if (scope === "environment") return { project: null, environment: selected.environment || "" };
  if (scope === "project") return { project: selected.project, environment: "" };
  return currentTarget();
}

function effectiveSection(effective) {
  const box = el(`<section class="mt-6">
    <h3 class="mb-2 text-xs font-semibold uppercase tracking-wide text-slate-400">Effective resolution</h3>
    <table class="w-full text-sm">
      <caption class="sr-only">Effective secrets for this project and environment</caption>
      <thead class="text-left text-xs text-slate-500"><tr>
        <th scope="col" class="py-1">Key</th>
        <th scope="col">Value</th>
        <th scope="col">Winning scope</th>
      </tr></thead>
      <tbody></tbody>
    </table>
  </section>`);
  const tbody = box.querySelector("tbody");
  if (!effective.length) tbody.appendChild(el(`<tr><td colspan="3" class="py-2 text-slate-500">No secrets resolve here yet.</td></tr>`));
  effective.forEach((s) => {
    const target = revealTargetFor(s.scope);
    const rid = "effective|" + scopeKey(selected) + "|" + s.scope + "|" + s.key;
    const revealedNow = revealed[rid] !== undefined;
    const tr = el(`<tr class="border-t border-slate-800 align-top">
      <td class="py-1.5 pr-4 font-mono">${esc(s.key)}</td>
      <td class="py-1.5 pr-4 font-mono text-slate-300"><span class="value">${revealedNow ? esc(revealed[rid]) : "••••••"}</span></td>
      <td class="py-1.5 pr-4"><span class="rounded px-1 text-xs ${SCOPE_BADGE[s.scope] || "bg-slate-700"}">${esc(SCOPE_LABEL[s.scope] || s.scope)}</span></td>
    </tr>`);
    if (target) {
      const td = tr.querySelector("td:last-child");
      const btn = el(`<button type="button" aria-pressed="${revealedNow}" class="rounded px-2 py-0.5 text-xs bg-slate-700 hover:bg-slate-600">reveal</button>`);
      btn.onclick = () => toggleReveal(rid, target, s.key, tr, btn);
      td.appendChild(btn);
    }
    tbody.appendChild(tr);
  });
  return box;
}

// --- Mutations ---

function createSecret(target, key, value) {
  if (target.project) {
    return api(`/api/projects/${enc(target.project)}/secrets`, {
      method: "POST",
      body: JSON.stringify({ environment: target.environment, key, value }),
    });
  }
  return api("/api/shared/secrets", {
    method: "POST",
    body: JSON.stringify({ environment: target.environment, key, value }),
  });
}

function revealSecret(target, key) {
  if (target.project) {
    return api(`/api/projects/${enc(target.project)}/secrets/reveal`, {
      method: "POST",
      body: JSON.stringify({ environment: target.environment, key }),
    });
  }
  return api("/api/shared/secrets/reveal", {
    method: "POST",
    body: JSON.stringify({ environment: target.environment, key }),
  });
}

function deleteSecretApi(target, key) {
  if (target.project) {
    return api(`/api/projects/${enc(target.project)}/secrets?environment=${enc(target.environment)}&key=${enc(key)}`, { method: "DELETE" });
  }
  return api(`/api/shared/secrets?environment=${enc(target.environment)}&key=${enc(key)}`, { method: "DELETE" });
}

function requestDelete(secret, target) {
  const impact = computeImpact(secret, target);
  const needsTyped = selected.kind !== "project_env";
  openConfirm({
    title: `Delete ${secret.key}?`,
    body: impactMessage(impact),
    needsTyped,
    confirmText: secret.key,
    onConfirm: () => action(() => deleteSecretApi(target, secret.key)),
  });
}

function computeImpact(secret, target) {
  const affected = [];
  const unaffected = [];
  state.projects.forEach((p) => {
    p.environments.forEach((e) => {
      const eff = (e.effective || []).find((x) => x.key === secret.key);
      if (!eff) return;
      const label = `${p.slug}/${e.name}`;
      if (selected.kind === "project" && eff.scope === "project") affected.push(label);
      else if ((selected.kind === "global") && eff.scope === "global") affected.push(label);
      else if ((selected.kind === "shared_env") && eff.scope === "environment" && e.name === selected.environment) affected.push(label);
      else unaffected.push(label);
    });
  });
  return { affected, unaffected };
}

function impactMessage({ affected, unaffected }) {
  const affectedText = affected.length
    ? `These scopes will lose this key: ${affected.join(", ")}.`
    : "No scope currently resolves this key from here.";
  const unaffectedText = unaffected.length ? ` ${unaffected.length} scope(s) are unaffected or already override it.` : "";
  return affectedText + unaffectedText;
}

// --- Confirm modal ---

function openConfirm({ title, body, needsTyped, confirmText, onConfirm }) {
  const root = document.getElementById("modal-root");
  const overlay = el(`<div class="fixed inset-0 z-50 flex items-center justify-center bg-slate-950/70 p-4">
    <div role="dialog" aria-modal="true" aria-label="${esc(title)}" class="w-full max-w-md rounded-lg border border-slate-700 bg-slate-900 p-5">
      <h2 class="mb-2 text-base font-semibold">${esc(title)}</h2>
      <p class="mb-3 text-sm text-slate-300">${esc(body)}</p>
      ${needsTyped ? `<label class="mb-3 block text-sm text-slate-300">Type <span class="font-mono">${esc(confirmText)}</span> to confirm
        <input id="confirm-input" class="mt-1 w-full rounded bg-slate-800 px-2 py-1 text-sm ring-1 ring-slate-700 focus:ring-rose-500" /></label>` : ""}
      <div class="flex justify-end gap-2">
        <button type="button" data-cancel class="rounded bg-slate-700 px-3 py-1.5 text-sm hover:bg-slate-600">Cancel</button>
        <button type="button" data-confirm ${needsTyped ? "disabled" : ""} class="rounded bg-rose-700 px-3 py-1.5 text-sm hover:bg-rose-600 disabled:opacity-50">Delete</button>
      </div>
    </div>
  </div>`);
  const close = () => overlay.remove();
  const confirmBtn = overlay.querySelector("[data-confirm]");
  const input = overlay.querySelector("#confirm-input");
  if (input) {
    input.focus();
    input.oninput = () => {
      confirmBtn.disabled = input.value !== confirmText;
    };
  } else {
    confirmBtn.focus();
  }
  overlay.querySelector("[data-cancel]").onclick = close;
  confirmBtn.onclick = () => {
    close();
    onConfirm();
  };
  overlay.onclick = (e) => {
    if (e.target === overlay) close();
  };
  overlay.addEventListener("keydown", (e) => {
    if (e.key === "Escape") close();
  });
  root.innerHTML = "";
  root.appendChild(overlay);
}

// --- Activity drawer ---

function openDrawer(open) {
  ui.drawerOpen = open;
  document.getElementById("activity-drawer").classList.toggle("hidden", !open);
  document.getElementById("activity-btn").setAttribute("aria-expanded", String(open));
  if (open) renderActivity();
}

function renderActivity() {
  if (!ui.drawerOpen) return;
  const filters = document.getElementById("activity-filters");
  const projects = Array.from(new Set(audit.map((a) => a.project).filter(Boolean))).sort();
  const environments = Array.from(new Set(audit.map((a) => a.environment).filter(Boolean))).sort();
  const tools = Array.from(new Set(audit.map((a) => a.tool).filter(Boolean))).sort();

  filters.innerHTML = "";
  filters.appendChild(selectFilter("project", projects, ui.auditFilter.project));
  filters.appendChild(selectFilter("environment", environments, ui.auditFilter.environment));
  filters.appendChild(selectFilter("tool", tools, ui.auditFilter.tool));

  const rows = audit.filter(
    (a) =>
      (!ui.auditFilter.project || a.project === ui.auditFilter.project) &&
      (!ui.auditFilter.environment || a.environment === ui.auditFilter.environment) &&
      (!ui.auditFilter.tool || a.tool === ui.auditFilter.tool)
  );

  const body = document.getElementById("activity-body");
  body.innerHTML = "";
  const table = el(`<table class="w-full">
    <caption class="sr-only">Audit log</caption>
    <thead class="text-left text-slate-500"><tr>
      <th scope="col" class="py-1">time</th><th scope="col">tool</th><th scope="col">project/env</th>
      <th scope="col">keys</th><th scope="col">exit</th><th scope="col">redactions</th>
    </tr></thead>
    <tbody></tbody>
  </table>`);
  const tbody = table.querySelector("tbody");
  if (!rows.length) tbody.appendChild(el(`<tr><td colspan="6" class="py-2 text-slate-500">no activity</td></tr>`));
  rows.forEach((a) => {
    const keys = (a.key_names || []).map((k, i) => {
      const sc = (a.key_scopes || [])[i];
      return sc ? `${k} (${sc})` : k;
    });
    tbody.appendChild(el(`<tr class="border-t border-slate-800 align-top">
      <td class="py-1 text-slate-400">${esc(new Date(a.timestamp).toLocaleString())}</td>
      <td>${esc(a.tool)}</td>
      <td>${esc(a.project)}${a.environment ? "/" + esc(a.environment) : ""}</td>
      <td class="font-mono">${esc(keys.join(", "))}</td>
      <td>${a.exit_code === null || a.exit_code === undefined ? "" : a.exit_code}</td>
      <td>${a.redactions || 0}</td>
    </tr>`));
  });
  body.appendChild(table);
}

function selectFilter(kind, values, current) {
  const select = el(`<label class="text-xs text-slate-400">${kind}
    <select class="ml-1 rounded bg-slate-800 px-2 py-1 text-slate-100 ring-1 ring-slate-700">
      <option value="">all</option>
      ${values.map((v) => `<option value="${esc(v)}" ${v === current ? "selected" : ""}>${esc(v)}</option>`).join("")}
    </select>
  </label>`);
  select.querySelector("select").onchange = (e) => {
    ui.auditFilter[kind] = e.target.value;
    renderActivity();
  };
  return select;
}

// --- Wiring ---

document.getElementById("activity-btn").onclick = () => openDrawer(!ui.drawerOpen);
document.querySelectorAll("[data-close-drawer]").forEach((btn) => (btn.onclick = () => openDrawer(false)));

document.getElementById("filter").addEventListener("input", (e) => {
  ui.filter = e.target.value;
  renderNav();
  renderEditor();
});

document.addEventListener("keydown", (e) => {
  if (e.key === "/" && document.activeElement.tagName !== "INPUT") {
    e.preventDefault();
    document.getElementById("filter").focus();
  }
  if (e.key === "Escape" && ui.drawerOpen) openDrawer(false);
});

refresh().catch((err) => {
  document.getElementById("editor").innerHTML = `<p class="rounded bg-rose-900 p-4">Failed to load: ${esc(err.message)}</p>`;
});
