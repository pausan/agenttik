/* agenttik state. One reactive object, the actions that change it, and the
   SSE connection that keeps it current. Components read S and call these;
   none of them touch the DOM.

   Everything open is a tab in S.tabs: a session (transcript and prompt bar),
   a project (its sessions) or a file. One leading project-or-session context
   tab is reused per project; independent views stay open. S.detail and
   S.project are derived from whichever tab is in front.

   The strip only shows one project at a time — S.strip is S.tabs narrowed to
   S.activeProjectID — so switching project switches the whole centre to that
   project's work. */

import { nextTick, reactive, watch } from "vue";
import { api } from "./api";
import { debounce } from "./debounce";
import { ACCENTS, DEFAULT_COLORS, NEUTRALS, applyColors } from "./theme";
import { ACTIONS, matches } from "./shortcuts";

/* Sidebar widths are the user's, so they are kept across reloads. */
const LAYOUT_KEY = "agenttik.layout";
const LAST_USED_KEY = "agenttik.lastUsed";
const OPEN_TABS_KEY = "agenttik.openTabs";
const FILE_MODE_KEY = "agenttik.fileMode";
const DIFF_VIEW_KEY = "agenttik.diffView";
const COLORS_KEY = "agenttik.colors";
const KEYS_KEY = "agenttik.keys";
const WINDOW_KEY = "agenttik.window";
const TASK_PAGE_KEY = "agenttik.taskPageSize";
const SCHEDULE_KEY = "agenttik.schedule";
const FOLD_KEY = "agenttik.foldOthers";
const LAYOUT_LIMITS = { left: [180, 520], right: [200, 620] };

/* The letters Alt reaches a project with. The first eight rows of the
   sidebar get one, so dragging a project changes its letter. */
export const PROJECT_KEYS = "ABCDEFGH";

/* Alt+1 … Alt+9 reach the first nine tasks in the selected project's
   sidebar order. */
export const TAB_CHORDS = 9;

/* How many tasks a project page draws at once. 25 is a page worth reading
   without scrolling past what was asked for; the rest are a click away. The
   choice is remembered, like the Sessions window below, because it is a
   preference rather than a question asked again on every project. */
export const TASK_PAGE_SIZES = [10, 25, 50, 100];

/* How far back the Sessions list reaches, and the order the picker offers.
   The first is the default: the usual question is what is running now, not
   what ran last week. The values are the ones /api/sessions takes, and the
   list is also what a remembered choice is checked against. */
export const TASK_WINDOWS = [
  { label: "Last hour", value: "1h" },
  { label: "Last day", value: "1d" },
  { label: "Last 3 days", value: "3d" },
  { label: "Last week", value: "7d" },
  { label: "Last month", value: "1mo" },
  { label: "All", value: "all" },
];

export const S = reactive({
  providers: [],
  stars: [],
  projects: [],
  // Archived projects: read when Settings opens, which is the only place
  // they are shown. See specs/041-project-archiving.md.
  archivedProjects: [],
  subscriptionLimits: {}, // "provider:account" -> its latest allowance buckets
  // Whether this desktop window's server is also exposed for a browser to
  // reach, read when Settings opens like the archived projects beside it.
  // available is false on a web launch, which already is the server. See
  // specs/043-exposed-server.md.
  serverConfig: { available: false, enabled: false, host: "", port: 0, listening: false,
    auth_enabled: false, has_password: false, totp_secret: "", totp_uri: "" },
  /* sessionID -> unsent prompt, for a conversation with no tab of its own.
     A project has one context slot, so opening anything else in it drops the
     session tab the text was typed into; the text is the user's and outlives
     that tab. */
  drafts: {},
  sessions: [],
  schedules: [], // repeating prompts, drawn above the sessions in both lists
  tabs: [], // every open view, of every project
  closedTabs: {}, // project id -> most recently closed tabs and archived sessions
  activeTab: "",
  activeProjectID: null, // the project the strip and the sidebar show
  /* The projects whose tasks are folded away in the sidebar. Held here rather
     than in the sidebar because Alt and the project's letter reaches it. */
  collapsedProjects: new Set(),
  // Whether selecting a project folds every other one away. On unless it has
  // been turned off, so a window nobody has answered for shows one project's
  // tasks rather than eight projects' at once. From Settings, and unlike the
  // fold state above it is remembered.
  foldOthers: true,
  fileMode: "edit", // "edit", "diff" or "preview", carried to the next file
  diffView: "unified", // "unified" or "split", likewise
  closing: null, // a close waiting on what to do with unsaved edits
  promptFocus: 0,
  queuedEdit: 0, // counter the transcript watches, to open its last queued prompt
  taskStatsOpen: false,
  inspector: { panes: ["changed", "logs"], active: "changed" },
  changed: [],
  repositories: [],
  repository: "",
  log: { branch: "", head: "", commits: [] },
  logFilter: "",
  logOpen: "", // the commit whose file list is expanded
  logFiles: {}, // commit hash -> what it touched, fetched the first time it opens
  // Every path in the project, and the subset git ignores. The Tree draws
  // both, the ignored ones grey. treeDirs are the folders holding no file,
  // which a listing of paths cannot imply on its own.
  tree: [],
  treeIgnored: [],
  treeDirs: [],
  treeFilter: "",
  query: "", // sidebar session filter
  window: TASK_WINDOWS[0].value,
  taskPageSize: 25,
  lastUsed: null,
  layout: { left: 272, right: 312 },
  colors: { ...DEFAULT_COLORS }, // the accent and the grey, from Settings
  keys: defaultKeys(), // action id -> the chords bound to it, from Settings

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
  // Two answers that do not depend on each other, and the providers one can
  // cost a CLI probe, so they are asked for together.
  const [providers, stars] = await Promise.all([
    api("GET", "/api/providers"),
    api("GET", "/api/stars"),
  ]);
  S.providers = providers;
  S.stars = stars;
}

export function providerOf(name) {
  return S.providers.find((p) => p.name === name);
}

/* ---------------------------------------------------------- subscriptions */

/* A provider can be signed in to more than one subscription — a company
   account and a personal one — and every place that offers a model says
   which subscription it would run on. The server always sends at least one
   account, the machine's own CLI login, so nothing here has to special-case
   a provider that has none. See specs/050-subscription-accounts.md. */

export function accountsOf(name) {
  return providerOf(name)?.accounts || [];
}

export function accountOf(name, id) {
  return accountsOf(name).find((a) => a.id === (id || 0));
}

/* accountLabel is what a subscription is called beside a model. The machine's
   own login on a provider with nothing else configured is not worth naming:
   there is no second one to tell it apart from.

   A task pointing at a subscription that has been removed is always named,
   though, however few are left — that task will refuse to start, and the
   label is where it says why. */
export function accountLabel(name, id) {
  const accounts = accountsOf(name);
  const account = accountOf(name, id);
  if (!account) return id ? "removed subscription" : "";
  return accounts.length < 2 ? "" : account.alias;
}

/* The subscription a new task on this provider starts on. */
export function defaultAccountOf(name) {
  return (accountsOf(name).find((a) => a.is_default) || { id: 0 }).id;
}

/* modelPickerGroups is the group list every model control draws — the prompt
   bar's, a queued prompt's, a job's. One group per provider and
   subscription, so which account a model would run on is visible where the
   model is chosen, and searchable with it. A provider with a single
   subscription keeps its plain name, which is every provider until a second
   one is configured.

   Values are `model:<provider>:<account>:<model>`, read back by
   parseModelChoice, so all three controls speak one format. */
export function modelPickerGroups() {
  return S.providers.flatMap((p) => {
    const accounts = p.accounts?.length ? p.accounts : [{ id: 0, alias: "" }];
    const named = accounts.length > 1;
    return accounts.map((account) => ({
      id: `${p.name}:${account.id}`,
      label: named ? `${p.display_name} · ${account.alias}` : p.display_name,
      items: p.models.map((m) => ({
        label: named ? `${m.label} · ${account.alias}` : m.label,
        description: named
          ? `${p.display_name} · ${account.alias} · ${m.id}`
          : `${p.display_name} · ${m.id}`,
        value: `model:${p.name}:${account.id}:${m.id}`,
        disabled: !p.available,
      })),
    }));
  });
}

/* parseModelChoice reads one picked entry back. The model id keeps whatever
   is left, so an id with a colon in it survives the round trip. */
export function parseModelChoice(value) {
  const parts = String(value).split(":");
  return {
    provider: parts[1],
    accountID: Number(parts[2]) || 0,
    model: parts.slice(3).join(":"),
  };
}

/* effortsFor is the levels one model accepts, falling back to the
   provider-wide list for a model that names none. */
export function effortsFor(provider, model) {
  const p = providerOf(provider);
  return p?.models.find((m) => m.id === model)?.efforts || p?.efforts || [];
}

/* addAccount, renameAccount and removeAccount are Settings' own; each
   re-reads the providers afterwards because the account list lives on them
   and the pickers read it from there. */
export async function addAccount(provider, alias, home) {
  await api("POST", `/api/providers/${encodeURIComponent(provider)}/accounts`, { alias, home });
  await loadProviders();
}

export async function updateAccount(provider, id, patch) {
  await api("PATCH", `/api/providers/${encodeURIComponent(provider)}/accounts/${id}`, patch);
  await loadProviders();
}

export async function removeAccount(provider, id) {
  await api("DELETE", `/api/providers/${encodeURIComponent(provider)}/accounts/${id}`);
  await loadProviders();
}

export async function accountUsage(provider, id) {
  return api("GET", `/api/providers/${encodeURIComponent(provider)}/accounts/${id}/usage`);
}

export async function setDefaultAccount(provider, id) {
  await api("PUT", `/api/providers/${encodeURIComponent(provider)}/account`, { account_id: id });
  await loadProviders();
}

/* signInAccount hands the CLI's own login to a terminal on the machine the
   CLIs are installed on, and returns the command either way — a machine with
   no terminal to open, or a browser reading this from another one, is told
   what to run instead. The account list is re-read afterwards, so a login
   that finished while the dialog was open shows as signed in. */
export async function signInAccount(provider, id) {
  const result = await api(
    "POST",
    `/api/providers/${encodeURIComponent(provider)}/accounts/${id}/login`,
    {},
  );
  await loadProviders();
  return result;
}

/* --------------------------------------------------------- server exposure */

/* Read when Settings opens, like loadArchivedProjects: it does not change
   while the dialog is shut, and nothing else needs it. */
export async function loadServerConfig() {
  S.serverConfig = await api("GET", "/api/server");
}

/* setServerConfig sends the whole setting every time — enabled, host and
   port together — because turning the server on with a fresh address and
   pointing a running one at a new address are the same request. patch is
   applied over the current value, so a caller changing one field does not
   have to know the other two. A rejected change (a bad address, a port
   already taken) leaves S.serverConfig, and so the form, exactly as it was. */
export async function setServerConfig(patch) {
  const body = { enabled: S.serverConfig.enabled, host: S.serverConfig.host, port: S.serverConfig.port, ...patch };
  try {
    S.serverConfig = await api("PUT", "/api/server", body);
  } catch (e) {
    fail(e);
  }
}

/* setServerAuth sets the lock in front of that exposed server: whether it is
   asked for, the password, and the authenticator seed. A blank password or
   seed means "leave that one alone", so changing one does not mean resending
   the other — and the password is never sent back down to be resent with.

   Any change that goes through ends every browser session already open, this
   one included when Settings is being read over the exposed server itself.
   Errors are thrown rather than swallowed: the caller draws them next to the
   field that caused them, since "the password is too short" belongs under the
   password box and not in a corner. */
export async function setServerAuth(patch) {
  S.serverConfig = await api("PUT", "/api/server/auth", {
    enabled: S.serverConfig.auth_enabled,
    password: "",
    totp_secret: "",
    ...patch,
  });
}

/* resetServerTOTP rolls a fresh random seed, for a seed that has been seen by
   the wrong person. Whatever was paired against the old one stops working at
   once, so the pane re-draws its QR straight after. */
export async function resetServerTOTP() {
  S.serverConfig = await api("POST", "/api/server/auth/totp", {});
}

/* contextWindow is how many tokens the session's model holds, which the
   prompt bar's gauge measures the live context against. 0 means unknown.

   The window the provider reported for the last turn wins: the same model
   alias runs in a 200k or a 1M variant, so the per-model figure is only what
   to show until a turn says otherwise. */
export function contextWindow(session, stats) {
  if (stats?.context_window) return stats.context_window;
  if (!session) return 0;
  const model = providerOf(session.provider)?.models.find((m) => m.id === session.model);
  return model?.context_window || 0;
}

// Subscription allowances are provider-reported account limits, not guesses
// from token totals. Codex and Claude Code both answer a local query — the
// latter through its own /usage — and the server falls back to the newest
// reading a turn volunteered if the query fails. A provider that does neither
// returns no buckets.
//
// Asking costs a CLI process — Claude Code spends about two seconds on its
// own /usage — so a burst of model changes shares the one read in flight
// rather than spawning one each. Every window comes back on any of them.
//
// Keyed by provider and subscription both: two subscriptions of one provider
// have two allowances, and a work account's bars shown against a personal one
// would be worse than no bars at all.
const limitReads = {};
export function limitsKey(provider, accountID) {
  return `${provider}:${accountID || 0}`;
}
export function refreshSubscriptionLimits(provider, accountID) {
  if (!provider) return Promise.resolve();
  const key = limitsKey(provider, accountID);
  if (limitReads[key]) return limitReads[key];
  const read = api(
    "GET",
    `/api/providers/${encodeURIComponent(provider)}/subscription-limits?account=${accountID || 0}`,
  )
    .then((limits) => {
      S.subscriptionLimits[key] = limits;
    })
    .finally(() => {
      delete limitReads[key];
    });
  limitReads[key] = read;
  return read;
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

/* addTab inserts an independent tab unless it is already open, in which case
   it just comes to the front. Project and session views use setContextTab. */
function addTab(tab) {
  if (!S.tabs.some((t) => t.id === tab.id)) insertTab(tab);
  selectTab(tab.id);
  resubscribe();
}


function isContextTab(tab) {
  return tab.kind === "project" || tab.kind === "session" || tab.kind === "schedule";
}

/* A project has one leading context slot. Selecting a project, a session or a
   scheduled job replaces that slot without disturbing files opened from the
   prior view.
   Those files follow the new context so their inspector and Tree context stay
   useful. This also folds old saved layouts with several session tabs into
   the current shape as they are restored. */
function setContextTab(tab) {
  const projectID = projectOfTab(tab);
  const previous = S.tabs.filter(
    (candidate) => isContextTab(candidate) && projectOfTab(candidate) === projectID,
  );
  const previousIDs = new Set(previous.map((candidate) => candidate.id));
  const first = S.tabs.findIndex((candidate) => projectOfTab(candidate) === projectID);

  if (previous.length === 1 && previous[0] === tab && first >= 0 && S.tabs[first] === tab) {
    return selectTab(tab.id);
  }

  const before = S.tabs.slice(0, Math.max(first, 0)).filter((candidate) => previousIDs.has(candidate.id)).length;
  for (const candidate of previous) keepDraft(candidate);
  S.tabs = S.tabs.filter((candidate) => !previousIDs.has(candidate.id));
  const at = first < 0 ? S.tabs.length : first - before;
  S.tabs.splice(at, 0, tab);
  for (const candidate of S.tabs) {
    if (previousIDs.has(candidate.owner)) candidate.owner = tab.id;
  }
  selectTab(tab.id);
  resubscribe();
}

/* An unsent prompt belongs to the conversation. keepDraft hands it to
   S.drafts as the tab goes, and openTask hands it back; closing or archiving
   the conversation is the one thing that throws it away. */
function keepDraft(tab) {
  if (tab.kind !== "session") return;
  if (tab.draft?.trim()) S.drafts[tab.sessionID] = tab.draft;
  else delete S.drafts[tab.sessionID];
}

/* insertTab puts every new tab at the end of its project workspace. Projects
   share one flat array, so the insertion point is immediately after that
   project's last tab rather than necessarily the end of the whole array. */
function insertTab(tab) {
  const id = projectOfTab(tab);
  let at = S.tabs.length;
  S.tabs.forEach((t, i) => {
    if (projectOfTab(t) !== id) return;
    at = i + 1;
  });
  S.tabs.splice(at, 0, tab);
}

/* swapTab puts a new tab where an old one sat, which is what reusing the
   temporary tab looks like: the strip keeps its length and its order, so the
   numbers do not shift under the pointer mid-click. */
function swapTab(id, tab) {
  const at = S.tabs.findIndex((t) => t.id === id);
  if (at < 0) return addTab(tab);
  S.tabs.splice(at, 1, tab);
  selectTab(tab.id);
  resubscribe();
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
   places, and the numbers follow the strip rather than the strip following
   the numbers. */
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
  if (project) reorderSidebarTasks(project, project.recent_sessions.map((session) => session.id));
}

/* The prompt bar watches this counter. A counter, rather than a Boolean,
   makes each newly created session request focus even when one is already
   active. */
export function focusPrompt() {
  S.promptFocus += 1;
}

/* Picking a task from a list is a prelude to typing in it, so the cursor
   lands in its prompt box rather than on the row that was clicked. The tick
   waits for the bar to be there: arriving from a project page mounts it with
   this very switch, and the watcher only sees requests made after it mounts.
   See specs/025-blank-task-reuse.md for the same ordering when starting one. */
export async function pickTask(id) {
  await openTask(id);
  await nextTick();
  focusPrompt();
}

/* Selecting a tab selects its project with it. Which tab a project comes
   back on is switchProject's rule, not a remembered one. */
export function selectTab(id) {
  const tab = S.tabs.find((t) => t.id === id);
  if (!tab) return;
  S.activeTab = id;
  const projectID = projectOfTab(tab);
  if (projectID) S.activeProjectID = projectID;
}

/* Alt+1 … Alt+9 selects a session by its top-to-bottom sidebar position and
   leaves the prompt ready for the next message, exactly as clicking it does. */
export function selectTaskAt(n) {
  const session = S.projects.find((project) => project.id === S.activeProjectID)?.recent_sessions[n - 1];
  if (!session) return;
  return pickTask(session.id);
}

/* Which field on the tab in front carries the id of the row it stands for. */
const ROW_ID = { project: "projectID", schedule: "scheduleID", session: "sessionID" };

/* Ctrl+PageUp and Ctrl+PageDown walk every row of the sidebar, top to bottom —
   each project and, under it, that project's scheduled jobs and then its
   tasks — wrapping at both ends rather than staying inside the active
   project. The order is the sidebar's own, jobs above tasks, so the keys walk
   what the eye reads. A project row opens its page, a job row its job and a
   task row the task. The tab in front only supplies where to start. */
export function selectAdjacentSidebarRow(step) {
  const rows = S.projects.flatMap((project) => [
    { kind: "project", id: project.id },
    ...project.schedules.map((schedule) => ({ kind: "schedule", id: schedule.id })),
    ...project.recent_sessions.map((session) => ({ kind: "session", id: session.id })),
  ]);
  if (!rows.length) return;
  const owner = S.owner;
  const isOwner = (row) => row.kind === owner?.kind && row.id === owner[ROW_ID[row.kind]];
  const at = rows.findIndex(isOwner);
  const next = at < 0 ? (step > 0 ? 0 : rows.length - 1) : (at + step + rows.length) % rows.length;
  const row = rows[next];
  if (row.kind === "project") openProject(row.id);
  else if (row.kind === "schedule") openSchedule(row.id);
  else openTask(row.id);
}

/* Ctrl+Tab and Ctrl+Shift+Tab walk every visible tab, wrapping at both ends. */
export function selectAdjacentTab(step) {
  const strip = S.strip;
  const n = strip.length;
  if (!n) return;
  const at = strip.findIndex((tab) => tab.id === S.activeTab);
  selectTab(strip[at < 0 ? (step > 0 ? 0 : n - 1) : (at + step + n) % n].id);
}

/* switchProject is Alt+A … Alt+H: the strip becomes that project's, on the
   work rather than on its page. The task it lands on is the last one opened
   there — the session in the project's context slot, since opening another
   replaces it — and otherwise the top of its task list, which is the one
   Alt+1 reaches. Only a project with no tasks at all shows its own page,
   where the first one is started from; a click on the project row is the way
   back to that page. */
export async function switchProject(id) {
  S.activeProjectID = id;
  const open = S.tabs.find((t) => t.kind === "session" && projectOfTab(t) === id);
  if (open) return selectTab(open.id);
  // Nothing is in front while the view is fetched: the strip is already this
  // project's, and the tab that was showing belongs to the one left behind.
  S.activeTab = "";
  const first = S.projects.find((p) => p.id === id)?.recent_sessions[0];
  if (first) return openTask(first.id);
  await openProject(id);
}

/* Alt+A … Alt+H address the first eight projects by where they sit in the
   sidebar, so dragging one changes the letter that reaches it. The letter of
   the project already showing folds its tasks away, and pressing it again
   brings them back: the same chord is how the row is reached and how it is
   opened and shut, so nothing needs the mouse. */
export function selectProjectAt(i) {
  const p = S.projects[i];
  if (!p) return;
  if (S.activeProjectID === p.id) toggleProjectTasks(p.id);
  else switchProject(p.id);
}

/* The chevron beside a project name, and Alt with its letter a second time. */
export function toggleProjectTasks(id) {
  const collapsed = S.collapsedProjects;
  if (collapsed.has(id)) collapsed.delete(id);
  else collapsed.add(id);
}

/* One project open at a time, or each keeping its own fold state — the
   General setting. Folding the others is applied on selection rather than
   stored as an extra flag per project, so the chevron and Alt-letter keep
   working on top of it: a project selected and then folded stays folded
   until it is selected again.

   Selection is watched rather than switchProject called from, because a
   project is also reached by Ctrl+PageDown and by Go to anywhere, which land
   on a task and take its project with them. Nothing selected folds nothing:
   a project being deleted deselects before the next one is chosen. */
export function setFoldOthers(on) {
  if (on === S.foldOthers) return;
  S.foldOthers = on;
  persist(FOLD_KEY, on ? "1" : "0");
  // At once, like the colours: the sidebar is behind the dialog.
  if (on) foldOtherProjects(S.activeProjectID);
}

function foldOtherProjects(id) {
  if (!S.foldOthers || !id) return;
  const collapsed = S.collapsedProjects;
  collapsed.clear();
  for (const p of S.projects) if (p.id !== id) collapsed.add(p.id);
}

watch(() => S.activeProjectID, foldOtherProjects);

/* Nothing stored is a window that has never been asked, which keeps the
   default on rather than reading as off — only an answer turns it off. "" is
   how off was written before the default changed, and still reads as off. */
function loadFoldOthers() {
  try {
    const saved = localStorage.getItem(FOLD_KEY);
    if (saved !== null) S.foldOthers = saved === "1";
  } catch {
    /* keep the default */
  }
}

/* closeTab also closes the files opened from the tab, which have nothing to
   belong to once it is gone, and moves to the neighbour in the same project
   rather than back to the first tab. `closeTabOnly` is the keyboard variant:
   it removes just the tab in front, leaving any files it opened in place.
   Closing a tab only removes that view; archiving is a separate action.

   Unsaved edits stop it: closing a file that has been typed into — or a
   conversation holding one — asks first, and resolveClosing comes back here
   once the answer has been carried out. `force` is for a project being
   deleted, which has already been confirmed and takes everything with it. */
/* Ctrl+W closes exactly the tab that is in front. The tab-strip × continues
   to use closeTab, whose cascade is useful when intentionally dismissing a
   whole conversation and its opened files. */
export function closeTabOnly(id) {
  return closeTab(id, true, false, false);
}

export async function closeTab(id, remember = true, force = false, cascade = true) {
  const tab = S.tabs.find((t) => t.id === id);
  if (!tab) return;
  const unsaved = force ? [] : unsavedUnder(id, cascade);
  if (unsaved.length) {
    S.closing = { id, remember, cascade, tabs: unsaved };
    return;
  }
  const projectID = projectOfTab(tab);
  const dependents = cascade ? S.tabs.filter((t) => t.owner === id) : [];
  if (remember) rememberClosedTab(projectID, tab, dependents);
  if (tab.kind === "session") delete S.drafts[tab.sessionID];
  // Its own project's list, not the strip: a project being deleted closes
  // tabs that are not on screen.
  const at = S.tabs.filter((t) => projectOfTab(t) === projectID).findIndex((t) => t.id === id);
  S.tabs = S.tabs.filter((t) => t.id !== id && (!cascade || t.owner !== id));
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
}

function rememberClosedTab(projectID, tab, dependents = [], archived = false) {
  if (!projectID) return;
  const closed = S.closedTabs[projectID] || [];
  S.closedTabs[projectID] = [{
    tab: savedTab(tab),
    dependents: dependents.map(savedTab).filter(Boolean),
    archived,
  }, ...closed];
}

async function openSavedTab(tab) {
  if (!tab) return false;
  if (tab.kind === "project") await openProject(tab.projectID, true);
  else if (tab.kind === "schedule") await openSchedule(tab.scheduleID, true);
  else if (tab.kind === "session") {
    await openTask(tab.sessionID, true);
    const open = S.tabs.find((candidate) => candidate.id === tab.id);
    if (open) open.draft = typeof tab.draft === "string" ? tab.draft : "";
  } else if (tab.kind === "file") {
    const owner = S.tabs.some((open) => open.id === tab.owner) ? tab.owner : "";
    await openFileIn(tab.projectID, owner, tab.path, restoreOpts(tab));
  }
  return S.tabs.some((open) => open.id === tab.id);
}

/* Ctrl+Shift+T walks the closed entries newest-first, scoped to the project
   in front. Archived sessions are reopened by unarchiving; ordinary tab
   closes never change the session itself. */
export async function reopenClosedTab() {
  const closed = S.closedTabs[S.activeProjectID];
  const entry = closed?.[0];
  if (!entry?.tab) return;
  let restored;
  if (entry.archived) {
    restored = await setTaskArchived({ id: entry.tab.sessionID }, false);
    if (restored) {
      restored = await openSavedTab(entry.tab);
      for (const tab of entry.dependents) await openSavedTab(tab);
      selectTab(entry.tab.id);
    }
  } else {
    restored = await openSavedTab(entry.tab);
    if (restored) {
      for (const tab of entry.dependents) await openSavedTab(tab);
      selectTab(entry.tab.id);
    }
  }
  if (restored) closed.shift();
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

/* Several things ask for the project list at once — a turn starting, a tab
   opening, a drag landing — and those answers can come back in any order.
   Only the newest is worth keeping: an older one would take a running task's
   dot off the sidebar, or put a finished one back, until the next event.

   The stream is reopened from here too, since the sidebar follows the turns
   of every project it draws. */
let projectsRead = 0;

export async function refreshProjects() {
  const read = ++projectsRead;
  const projects = await api("GET", "/api/projects");
  if (read !== projectsRead) return;
  // A project that turns up while one project at a time is the rule arrives
  // folded: it is not the selected one, and nothing has opened it yet.
  if (S.foldOthers) {
    const known = new Set(S.projects.map((project) => project.id));
    for (const p of projects) {
      if (p.id !== S.activeProjectID && !known.has(p.id)) S.collapsedProjects.add(p.id);
    }
  }
  S.projects = projects;
  for (const project of S.projects) {
    syncProjectSessionOrder(project.id, project.recent_sessions.map((session) => session.id));
  }
  resubscribe();
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
  const open = S.tabs.find((t) => t.id === "project:" + id);
  if (open) return setContextTab(open);
  const show = await loadProjectTab(id, silent);
  if (!show) return;
  show();
  await Promise.all([refreshProjects(), refreshSessions()]).catch(fail);
}

/* loadProjectTab fetches the page and hands back what puts it on the strip,
   or nothing when there is no page to put there. Fetching is split from
   showing so a restored strip can ask for all its tabs at once and still
   rebuild them in the order they were saved in — see restoreOpenTabs. */
async function loadProjectTab(id, silent = false) {
  let project, stats, metrics, sessions, archived, schedules;
  try {
    [project, stats, metrics, sessions, archived, schedules] = await Promise.all([
      api("GET", "/api/projects/" + id),
      api("GET", `/api/projects/${id}/stats`),
      api("GET", `/api/projects/${id}/metrics`),
      projectSessions(id),
      projectArchived(id),
      projectSchedules(id),
    ]);
  } catch (e) {
    if (!silent) fail(e);
    return null;
  }
  return () =>
    setContextTab({
      id: "project:" + id,
      kind: "project",
      label: project.name,
      projectID: id,
      data: { project, stats, metrics, sessions, archived, schedules },
    });
}

/* The project page's own list is what is still open in the project, in the
   order it was dragged into. */
function projectSessions(id) {
  return api("GET", `/api/sessions?window=all&project_id=${id}&include_done=false`);
}

/* Archived ones are the second list under it, newest first, which is the
   order the server sends them in. */
function projectArchived(id) {
  return api("GET", `/api/sessions?window=all&project_id=${id}&only_done=true`);
}

/* The project's scheduled jobs, both states in one request: the sidebar shows
   only the open ones, so the page is the only place an archived job can be
   seen or brought back. One request rather than the two the sessions take,
   because jobs are made by hand and a project has a handful, not the hundreds
   a limit could cut. */
function projectSchedules(id) {
  return api("GET", `/api/schedules?window=all&project_id=${id}&include_done=true`);
}

async function reloadProjectTab(tab) {
  const id = tab.projectID;
  try {
    const [metrics, stats, sessions, archived, schedules] = await Promise.all([
      api("GET", `/api/projects/${id}/metrics`),
      api("GET", `/api/projects/${id}/stats`),
      projectSessions(id),
      projectArchived(id),
      projectSchedules(id),
    ]);
    tab.data.stats = stats;
    tab.data.metrics = metrics;
    tab.data.sessions = sessions;
    tab.data.archived = archived;
    tab.data.schedules = schedules;
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

/* reloadLists re-reads the two listings a turn moves: the sidebar's projects
   and the Sessions list. Debounced for the same reason reloadProjects is —
   the end of a turn arrives twice, and a busy project ends several at once. */
const reloadLists = debounce(() => {
  refreshProjects().catch(() => {});
  refreshSessions().catch(() => {});
}, 150);

export async function addProject(path, name) {
  await api("POST", "/api/projects", { path, name });
  await refreshProjects();
}

export async function removeProject(p) {
  try {
    await api("DELETE", "/api/projects/" + p.id);
    const wasShowing = detachProject(p.id);
    await Promise.all([refreshProjects(), refreshSessions()]);
    if (wasShowing && S.projects.length) await switchProject(S.projects[0].id);
  } catch (e) {
    fail(e);
  }
}

/* detachProject takes a project off the screen, and says whether it was the
   selected one. Nothing in the strip can belong to a project that has left
   the sidebar, deleted or archived. Deselecting comes first, so an emptied
   workspace does not try to reopen its page. Files close unsaved on purpose:
   a deleted project has nowhere to save to, and an archived one is being put
   away. */
function detachProject(id) {
  const wasShowing = S.activeProjectID === id;
  if (wasShowing) S.activeProjectID = null;
  for (const t of [...S.tabs]) {
    if (projectOfTab(t) === id) closeTab(t.id, false, true);
  }
  return wasShowing;
}

/* setProjectArchived puts a project away, or brings it back. Unlike deleting,
   nothing is destroyed: its tasks, its history and its place in the order all
   wait for it. Archiving takes it out of the sidebar, the Go To list and the
   scheduler; Settings is where the archived ones are, and where they come
   back from. */
export async function setProjectArchived(p, archived) {
  try {
    await api("PATCH", "/api/projects/" + p.id, { archived });
    const wasShowing = archived && detachProject(p.id);
    await Promise.all([refreshProjects(), refreshSessions(), loadArchivedProjects()]);
    // Restoring the only project there is leaves nothing selected, and the
    // sidebar, the Tree and Ctrl+N all want one.
    if ((wasShowing || !S.activeProjectID) && S.projects.length) {
      await switchProject(archived ? S.projects[0].id : p.id);
    }
  } catch (e) {
    fail(e);
  }
}

/* Read when Settings opens rather than with every project list: the archived
   ones do not change while it is shut, and refreshProjects runs on every turn
   that starts or ends. */
export async function loadArchivedProjects() {
  S.archivedProjects = await api("GET", "/api/projects?archived=true");
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

/* updateProjectPath repoints a project at a new folder, for when the one on
   disk has moved. The Tree and Changed panes read the old folder until told
   otherwise, so a successful move refreshes them like switching projects
   does. */
export async function updateProjectPath(p, path) {
  // The picker hands back a trailing slash so the next keystroke filters
  // inside the folder; a stored path never has one, and without stripping it
  // here re-picking the folder already in use would look like a change.
  path = path.trim().replace(/(?!^)\/+$/, "");
  if (!path || path === p.path) return;
  try {
    const updated = await api("PATCH", "/api/projects/" + p.id, { path });
    const tab = S.tabs.find((t) => t.kind === "project" && t.projectID === p.id);
    if (tab) tab.data.project = updated;
    await Promise.all([refreshProjects(), refreshInspector()]);
  } catch (e) {
    fail(e);
  }
}

/* setProjectPrompt sets the standing prompt every conversation started in the
   project opens with. Conversations already under way keep the copy they were
   given, so this only reaches the next one — which is why nothing else needs
   refreshing here beyond the lists that draw the project itself. An empty
   prompt is a real value: it is how the injection is turned off. */
export async function setProjectPrompt(p, prompt) {
  prompt = prompt.trim();
  if (prompt === (p.prompt || "")) return;
  try {
    const updated = await api("PATCH", "/api/projects/" + p.id, { prompt });
    const tab = S.tabs.find((t) => t.kind === "project" && t.projectID === p.id);
    if (tab) tab.data.project = updated;
    await refreshProjects();
  } catch (e) {
    fail(e);
  }
}

/* openProjectPrompt is where the transcript's injected-prompt chip goes: the
   project page, open on the prompt that was injected rather than on its task
   list. The pane is set on the tab because one ProjectView serves every
   project and reopening an already-open page does not remount it. */
export async function openProjectPrompt(projectID) {
  await openProject(projectID);
  const tab = S.tabs.find((t) => t.kind === "project" && t.projectID === projectID);
  if (tab) tab.pane = "prompt";
}

/* ------------------------------------------------------------- schedules */

/* A schedule is a prompt plus a clock: every time it comes due it starts a
   new session with the same prompt. It is not a session itself — no
   transcript, no turns — which is why it is its own list and its own kind of
   tab. See specs/028-scheduled-jobs.md. */

/* What the dialog opens on. A quarter of an hour is the common case, and
   whatever was entered last replaces it, so the second schedule is one
   click. */
export const DEFAULT_SCHEDULE = { every: "interval", hours: 0, minutes: 15, at: "09:00", remaining: -1 };

/* The forms a schedule can take. The dialog asks for one and the schedule
   view offers the same four, since a clock can be changed after the fact. */
export const EVERY = [
  { label: "Hours and minutes", value: "interval" },
  { label: "Day", value: "day" },
  { label: "Week", value: "week" },
  { label: "Month", value: "month" },
];

export function loadScheduleDefaults() {
  try {
    const saved = JSON.parse(localStorage.getItem(SCHEDULE_KEY));
    return saved ? { ...DEFAULT_SCHEDULE, ...saved } : { ...DEFAULT_SCHEDULE };
  } catch {
    return { ...DEFAULT_SCHEDULE };
  }
}

function rememberScheduleDefaults(form) {
  persist(SCHEDULE_KEY, JSON.stringify(form));
}

export async function refreshSchedules() {
  const params = new URLSearchParams({ window: S.window });
  if (S.query.trim()) params.set("q", S.query.trim());
  S.schedules = await api("GET", "/api/schedules?" + params);
}

/* createSchedule turns the prompt in the box into a repeating one. Everything
   else is already on screen — the project, the model, the effort — so the
   dialog only asked for the recurrence and the number of runs. The schedule
   opens in front, since a clock nobody can see is worth nothing. */
export async function createSchedule(form) {
  const tab = S.owner;
  const prompt = (tab?.kind === "session" ? tab.draft : "").trim();
  if (!tab || tab.kind !== "session" || !prompt) return;
  const s = tab.detail.session;
  // Cleared before the request, as sending and queueing both do.
  tab.draft = "";
  try {
    const created = await api("POST", "/api/schedules", {
      project_id: s.project_id,
      prompt,
      provider: s.provider,
      account_id: s.account_id,
      model: s.model,
      effort: s.effort || "",
      permission: s.permission,
      every: form.every,
      interval_minutes: form.every === "interval" ? intervalMinutes(form) : 0,
      at_minute: form.every === "interval" ? 0 : atMinute(form.at),
      remaining: form.remaining,
    });
    rememberScheduleDefaults(form);
    await Promise.all([refreshProjects(), refreshSchedules()]).catch(() => {});
    openSchedule(created.schedule.id);
  } catch (e) {
    if (!tab.draft) tab.draft = prompt;
    fail(e);
  }
}

/* The dialog asks for hours and minutes; the server stores the one number. */
export function intervalMinutes(form) {
  return Math.max(1, Number(form.hours || 0) * 60 + Number(form.minutes || 0));
}

/* "09:30" as minutes past midnight, local — the clock on the wall is what
   "every day at 9" is read off. */
export function atMinute(at) {
  const [h, m] = String(at || "").split(":");
  return Math.min(24 * 60 - 1, Math.max(0, (Number(h) || 0) * 60 + (Number(m) || 0)));
}

/* "09:30" from minutes past local midnight, the way the field wants it. */
function clock(atMinute) {
  return String(Math.floor(atMinute / 60)).padStart(2, "0") +
    ":" + String(atMinute % 60).padStart(2, "0");
}

export function scheduleLabel(schedule) {
  if (schedule.every === "interval") {
    const h = Math.floor(schedule.interval_minutes / 60);
    const m = schedule.interval_minutes % 60;
    return "Every " + [h ? h + "h" : "", m ? m + "m" : ""].filter(Boolean).join(" ");
  }
  return `Every ${schedule.every} at ${clock(schedule.at_minute)}`;
}

/* A schedule as the dialog's fields, for the view that edits them. Only the
   half of the clock its form uses is stored — the other is 0 — so switching
   form offers what the dialog would have, not midnight or no interval. */
export function scheduleForm(schedule) {
  const interval = schedule.every === "interval";
  return {
    every: schedule.every,
    hours: interval ? Math.floor(schedule.interval_minutes / 60) : DEFAULT_SCHEDULE.hours,
    minutes: interval ? schedule.interval_minutes % 60 : DEFAULT_SCHEDULE.minutes,
    at: interval ? DEFAULT_SCHEDULE.at : clock(schedule.at_minute),
  };
}

/* openSchedule puts a schedule in the project's context slot — the same one a
   project page and a conversation share, so a job read on the way past does
   not leave a tab behind. It shows its prompt, its clock, and every run it has
   spawned or skipped. */
export async function openSchedule(id, silent = false) {
  const tabID = "schedule:" + id;
  const open = S.tabs.find((t) => t.id === tabID);
  if (open) {
    await reloadScheduleTab(open);
    return selectTab(tabID);
  }
  (await loadScheduleTab(id, silent))?.();
}

/* loadScheduleTab reads the job and hands back what puts it on the strip, the
   same way loadProjectTab does. */
async function loadScheduleTab(id, silent = false) {
  let detail;
  try {
    detail = await api("GET", "/api/schedules/" + id);
  } catch (e) {
    if (!silent) fail(e);
    return null;
  }
  return () =>
    setContextTab({
      id: "schedule:" + id,
      kind: "schedule",
      label: detail.schedule.title,
      scheduleID: id,
      projectID: detail.schedule.project_id,
      data: detail,
    });
}

async function reloadScheduleTab(tab) {
  try {
    tab.data = await api("GET", "/api/schedules/" + tab.scheduleID);
    tab.label = tab.data.schedule.title;
  } catch {
    /* a refresh that fails is not worth interrupting anyone over */
  }
}

/* Debounced for the same reason project tabs are: a fire publishes on spawn
   and again when the run ends. */
const reloadSchedules = debounce(() => {
  for (const t of S.tabs) if (t.kind === "schedule") reloadScheduleTab(t);
  refreshProjects().catch(() => {});
  refreshSchedules().catch(() => {});
  // The project page lists the project's jobs, archived ones included, so a
  // schedule that moved moves there too.
  reloadProjects();
  // A finished run is archived rather than deleted, so the Sessions list is
  // where it goes. Nothing else re-reads it: the run had no tab of its own,
  // so its done event reached no session watcher here.
  refreshSessions().catch(() => {});
}, 150);

async function patchSchedule(schedule, body) {
  try {
    const updated = await api("PATCH", "/api/schedules/" + schedule.id, body);
    const tab = S.tabs.find((t) => t.kind === "schedule" && t.scheduleID === schedule.id);
    if (tab) {
      tab.data.schedule = updated;
      tab.label = updated.title;
    }
    await Promise.all([refreshProjects(), refreshSchedules()]).catch(() => {});
    // The sidebar holds the open jobs; the project page holds those and the
    // archived ones, which is where archiving and restoring are done.
    reloadProjects();
    return updated;
  } catch (e) {
    fail(e);
  }
}

/* Paused, nothing is scheduled at all until it is resumed, and resuming
   restarts the cadence from now rather than firing what the pause held back —
   the server books the next run. */
export function setSchedulePaused(schedule, paused) {
  return patchSchedule(schedule, { paused });
}

/* runScheduleNow forces one run right now, from the schedule's own Run menu
   (specs/028-scheduled-jobs.md). It runs even if a previous fire is still
   going and never spends the counter — both the server's doing, not this
   call's — so there is nothing to reconcile here beyond the request itself:
   the schedule_changed event that follows redraws the run it started. */
export function runScheduleNow(schedule, enqueue) {
  return api("POST", `/api/schedules/${schedule.id}/run`, { enqueue }).catch(fail);
}

/* How many runs are left is not a decision made once: -1 is forever, and the
   view can move it up or down whenever. */
export function setScheduleRemaining(schedule, remaining) {
  const n = Math.trunc(Number(remaining));
  if (!Number.isFinite(n) || n === schedule.remaining) return;
  return patchSchedule(schedule, { remaining: n < -1 ? -1 : n });
}

/* Nor is the prompt: a schedule worth keeping is not worth remaking over a
   typo. Runs already spawned are ordinary sessions and keep the text they
   were started with; only future ones follow the new prompt. */
export function setSchedulePrompt(schedule, prompt) {
  prompt = prompt.trim();
  if (!prompt || prompt === schedule.prompt) return;
  return patchSchedule(schedule, { prompt });
}

/* Nor the model. Provider, model and effort travel together, since an effort
   belongs to a model and a model to a provider. */
export function setScheduleModel(schedule, provider, model, effort, accountID) {
  const account = accountID === undefined ? schedule.account_id || 0 : accountID;
  if (
    provider === schedule.provider &&
    account === (schedule.account_id || 0) &&
    model === schedule.model &&
    effort === (schedule.effort || "")
  ) {
    return;
  }
  return patchSchedule(schedule, { provider, account_id: account, model, effort });
}

/* Neither is the clock: a schedule can be moved from every 15 minutes to
   every morning without being made again. Changing the form re-anchors on the
   server, so one switched to weekly today runs on today's weekday. */
export function setScheduleFrequency(schedule, form) {
  const body = {
    every: form.every,
    interval_minutes: form.every === "interval" ? intervalMinutes(form) : 0,
    at_minute: form.every === "interval" ? 0 : atMinute(form.at),
  };
  if (
    body.every === schedule.every &&
    body.interval_minutes === schedule.interval_minutes &&
    body.at_minute === schedule.at_minute
  ) {
    return;
  }
  return patchSchedule(schedule, body);
}

export function renameSchedule(schedule, title) {
  title = title.trim();
  if (!title || title === schedule.title) return;
  return patchSchedule(schedule, { title });
}

/* Only a paused schedule can be archived: one that was hidden while it kept
   starting sessions is the one state nothing here would explain. Archiving
   pauses on the server too, so a restored schedule comes back paused. */
export async function setScheduleArchived(schedule, archived) {
  const updated = await patchSchedule(schedule, { done: archived });
  if (archived && updated) closeTab("schedule:" + schedule.id, false);
  return updated;
}

export async function removeSchedule(schedule) {
  try {
    await api("DELETE", "/api/schedules/" + schedule.id);
    closeTab("schedule:" + schedule.id, false);
    await Promise.all([refreshProjects(), refreshSchedules()]).catch(() => {});
    reloadProjects();
  } catch (e) {
    fail(e);
  }
}

/* reorderSchedules records the sidebar order, as the session lists do. */
export async function reorderSchedules(projectID, ids) {
  try {
    await api("POST", `/api/projects/${projectID}/schedules/order`, { ids });
    await refreshProjects();
  } catch (e) {
    fail(e);
    refreshProjects().catch(() => {});
  }
}

/* -------------------------------------------------------------- sessions */

export async function refreshSessions() {
  const params = new URLSearchParams({ window: S.window });
  if (S.query.trim()) params.set("q", S.query.trim());
  S.sessions = await api("GET", "/api/sessions?" + params);
  syncSessionTabs();
}

/* setWindow moves how far back the list reaches and remembers it, so the
   next launch opens on the same reach rather than back on the default. */
export function setWindow(value) {
  if (value === S.window || !TASK_WINDOWS.some((w) => w.value === value)) return;
  S.window = value;
  persist(WINDOW_KEY, value);
  refreshSessions().catch(() => {});
  refreshSchedules().catch(() => {});
}

/* setTaskPageSize changes how many task rows a project page draws and
   remembers it. It is a slice of a list already loaded, so nothing is
   fetched. */
export function setTaskPageSize(size) {
  if (size === S.taskPageSize || !TASK_PAGE_SIZES.includes(size)) return;
  S.taskPageSize = size;
  persist(TASK_PAGE_KEY, size);
}

function loadTaskPageSize() {
  try {
    const saved = Number(localStorage.getItem(TASK_PAGE_KEY));
    if (TASK_PAGE_SIZES.includes(saved)) S.taskPageSize = saved;
  } catch {
    /* keep the default */
  }
}

/* A window the app no longer offers is dropped rather than sent on, since an
   unknown one is a 400 and an empty Sessions tab. */
function loadWindow() {
  try {
    const saved = localStorage.getItem(WINDOW_KEY);
    if (TASK_WINDOWS.some((w) => w.value === saved)) S.window = saved;
  } catch {
    /* keep the default */
  }
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
  return session.title?.trim() || "New task";
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

/* blankSession is an untouched conversation of the project: nothing said,
   nothing queued, nothing typed. A single letter in the box makes one worth
   keeping. The one in front is preferred, then any other tab of the project,
   so which conversation happens to be selected does not decide this. */
function blankSession(projectID) {
  const blank = (t) =>
    t.kind === "session" &&
    projectOfTab(t) === projectID &&
    !t.detail.session.title &&
    !t.detail.messages.length &&
    !t.detail.queued.length &&
    !(t.detail.pendingQueued || []).length &&
    !t.draft.trim();
  const owner = S.owner;
  if (owner && blank(owner)) return owner;
  return S.tabs.find(blank) || null;
}

/* blankSessionID is the same search over the project's conversations with no
   tab open. The first prompt names a session, sent or queued, so an untitled
   one is still empty and has no draft to lose: reopening it is what starting
   a session would produce anyway. The newest comes first, and a list that
   fails to load is not worth reporting — a new session is still correct. */
async function blankSessionID(projectID) {
  const open = new Set(S.tabs.filter((t) => t.kind === "session").map((t) => t.sessionID));
  try {
    const free = (await projectSessions(projectID))
      .filter((s) => !s.title && !s.queue_count && !open.has(s.id) && !S.drafts[s.id]);
    return free.reduce((best, s) => (!best || s.created_at > best.created_at ? s : best), null)?.id || "";
  } catch {
    return "";
  }
}

/* A project page is the leading context slot, so starting a session replaces
   it without disturbing independent file tabs.

   An untouched conversation is already what this would produce, so every way
   in — the chord, the strip's +, the page's button — stops at one of those
   rather than leaving a trail of empty tabs, open in a tab or not. It comes
   to the front with the cursor in the box, which is what was being asked
   for. */
export async function startTask(project) {
  /* The prompt bar's focus watcher only sees a request made after it mounts,
     and coming from a project page mounts it with this very switch, so the
     cursor is asked for once the switch has been drawn. */
  const reuse = async (tabID) => {
    selectTab(tabID);
    await nextTick();
    focusPrompt();
  };
  const blank = blankSession(project.id);
  if (blank) return reuse(blank.id);
  const reusable = await blankSessionID(project.id);
  if (reusable) {
    await openTask(reusable);
    return reuse("session:" + reusable);
  }
  const cfg = sessionDefaults();
  if (!cfg) return fail(new Error("No agent CLI is available. Open Settings to see why."));
  try {
    const sess = await api("POST", "/api/sessions", { project_id: project.id, ...cfg });
    await Promise.all([refreshProjects(), refreshSessions()]);
    await openTask(sess.id);
    focusPrompt();
    reloadProjects();
  } catch (e) {
    fail(e);
  }
}

/* startCurrentTask is the keyboard and command-palette version of the
   project page's New task button. A session, project, or file tab all
   identify their owning project. */
export function startCurrentTask() {
  const id = currentProjectID();
  const project = S.project?.project?.id === id
    ? S.project.project
    : S.projects.find((p) => p.id === id);
  if (!project) return fail(new Error("Open a project or session first."));
  return startTask(project);
}

export async function openTask(id, silent = false) {
  const open = S.tabs.find((t) => t.id === "session:" + id);
  if (open) return setContextTab(open);
  const show = await loadTaskTab(id, silent);
  if (!show) return;
  show();
  await Promise.all([refreshProjects(), refreshSessions()]).catch(fail);
}

/* loadTaskTab reads the conversation and hands back what puts it on the
   strip, the same way loadProjectTab does. */
async function loadTaskTab(id, silent = false) {
  let detail;
  try {
    detail = await api("GET", "/api/sessions/" + id);
  } catch (e) {
    if (!silent) fail(e);
    return null;
  }
  return () => {
    rememberUsed(detail.session);
    setContextTab({
      id: "session:" + id,
      kind: "session",
      label: tabLabel(detail.session),
      sessionID: id,
      detail,
      pendingQueued: [],
      // Unsent text belongs to the conversation, not to the prompt bar, so it
      // survives every tab switch and only closing throws it away.
      draft: S.drafts[id] || "",
    });
  };
}

/* accountID is which subscription of that provider answers the next prompt.
   Left out, the server keeps the one the task has — or moves to the new
   provider's default when the provider itself changed. */
export async function setModel(provider, model, effort, accountID) {
  const tab = S.owner;
  if (tab?.kind !== "session") return;
  try {
    tab.detail.session = await api("PATCH", "/api/sessions/" + tab.sessionID, {
      provider,
      model,
      effort,
      ...(accountID === undefined ? {} : { account_id: accountID }),
    });
    rememberUsed(tab.detail.session);
  } catch (e) {
    fail(e);
  }
}

/* renameTask retitles a session. The same session shows in the sidebar, in
   its project's view and on the tab strip, so every list is re-read; the tab
   label is set here too, because a session outside the sidebar's window is
   not in the list that would otherwise carry the new name back. */
export async function renameTask(session, title) {
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

/* A default session with no history, queued work, or local draft is just an
   abandoned placeholder. Drop it rather than making it an archived row the
   user has to find later. Checking the detail also keeps a titled or used
   session safe when archive is clicked from a compact project-list row. */
async function shouldDeleteBlankSession(session, tab) {
  if (tab?.detail?.pendingQueued?.length) return false;
  if (session.title || (tab?.draft || S.drafts[session.id] || "").trim()) return false;
  if (tab && unsavedUnder(tab.id).length) return false;
  const detail = tab?.detail || await api("GET", "/api/sessions/" + session.id);
  return !detail.session.title &&
    !detail.messages.length &&
    !detail.turns.length &&
    !detail.queued.length &&
    !detail.running;
}
/* setTaskArchived removes a session from project views and closes its tab.
   Archived sessions remain restorable; an untouched untitled placeholder is
   deleted instead. */
export async function setTaskArchived(session, archived) {
  try {
    const tab = S.tabs.find((t) => t.kind === "session" && t.sessionID === session.id);
    const deleted = archived && await shouldDeleteBlankSession(session, tab);
    const updated = deleted
      ? await api("DELETE", "/api/sessions/" + session.id)
      : await api("PATCH", "/api/sessions/" + session.id, { done: archived });
    const dependents = tab ? S.tabs.filter((t) => t.owner === tab.id) : [];
    if (archived) delete S.drafts[session.id];
    if (tab && !deleted) tab.detail.session = updated;
    await Promise.all([refreshProjects(), refreshSessions()]);
    reloadProjects();
    if (archived && tab) {
      await closeTab(tab.id, false);
      if (!deleted && !S.tabs.some((open) => open.id === tab.id)) {
        rememberClosedTab(projectOfTab(tab), tab, dependents, true);
      }
    }
    return updated;
  } catch (e) {
    fail(e);
  }
}

export function isArchived(session) {
  return !!session?.done_at;
}

/* removeTask permanently deletes a task and its history, once the Tasks tab's
   own confirmation has been answered. Unlike archiving there is no restore,
   so its tab and any files opened from it close unsaved rather than asking a
   second time — the confirmation already given is the one gate, the same
   rule a deleted project's tabs follow (detachProject). */
export async function removeTask(session) {
  try {
    await api("DELETE", "/api/sessions/" + session.id);
    delete S.drafts[session.id];
    const tab = S.tabs.find((t) => t.kind === "session" && t.sessionID === session.id);
    if (tab) closeTab(tab.id, false, true);
    await Promise.all([refreshProjects(), refreshSessions()]);
    reloadProjects();
  } catch (e) {
    fail(e);
  }
}

/* reorderTasks records the order a project view was dragged into. The list
   is put in that order first, so the drop lands where it was let go rather
   than a request later; if the server refuses, the project is re-read, which
   is the only reliable way back — the row has been moved under the cursor
   since the drag began and the order it started in is gone. */
export async function reorderTasks(tab, ids) {
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
export async function reorderSidebarTasks(project, ids) {
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

/* A file tab shows the file, its diff, or — for markdown, HTML, SVG and
   images — what it renders as, and opens in whichever was read last: looking
   at one diff usually means the next changed file wants a diff too. Each view
   is fetched only when it is first asked for. An image has no text at all, so
   it carries neither the editor nor the text diff: its Diff is the two
   pictures, side by side.

   Editing lives on the tab, in `edited`: null until a key is pressed, so a
   file that has only been read carries nothing extra and switching tabs and
   coming back finds the edits where they were left. */
const FILE_MODES = ["edit", "diff", "preview"];
const PREVIEWABLE = /\.(md|markdown|html?|svg)$/i;

/* The images the server will hand over as bytes — the same list, because a
   preview can only ask for what the raw endpoint agrees to send. An SVG is
   not among them: it is text, so it keeps its editor and its preview renders
   the tab rather than a URL. */
const IMAGE = /\.(png|jpe?g|gif|webp|avif|bmp|ico|apng)$/i;

export function canPreview(path) {
  const name = String(path || "");
  return PREVIEWABLE.test(name) || IMAGE.test(name);
}

/* An image has no text: nothing to edit, and a diff that is two pictures
   rather than two columns of lines. */
export function isImage(path) {
  return IMAGE.test(String(path || ""));
}

/* rawURL is where a file's bytes are, for the views that render it instead of
   reading it. rev names a revision — the baseline side of a diff — in place
   of the working tree. */
export function rawURL(tab, rev = "") {
  const at = rev ? `&rev=${encodeURIComponent(rev)}` : "";
  return `/api/projects/${projectOfTab(tab)}/raw?path=${encodeURIComponent(tab.path)}${at}`;
}

export function isDirty(tab) {
  return tab?.kind === "file" && tab.edited !== null && tab.edited !== tab.content;
}

/* One click opens a file in the project's temporary tab, which the next file
   clicked takes over. Reading through a tree then leaves one tab behind
   rather than twenty. A double click pins the tab instead, and so does typing
   in it; a pinned tab is only closed by hand. */
export function openFile(path, pin = false) {
  return openFileIn(currentProjectID(), S.owner?.id || "", path, { pin });
}

/* openFileRef answers a file mentioned in a transcript: same temporary tab a
   click in the Tree uses, scrolled to the line the reference named.

   The path is the agent's, so it may be absolute. The file API only takes
   paths inside the project, so the project's own folder is trimmed off the
   front; anything else is passed as it was and refused with a message that
   says so, which beats guessing at what an outside path meant. */
export function openFileRef(path, line = 0) {
  const id = currentProjectID();
  const root = S.projects.find((p) => p.id === id)?.path || "";
  const rel = root && path.startsWith(root + "/") ? path.slice(root.length + 1) : path;
  return openFileIn(id, S.owner?.id || "", rel, { line });
}

/* At most one temporary tab per project, because the strip is per project:
   the tab a click takes over is one the user can see. */
function tempFileIn(projectID) {
  return S.tabs.find((t) => t.kind === "file" && t.temp && projectOfTab(t) === projectID);
}

/* selectOpenFile brings a file already open to the front, pinning it when
   that is what the click asked for — never the other way round, so clicking
   a kept file's row does not make it temporary again. */
function selectOpenFile(tabID, pin, line = 0) {
  const open = S.tabs.find((t) => t.id === tabID);
  if (!open) return false;
  if (pin) open.temp = false;
  if (line) gotoLine(open, line);
  selectTab(tabID);
  return true;
}

/* goto is a request rather than state: the editor scrolls to the line and
   clears it, so the same reference clicked twice scrolls twice. A line only
   means anything in the editor, so a tab left on Diff or Preview is switched
   — without moving S.fileMode, which is the choice the user made and not one
   a link should make for them. A commit's file has only its diff. */
async function gotoLine(tab, line) {
  if (tab.commit || isImage(tab.path)) return;
  if (tab.mode !== "edit") {
    tab.mode = "edit";
    try {
      await loadFileTab(tab);
    } catch (e) {
      return fail(e);
    }
  }
  tab.goto = line;
}

/* openingMode is the view a file opens in: whichever was read last, mostly.
   A file opened at a line opens in the editor, the one view a line can be
   shown in. Preview is remembered like the other two, but a file that renders
   as nothing opens in the editor instead of on a blank pane — and an image,
   which has no editor at all, opens on its picture. */
function openingMode(path, line) {
  if (isImage(path)) return S.fileMode === "diff" ? "diff" : "preview";
  if (line || (S.fileMode === "preview" && !canPreview(path))) return "edit";
  return S.fileMode;
}

/* openFileIn also rebuilds a saved tab, whose owner may be gone. A file
   belongs to the view it was opened from, but it belongs to its project even
   when nothing else in that project is open.

   commit names a revision when the tab is a file as one commit changed it.
   Such a tab only ever shows a diff — the working copy is a different thing
   from what was committed, and the commit itself cannot be edited — so it
   opens read-only in Diff and stays there. Its id carries the revision, so
   the same path can be open once per commit and once for the working tree. */
async function openFileIn(projectID, ownerID, path, opts = {}) {
  (await loadFileTabIn(projectID, ownerID, path, opts))?.();
}

/* loadFileTabIn reads the file and hands back what puts its tab on the strip,
   the same way loadProjectTab does — nothing, when the file cannot be read or
   is already open and was only brought to the front. */
async function loadFileTabIn(projectID, ownerID, path, opts = {}) {
  const { silent = false, commit = "", pin = false, line = 0 } = opts;
  if (!projectID) return null;
  const tabID = commit ? `file:${projectID}:${commit}:${path}` : `file:${projectID}:${path}`;
  if (selectOpenFile(tabID, pin, line)) return null;
  const tab = {
    id: tabID,
    kind: "file",
    label: path.split("/").pop(),
    owner: ownerID,
    projectID,
    path,
    commit,
    // temp is the tab a click opens: the next file clicked takes it over,
    // until a double click or a keystroke pins it.
    temp: !pin,
    mode: commit ? "diff" : openingMode(path, line),
    // The line to scroll to once the editor has the text, cleared by it.
    goto: line,
    content: null,
    diff: null,
    edited: null,
    // What was not read whole must never be written back over its source, and
    // an image is never read as text at all.
    readOnly: !!commit || isImage(path),
    saving: false,
  };
  try {
    await loadFileTab(tab);
  } catch (e) {
    if (!silent) fail(e);
    return null;
  }
  return () => {
    // A double click is two clicks, so the second call can arrive while this
    // one is still fetching: whichever tab is open by now is the tab.
    if (selectOpenFile(tabID, pin, line)) return;
    // The owner is checked here rather than before the read: a restored strip
    // rebuilds in the saved order, so the view a file was opened from may
    // only have landed while the file itself was being fetched.
    if (!S.tabs.some((open) => open.id === tab.owner)) tab.owner = "";
    const previous = tab.temp ? tempFileIn(projectID) : null;
    if (previous) swapTab(previous.id, tab);
    else addTab(tab);
  };
}

/* loadFileTab fetches the view the tab is showing, once. The tab may not be
   open yet — a file that cannot be read opens no tab at all. */
async function loadFileTab(tab) {
  const id = projectOfTab(tab);
  if (!id) return;
  const query = `path=${encodeURIComponent(tab.path)}`;
  if (tab.commit) {
    if (tab.diff !== null) return;
    const at = `hash=${encodeURIComponent(tab.commit)}`;
    tab.diff = (await api("GET", `/api/projects/${id}/commit/diff?${at}&${query}`)).diff || "";
    return;
  }
  if (tab.mode === "diff") {
    if (tab.diff !== null) return;
    tab.diff = (await api("GET", `/api/projects/${id}/diff?${query}`)).diff || "";
    return;
  }
  if (tab.content !== null) return;
  if (isImage(tab.path)) {
    // The bytes go straight to the <img> that draws them. Reading a megabyte
    // of them into a string first would move it twice and display it never.
    tab.content = "";
    return;
  }
  const f = await api("GET", `/api/projects/${id}/file?${query}`);
  tab.readOnly = !!(f.binary || f.partial);
  tab.content = f.binary ? "(binary file)" : f.content + (f.partial ? "\n\n… truncated" : "");
}

/* setFileMode switches one tab and remembers the choice for the next file —
   but only once the switch has worked, so a view that cannot be fetched puts
   the tab back rather than leaving it on an empty pane. */
export async function setFileMode(tab, mode) {
  if (tab?.kind !== "file" || tab.commit || tab.mode === mode || !FILE_MODES.includes(mode)) return;
  if (mode === "edit" && isImage(tab.path)) return;
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
  // A file typed into is worth keeping, so the next file clicked opens its
  // own tab rather than taking this one's place. Taking the edit back does
  // not hand the tab over again.
  tab.temp = false;
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
function unsavedUnder(id, cascade = true) {
  return S.tabs.filter((t) => (t.id === id || (cascade && t.owner === id)) && isDirty(t));
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
  await closeTab(pending.id, pending.remember, false, pending.cascade);
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
  if (tab.kind === "schedule") {
    return { id: tab.id, kind: tab.kind, scheduleID: tab.scheduleID, projectID: tab.projectID };
  }
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
      commit: tab.commit || "",
      temp: !!tab.temp,
    };
  }
  return null;
}

/* A rebuilt file tab opens as it was saved, silently. Tabs saved before
   there was a temporary one have no flag, and a tab the user chose to keep is
   the safer reading of those. */
function restoreOpts(tab) {
  return { silent: true, commit: tab.commit || "", pin: !tab.temp };
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
        // The drafts of conversations with no tab of their own; an open tab
        // carries its own.
        drafts: S.drafts,
        activeTab: S.activeTab,
        activeProjectID: S.activeProjectID,
      }),
    );
  } catch {
    /* private mode or a full quota just means tabs open fresh next time */
  }
}, 300);

/* Restoring asks for every saved tab at once and rebuilds the strip from the
   answers, in the order the tabs were saved in. Their data is fetched anew,
   so missing projects and sessions are skipped; a file is restored against
   its own project, and keeps the view it was opened from only if that one
   came back too.

   The order the strip is rebuilt in matters — a project keeps one context
   tab, the last one wins, and a file follows its view — but nothing about the
   order it is *fetched* in does. Asking for a tab only once the one before it
   has answered was most of what a launch cost: a strip of eight was eight
   round trips deep before the first thing was on screen. */
async function restoreOpenTabs() {
  let saved;
  try {
    saved = JSON.parse(localStorage.getItem(OPEN_TABS_KEY));
  } catch {
    return;
  }
  if (!Array.isArray(saved?.tabs)) return;
  if (saved.drafts && typeof saved.drafts === "object") Object.assign(S.drafts, saved.drafts);

  restoringTabs = true;
  try {
    const loaded = await Promise.all(saved.tabs.map(loadSavedTab));
    for (const show of loaded) show?.();
    if (S.projects.some((p) => p.id === saved.activeProjectID)) {
      S.activeProjectID = saved.activeProjectID;
    }
    if (S.tabs.some((tab) => tab.id === saved.activeTab)) selectTab(saved.activeTab);
  } finally {
    restoringTabs = false;
    saveOpenTabs();
    // The strip is whole by now, so the stream is opened once for it rather
    // than reopened between every two tabs.
    resubscribe();
  }
}

/* loadSavedTab fetches one saved tab and hands back what puts it on the
   strip, or nothing when it cannot be rebuilt. */
function loadSavedTab(tab) {
  if (tab?.kind === "project" && tab.projectID) return loadProjectTab(tab.projectID, true);
  if (tab?.kind === "schedule" && tab.scheduleID) return loadScheduleTab(tab.scheduleID, true);
  if (tab?.kind === "file" && typeof tab.path === "string") {
    return loadFileTabIn(tab.projectID, tab.owner, tab.path, restoreOpts(tab));
  }
  if (tab?.kind === "session" && tab.sessionID) {
    return loadTaskTab(tab.sessionID, true).then((show) => {
      if (!show) return null;
      return () => {
        show();
        const open = S.tabs.find((candidate) => candidate.id === tab.id);
        if (open) open.draft = typeof tab.draft === "string" ? tab.draft : "";
      };
    });
  }
  return Promise.resolve(null);
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
  await refreshRepositories();
  await Promise.all([refreshChanged(), refreshLog(), refreshTree()]);
}

async function refreshRepositories() {
  const id = currentProjectID();
  if (!id) return;
  try {
    const repos = await api("GET", `/api/projects/${id}/repositories`);
    if (currentProjectID() !== id) return;
    S.repositories = repos;
    if (!repos.includes(S.repository)) {
      S.repository = repos[0] || "";
      S.logOpen = "";
      S.logFiles = {};
    }
  } catch {
    if (currentProjectID() === id) {
      S.repositories = [];
      S.repository = "";
    }
  }
}

export function selectRepository(repo) {
  if (!S.repositories.includes(repo) || repo === S.repository) return;
  S.repository = repo;
  S.changed = [];
  S.log = EMPTY_LOG();
  S.logOpen = "";
  S.logFiles = {};
  Promise.all([refreshChanged(), refreshLog()]).catch(fail);
}

export async function refreshChanged() {
  const id = currentProjectID();
  const repo = S.repository;
  if (!id) return (S.changed = []);
  try {
    const changed = await api("GET", `/api/projects/${id}/changes?repo=${encodeURIComponent(repo)}`);
    // The watcher can fire this often, so a slow answer for the project we
    // just left must not replace the list of the one now in front.
    if (currentProjectID() === id && S.repository === repo) S.changed = changed;
  } catch {
    if (currentProjectID() === id && S.repository === repo) S.changed = [];
  }
}

/* refreshLog reads the history of the branch the project is on. A folder that
   is not a repository answers with an empty log, so there is nothing to
   special-case here. */
export async function refreshLog() {
  const id = currentProjectID();
  const repo = S.repository;
  if (!id) return (S.log = EMPTY_LOG());
  try {
    const log = await api("GET", `/api/projects/${id}/log?repo=${encodeURIComponent(repo)}`);
    // A slow request for the project we just left must not replace the log of
    // the one now in front.
    if (currentProjectID() === id && S.repository === repo) S.log = log;
  } catch {
    if (currentProjectID() === id && S.repository === repo) S.log = EMPTY_LOG();
  }
}

const EMPTY_LOG = () => ({ branch: "", head: "", commits: [] });

/* toggleCommit expands one row into the files it touched. The list is fetched
   once per commit: history does not change under us, so a row reopened later
   costs nothing. */
export async function toggleCommit(hash) {
  if (S.logOpen === hash) {
    S.logOpen = "";
    return;
  }
  S.logOpen = hash;
  const repo = S.repository;
  if (S.logFiles[hash]) return;
  const id = currentProjectID();
  if (!id) return;
  try {
    const detail = await api("GET", `/api/projects/${id}/commit?hash=${hash}&repo=${encodeURIComponent(repo)}`);
    if (currentProjectID() === id && S.repository === repo) S.logFiles[hash] = detail.files;
  } catch (e) {
    if (currentProjectID() !== id || S.repository !== repo) return;
    if (S.logOpen === hash) S.logOpen = "";
    fail(e);
  }
}

/* openCommitFile shows one file as that commit changed it. It opens the same
   kind of tab a working-tree diff does, fixed to the Diff view: there is
   nothing to edit in a commit that has already been made. */
export function openCommitFile(hash, path, pin = false) {
  return openFileIn(currentProjectID(), S.owner?.id || "", path, { commit: hash, pin });
}

async function refreshTree() {
  const id = currentProjectID();
  if (!id) return clearTree();
  try {
    const { files = [], ignored = [], dirs = [] } =
      (await api("GET", `/api/projects/${id}/tree`)) || {};
    // A slow request for the tab we just left must not replace the active
    // project's sidebar Tree.
    if (currentProjectID() !== id) return;
    // The two lists arrive apart so the ignored half can be capped on its own;
    // the pane wants one listing, and `tree` being empty is what "No files."
    // is read from.
    S.tree = ignored.length ? files.concat(ignored) : files;
    S.treeIgnored = ignored;
    S.treeDirs = dirs;
  } catch {
    if (currentProjectID() === id) clearTree();
  }
}

function clearTree() {
  S.tree = [];
  S.treeIgnored = [];
  S.treeDirs = [];
}

/* --------------------------------------------------------- editing files */

/* The Tree's own file operations. Each asks the server and then re-reads the
   listing rather than patching it: the watcher would refresh a beat later
   anyway, and doing it here is what puts the row on screen with the
   keystroke. Changed is re-read with it, because a file made or deleted is a
   change to the working tree like any other.

   Open tabs are kept in step by hand. A renamed file's tab follows it — a
   file tab's id carries its path, so the id moves too — and a deleted file's
   tab is closed outright: force, because there is nothing left to save, and
   without remembering it, because Ctrl+Shift+T could only fail on a file that
   is gone. Tabs showing a file at a commit are left alone; history did not
   move. */

export async function createEntry(parent, name, dir = false) {
  const id = currentProjectID();
  const path = joinPath(parent, name);
  if (!id || !path) return "";
  const made = await api("POST", `/api/projects/${id}/entry`, { path, dir });
  await refreshFiles();
  return made.path;
}

export async function renameEntry(path, name) {
  const id = currentProjectID();
  const to = joinPath(parentPath(path), name);
  if (!id || !to || to === path) return "";
  const moved = await api("POST", `/api/projects/${id}/entry/rename`, { path, to });
  for (const tab of fileTabsUnder(id, path)) {
    const was = tab.id;
    tab.path = moved.path + tab.path.slice(path.length);
    tab.label = tab.path.split("/").pop();
    tab.id = `file:${id}:${tab.path}`;
    if (S.activeTab === was) S.activeTab = tab.id;
    // The text is the same bytes it was, but a diff is against a path: the
    // move itself is part of the new one, so a tab on Diff re-reads it.
    if (tab.mode === "diff") {
      tab.diff = null;
      loadFileTab(tab).catch(fail);
    }
  }
  await refreshFiles();
  return moved.path;
}

export async function deleteEntry(path) {
  const id = currentProjectID();
  if (!id || !path) return;
  await api("DELETE", `/api/projects/${id}/entry?path=${encodeURIComponent(path)}`);
  for (const tab of fileTabsUnder(id, path)) await closeTab(tab.id, false, true, false);
  await refreshFiles();
}

function refreshFiles() {
  return Promise.all([refreshTree(), refreshChanged()]);
}

/* A folder takes everything under it with it, so both the rename and the
   delete ask for the whole subtree rather than the one path. */
function fileTabsUnder(projectID, path) {
  return S.tabs.filter(
    (t) =>
      t.kind === "file" &&
      !t.commit &&
      t.projectID === projectID &&
      (t.path === path || t.path.startsWith(path + "/")),
  );
}

const parentPath = (path) => (path.includes("/") ? path.slice(0, path.lastIndexOf("/")) : "");

/* A name may be a path of its own — "sub/thing.go" — which is how a file is
   made in a folder that is not there yet, and how a rename moves one. The
   server cleans and bounds it. */
const joinPath = (parent, name) => [parent, name.trim()].filter(Boolean).join("/");

/* openInSystem hands a path to the machine agenttik runs on, which opens it
   the way a double click in a file manager would: a folder in the file
   browser, a file in whatever is registered for its type. An empty path is
   the project folder itself, which is what a sidebar project row asks for.
   The project is named rather than assumed, because the sidebar offers this
   on every row and not only on the one in front. */
export function openInSystem(path = "", projectID = currentProjectID()) {
  if (!projectID) return;
  return api("POST", `/api/projects/${projectID}/open`, { path }).catch(fail);
}

/* The panel follows the tab in front: which panes it offers depends on the
   kind of view, and what they list depends on its project. Two tabs in the
   same project share the listing, so it is only re-read when the project
   changes. */
watch(
  () => S.owner?.kind || "",
  (kind) =>
    setInspectorPanes(kind === "project" ? ["options", "logs"] : ["changed", "logs"]),
  { immediate: true },
);

/* A schedule tab has no right-hand panel at all: Changed and Commits describe
   a working tree, while a schedule is only a clock and its runs. */
export function hasInspector() {
  return S.owner?.kind !== "schedule";
}

watch(
  currentProjectID,
  () => {
    S.repositories = [];
    S.repository = "";
    S.changed = [];
    S.log = EMPTY_LOG();
    S.treeFilter = "";
    S.logFilter = "";
    S.logOpen = "";
    S.logFiles = {};
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
  // Every project, not just the ones with a page open: the sidebar draws a
  // dot per task, and a task loses its tab as soon as another replaces it, so
  // the turn events that move its dot can only come from its project.
  const projects = S.projects.map((p) => p.id);
  // The project in front is also the one whose folder the server watches for
  // us: only its files are on screen, so only its files are worth following.
  const shown = currentProjectID();
  const params = new URLSearchParams();
  if (sessions.length) params.set("sessions", sessions.join(","));
  if (projects.length) params.set("projects", projects.join(","));
  if (shown) params.set("watch", shown);
  /* Opened even with nothing to name. Every stream also carries "a project was
     added", and a window showing no project yet — a fresh install, waiting on
     `agenttik --init` — is the one that most needs to hear it. */
  const query = String(params);
  return query ? "/api/stream?" + query : "/api/stream";
}

/* The stream also carries which project's folder the server should watch, so
   moving between projects reopens it. Opening and closing tabs is not enough
   on its own: two tabs can share a project, and switching between them opens
   no tab at all. Watching from here rather than beside refreshInspector keeps
   the stream's triggers together, and runs after `stream` exists. */
watch(currentProjectID, () => resubscribe());

/* resubscribe reopens the stream when the set of open tabs changes, and does
   nothing when it has not. */
function resubscribe() {
  // A strip being restored changes what the stream would carry once per tab
  // it rebuilds. It is opened once, after the last of them — see
  // restoreOpenTabs.
  if (restoringTabs) return;
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

/* The start and end of a turn are published to the session's topic and to its
   project's, and this window is subscribed to both, so each arrives twice.
   A provider's done event precedes the persisted final done event, which
   carries stats, so each stage needs its own deduplication key. */
let lastTurnEvent = "";

function onEvent(msg) {
  // The working tree moved on disk: from a turn, an editor, or a git command
  // in a terminal. Either way what is on screen is now out of date.
  if (msg.event?.type === "files_changed") {
    if (msg.project_id === currentProjectID()) {
      refreshChanged();
      refreshTree();
    }
    return;
  }
  // A project was added outside this window: by `agenttik --init` in a
  // terminal, or by another tab. It names no project, so the sidebar re-reads
  // its list whole, which resubscribes the stream to the new project too.
  if (msg.event?.type === "projects_changed") {
    refreshProjects().catch(() => {});
    return;
  }
  // A schedule fired, skipped a run, or spent one. It names no schedule: the
  // views re-read what they are showing, as the file watcher's event does.
  if (msg.event?.type === "schedule_changed") return reloadSchedules();
  // A background request has named a task. The sidebar, the project lists and
  // the tab strip all draw that name; the tab is set from the event because a
  // task outside the sidebar's window is in neither list that would carry it.
  if (msg.event?.type === "session_titled") {
    const named = S.tabs.find((t) => t.kind === "session" && t.sessionID === msg.session_id);
    if (named && msg.session) {
      named.detail.session = { ...named.detail.session, ...msg.session };
      named.label = tabLabel(msg.session);
    }
    reloadLists();
    reloadProjects();
    return;
  }
  // A background request has written what an archived task came to. Only the
  // project's task list draws it, and only the archived half of it, so the
  // lists that hold open tasks are left alone. See specs/051-task-outcomes.md.
  if (msg.event?.type === "session_summarized") {
    reloadProjects();
    return;
  }
  const turnMoved = ["started", "done"].includes(msg.event?.type);
  if (turnMoved) {
    const stage = msg.event.type === "done" && msg.stats ? "final" : "event";
    const seen = `${stage}:${msg.event.type}:${msg.session_id}:${msg.turn_id}`;
    if (seen === lastTurnEvent) return;
    lastTurnEvent = seen;
  }
  const tab = S.tabs.find((t) => t.kind === "session" && t.sessionID === msg.session_id);
  if (tab) onSessionEvent(tab, msg);
  // A turn starting or ending anywhere in a project moves its totals, its
  // sidebar dot and the Sessions list, whether or not that session has a tab
  // open here. Most do not — one conversation per project is on screen at a
  // time — and theirs would otherwise sit at whatever the last full read said
  // for as long as the turn lasts.
  if (turnMoved) {
    reloadLists();
    reloadProjects();
  }
}

function onSessionEvent(tab, msg) {
  const ev = msg.event;
  switch (ev.type) {
    case "started":
      startLocal(tab, msg.prompt, msg.turn);
      if (msg.session) tab.detail.session = msg.session;
      takeQueued(tab, msg.prompt);
      break;
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
      if (ev.usage?.context_window) tab.detail.stats.context_window = ev.usage.context_window;
      break;
    case "limits":
      // The provider named its own allowance mid-turn, so the bars move now
      // rather than after the next reload. It names one bucket and not the
      // whole allowance, so the reading updates that bar by id and leaves the
      // other windows standing.
      if (ev.limits?.length) {
        const key = limitsKey(tab.detail.session.provider, tab.detail.session.account_id);
        const known = S.subscriptionLimits[key] || [];
        const fresh = ev.limits.map((limit) => ({ ...limit, reported_at: Date.now() }));
        S.subscriptionLimits[key] = [
          ...known.map((limit) => fresh.find((f) => f.limit_id === limit.limit_id) || limit),
          ...fresh.filter((f) => !known.some((limit) => limit.limit_id === f.limit_id)),
        ];
      }
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
        // The transcript grown from the stream has no message ids, and editing
        // a prompt needs one. Re-reading it at the end of a turn is also what
        // reconciles anything the stream and the store disagree about.
        syncMessages(tab);
        if (tab.id === S.owner?.id) {
          refreshChanged();
          // A turn that commits has changed the history as well as the tree.
          refreshLog();
        }
        refreshSubscriptionLimits(tab.detail.session.provider).catch(() => {});
      }
      break;
  }
}

/* push hands back the bubble it added, so a caller that put one up before the
   server agreed can take that exact one out again. */
function push(tab, role, content) {
  const message = { role, content, created_at: Date.now() };
  tab.detail.messages.push(message);
  return message;
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
  tab.detail.messages.push({ role, content: text, created_at: Date.now(), streaming: true });
}

function endLive(tab) {
  const last = tab.detail.messages.at(-1);
  if (last && last.streaming) last.streaming = false;
}

/* takeQueued drops the waiting prompt a turn has just claimed, so its text
   moves from the queued bubble to the transcript rather than sitting in both.

   A session's queue is drained oldest first, so the first row with that text
   is the one that started. A prompt typed twice can pick the wrong row; the
   end of the turn re-reads the queue anyway. */
function takeQueued(tab, prompt) {
  const queued = tab.detail.queued || [];
  const pending = tab.detail.pendingQueued || [];
  let list = queued;
  let at = queued.findIndex((q) => q.prompt === prompt);
  if (at < 0) {
    list = pending;
    at = pending.findIndex((q) => q.prompt === prompt);
  }
  if (at < 0) return;
  list.splice(at, 1);
  tab.detail.session.queue_count = queued.length + pending.length;
}

/* ---------------------------------------------------------------- prompt */

function startLocal(tab, prompt, turn) {
  if (turn && !tab.detail.turns.some((current) => current.id === turn.id)) tab.detail.turns.push(turn);
  if (prompt && tab.detail.messages.at(-1)?.content !== prompt) push(tab, "user", prompt);
  tab.detail.running = true;
}

/* syncMessages replaces a tab's transcript with the stored one. Streamed
   messages are built locally and carry no ids; the stored rows do, which is
   what makes a prompt editable. */
async function syncMessages(tab) {
  try {
    const detail = await api("GET", "/api/sessions/" + tab.sessionID);
    // The tab can have been closed, or a new turn started, while we waited.
    if (!S.tabs.some((open) => open.id === tab.id)) return;
    tab.detail.messages = detail.messages;
    tab.detail.turns = detail.turns;
    tab.detail.queued = detail.queued;
    tab.detail.session.queue_count = detail.session.queue_count;
    // A turn that ended on a provider being away leaves its prompt queued and
    // the task waiting, so the status comes back with the queue that explains
    // it. See specs/045-provider-outage-retry.md.
    tab.detail.session.status = detail.session.status;
  } catch {
    /* the transcript on screen is still the one the stream produced */
  }
}

/* editMessage rewrites one of your own prompts and continues from there: the
   transcript is truncated at that message and the new text is sent as a turn.

   Two things it does not do, both of which the UI says out loud. The agent is
   not rewound — continuity is the provider's own session id and neither CLI
   can truncate its history, so it still remembers the exchange that left the
   transcript and reads the edit as "I meant this instead". And nothing on disk
   is reverted: the files are whatever the earlier turns left behind. */
export async function editMessage(message, prompt) {
  const tab = S.owner;
  if (tab?.kind !== "session" || !message?.id) return false;
  prompt = prompt.trim();
  if (!prompt) return false;
  try {
    const turn = await api(
      "POST",
      `/api/sessions/${tab.sessionID}/messages/${message.id}/edit`,
      { prompt },
    );
    const at = tab.detail.messages.findIndex((m) => m.id === message.id);
    if (at >= 0) tab.detail.messages.splice(at);
    startLocal(tab, prompt, turn);
    refreshSessions();
    refreshProjects();
    return true;
  } catch (e) {
    fail(e);
    return false;
  }
}

/* copyText is the transcript's copy button. The clipboard is only reachable
   from a secure context, which loopback counts as, but a refusal should say
   so rather than look like nothing happened. */
export async function copyText(text) {
  try {
    await navigator.clipboard.writeText(String(text ?? ""));
    return true;
  } catch (e) {
    fail(e);
    return false;
  }
}

/* openExternal hands a URL to the machine's own browser. A browser tab
   follows the link by itself, so this only has to step in for the desktop
   window, whose webview opens nothing at all for target="_blank". Wails'
   runtime is both the way to do it there and the way to tell the two apart:
   it is injected into the window's own page and into no other. Answers
   whether it took the click. */
export function openExternal(url) {
  if (!window.runtime?.BrowserOpenURL) return false;
  window.runtime.BrowserOpenURL(url);
  return true;
}

/* claimProjectPrompt is the client half of the rule the runner applies: a
   conversation accepting its first prompt — sent or queued — opens with the
   project's standing prompt in front of it. The text is already here, so the
   chip that says so goes up on the keypress with the bubble rather than a
   round trip later. The server writes the same copy and sends it back on the
   started event, which is what reconciles a disagreement.

   Nothing said, nothing waiting to be said, and nothing run: the same three
   disqualifications, in the terms the client has them. It reports whether it
   claimed, so a refused prompt can take the chip back with its bubble. */
function claimProjectPrompt(tab) {
  const session = tab.detail.session;
  if (
    session.project_prompt ||
    tab.detail.messages.length ||
    tab.detail.turns.length ||
    tab.detail.queued.length ||
    (tab.detail.pendingQueued || []).length
  ) {
    return false;
  }
  const project = S.projects.find((p) => p.id === session.project_id);
  if (!project?.prompt) return false;
  session.project_prompt = project.prompt;
  return true;
}

/* The other half of that: the prompt was refused, so the conversation has not
   started after all and nothing was injected into it. */
function releaseProjectPrompt(tab, claimed) {
  if (claimed) tab.detail.session.project_prompt = "";
}

/* The prompt is on screen before the request leaves. Its text is already known
   here, so waiting on the round trip — the turn and message rows, then the CLI
   launch — to draw your own words is latency that buys nothing. Marking the
   session running in the same breath starts the working indicator from the
   keypress and takes the Send button out of reach of a second Enter.

   Nothing draws it twice: the reply contributes only the turn, and the started
   event's startLocal skips a prompt that is already the last message. A
   failure takes the bubble back out and returns the text to the box. */
export async function send(prompt) {
  const tab = S.owner;
  if (tab?.kind !== "session") return;
  prompt = prompt.trim();
  if (!prompt) return;

  const typed = tab.draft;
  const wasRunning = tab.detail.running;
  tab.draft = "";
  endLive(tab);
  const claimed = claimProjectPrompt(tab);
  const echo = push(tab, "user", prompt);
  tab.detail.running = true;

  try {
    const turn = await api("POST", `/api/sessions/${tab.sessionID}/messages`, { prompt });
    tab.detail.session.done_at = 0;
    startLocal(tab, "", turn);
    refreshSessions();
    refreshProjects();
    reloadProjects();
  } catch (e) {
    const at = tab.detail.messages.indexOf(echo);
    if (at >= 0) tab.detail.messages.splice(at, 1);
    releaseProjectPrompt(tab, claimed);
    // Whatever was typed while the request was in flight is the next prompt,
    // not this one.
    if (!tab.draft) tab.draft = typed;
    // A refusal because the session is busy leaves that turn running.
    tab.detail.running = wasRunning;
    fail(e);
  }
}

export async function enqueue(prompt) {
  const tab = S.owner;
  if (tab?.kind !== "session") return;
  prompt = prompt.trim();
  if (!prompt) return;
  const typed = tab.draft;
  // Cleared before the request, as sending does: whatever is typed while it
  // is in flight is the next prompt, not this one.
  tab.draft = "";
  const claimed = claimProjectPrompt(tab);
  // Keep optimistic queue rows separate from messages, so an assistant stream
  // can finish in order while the server is still accepting this prompt.
  const pendingQueued = tab.detail.pendingQueued || (tab.detail.pendingQueued = []);
  const createdAt = Date.now();
  const pending = {
    id: `pending:${tab.sessionID}:${createdAt}:${pendingQueued.length}`,
    session_id: tab.sessionID,
    prompt,
    provider: tab.detail.session.provider,
    model: tab.detail.session.model,
    effort: tab.detail.session.effort || "",
    created_at: createdAt,
    pending: true,
  };
  pendingQueued.push(pending);
  try {
    const result = await api("POST", `/api/sessions/${tab.sessionID}/queue`, { prompt });
    tab.detail.session.done_at = 0;
    const at = pendingQueued.indexOf(pending);
    if (at >= 0) pendingQueued.splice(at, 1);
    tab.detail.queued = result.queued;
    tab.detail.session.queue_count = result.queue_count + pendingQueued.length;
    refreshSessions();
    refreshProjects();
    reloadProjects();
  } catch (e) {
    const at = pendingQueued.indexOf(pending);
    if (at >= 0) pendingQueued.splice(at, 1);
    releaseProjectPrompt(tab, claimed);
    // Text entered while this request was in flight belongs to the next
    // prompt, so only restore the submitted text when the box is still empty.
    if (!tab.draft) tab.draft = typed;
    fail(e);
  }
}

// The PATCH endpoint replaces a queued row whole, so a change to one field
// travels with the other three as they stand on screen. Both callers below
// hand patchQueued the row they already have and override just what changed.
async function patchQueued(q, fields) {
  const tab = S.owner;
  if (tab?.kind !== "session" || !q?.id) return;
  try {
    const updated = await api("PATCH", "/api/sessions/" + tab.sessionID + "/queue/" + q.id, {
      prompt: q.prompt,
      provider: q.provider,
      account_id: q.account_id || 0,
      model: q.model,
      effort: q.effort,
      ...fields,
    });
    Object.assign(q, updated);
  } catch (e) {
    fail(e);
    throw e;
  }
}

// updateQueuedModel changes one waiting prompt's saved choice. The session
// picker is left alone until that prompt starts, so other queued prompts keep
// their own choices too.
export async function updateQueuedModel(q, provider, model, effort, accountID) {
  const account = accountID === undefined ? q.account_id || 0 : accountID;
  return patchQueued(q, { provider, account_id: account, model, effort });
}

// updateQueuedPrompt rewrites the text of a prompt still waiting in the
// queue, leaving its saved model choice untouched.
export async function updateQueuedPrompt(q, prompt) {
  return patchQueued(q, { prompt });
}

/* The chord that opens that editor is pressed where the caret already is —
   the prompt box, or the transcript above it — and neither owns the editor,
   so the request travels as a counter the transcript watches, the same shape
   as promptFocus. Counting rather than naming a row is what lets a second
   press ask again for one it has already picked. */
export function editLastQueued() {
  if (!S.detail) return;
  S.queuedEdit += 1;
}

// forceQueued runs a waiting prompt now instead of when the project's queue
// reaches it. Other sessions keep running; only this session's own turn, which
// a second prompt cannot share, is interrupted. The queue stays untouched here
// until the started event claims it, so the UI cannot hide a prompt if
// cancellation or startup fails.
export async function forceQueued(queuedID) {
  const tab = S.owner;
  if (tab?.kind !== "session" || !queuedID) return;
  try {
    await api("POST", `/api/sessions/${tab.sessionID}/queue/force`, { queued_id: queuedID });
  } catch (e) {
    fail(e);
    throw e;
  }
}

export async function stopTurn() {
  const tab = S.owner;
  if (tab?.kind !== "session") return;
  return stopTask(tab.sessionID);
}

// Stop preserves queued text on the server. Reload it even if the queue was
// already cleared by a turn's final event; local drafts remain untouched.
export async function stopTask(sessionID) {
  if (!sessionID) return;
  try {
    await api("POST", `/api/sessions/${sessionID}/stop`);
    const tab = S.tabs.find((open) => open.kind === "session" && open.sessionID === sessionID);
    if (tab) await syncMessages(tab);
    await Promise.all([refreshProjects(), refreshSessions()]);
    reloadProjects();
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

/* ------------------------------------------------------------- shortcuts */

/* The chords live in one map so the handlers ask a question — "is this the
   send chord?" — instead of spelling a key out. Only what differs from the
   default is stored, so a default that changes later reaches everyone who
   never touched it. */
const LEGACY_TASK_ACTIONS = {
  "task.new": "session.new",
  "task.prev": "session.prev",
  "task.next": "session.next",
};

function defaultKeys() {
  const bound = {};
  for (const a of ACTIONS) bound[a.id] = a.keys;
  return bound;
}

export function loadKeys() {
  let saved = null;
  try {
    saved = JSON.parse(localStorage.getItem(KEYS_KEY));
  } catch {
    /* keep the defaults */
  }
  if (!saved) return;
  for (const a of ACTIONS) {
    const chords = saved[a.id] || saved[LEGACY_TASK_ACTIONS[a.id]];
    if (a.fixed || !Array.isArray(chords) || !chords.length) continue;
    if (chords.every((c) => typeof c === "string" && c)) S.keys[a.id] = chords;
  }
}

function saveKeys() {
  const overrides = {};
  for (const a of ACTIONS) {
    if (!sameChords(S.keys[a.id], a.keys)) overrides[a.id] = S.keys[a.id];
  }
  try {
    localStorage.setItem(KEYS_KEY, JSON.stringify(overrides));
  } catch {
    /* private mode, a full quota — the chords just do not persist */
  }
}

/* hit answers the only question a handler has: was this event the chord bound
   to this action? */
export function hit(e, id) {
  const chords = S.keys[id];
  if (!chords) return false;
  for (const chord of chords) if (matches(e, chord)) return true;
  return false;
}

/* Recording replaces the whole binding: an action with two default chords
   ("Ctrl+N" and "Ctrl+T") answers to the one that was pressed instead. */
export function bindKey(id, chord) {
  S.keys[id] = [chord];
  saveKeys();
}

export function resetKey(id) {
  S.keys[id] = actionKeys(id);
  saveKeys();
}

export function resetAllKeys() {
  S.keys = defaultKeys();
  saveKeys();
}

function actionKeys(id) {
  return ACTIONS.find((a) => a.id === id).keys;
}

export function isDefaultKey(id) {
  return sameChords(S.keys[id], actionKeys(id));
}

function sameChords(a, b) {
  return a.length === b.length && a.every((c, i) => c === b[i]);
}

/* ---------------------------------------------------------- prompt keys */

/* Send and enqueue share one pair of chords, and which action gets the plain
   Enter is a preference rather than a rebinding. General offers the swap, and
   it writes the two bindings instead of keeping a flag beside them: the pane
   and the Shortcuts list are then one fact, not two that can disagree.

   Moving either chord somewhere else in Shortcuts is still allowed, and reads
   back here as neither arrangement. */
export const PROMPT_CHORDS = { plain: "Enter", modified: "Ctrl+Enter" };

/* enterDoes names what the plain Enter submits with: "send", "enqueue", or
   "custom" once Shortcuts has moved one of them off this pair. */
export function enterDoes() {
  const send = S.keys["prompt.send"];
  const queue = S.keys["prompt.enqueue"];
  const on = (chords, chord) => sameChords(chords, [chord]);
  if (on(send, PROMPT_CHORDS.plain) && on(queue, PROMPT_CHORDS.modified)) return "send";
  if (on(queue, PROMPT_CHORDS.plain) && on(send, PROMPT_CHORDS.modified)) return "enqueue";
  return "custom";
}

export function setEnterDoes(what) {
  const plain = what === "enqueue" ? "prompt.enqueue" : "prompt.send";
  const modified = what === "enqueue" ? "prompt.send" : "prompt.enqueue";
  S.keys[plain] = [PROMPT_CHORDS.plain];
  S.keys[modified] = [PROMPT_CHORDS.modified];
  saveKeys();
}

/* keyConflicts maps a chord bound more than once to the actions that answer
   to it, so Settings can say which one it now shares with. */
export function keyConflicts() {
  const byChord = new Map();
  for (const a of ACTIONS) {
    for (const chord of S.keys[a.id]) {
      const ids = byChord.get(chord);
      if (ids) ids.push(a.id);
      else byChord.set(chord, [a.id]);
    }
  }
  for (const [chord, ids] of byChord) if (ids.length < 2) byChord.delete(chord);
  return byChord;
}

/* ------------------------------------------------------------------ init */

export async function init() {
  loadLastUsed();
  loadLayout();
  loadKeys();
  loadFileMode();
  loadWindow();
  loadTaskPageSize();
  loadFoldOthers();
  // Open tabs are saved, but an edited file is not — it would put a whole
  // working copy in localStorage — so the browser's own warning is what
  // stands between unsaved edits and a reload.
  window.addEventListener("beforeunload", (e) => {
    if (S.tabs.some(isDirty)) e.preventDefault();
  });
  try {
    // What the first paint draws is the sidebar's projects, its task rows and
    // the strip that was left open, and those need exactly these two. The
    // rest is fetched beside them but never waited for: a provider's answer
    // can cost a CLI probe of a second or more, and the model it names is
    // worth showing late rather than holding an otherwise ready window on.
    const behind = Promise.all([loadProviders(), refreshSchedules()]);
    behind.catch(fail);
    await Promise.all([refreshProjects(), refreshSessions()]);
    await restoreOpenTabs();
    // A first launch has no tabs to say which project is selected, and the
    // sidebar, the Tree and Ctrl+N all want one.
    if (!S.activeProjectID && S.projects.length) S.activeProjectID = S.projects[0].id;
  } catch (e) {
    fail(e);
  }
}
