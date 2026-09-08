"use strict";

/* agenttik UI: one page, no framework. State lives in `S`, the DOM is
   re-rendered from it, and live turn output arrives over SSE. */

const S = {
  providers: [],
  stars: [],
  projects: [],
  sessions: [],
  detail: null,        // { session, messages, turns, stats, running }
  tabs: [{ id: "conversation", label: "Conversation", closable: false }],
  activeTab: "conversation",
  stream: null,        // EventSource
  live: null,          // element receiving streamed text
};

const $ = (sel) => document.querySelector(sel);
const el = (tag, cls, text) => {
  const n = document.createElement(tag);
  if (cls) n.className = cls;
  if (text !== undefined) n.textContent = text;
  return n;
};

/* ------------------------------------------------------------------ API */

async function api(method, path, body) {
  const res = await fetch(path, {
    method,
    headers: body ? { "Content-Type": "application/json" } : undefined,
    body: body ? JSON.stringify(body) : undefined,
  });
  if (res.status === 204) return null;
  const text = await res.text();
  const data = text ? JSON.parse(text) : null;
  if (!res.ok) throw new Error((data && data.error) || res.statusText);
  return data;
}

function fail(err) {
  console.error(err);
  alert(err.message || String(err));
}

/* ------------------------------------------------------------- formatting */

const nf = new Intl.NumberFormat();

function tokens(n) {
  if (n >= 1_000_000) return (n / 1_000_000).toFixed(2) + "M";
  if (n >= 1000) return (n / 1000).toFixed(1) + "k";
  return nf.format(n || 0);
}

function duration(ms) {
  if (!ms) return "0s";
  const s = Math.round(ms / 1000);
  if (s < 60) return s + "s";
  const m = Math.floor(s / 60);
  if (m < 60) return `${m}m ${s % 60}s`;
  return `${Math.floor(m / 60)}h ${m % 60}m`;
}

function ago(ts) {
  const s = Math.round((Date.now() - ts) / 1000);
  if (s < 60) return "just now";
  if (s < 3600) return Math.floor(s / 60) + "m ago";
  if (s < 86400) return Math.floor(s / 3600) + "h ago";
  return Math.floor(s / 86400) + "d ago";
}

/* ----------------------------------------------------------------- tabs */

/* wireTabs switches panes inside one column only, so the sidebar and the
   inspector do not fight over each other's panes. */
function wireTabs(strip, root) {
  strip.addEventListener("click", (e) => {
    const tab = e.target.closest(".tab");
    if (!tab) return;
    [...strip.children].forEach((t) => t.classList.toggle("active", t === tab));
    root.querySelectorAll(".pane").forEach((p) => {
      p.classList.toggle("active", p.id === "pane-" + tab.dataset.pane);
    });
  });
}

/* ------------------------------------------------------------- providers */

async function loadProviders() {
  S.providers = await api("GET", "/api/providers");
  S.stars = await api("GET", "/api/stars");
}

function providerOf(name) {
  return S.providers.find((p) => p.name === name);
}

function isStarred(provider, model, effort) {
  return S.stars.some((s) => s.provider === provider && s.model === model && s.effort === effort);
}

/* Starred combinations sort to the top of the model picker. */
function renderModelPickers() {
  const modelSel = $("#model-select");
  const effortSel = $("#effort-select");
  modelSel.innerHTML = "";
  effortSel.innerHTML = "";
  if (!S.detail) return;

  const sess = S.detail.session;
  const p = providerOf(sess.provider);
  if (!p) return;

  const starredFirst = [...p.models].sort((a, b) => {
    const sa = S.stars.some((s) => s.provider === p.name && s.model === a.id);
    const sb = S.stars.some((s) => s.provider === p.name && s.model === b.id);
    return sa === sb ? 0 : sa ? -1 : 1;
  });
  for (const m of starredFirst) {
    const star = S.stars.some((s) => s.provider === p.name && s.model === m.id) ? "★ " : "";
    modelSel.append(new Option(star + m.label, m.id, false, m.id === sess.model));
  }
  effortSel.append(new Option("default effort", ""));
  for (const e of p.efforts) {
    const star = isStarred(p.name, sess.model, e) ? "★ " : "";
    effortSel.append(new Option(star + e, e, false, e === sess.effort));
  }
  $("#star-btn").classList.toggle("on", isStarred(sess.provider, sess.model, sess.effort));
  $("#star-btn").textContent = isStarred(sess.provider, sess.model, sess.effort) ? "★" : "☆";
}

/* ------------------------------------------------------------- projects */

async function refreshProjects() {
  S.projects = await api("GET", "/api/projects");
  const list = $("#project-list");
  list.innerHTML = "";
  if (!S.projects.length) {
    list.append(el("div", "empty", "No projects yet. Add the folder you want to work in."));
    return;
  }
  for (const p of S.projects) {
    const box = el("div", "project");
    const head = el("div", "project-head");
    head.append(el("span", "project-name", p.name));
    const nu = el("button", "ghost new", "+");
    nu.title = "New session in this project";
    nu.onclick = () => openNewSessionDialog(p);
    const del = el("button", "ghost del", "×");
    del.title = "Remove project";
    del.onclick = () => removeProject(p);
    head.append(nu, del);
    box.append(head, el("div", "project-path", p.path));

    for (const s of p.recent_sessions) {
      box.append(sessionRow(s.id, s.title || "Untitled session", s.status, null));
    }
    list.append(box);
  }
}

async function removeProject(p) {
  if (!confirm(`Remove project "${p.name}"?\n\nIts sessions and their history are deleted from agenttik. The folder on disk is untouched.`)) return;
  try {
    await api("DELETE", "/api/projects/" + p.id);
    if (S.detail && S.detail.session.project_id === p.id) closeSession();
    await refreshProjects();
    await refreshSessions();
  } catch (e) { fail(e); }
}

function sessionRow(id, title, status, sub) {
  const row = el("button", "row");
  if (S.detail && S.detail.session.id === id) row.classList.add("active");
  const t = el("span", "title");
  t.append(el("span", "dot " + (status || "idle")), document.createTextNode(title));
  row.append(t);
  if (sub) row.append(el("span", "sub", sub));
  row.onclick = () => openSession(id);
  return row;
}

/* ------------------------------------------------------------- sessions */

async function refreshSessions() {
  const q = $("#session-filter").value.trim();
  const w = $("#session-window").value;
  const params = new URLSearchParams({ window: w });
  if (q) params.set("q", q);
  S.sessions = await api("GET", "/api/sessions?" + params);

  const list = $("#session-list");
  list.innerHTML = "";
  if (!S.sessions.length) {
    list.append(el("div", "empty", "No sessions in this window."));
    return;
  }
  for (const s of S.sessions) {
    list.append(sessionRow(s.id, s.title || "Untitled session", s.status,
      `${s.project_name} · ${s.project_path} · ${ago(s.last_active_at)}`));
  }
}

function openNewSessionDialog(project) {
  const dlg = $("#new-session");
  const form = dlg.querySelector("form");
  const provSel = form.provider;
  provSel.innerHTML = "";
  for (const p of S.providers) {
    const opt = new Option(p.available ? p.display_name : `${p.display_name} (unavailable)`, p.name);
    opt.disabled = !p.available;
    provSel.append(opt);
  }
  const first = S.providers.find((p) => p.available) || S.providers[0];
  if (first) provSel.value = first.name;

  const syncModels = () => {
    const p = providerOf(provSel.value);
    form.model.innerHTML = "";
    form.effort.innerHTML = "";
    if (!p) return;
    for (const m of p.models) form.model.append(new Option(m.label, m.id));
    form.effort.append(new Option("default", ""));
    for (const e of p.efforts) form.effort.append(new Option(e, e));
  };
  provSel.onchange = syncModels;
  syncModels();

  dlg.onclose = async () => {
    if (dlg.returnValue !== "ok") return;
    try {
      const sess = await api("POST", "/api/sessions", {
        project_id: project.id,
        provider: form.provider.value,
        model: form.model.value,
        effort: form.effort.value,
        permission: form.permission.value,
      });
      await refreshProjects();
      await refreshSessions();
      await openSession(sess.id);
      $("#prompt").focus();
    } catch (e) { fail(e); }
  };
  dlg.showModal();
}

async function openSession(id) {
  try {
    S.detail = await api("GET", "/api/sessions/" + id);
  } catch (e) { return fail(e); }

  S.tabs = [{ id: "conversation", label: "Conversation", closable: false }];
  S.activeTab = "conversation";
  renderTabs();
  renderTranscript();
  renderModelPickers();
  updateTurnControls();
  await refreshInspector();
  await refreshProjects();
  await refreshSessions();
  connectStream(id);
}

function closeSession() {
  S.detail = null;
  if (S.stream) { S.stream.close(); S.stream = null; }
  renderTabs();
  $("#main-body").innerHTML = "";
  $("#pane-stats").innerHTML = "";
  updateTurnControls();
}

/* ---------------------------------------------------------------- tabs */

function renderTabs() {
  const strip = $("#main-tabs");
  strip.innerHTML = "";
  if (!S.detail) return;
  for (const tab of S.tabs) {
    const b = el("button", "tab" + (tab.id === S.activeTab ? " active" : ""));
    b.append(document.createTextNode(tab.label));
    if (tab.closable) {
      const x = el("span", "close", "×");
      x.onclick = (e) => { e.stopPropagation(); closeTab(tab.id); };
      b.append(x);
    }
    b.onclick = () => { S.activeTab = tab.id; renderTabs(); renderBody(); };
    strip.append(b);
  }
}

function closeTab(id) {
  S.tabs = S.tabs.filter((t) => t.id !== id);
  if (S.activeTab === id) S.activeTab = "conversation";
  renderTabs();
  renderBody();
}

function renderBody() {
  const tab = S.tabs.find((t) => t.id === S.activeTab);
  if (!tab || tab.id === "conversation") return renderTranscript();
  const body = $("#main-body");
  body.innerHTML = "";
  const pre = el("pre", null);
  pre.id = "file-view";
  const code = el("code", null, tab.content);
  pre.append(code);
  body.append(pre);
}

async function openFile(path) {
  if (!S.detail) return;
  const id = "file:" + path;
  const existing = S.tabs.find((t) => t.id === id);
  if (!existing) {
    try {
      const f = await api("GET",
        `/api/projects/${S.detail.session.project_id}/file?path=${encodeURIComponent(path)}`);
      S.tabs.push({
        id, label: path.split("/").pop(), closable: true,
        content: f.binary ? "(binary file)" : f.content + (f.partial ? "\n\n… truncated" : ""),
      });
    } catch (e) { return fail(e); }
  }
  S.activeTab = id;
  renderTabs();
  renderBody();
}

/* ---------------------------------------------------------- transcript */

function renderTranscript() {
  const body = $("#main-body");
  body.innerHTML = "";
  if (!S.detail) {
    body.append(el("div", "empty", "Pick a session, or start one from a project."));
    return;
  }
  const wrap = el("div");
  wrap.id = "transcript";
  for (const m of S.detail.messages) wrap.append(bubble(m.role, m.content));
  body.append(wrap);
  scrollDown();
}

const WHO = { user: "You", assistant: "Agent", tool: "Tool", thinking: "Thinking", error: "Error" };

function bubble(role, content) {
  const box = el("div", "msg " + role);
  box.append(el("div", "who", WHO[role] || role));
  box.append(el("div", "body", content));
  return box;
}

function scrollDown() {
  const body = $("#main-body");
  body.scrollTop = body.scrollHeight;
}

/* ------------------------------------------------------------ streaming */

function connectStream(id) {
  if (S.stream) S.stream.close();
  const es = new EventSource(`/api/sessions/${id}/stream`);
  S.stream = es;
  es.onmessage = (e) => {
    let msg;
    try { msg = JSON.parse(e.data); } catch { return; }
    handleEvent(msg);
  };
  es.onerror = () => { /* EventSource reconnects on its own */ };
}

function handleEvent(msg) {
  if (!S.detail || msg.session_id !== S.detail.session.id) return;
  const ev = msg.event;
  const atConversation = S.activeTab === "conversation";

  switch (ev.type) {
    case "text":
      appendLive("assistant", ev.text);
      break;
    case "thinking":
      appendLive("thinking", ev.text);
      break;
    case "tool_use":
      S.live = null;
      if (atConversation && ev.tool) {
        $("#transcript").append(bubble("tool", `${ev.tool.name} ${ev.tool.input || ""}`));
        scrollDown();
      }
      break;
    case "error":
      S.live = null;
      if (atConversation) { $("#transcript").append(bubble("error", ev.text)); scrollDown(); }
      break;
    case "done":
      S.live = null;
      if (msg.stats) {
        S.detail.stats = msg.stats;
        S.detail.running = false;
        updateTurnControls();
        renderStats();
        refreshChanged();
        refreshSessions();
        refreshProjects();
      }
      break;
  }
}

/* appendLive grows one bubble as deltas arrive instead of adding a bubble
   per token. */
function appendLive(role, text) {
  if (S.activeTab !== "conversation") return;
  if (!S.live || S.live.dataset.role !== role) {
    const box = bubble(role, "");
    box.dataset.role = role;
    box.querySelector(".body").classList.add("cursor");
    $("#transcript").append(box);
    S.live = box;
  }
  S.live.querySelector(".body").textContent += text;
  scrollDown();
}

/* --------------------------------------------------------------- prompt */

function updateTurnControls() {
  const running = !!(S.detail && S.detail.running);
  $("#send-btn").disabled = !S.detail || running;
  $("#stop-btn").hidden = !running;
  $("#turn-state").textContent = !S.detail ? "" : running ? "running…" : "";
  $("#prompt").disabled = !S.detail;
}

async function send() {
  if (!S.detail) return;
  const box = $("#prompt");
  const prompt = box.value.trim();
  if (!prompt) return;

  box.value = "";
  S.live = null;
  if (S.activeTab === "conversation") {
    $("#transcript").append(bubble("user", prompt));
    scrollDown();
  }
  S.detail.running = true;
  updateTurnControls();

  try {
    await api("POST", `/api/sessions/${S.detail.session.id}/messages`, { prompt });
    refreshSessions();
    refreshProjects();
  } catch (e) {
    S.detail.running = false;
    updateTurnControls();
    fail(e);
  }
}

/* ------------------------------------------------------------ inspector */

async function refreshInspector() {
  renderStats();
  await Promise.all([refreshChanged(), refreshTree()]);
}

function renderStats() {
  const pane = $("#pane-stats");
  pane.innerHTML = "";
  if (!S.detail) return;
  const { session: s, stats } = S.detail;
  const rows = [
    ["Provider", s.provider],
    ["Model", s.model],
    ["Effort", s.effort || "default"],
    ["Permission", s.permission],
    ["Project", s.project_name],
    ["Folder", s.project_path],
    ["Turns", nf.format(stats.turns)],
    ["Input tokens", tokens(stats.input_tokens)],
    ["Output tokens", tokens(stats.output_tokens)],
    ["Cache read", tokens(stats.cache_read_tokens)],
    ["Cache write", tokens(stats.cache_write_tokens)],
    ["Cost", "$" + (stats.cost_usd || 0).toFixed(4)],
    ["Agent time", duration(stats.duration_ms)],
    ["Started", new Date(s.created_at).toLocaleString()],
  ];
  const dl = el("dl");
  dl.style.margin = "0";
  for (const [k, v] of rows) {
    const row = el("div", "stat");
    row.append(el("dt", null, k), el("dd", null, String(v)));
    dl.append(row);
  }
  pane.append(dl);
}

async function refreshChanged() {
  const pane = $("#pane-changed");
  pane.innerHTML = "";
  if (!S.detail) return;
  let files;
  try {
    files = await api("GET", `/api/projects/${S.detail.session.project_id}/changes`);
  } catch { return; }
  if (!files.length) {
    pane.append(el("div", "empty", "No edited files."));
    return;
  }
  for (const f of files) {
    const b = el("button", "file-row");
    b.append(el("span", "st", f.status));
    b.append(el("span", null, f.path));
    b.onclick = () => openFile(f.path);
    pane.append(b);
  }
}

async function refreshTree() {
  const pane = $("#pane-tree");
  pane.innerHTML = "";
  if (!S.detail) return;
  let paths;
  try {
    paths = await api("GET", `/api/projects/${S.detail.session.project_id}/tree`);
  } catch { return; }
  if (!paths.length) {
    pane.append(el("div", "empty", "No files."));
    return;
  }
  for (const p of paths) {
    const b = el("button", "file-row");
    b.append(el("span", null, p));
    b.onclick = () => openFile(p);
    pane.append(b);
  }
}

/* ------------------------------------------------------------------ init */

function wire() {
  wireTabs($("#side-tabs"), $("#sidebar"));
  wireTabs($("#inspector-tabs"), $("#inspector"));

  $("#add-project").onclick = () => {
    const dlg = $("#new-project");
    const form = dlg.querySelector("form");
    form.reset();
    dlg.onclose = async () => {
      if (dlg.returnValue !== "ok" || !form.path.value.trim()) return;
      try {
        await api("POST", "/api/projects",
          { path: form.path.value.trim(), name: form.name.value.trim() });
        await refreshProjects();
      } catch (e) { fail(e); }
    };
    dlg.showModal();
  };

  let filterTimer;
  $("#session-filter").oninput = () => {
    clearTimeout(filterTimer);
    filterTimer = setTimeout(refreshSessions, 150);
  };
  $("#session-window").onchange = refreshSessions;

  $("#prompt-bar").onsubmit = (e) => { e.preventDefault(); send(); };
  $("#prompt").onkeydown = (e) => {
    if (e.key === "Enter" && !e.shiftKey) { e.preventDefault(); send(); }
  };
  $("#stop-btn").onclick = async () => {
    try { await api("POST", `/api/sessions/${S.detail.session.id}/stop`); }
    catch (e) { fail(e); }
  };

  const applyModel = async () => {
    if (!S.detail) return;
    try {
      S.detail.session = await api("PATCH", "/api/sessions/" + S.detail.session.id, {
        model: $("#model-select").value, effort: $("#effort-select").value,
      });
      renderModelPickers();
      renderStats();
    } catch (e) { fail(e); }
  };
  $("#model-select").onchange = applyModel;
  $("#effort-select").onchange = applyModel;

  $("#star-btn").onclick = async () => {
    if (!S.detail) return;
    const { provider, model, effort } = S.detail.session;
    const on = isStarred(provider, model, effort);
    try {
      await api(on ? "DELETE" : "POST", "/api/stars", { provider, model, effort });
      S.stars = await api("GET", "/api/stars");
      renderModelPickers();
    } catch (e) { fail(e); }
  };
}

async function init() {
  wire();
  updateTurnControls();
  try {
    await loadProviders();
    await refreshProjects();
    await refreshSessions();
  } catch (e) { fail(e); }
  renderTranscript();
}

init();
