/* agenttik state. One reactive object, the actions that change it, and the
   SSE connection that keeps it current. Components read S and call these;
   none of them touch the DOM.

   Everything open is a tab in S.tabs: a session (transcript and prompt bar),
   a project (its sessions) or a file. Several of each can be open at once,
   including several sessions from the same project, and all of them stay
   live. S.detail and S.project are derived from whichever tab is in front.

   The strip only shows one project at a time — S.strip is S.tabs narrowed to
   S.activeProjectID and grouped project, sessions, files — so switching
   project switches the whole centre to that project's work. */

import { reactive, watch } from "vue";
import { api } from "./api";
import { debounce } from "./debounce";
import { ACCENTS, DEFAULT_COLORS, NEUTRALS, applyColors } from "./theme";

/* Sidebar widths are the user's, so they are kept across reloads. */
const LAYOUT_KEY = "agenttik.layout";
const LAST_USED_KEY = "agenttik.lastUsed";
const OPEN_TABS_KEY = "agenttik.openTabs";
const FILE_MODE_KEY = "agenttik.fileMode";
const DIFF_VIEW_KEY = "agenttik.diffView";
const COLORS_KEY = "agenttik.colors";
const LAYOUT_LIMITS = { left: [180, 520], right: [200, 620] };

/* Tabs are kept in their groups: a project's page first, then its
   conversations, then the files read from them. A new tab joins the end of
   its own group, and a drag only moves a tab within it. */
const TAB_RANK = { project: 0, session: 1, file: 2 };

/* The letters Alt reaches a project with. The first eight rows of the
   sidebar get one, so dragging a project changes its letter. */
export const PROJECT_KEYS = "ABCDEFGH";

export const S = reactive({
  providers: [],
  stars: [],
  projects: [],
  sessions: [],
  tabs: [], // every open view, of every project
  activeTab: "",
  activeProjectID: null, // the project the strip and the sidebar show
  lastTab: {}, // per project, the tab it was last left on
  fileMode: "edit", // "edit", "diff" or "preview", carried to the next file
  diffView: "unified", // "unified" or "split", likewise
  closing: null, // a close waiting on what to do with unsaved edits
  promptFocus: 0,
  inspector: { panes: ["changed", "stats"], active: "changed" },
  changed: [],
  tree: [],
  treeFilter: "",
  query: "", // sidebar session filter
  window: "3d",
  lastUsed: null,
  layout: { left: 272, right: 312 },
  colors: { ...DEFAULT_COLORS }, // the accent and the grey, from Settings

  get tab() {
    return this.tabs.find((t) => t.id === this.activeTab) || null;
  },
  /* strip is what the tab bar draws: the active project's tabs, in the order
     they were dragged into. Everything else stays open and live, it is just
     not on screen. */
  get strip() {
    return this.tabs.filter((t) => projectOfTab(t) === this.activeProjectID);
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

/* addTab inserts unless the tab is already open, in which case it just comes
   to the front — clicking a session twice must not open it twice. */
function addTab(tab) {
  if (!S.tabs.some((t) => t.id === tab.id)) insertTab(tab);
  selectTab(tab.id);
  resubscribe();
}

/* insertTab puts a tab at the end of its own group inside its project, so a
   new conversation lands after the last one and always before the files. */
function insertTab(tab) {
  const id = projectOfTab(tab);
  const rank = TAB_RANK[tab.kind];
  let at = -1;
  let first = -1;
  S.tabs.forEach((t, i) => {
    if (projectOfTab(t) !== id) return;
    if (first < 0) first = i;
    if (TAB_RANK[t.kind] <= rank) at = i + 1;
  });
  if (at < 0) at = first < 0 ? S.tabs.length : first;
  S.tabs.splice(at, 0, tab);
}

/* The sidebar owns a project's complete session order. Project pages and the
   open subset in the tab strip are projections of it, so every drag starts by
   bringing those projections into the same order. Entries absent from a list
   (for example an archived session tab) keep their relative place after the
   ordered sessions. */
function orderBySessionID(items, ids, idOf) {
  const rank = new Map(ids.map((id, i) => [id, i]));
  return items
    .map((item, i) => ({ item, i }))
    .sort(
      (a, b) =>
        (rank.get(idOf(a.item)) ?? Infinity) - (rank.get(idOf(b.item)) ?? Infinity) || a.i - b.i,
    )
    .map(({ item }) => item);
}

function syncProjectSessionOrder(projectID, ids) {
  const project = S.projects.find((p) => p.id === projectID);
  if (project) {
    project.recent_sessions = orderBySessionID(project.recent_sessions, ids, (session) => session.id);
  }
  const projectTab = S.tabs.find((tab) => tab.kind === "project" && tab.projectID === projectID);
  if (projectTab) {
    projectTab.data.sessions = orderBySessionID(projectTab.data.sessions, ids, (session) => session.id);
  }
  const slots = [];
  const sessions = [];
  S.tabs.forEach((tab, i) => {
    if (tab.kind === "session" && projectOfTab(tab) === projectID) {
      slots.push(i);
      sessions.push(tab);
    }
  });
  const orderedTabs = orderBySessionID(sessions, ids, (tab) => tab.sessionID);
  slots.forEach((i, j) => (S.tabs[i] = orderedTabs[j]));
}

/* moveTab is a tab dragged along the strip. Only tabs of the same kind trade
   places: the groups keep their order, and the numbers follow the strip
   rather than the strip following the numbers. */
export function moveTab(dragID, overID) {
  const from = S.tabs.findIndex((t) => t.id === dragID);
  const to = S.tabs.findIndex((t) => t.id === overID);
  if (from < 0 || to < 0 || from === to) return;
  if (S.tabs[from].kind !== S.tabs[to].kind) return;
  if (projectOfTab(S.tabs[from]) !== projectOfTab(S.tabs[to])) return;
  if (S.tabs[from].kind === "session") {
    const projectID = projectOfTab(S.tabs[from]);
    const project = S.projects.find((p) => p.id === projectID);
    if (!project) return;
    const fromSidebar = project.recent_sessions.findIndex((session) => session.id === S.tabs[from].sessionID);
    const toSidebar = project.recent_sessions.findIndex((session) => session.id === S.tabs[to].sessionID);
    if (fromSidebar < 0 || toSidebar < 0) return;
    project.recent_sessions.splice(toSidebar, 0, ...project.recent_sessions.splice(fromSidebar, 1));
    syncProjectSessionOrder(projectID, project.recent_sessions.map((session) => session.id));
    return;
  }
  S.tabs.splice(to, 0, ...S.tabs.splice(from, 1));
}

/* Session tab drags already change the sidebar order under the pointer. This
   persists that shared order after the drag ends, rather than per hover. */
export function persistTabOrder(id) {
  const tab = S.tabs.find((candidate) => candidate.id === id);
  if (tab?.kind !== "session") return;
  const project = S.projects.find((p) => p.id === projectOfTab(tab));
  if (project) reorderSidebarSessions(project, project.recent_sessions.map((session) => session.id));
}

/* The prompt bar watches this counter. A counter, rather than a Boolean,
   makes each newly created session request focus even when one is already
   active. */
function focusPrompt() {
  S.promptFocus += 1;
}

/* Selecting a tab selects its project with it, and the project remembers
   where it was left so coming back lands on the same tab. */
export function selectTab(id) {
  const tab = S.tabs.find((t) => t.id === id);
  if (!tab) return;
  S.activeTab = id;
  const projectID = projectOfTab(tab);
  if (!projectID) return;
  S.activeProjectID = projectID;
  S.lastTab[projectID] = id;
}

/* selectTabAt is Alt+1 … Alt+9, so the number matches what the strip shows —
   which is this project's tabs, not every tab open. */
export function selectTabAt(n) {
  const tab = S.strip[n - 1];
  if (tab) selectTab(tab.id);
}

/* selectAdjacentTab is Ctrl+PageUp and Ctrl+PageDown: one step along the
   strip, wrapping at both ends so every tab is reachable without turning
   round. */
export function selectAdjacentTab(step) {
  const strip = S.strip;
  const n = strip.length;
  if (!n) return;
  const at = strip.findIndex((t) => t.id === S.activeTab);
  selectTab(strip[at < 0 ? (step > 0 ? 0 : n - 1) : (at + step + n) % n].id);
}

/* switchProject is Alt+A … Alt+H and a click on a project row: the strip
   becomes that project's, on the tab it was last left on. A project with
   nothing open shows its own page rather than an empty centre. */
export async function switchProject(id) {
  S.activeProjectID = id;
  const mine = S.strip;
  const last = mine.find((t) => t.id === S.lastTab[id]) || mine[0];
  if (last) return selectTab(last.id);
  S.activeTab = "";
  await openProject(id);
}

/* Alt+A … Alt+H address the first eight projects by where they sit in the
   sidebar, so dragging one changes the letter that reaches it. */
export function selectProjectAt(i) {
  const p = S.projects[i];
  if (p) switchProject(p.id);
}

/* closeTab also closes the files opened from the tab, which have nothing to
   belong to once it is gone, and moves to the neighbour in the same project
   rather than back to the first tab. A never-used session is disposable;
   every other session is archived.

   Unsaved edits stop it: closing a file that has been typed into — or a
   conversation holding one — asks first, and resolveClosing comes back here
   once the answer has been carried out. `force` is for a project being
   deleted, which has already been confirmed and takes everything with it. */
export async function closeTab(id, remember = true, force = false) {
  const tab = S.tabs.find((t) => t.id === id);
  if (!tab) return;
  const unsaved = force ? [] : unsavedUnder(id);
  if (unsaved.length) {
    S.closing = { id, remember, tabs: unsaved };
    return;
  }
  const projectID = projectOfTab(tab);
  // Its own project's list, not the strip: a project being deleted closes
  // tabs that are not on screen.
  const at = S.tabs.filter((t) => projectOfTab(t) === projectID).findIndex((t) => t.id === id);
  S.tabs = S.tabs.filter((t) => t.id !== id && t.owner !== id);
  if (S.lastTab[projectID] === id) delete S.lastTab[projectID];
  if (S.activeTab === id || !S.tabs.some((t) => t.id === S.activeTab)) {
    const left = S.tabs.filter((t) => projectOfTab(t) === projectID);
    const next = left[Math.min(at, left.length - 1)];
    if (next) selectTab(next.id);
    else {
      S.activeTab = "";
      // Emptying a workspace falls back to its project page, which is where
      // the next conversation is started from. Closing the page itself does
      // not bring it back — that one is deliberate.
      if (tab.kind !== "project" && projectID === S.activeProjectID) {
        openProject(projectID, true);
      }
    }
  }
  resubscribe();

  if (!remember || tab.kind !== "session") return;
  if (tab.detail.messages.length) return setSessionArchived(tab.detail.session, true);
  try {
    await api("DELETE", "/api/sessions/" + tab.sessionID);
    await Promise.all([refreshProjects(), refreshSessions()]);
    reloadProjects();
  } catch (e) {
    fail(e);
  }
}

/* currentProjectID is the project the right panel works against, whichever
   kind of view is in front — and the selected project when the strip is
   empty, so closing the last tab does not empty the Tree as well. */
export function currentProjectID() {
  if (S.detail) return S.detail.session.project_id;
  if (S.project) return S.project.project.id;
  return S.activeProjectID;
}

/* -------------------------------------------------------------- projects */

export async function refreshProjects() {
  S.projects = await api("GET", "/api/projects");
  for (const project of S.projects) {
    syncProjectSessionOrder(project.id, project.recent_sessions.map((session) => session.id));
  }
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
    // Deselect before closing: the strip cannot show a project that is gone,
    // and an emptied workspace must not try to reopen its page.
    const wasShowing = S.activeProjectID === p.id;
    if (wasShowing) S.activeProjectID = null;
    // Its sessions went with it, so their tabs cannot stay open. The project
    // is already gone by here, so an unsaved file has nowhere to be saved to.
    for (const t of [...S.tabs]) {
      if (projectOfTab(t) === p.id) closeTab(t.id, false, true);
    }
    await Promise.all([refreshProjects(), refreshSessions()]);
    if (wasShowing && S.projects.length) await switchProject(S.projects[0].id);
  } catch (e) {
    fail(e);
  }
}

/* Every tab names its project: a file carries one of its own because the view
   it was opened from can be closed while it stays open. */
function projectOfTab(t) {
  if (t.kind === "project") return t.projectID;
  if (t.kind === "session") return t.detail.session.project_id;
  return t.projectID || null;
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

/* A project page is where a session is started from, so it hands its tab over
   to the new conversation rather than staying open behind it. The session
   still lands at the end of the session group; only the page goes. */
export async function startSession(project) {
  const cfg = sessionDefaults();
  if (!cfg) return fail(new Error("No agent CLI is available. Open Settings to see why."));
  const replaced = S.tab?.kind === "project" ? S.tab.id : "";
  try {
    const sess = await api("POST", "/api/sessions", { project_id: project.id, ...cfg });
    await Promise.all([refreshProjects(), refreshSessions()]);
    await openSession(sess.id);
    if (replaced) closeTab(replaced, false);
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
    // Unsent text belongs to the conversation, not to the prompt bar, so it
    // survives every tab switch and only closing throws it away.
    draft: "",
  });
  await Promise.all([refreshProjects(), refreshSessions()]).catch(fail);
}

export async function setModel(provider, model, effort) {
  const tab = S.owner;
  if (tab?.kind !== "session") return;
  try {
    tab.detail.session = await api("PATCH", "/api/sessions/" + tab.sessionID, {
      provider,
      model,
      effort,
    });
    rememberUsed(tab.detail.session);
  } catch (e) {
    fail(e);
  }
}

/* renameSession retitles a session. The same session shows in the sidebar, in
   its project's view and on the tab strip, so every list is re-read; the tab
   label is set here too, because a session outside the sidebar's window is
   not in the list that would otherwise carry the new name back. */
export async function renameSession(session, title) {
  title = title.trim();
  if (!title || title === session.title) return;
  try {
    const updated = await api("PATCH", "/api/sessions/" + session.id, { title });
    const tab = S.tabs.find((t) => t.kind === "session" && t.sessionID === session.id);
    if (tab) {
      tab.detail.session = updated;
      tab.label = tabLabel(updated);
    }
    await Promise.all([refreshProjects(), refreshSessions()]);
    reloadProjects();
  } catch (e) {
    fail(e);
  }
}

/* setSessionArchived removes a session from project views and closes its tab
   without deleting it. The Sessions list keeps archived sessions so they can
   be restored. */
export async function setSessionArchived(session, archived) {
  try {
    const updated = await api("PATCH", "/api/sessions/" + session.id, { done: archived });
    const tab = S.tabs.find((t) => t.kind === "session" && t.sessionID === session.id);
    if (tab) tab.detail.session = updated;
    await Promise.all([refreshProjects(), refreshSessions()]);
    reloadProjects();
    if (archived && tab) await closeTab(tab.id, false);
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
  syncProjectSessionOrder(tab.projectID, ids);
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
  syncProjectSessionOrder(project.id, ids);
  try {
    await api("POST", `/api/projects/${project.id}/sessions/order`, { ids });
    await refreshProjects();
  } catch (e) {
    fail(e);
    refreshProjects().catch(() => {});
  }
}

/* ------------------------------------------------------------------ files */

/* A file tab shows the file, its diff, or — for markdown and HTML — what it
   renders as, and opens in whichever was read last: looking at one diff
   usually means the next changed file wants a diff too. Each view is fetched
   only when it is first asked for.

   Editing lives on the tab, in `edited`: null until a key is pressed, so a
   file that has only been read carries nothing extra and switching tabs and
   coming back finds the edits where they were left. */
const FILE_MODES = ["edit", "diff", "preview"];
const PREVIEWABLE = /\.(md|markdown|html?)$/i;

export function canPreview(path) {
  return PREVIEWABLE.test(String(path || ""));
}

export function isDirty(tab) {
  return tab?.kind === "file" && tab.edited !== null && tab.edited !== tab.content;
}

export function openFile(path, silent = false) {
  return openFileIn(currentProjectID(), S.owner?.id || "", path, silent);
}

/* openFileIn also rebuilds a saved tab, whose owner may be gone. A file
   belongs to the view it was opened from, but it belongs to its project even
   when nothing else in that project is open. */
async function openFileIn(projectID, ownerID, path, silent) {
  if (!projectID) return;
  const tabID = `file:${projectID}:${path}`;
  if (S.tabs.some((t) => t.id === tabID)) return selectTab(tabID);
  const tab = {
    id: tabID,
    kind: "file",
    label: path.split("/").pop(),
    owner: ownerID,
    projectID,
    path,
    // Preview is remembered like the other two, but a file that renders as
    // nothing opens in the editor instead of on a blank pane.
    mode: S.fileMode === "preview" && !canPreview(path) ? "edit" : S.fileMode,
    content: null,
    diff: null,
    edited: null,
    // What was not read whole must never be written back over its source.
    readOnly: false,
    saving: false,
  };
  try {
    await loadFileTab(tab);
  } catch (e) {
    if (!silent) fail(e);
    return;
  }
  addTab(tab);
}

/* loadFileTab fetches the view the tab is showing, once. The tab may not be
   open yet — a file that cannot be read opens no tab at all. */
async function loadFileTab(tab) {
  const id = projectOfTab(tab);
  if (!id) return;
  const query = `path=${encodeURIComponent(tab.path)}`;
  if (tab.mode === "diff") {
    if (tab.diff !== null) return;
    tab.diff = (await api("GET", `/api/projects/${id}/diff?${query}`)).diff || "";
    return;
  }
  if (tab.content !== null) return;
  const f = await api("GET", `/api/projects/${id}/file?${query}`);
  tab.readOnly = !!(f.binary || f.partial);
  tab.content = f.binary ? "(binary file)" : f.content + (f.partial ? "\n\n… truncated" : "");
}

/* setFileMode switches one tab and remembers the choice for the next file —
   but only once the switch has worked, so a view that cannot be fetched puts
   the tab back rather than leaving it on an empty pane. */
export async function setFileMode(tab, mode) {
  if (tab?.kind !== "file" || tab.mode === mode || !FILE_MODES.includes(mode)) return;
  const previous = tab.mode;
  tab.mode = mode;
  try {
    await loadFileTab(tab);
  } catch (e) {
    tab.mode = previous;
    return fail(e);
  }
  S.fileMode = mode;
  persist(FILE_MODE_KEY, mode);
}

/* The two shapes of the same diff, on the same toggle, remembered the same
   way. Nothing is refetched: it is one parse of text already here. */
export function setDiffView(view) {
  if (view !== "unified" && view !== "split") return;
  S.diffView = view;
  persist(DIFF_VIEW_KEY, view);
}

/* editFile compares against what was loaded rather than latching a flag, so
   typing something and taking it back leaves the tab clean again. */
export function editFile(tab, text) {
  if (tab?.kind !== "file" || tab.readOnly) return;
  tab.edited = text === tab.content ? null : text;
}

/* saveFile writes the edits back and moves the tab's own baseline with them.
   The diff is dropped rather than patched: the working tree has changed, and
   git is the only thing that knows what it now says. */
export async function saveFile(tab) {
  if (!isDirty(tab) || tab.saving) return !isDirty(tab);
  const id = projectOfTab(tab);
  if (!id) return false;
  const content = tab.edited;
  tab.saving = true;
  try {
    await api("PUT", `/api/projects/${id}/file?path=${encodeURIComponent(tab.path)}`, { content });
  } catch (e) {
    fail(e);
    return false;
  } finally {
    tab.saving = false;
  }
  // Only what was actually sent is now on disk: anything typed while the
  // request was in flight is still an edit.
  tab.content = content;
  if (tab.edited === content) tab.edited = null;
  tab.diff = null;
  if (tab.mode === "diff") await loadFileTab(tab).catch(fail);
  refreshChanged().catch(() => {});
  return true;
}

/* saveActiveFile is Ctrl+S, which belongs to whatever file is in front. */
export function saveActiveFile() {
  const tab = S.tab;
  if (tab?.kind === "file") return saveFile(tab);
}

/* unsavedUnder is a tab and everything that would close with it, narrowed to
   the files carrying edits — what the close dialog is about. */
function unsavedUnder(id) {
  return S.tabs.filter((t) => (t.id === id || t.owner === id) && isDirty(t));
}

/* resolveClosing answers the dialog. Saving that fails leaves it open, since
   the alternative is throwing the edits away on the user's behalf. */
export async function resolveClosing(action) {
  const pending = S.closing;
  if (!pending) return;
  if (action === "cancel") {
    S.closing = null;
    return selectTab(pending.tabs[0].id);
  }
  if (action === "save") {
    for (const tab of pending.tabs) if (!(await saveFile(tab))) return;
  } else {
    for (const tab of pending.tabs) tab.edited = null;
  }
  S.closing = null;
  await closeTab(pending.id, pending.remember);
}

/* Not named `remember`: closeTab already takes a parameter by that name. */
function persist(key, value) {
  try {
    localStorage.setItem(key, value);
  } catch {
    /* private mode or a full quota only costs the remembered choice */
  }
}

function loadFileMode() {
  try {
    const mode = localStorage.getItem(FILE_MODE_KEY);
    // "file" is what the editor used to be called.
    if (mode === "diff" || mode === "preview") S.fileMode = mode;
    if (localStorage.getItem(DIFF_VIEW_KEY) === "split") S.diffView = "split";
  } catch {
    /* keep the defaults */
  }
}

/* ---------------------------------------------------------- saved tabs */

/* Tabs contain live API data, so only save the small identifiers needed to
   rebuild them. This avoids restoring an old transcript over the server's
   current version after the app is relaunched. */
function savedTab(tab) {
  if (tab.kind === "project") return { id: tab.id, kind: tab.kind, projectID: tab.projectID };
  if (tab.kind === "session") {
    return { id: tab.id, kind: tab.kind, sessionID: tab.sessionID, draft: tab.draft || "" };
  }
  if (tab.kind === "file") {
    return {
      id: tab.id,
      kind: tab.kind,
      owner: tab.owner,
      projectID: tab.projectID,
      path: tab.path,
    };
  }
  return null;
}

let restoringTabs = false;

/* Debounced because an unsent prompt is saved with the tabs, and a keystroke
   is not worth a trip through JSON and localStorage. */
const saveOpenTabs = debounce(() => {
  if (restoringTabs) return;
  try {
    localStorage.setItem(
      OPEN_TABS_KEY,
      JSON.stringify({
        tabs: S.tabs.map(savedTab).filter(Boolean),
        activeTab: S.activeTab,
        activeProjectID: S.activeProjectID,
      }),
    );
  } catch {
    /* private mode or a full quota just means tabs open fresh next time */
  }
}, 300);

/* Restoring opens the same kinds of tabs through the ordinary actions, so
   their data is fetched anew and missing projects or sessions are skipped. A
   file is restored against its own project, and keeps the view it was opened
   from only if that one came back too. */
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
        const open = S.tabs.find((t) => t.id === tab.id);
        if (open) open.draft = typeof tab.draft === "string" ? tab.draft : "";
      } else if (tab?.kind === "file" && typeof tab.path === "string") {
        const owner = S.tabs.some((open) => open.id === tab.owner) ? tab.owner : "";
        await openFileIn(tab.projectID, owner, tab.path, true);
      }
    }
    if (S.projects.some((p) => p.id === saved.activeProjectID)) {
      S.activeProjectID = saved.activeProjectID;
    }
    if (S.tabs.some((tab) => tab.id === saved.activeTab)) selectTab(saved.activeTab);
  } finally {
    restoringTabs = false;
    saveOpenTabs();
  }
}

watch(
  () => ({
    activeTab: S.activeTab,
    activeProjectID: S.activeProjectID,
    tabs: S.tabs.map(savedTab),
  }),
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

  tab.draft = "";
  endLive(tab);
  push(tab, "user", prompt);
  tab.detail.running = true;

  try {
    const turn = await api("POST", `/api/sessions/${tab.sessionID}/messages`, { prompt });
    tab.detail.turns.push(turn);
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

/* --------------------------------------------------------------- colours */

/* The accent and the grey are the user's too, and they are read before the
   app mounts so the first paint is already in the chosen colours. A name
   that is no longer offered is dropped rather than passed on: an unknown
   palette resolves to no colour at all. */
export function loadColors() {
  try {
    const saved = JSON.parse(localStorage.getItem(COLORS_KEY));
    if (ACCENTS.some((c) => c.name === saved?.accent)) S.colors.accent = saved.accent;
    if (NEUTRALS.some((c) => c.name === saved?.neutral)) S.colors.neutral = saved.neutral;
  } catch {
    /* keep the defaults */
  }
  applyColors(S.colors);
}

/* key is "accent" or "neutral". */
export function setColor(key, name) {
  S.colors[key] = name;
  applyColors(S.colors);
  try {
    localStorage.setItem(COLORS_KEY, JSON.stringify(S.colors));
  } catch {
    /* private mode, a full quota — the colours just do not persist */
  }
}

/* ------------------------------------------------------------------ init */

export async function init() {
  loadLastUsed();
  loadLayout();
  loadFileMode();
  // Open tabs are saved, but an edited file is not — it would put a whole
  // working copy in localStorage — so the browser's own warning is what
  // stands between unsaved edits and a reload.
  window.addEventListener("beforeunload", (e) => {
    if (S.tabs.some(isDirty)) e.preventDefault();
  });
  try {
    await loadProviders();
    await refreshProjects();
    await refreshSessions();
    await restoreOpenTabs();
    // A first launch has no tabs to say which project is selected, and the
    // sidebar, the Tree and Ctrl+N all want one.
    if (!S.activeProjectID && S.projects.length) S.activeProjectID = S.projects[0].id;
  } catch (e) {
    fail(e);
  }
}
