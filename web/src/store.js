/* agenttik state. One reactive object, the actions that change it, and the
   SSE connection that keeps it current. Components read S and call these;
   none of them touch the DOM.

   Two things can be open in the centre: a session (transcript, prompt bar,
   Changed | Stats | Tree) or a project (its sessions, no prompt bar,
   Options | Stats | Tree). Exactly one of S.detail and S.project is set. */

import { reactive } from "vue";
import { api } from "./api";

export const S = reactive({
  providers: [],
  stars: [],
  projects: [],
  sessions: [],
  detail: null, // open session: { session, messages, stats, running }
  project: null, // open project: { project, stats, sessions }
  tabs: [], // centre tabs; the first one is the view, the rest files
  activeTab: "",
  inspector: { panes: ["changed", "stats", "tree"], active: "changed" },
  changed: [],
  tree: [],
  query: "", // sidebar session filter
  window: "3d",
  lastUsed: null,
});

/* The toast handle comes from App.vue's setup, because useToast needs the app
   context that only a component has. */
let toast = null;
export function useErrors(handle) {
  toast = handle;
}

export function fail(err) {
  console.error(err);
  const description = err?.message || String(err);
  if (toast) toast.add({ title: "Something went wrong", description, color: "error" });
}

/* ------------------------------------------------------------- providers */

export async function loadProviders() {
  S.providers = await api("GET", "/api/providers");
  S.stars = await api("GET", "/api/stars");
}

export function providerOf(name) {
  return S.providers.find((p) => p.name === name);
}

export function isStarred(provider, model, effort) {
  return S.stars.some(
    (s) => s.provider === provider && s.model === model && (s.effort || "") === (effort || ""),
  );
}

/* toggleStar flips one combination and keeps S.stars in step without
   re-fetching the list. */
export async function toggleStar(provider, model, effort) {
  const on = isStarred(provider, model, effort);
  await api(on ? "DELETE" : "POST", "/api/stars", { provider, model, effort });
  S.stars = on
    ? S.stars.filter(
        (s) => !(s.provider === provider && s.model === model && (s.effort || "") === (effort || "")),
      )
    : [...S.stars, { provider, model, effort, created_at: Date.now() }];
}

/* -------------------------------------------------------------- projects */

export async function refreshProjects() {
  S.projects = await api("GET", "/api/projects");
}

/* openProject puts the project in the centre: everything ever run in it,
   with its totals, files and options on the right. */
export async function openProject(id) {
  let project, stats, sessions;
  try {
    [project, stats, sessions] = await Promise.all([
      api("GET", "/api/projects/" + id),
      api("GET", `/api/projects/${id}/stats`),
      api("GET", `/api/sessions?window=all&project_id=${id}`),
    ]);
  } catch (e) {
    return fail(e);
  }

  S.detail = null;
  S.project = { project, stats, sessions };
  S.tabs = [{ id: "project", label: project.name, closable: false }];
  S.activeTab = "project";
  setInspectorPanes(["options", "stats", "tree"]);
  await Promise.all([refreshInspector(), refreshProjects(), refreshSessions()]).catch(fail);
  connectStream(`/api/projects/${id}/stream`, onProjectEvent);
}

async function reloadProject() {
  if (!S.project) return;
  const id = S.project.project.id;
  try {
    const [stats, sessions] = await Promise.all([
      api("GET", `/api/projects/${id}/stats`),
      api("GET", `/api/sessions?window=all&project_id=${id}`),
    ]);
    S.project.stats = stats;
    S.project.sessions = sessions;
  } catch {
    /* a refresh that fails is not worth interrupting anyone over */
  }
}

export async function addProject(path, name) {
  await api("POST", "/api/projects", { path, name });
  await refreshProjects();
}

export async function removeProject(p) {
  try {
    await api("DELETE", "/api/projects/" + p.id);
    if (currentProjectID() === p.id) closeCentre();
    await Promise.all([refreshProjects(), refreshSessions()]);
  } catch (e) {
    fail(e);
  }
}

export async function renameProject(p, name) {
  name = name.trim();
  if (!name || name === p.name) return;
  try {
    S.project.project = await api("PATCH", "/api/projects/" + p.id, { name });
    S.tabs[0].label = S.project.project.name;
    await refreshProjects();
  } catch (e) {
    fail(e);
  }
}

/* -------------------------------------------------------------- sessions */

export async function refreshSessions() {
  const params = new URLSearchParams({ window: S.window });
  if (S.query.trim()) params.set("q", S.query.trim());
  S.sessions = await api("GET", "/api/sessions?" + params);
}

/* A new session asks nothing: it opens empty on the model, effort and
   permission the last conversation was using. localStorage keeps that across
   a reload; without it we fall back to the most recent session, then to a
   starred combination, then to whatever is installed. */
const LAST_USED_KEY = "agenttik.lastUsed";

export function loadLastUsed() {
  try {
    S.lastUsed = JSON.parse(localStorage.getItem(LAST_USED_KEY));
  } catch {
    S.lastUsed = null;
  }
}

function rememberUsed(sess) {
  S.lastUsed = {
    provider: sess.provider,
    model: sess.model,
    effort: sess.effort || "",
    permission: sess.permission,
  };
  try {
    localStorage.setItem(LAST_USED_KEY, JSON.stringify(S.lastUsed));
  } catch {
    /* private mode, a full quota — not worth failing a session over */
  }
}

/* sessionDefaults skips any choice that no longer works — a CLI uninstalled,
   a model retired — rather than starting a session that cannot run. */
function sessionDefaults() {
  const recent = S.sessions.reduce(
    (best, s) => (!best || s.last_active_at > best.last_active_at ? s : best),
    null,
  );
  for (const c of [S.lastUsed, recent, S.stars[0]]) {
    if (!c) continue;
    const p = providerOf(c.provider);
    if (!p || !p.available || !p.models.some((m) => m.id === c.model)) continue;
    return {
      provider: p.name,
      model: c.model,
      effort: c.effort || "",
      permission: c.permission || "workspace",
    };
  }
  const p = S.providers.find((x) => x.available && x.models.length);
  if (!p) return null;
  return { provider: p.name, model: p.models[0].id, effort: "", permission: "workspace" };
}

export async function startSession(project) {
  const cfg = sessionDefaults();
  if (!cfg) return fail(new Error("No agent CLI is available. Open Setup to see why."));
  try {
    const sess = await api("POST", "/api/sessions", { project_id: project.id, ...cfg });
    await Promise.all([refreshProjects(), refreshSessions()]);
    await openSession(sess.id);
  } catch (e) {
    fail(e);
  }
}

export async function openSession(id) {
  let detail;
  try {
    detail = await api("GET", "/api/sessions/" + id);
  } catch (e) {
    return fail(e);
  }

  S.detail = detail;
  S.project = null;
  rememberUsed(detail.session);
  S.tabs = [{ id: "conversation", label: "Conversation", closable: false }];
  S.activeTab = "conversation";
  setInspectorPanes(["changed", "stats", "tree"]);
  await Promise.all([refreshInspector(), refreshProjects(), refreshSessions()]).catch(fail);
  connectStream(`/api/sessions/${id}/stream`, onSessionEvent);
}

export async function setModel(model, effort) {
  if (!S.detail) return;
  try {
    S.detail.session = await api("PATCH", "/api/sessions/" + S.detail.session.id, { model, effort });
    rememberUsed(S.detail.session);
  } catch (e) {
    fail(e);
  }
}

/* closeCentre returns the page to its opening state: nothing selected, the
   right panel back to its neutral set rather than a deleted project's. */
export function closeCentre() {
  S.detail = null;
  S.project = null;
  S.tabs = [];
  S.activeTab = "";
  S.changed = [];
  S.tree = [];
  closeStream();
  setInspectorPanes(["changed", "stats", "tree"]);
}

/* ------------------------------------------------------------------ tabs */

export function selectTab(id) {
  S.activeTab = id;
}

export function closeTab(id) {
  S.tabs = S.tabs.filter((t) => t.id !== id);
  if (S.activeTab === id) S.activeTab = S.tabs.length ? S.tabs[0].id : "";
}

/* currentProjectID is the project the right panel works against, whichever
   of the two views is open. */
export function currentProjectID() {
  if (S.detail) return S.detail.session.project_id;
  if (S.project) return S.project.project.id;
  return null;
}

export async function openFile(path) {
  const id = currentProjectID();
  if (!id) return;
  const tabID = "file:" + path;
  if (!S.tabs.some((t) => t.id === tabID)) {
    try {
      const f = await api("GET", `/api/projects/${id}/file?path=${encodeURIComponent(path)}`);
      S.tabs.push({
        id: tabID,
        label: path.split("/").pop(),
        closable: true,
        content: f.binary ? "(binary file)" : f.content + (f.partial ? "\n\n… truncated" : ""),
      });
    } catch (e) {
      return fail(e);
    }
  }
  S.activeTab = tabID;
}

/* ------------------------------------------------------------- inspector */

/* The right-hand strip depends on what is open. The pane in use is kept when
   the new view also has it, so moving between sessions does not throw you
   back to the first one. */
function setInspectorPanes(panes) {
  S.inspector.panes = panes;
  if (!panes.includes(S.inspector.active)) S.inspector.active = panes[0];
}

export async function refreshInspector() {
  await Promise.all([refreshChanged(), refreshTree()]);
}

export async function refreshChanged() {
  const id = currentProjectID();
  if (!id) return (S.changed = []);
  try {
    S.changed = await api("GET", `/api/projects/${id}/changes`);
  } catch {
    S.changed = [];
  }
}

async function refreshTree() {
  const id = currentProjectID();
  if (!id) return (S.tree = []);
  try {
    S.tree = await api("GET", `/api/projects/${id}/tree`);
  } catch {
    S.tree = [];
  }
}

/* ------------------------------------------------------------- streaming */

let stream = null;

function closeStream() {
  if (stream) stream.close();
  stream = null;
}

function connectStream(path, onEvent) {
  closeStream();
  const es = new EventSource(path);
  stream = es;
  es.onmessage = (e) => {
    let msg;
    try {
      msg = JSON.parse(e.data);
    } catch {
      return;
    }
    onEvent(msg);
  };
  es.onerror = () => {
    /* EventSource reconnects on its own */
  };
}

/* A project stream carries the end of every turn run in it, from any number
   of sessions at once, so the totals stay current without polling. */
function onProjectEvent(msg) {
  if (!S.project || !msg.stats) return;
  reloadProject();
  refreshSessions();
  refreshProjects();
}

function onSessionEvent(msg) {
  if (!S.detail || msg.session_id !== S.detail.session.id) return;
  const ev = msg.event;

  switch (ev.type) {
    case "text":
      appendLive("assistant", ev.text);
      break;
    case "thinking":
      appendLive("thinking", ev.text);
      break;
    case "tool_use":
      endLive();
      if (ev.tool) push("tool", `${ev.tool.name} ${ev.tool.input || ""}`);
      break;
    case "error":
      endLive();
      push("error", ev.text);
      break;
    case "done":
      endLive();
      if (msg.stats) {
        S.detail.stats = msg.stats;
        S.detail.running = false;
        refreshChanged();
        refreshSessions();
        refreshProjects();
      }
      break;
  }
}

function push(role, content) {
  S.detail.messages.push({ role, content });
}

/* appendLive grows one message as deltas arrive instead of adding a bubble
   per token. */
function appendLive(role, text) {
  const last = S.detail.messages.at(-1);
  if (last && last.streaming && last.role === role) {
    last.content += text;
    return;
  }
  endLive();
  S.detail.messages.push({ role, content: text, streaming: true });
}

function endLive() {
  const last = S.detail.messages.at(-1);
  if (last && last.streaming) last.streaming = false;
}

/* ---------------------------------------------------------------- prompt */

export async function send(prompt) {
  if (!S.detail) return;
  prompt = prompt.trim();
  if (!prompt) return;

  endLive();
  push("user", prompt);
  S.detail.running = true;

  try {
    await api("POST", `/api/sessions/${S.detail.session.id}/messages`, { prompt });
    refreshSessions();
    refreshProjects();
  } catch (e) {
    S.detail.running = false;
    fail(e);
  }
}

export async function stopTurn() {
  if (!S.detail) return;
  try {
    await api("POST", `/api/sessions/${S.detail.session.id}/stop`);
  } catch (e) {
    fail(e);
  }
}

/* ------------------------------------------------------------------ init */

export async function init() {
  loadLastUsed();
  try {
    await loadProviders();
    await refreshProjects();
    await refreshSessions();
  } catch (e) {
    fail(e);
  }
}
