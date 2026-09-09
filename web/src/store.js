/* agenttik state. One reactive object, the actions that change it, and the
   SSE connection that keeps it current. Components read S and call these;
   none of them touch the DOM.

   Everything open is a tab in S.tabs, in the order the strip shows them: a
   session (transcript and prompt bar), a project (its sessions) or a file.
   Several of each can be open at once, including several sessions from the
   same project, and all of them stay live. S.detail and S.project are derived
   from whichever tab is in front. */

import { reactive, watch } from "vue";
import { api } from "./api";
import { debounce } from "./debounce";

/* Sidebar widths are the user's, so they are kept across reloads. */
const LAYOUT_KEY = "agenttik.layout";
const LAST_USED_KEY = "agenttik.lastUsed";
const OPEN_TABS_KEY = "agenttik.openTabs";
const LAYOUT_LIMITS = { left: [180, 520], right: [200, 620] };

export const S = reactive({
  providers: [],
  stars: [],
  projects: [],
  sessions: [],
  tabs: [], // every open view, in strip order
  closedSessions: [], // most recently closed non-empty session ids
  activeTab: "",
  promptFocus: 0,
  inspector: { panes: ["changed", "stats"], active: "changed" },
  changed: [],
  tree: [],
  treeFilter: "",
  query: "", // sidebar session filter
  window: "3d",
  lastUsed: null,
  layout: { left: 272, right: 312 },

  get tab() {
    return this.tabs.find((t) => t.id === this.activeTab) || null;
  },
  /* A file belongs to the view that opened it, so reading one keeps the
     prompt bar and the right-hand panel it came from. */
  get owner() {
    const t = this.tab;
    if (!t) return null;
    return t.kind === "file" ? this.tabs.find((x) => x.id === t.owner) || null : t;
  },
  get detail() {
    return this.owner?.kind === "session" ? this.owner.detail : null;
  },
  get project() {
    return this.owner?.kind === "project" ? this.owner.data : null;
  },
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

/* contextWindow is how many tokens the session's model holds, which the
   prompt bar's gauge measures the live context against. 0 means unknown. */
export function contextWindow(session) {
  if (!session) return 0;
  const model = providerOf(session.provider)?.models.find((m) => m.id === session.model);
  return model?.context_window || 0;
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

/* ------------------------------------------------------------------ tabs */

/* addTab appends unless the tab is already open, in which case it just comes
   to the front — clicking a session twice must not open it twice. */
function addTab(tab) {
  if (!S.tabs.some((t) => t.id === tab.id)) S.tabs.push(tab);
  S.activeTab = tab.id;
  resubscribe();
}

/* The prompt bar watches this counter. A counter, rather than a Boolean,
   makes each newly created session request focus even when one is already
   active. */
function focusPrompt() {
  S.promptFocus += 1;
}

export function selectTab(id) {
  S.activeTab = id;
}

/* selectTabAt is Alt+1 … Alt+9, so the number matches what the strip shows. */
export function selectTabAt(n) {
  const tab = S.tabs[n - 1];
  if (tab) S.activeTab = tab.id;
}

/* closeTab also closes the files opened from the tab, which have nothing to
   belong to once it is gone, and moves to the neighbour rather than back to
   the first tab. A never-used session is disposable; every other session is
   kept in a most-recent-first reopen stack. */
export async function closeTab(id, remember = true) {
  const at = S.tabs.findIndex((t) => t.id === id);
  if (at < 0) return;
  const tab = S.tabs[at];
  S.tabs = S.tabs.filter((t) => t.id !== id && t.owner !== id);
  if (S.activeTab === id || !S.tabs.some((t) => t.id === S.activeTab)) {
    S.activeTab = S.tabs[Math.min(at, S.tabs.length - 1)]?.id || "";
  }
  resubscribe();

  if (!remember || tab.kind !== "session") return;
  if (tab.detail.messages.length) {
    S.closedSessions = [
      tab.sessionID,
      ...S.closedSessions.filter((sessionID) => sessionID !== tab.sessionID),
    ];
    return;
  }
  try {
    await api("DELETE", "/api/sessions/" + tab.sessionID);
    await Promise.all([refreshProjects(), refreshSessions()]);
    reloadProjects();
  } catch (e) {
    fail(e);
  }
}

/* reopenClosedSession consumes one entry, so repeating Ctrl+Shift+T walks
   backwards through the sessions closed in this browser window. */
export async function reopenClosedSession() {
  const id = S.closedSessions.shift();
  if (id) await openSession(id);
}

/* currentProjectID is the project the right panel works against, whichever
   kind of view is in front. */
export function currentProjectID() {
  if (S.detail) return S.detail.session.project_id;
  if (S.project) return S.project.project.id;
  return null;
}

/* -------------------------------------------------------------- projects */

export async function refreshProjects() {
  S.projects = await api("GET", "/api/projects");
}

/* reorderProjects persists the sidebar order after it has already moved under
   the pointer. Re-reading on failure is the one reliable way to undo a drag. */
export async function reorderProjects(ids) {
  try {
    await api("POST", "/api/projects/order", { ids });
    await refreshProjects();
  } catch (e) {
    fail(e);
    refreshProjects().catch(() => {});
  }
}

/* openProject puts the project in a tab: everything still open in it, with
   its totals, files and options on the right. */
export async function openProject(id, silent = false) {
  const tabID = "project:" + id;
  const open = S.tabs.find((t) => t.id === tabID);
  if (open) return selectTab(tabID);

  let project, stats, sessions;
  try {
    [project, stats, sessions] = await Promise.all([
      api("GET", "/api/projects/" + id),
      api("GET", `/api/projects/${id}/stats`),
      projectSessions(id),
    ]);
  } catch (e) {
    if (!silent) fail(e);
    return;
  }
  addTab({
    id: tabID,
    kind: "project",
    label: project.name,
    projectID: id,
    data: { project, stats, sessions },
  });
  await Promise.all([refreshProjects(), refreshSessions()]).catch(fail);
}

/* Project views hide archived sessions; the Sessions list keeps them. */
function projectSessions(id) {
  return api("GET", `/api/sessions?window=all&project_id=${id}&include_done=false`);
}

async function reloadProjectTab(tab) {
  const id = tab.projectID;
  try {
    const [stats, sessions] = await Promise.all([
      api("GET", `/api/projects/${id}/stats`),
      projectSessions(id),
    ]);
    tab.data.stats = stats;
    tab.data.sessions = sessions;
  } catch {
    /* a refresh that fails is not worth interrupting anyone over */
  }
}

/* reloadProjects refreshes every open project tab. Debounced because the end
   of a turn arrives on both the session's topic and its project's, and takes
   no argument so two projects cannot collapse into one reload. */
const reloadProjects = debounce(() => {
  for (const t of S.tabs) if (t.kind === "project") reloadProjectTab(t);
}, 150);

export async function addProject(path, name) {
  await api("POST", "/api/projects", { path, name });
  await refreshProjects();
}

export async function removeProject(p) {
  try {
    await api("DELETE", "/api/projects/" + p.id);
    // Its sessions went with it, so their tabs cannot stay open.
    for (const t of [...S.tabs]) {
      if (projectOfTab(t) === p.id) closeTab(t.id, false);
    }
    await Promise.all([refreshProjects(), refreshSessions()]);
  } catch (e) {
    fail(e);
  }
}

function projectOfTab(t) {
  if (t.kind === "project") return t.projectID;
  if (t.kind === "session") return t.detail.session.project_id;
  const owner = S.tabs.find((x) => x.id === t.owner);
  return owner ? projectOfTab(owner) : null;
}

export async function renameProject(p, name) {
  name = name.trim();
  if (!name || name === p.name) return;
  try {
    const updated = await api("PATCH", "/api/projects/" + p.id, { name });
    const tab = S.tabs.find((t) => t.kind === "project" && t.projectID === p.id);
    if (tab) {
      tab.data.project = updated;
      tab.label = updated.name;
    }
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
  syncSessionTabs();
}

/* The first prompt names a session on the server, so the tab strip takes its
   label from the list rather than guessing at the title rule. */
function syncSessionTabs() {
  for (const tab of S.tabs) {
    if (tab.kind !== "session") continue;
    const fresh = S.sessions.find((s) => s.id === tab.sessionID);
    if (!fresh) continue;
    tab.detail.session = { ...tab.detail.session, ...fresh };
    tab.label = tabLabel(fresh);
  }
}

function tabLabel(session) {
  return session.title?.trim() || "New session";
}

/* A new session asks nothing: it opens empty on the model, effort and
   permission the last conversation was using. localStorage keeps that across
   a reload; without it we fall back to the most recent session, then to a
   starred combination, then to whatever is installed. */
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
  if (!cfg) return fail(new Error("No agent CLI is available. Open Settings to see why."));
  try {
    const sess = await api("POST", "/api/sessions", { project_id: project.id, ...cfg });
    await Promise.all([refreshProjects(), refreshSessions()]);
    await openSession(sess.id);
    focusPrompt();
    reloadProjects();
  } catch (e) {
    fail(e);
  }
}

/* startCurrentSession is the keyboard and command-palette version of the
   project page's New session button. A session, project, or file tab all
   identify their owning project. */
export function startCurrentSession() {
  const id = currentProjectID();
  const project = S.project?.project?.id === id
    ? S.project.project
    : S.projects.find((p) => p.id === id);
  if (!project) return fail(new Error("Open a project or session first."));
  return startSession(project);
}

export async function openSession(id, silent = false) {
  const tabID = "session:" + id;
  if (S.tabs.some((t) => t.id === tabID)) return selectTab(tabID);

  let detail;
  try {
    detail = await api("GET", "/api/sessions/" + id);
  } catch (e) {
    if (!silent) fail(e);
    return;
  }
  rememberUsed(detail.session);
  addTab({
    id: tabID,
    kind: "session",
    label: tabLabel(detail.session),
    sessionID: id,
    detail,
  });
  await Promise.all([refreshProjects(), refreshSessions()]).catch(fail);
}

export async function setModel(model, effort) {
  const tab = S.owner;
  if (tab?.kind !== "session") return;
  try {
    tab.detail.session = await api("PATCH", "/api/sessions/" + tab.sessionID, { model, effort });
    rememberUsed(tab.detail.session);
  } catch (e) {
    fail(e);
  }
}

/* setSessionArchived removes a session from project views without deleting
   it. The Sessions list keeps archived sessions so they can be restored. */
export async function setSessionArchived(session, archived) {
  try {
    const updated = await api("PATCH", "/api/sessions/" + session.id, { done: archived });
    const tab = S.tabs.find((t) => t.kind === "session" && t.sessionID === session.id);
    if (tab) tab.detail.session = updated;
    await Promise.all([refreshProjects(), refreshSessions()]);
    reloadProjects();
  } catch (e) {
    fail(e);
  }
}

export function isArchived(session) {
  return !!session?.done_at;
}

/* reorderSessions records the order a project view was dragged into. The list
   is put in that order first, so the drop lands where it was let go rather
   than a request later; if the server refuses, the project is re-read, which
   is the only reliable way back — the row has been moved under the cursor
   since the drag began and the order it started in is gone. */
export async function reorderSessions(tab, ids) {
  const byID = new Map(tab.data.sessions.map((s) => [s.id, s]));
  tab.data.sessions = ids.map((id) => byID.get(id)).filter(Boolean);
  try {
    await api("POST", `/api/projects/${tab.projectID}/sessions/order`, { ids });
    await refreshProjects();
  } catch (e) {
    fail(e);
    reloadProjectTab(tab);
  }
}

/* The sidebar has the same project session order as the centre project view,
   but owns a different array. It can therefore use the same endpoint without
   coupling a sidebar drag to an open project tab. */
export async function reorderSidebarSessions(project, ids) {
  try {
    await api("POST", `/api/projects/${project.id}/sessions/order`, { ids });
    await refreshProjects();
  } catch (e) {
    fail(e);
    refreshProjects().catch(() => {});
  }
}

/* ------------------------------------------------------------------ files */

export async function openFile(path, silent = false) {
  const owner = S.owner;
  const projectID = currentProjectID();
  if (!owner || !projectID) return;
  const tabID = `file:${projectID}:${path}`;
  if (S.tabs.some((t) => t.id === tabID)) return selectTab(tabID);
  try {
    const f = await api("GET", `/api/projects/${projectID}/file?path=${encodeURIComponent(path)}`);
    addTab({
      id: tabID,
      kind: "file",
      label: path.split("/").pop(),
      owner: owner.id,
      path,
      content: f.binary ? "(binary file)" : f.content + (f.partial ? "\n\n… truncated" : ""),
    });
  } catch (e) {
    if (!silent) fail(e);
  }
}

/* ---------------------------------------------------------- saved tabs */

/* Tabs contain live API data, so only save the small identifiers needed to
   rebuild them. This avoids restoring an old transcript over the server's
   current version after the app is relaunched. */
function savedTab(tab) {
  if (tab.kind === "project") return { id: tab.id, kind: tab.kind, projectID: tab.projectID };
  if (tab.kind === "session") return { id: tab.id, kind: tab.kind, sessionID: tab.sessionID };
  if (tab.kind === "file") return { id: tab.id, kind: tab.kind, owner: tab.owner, path: tab.path };
  return null;
}

let restoringTabs = false;

function saveOpenTabs() {
  if (restoringTabs) return;
  try {
    localStorage.setItem(
      OPEN_TABS_KEY,
      JSON.stringify({ tabs: S.tabs.map(savedTab).filter(Boolean), activeTab: S.activeTab }),
    );
  } catch {
    /* private mode or a full quota just means tabs open fresh next time */
  }
}

/* Restoring opens the same kinds of tabs through the ordinary actions, so
   their data is fetched anew and missing projects or sessions are skipped. A
   file's owner is always opened before it, because files can only be opened
   from an already-open session or project. */
async function restoreOpenTabs() {
  let saved;
  try {
    saved = JSON.parse(localStorage.getItem(OPEN_TABS_KEY));
  } catch {
    return;
  }
  if (!Array.isArray(saved?.tabs)) return;

  restoringTabs = true;
  try {
    for (const tab of saved.tabs) {
      if (tab?.kind === "project" && tab.projectID) {
        await openProject(tab.projectID, true);
      } else if (tab?.kind === "session" && tab.sessionID) {
        await openSession(tab.sessionID, true);
      } else if (tab?.kind === "file" && tab.owner && typeof tab.path === "string") {
        if (!S.tabs.some((open) => open.id === tab.owner)) continue;
        selectTab(tab.owner);
        await openFile(tab.path, true);
      }
    }
    if (S.tabs.some((tab) => tab.id === saved.activeTab)) S.activeTab = saved.activeTab;
  } finally {
    restoringTabs = false;
    saveOpenTabs();
  }
}

watch(
  () => ({ activeTab: S.activeTab, tabs: S.tabs.map(savedTab) }),
  saveOpenTabs,
);

/* ------------------------------------------------------------- inspector */

/* The right-hand strip depends on what is open. The pane in use is kept when
   the new view also has it, so moving between tabs does not throw you back to
   the first one. */
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
    const tree = await api("GET", `/api/projects/${id}/tree`);
    // A slow request for the tab we just left must not replace the active
    // project's sidebar Tree.
    if (currentProjectID() === id) S.tree = tree;
  } catch {
    if (currentProjectID() === id) S.tree = [];
  }
}

/* The panel follows the tab in front: which panes it offers depends on the
   kind of view, and what they list depends on its project. Two tabs in the
   same project share the listing, so it is only re-read when the project
   changes. */
watch(
  () => S.owner?.kind || "",
  (kind) =>
    setInspectorPanes(
      kind === "project" ? ["options", "stats"] : ["changed", "stats"],
    ),
  { immediate: true },
);

watch(
  currentProjectID,
  () => {
    S.treeFilter = "";
    refreshInspector().catch(fail);
  },
  { immediate: true },
);

/* ------------------------------------------------------------- streaming */

/* One connection carries every open tab. A browser holds only a handful of
   connections to one origin, so a stream per tab would stall the API as soon
   as a few conversations were open. */
let stream = null;
let streamURL = "";

function subscriptionURL() {
  const sessions = S.tabs.filter((t) => t.kind === "session").map((t) => t.sessionID);
  const projects = S.tabs.filter((t) => t.kind === "project").map((t) => t.projectID);
  if (!sessions.length && !projects.length) return "";
  const params = new URLSearchParams();
  if (sessions.length) params.set("sessions", sessions.join(","));
  if (projects.length) params.set("projects", projects.join(","));
  return "/api/stream?" + params;
}

/* resubscribe reopens the stream when the set of open tabs changes, and does
   nothing when it has not. */
function resubscribe() {
  const url = subscriptionURL();
  if (url === streamURL) return;
  streamURL = url;
  if (stream) {
    stream.close();
    stream = null;
  }
  if (!url) return;
  const es = new EventSource(url);
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

function onEvent(msg) {
  const tab = S.tabs.find((t) => t.kind === "session" && t.sessionID === msg.session_id);
  if (tab) onSessionEvent(tab, msg);
  // A turn ending anywhere in a project moves its totals, whether or not that
  // session is open here.
  if (msg.event?.type === "done") reloadProjects();
}

function onSessionEvent(tab, msg) {
  const ev = msg.event;
  switch (ev.type) {
    case "text":
      appendLive(tab, "assistant", ev.text);
      break;
    case "thinking":
      appendLive(tab, "thinking", ev.text);
      break;
    case "tool_use":
      endLive(tab);
      if (ev.tool) push(tab, "tool", `${ev.tool.name} ${ev.tool.input || ""}`);
      break;
    case "usage":
      // Mid-turn usage only reports the context in use, so the gauge can
      // move while the turn runs.
      if (ev.usage?.context_tokens) tab.detail.stats.context_tokens = ev.usage.context_tokens;
      break;
    case "error":
      endLive(tab);
      push(tab, "error", ev.text);
      break;
    case "done":
      endLive(tab);
      if (msg.stats) {
        tab.detail.stats = msg.stats;
        tab.detail.running = false;
        if (tab.id === S.owner?.id) refreshChanged();
        refreshSessions();
        refreshProjects();
      }
      break;
  }
}

function push(tab, role, content) {
  tab.detail.messages.push({ role, content });
}

/* appendLive grows one message as deltas arrive instead of adding a bubble
   per token. */
function appendLive(tab, role, text) {
  const last = tab.detail.messages.at(-1);
  if (last && last.streaming && last.role === role) {
    last.content += text;
    return;
  }
  endLive(tab);
  tab.detail.messages.push({ role, content: text, streaming: true });
}

function endLive(tab) {
  const last = tab.detail.messages.at(-1);
  if (last && last.streaming) last.streaming = false;
}

/* ---------------------------------------------------------------- prompt */

export async function send(prompt) {
  const tab = S.owner;
  if (tab?.kind !== "session") return;
  prompt = prompt.trim();
  if (!prompt) return;

  endLive(tab);
  push(tab, "user", prompt);
  tab.detail.running = true;

  try {
    await api("POST", `/api/sessions/${tab.sessionID}/messages`, { prompt });
    refreshSessions();
    refreshProjects();
  } catch (e) {
    tab.detail.running = false;
    fail(e);
  }
}

export async function stopTurn() {
  const tab = S.owner;
  if (tab?.kind !== "session") return;
  try {
    await api("POST", `/api/sessions/${tab.sessionID}/stop`);
  } catch (e) {
    fail(e);
  }
}

/* ---------------------------------------------------------------- layout */

export function loadLayout() {
  try {
    const saved = JSON.parse(localStorage.getItem(LAYOUT_KEY));
    if (saved) {
      S.layout.left = clampWidth("left", saved.left);
      S.layout.right = clampWidth("right", saved.right);
    }
  } catch {
    /* keep the defaults */
  }
}

function clampWidth(side, px) {
  const [min, max] = LAYOUT_LIMITS[side];
  return Math.min(max, Math.max(min, Math.round(Number(px) || 0) || S.layout[side]));
}

/* setSidebarWidth is called per pointer move while a divider is dragged, so
   it only touches state; saveLayout runs once on release. */
export function setSidebarWidth(side, px) {
  S.layout[side] = clampWidth(side, px);
}

export function saveLayout() {
  try {
    localStorage.setItem(LAYOUT_KEY, JSON.stringify(S.layout));
  } catch {
    /* private mode, a full quota — the widths just do not persist */
  }
}

/* ------------------------------------------------------------------ init */

export async function init() {
  loadLastUsed();
  loadLayout();
  try {
    await loadProviders();
    await refreshProjects();
    await refreshSessions();
    await restoreOpenTabs();
  } catch (e) {
    fail(e);
  }
}
