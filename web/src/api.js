/* The HTTP API and the few formatters the panels share. */

// Desktop windows share an origin across servers. Keep restored tabs and
// drafts tied to the instance that owns their project and task IDs.
let remoteInstance = "";
let localInstance = "";
let privateMode = false;
export const profileID = new URLSearchParams(globalThis.location?.search || "").get("profile") || "default";

export function apiURL(path) {
  if (profileID === "default" || !path.startsWith("/api/")) return path;
  return `${path}${path.includes("?") ? "&" : "?"}profile=${encodeURIComponent(profileID)}`;
}

export function setPrivateMode(value) { privateMode = value; }

// Private UI state lasts only in memory, including drafts. A private window
// never writes app state to the normal browser storage.
const privateState = new Map();
export const storage = {
  getItem(key) { return privateMode ? privateState.get(key) ?? null : localStorage.getItem(instanceKey(key)); },
  setItem(key, value) { if (privateMode) privateState.set(key, String(value)); else localStorage.setItem(instanceKey(key), value); },
  removeItem(key) { if (privateMode) privateState.delete(key); else localStorage.removeItem(instanceKey(key)); },
};
export function instanceKey(key) {
  const scope = [remoteInstance, localInstance, profileID === "default" ? "" : profileID].filter(Boolean).join(":");
  return scope ? `${key}:${scope}` : key;
}

export async function api(method, path, body) {
  const res = await fetch(apiURL(path), {
    method,
    headers: body ? { "Content-Type": "application/json" } : undefined,
    body: body ? JSON.stringify(body) : undefined,
  });
  const local = res.headers.get("X-Agenttik-Instance");
  if (local !== null) localInstance = local;
  const instance = res.headers.get("X-Agenttik-Remote");
  if (instance !== null) remoteInstance = instance;
  if (res.status === 204) return null;
  const text = await res.text();
  // Endpoints that only accept work answer with a bare status, whose body is
  // the reason phrase rather than JSON. Failures are always JSON.
  const isJSON = res.headers.get("content-type")?.includes("json");
  const data = text && isJSON ? JSON.parse(text) : null;
  // A 401 can only be the lock on the exposed server: agenttik's own API
  // never asks for one. The session has lapsed mid-use, so the page is
  // reloaded into the login form rather than left showing a toast on a UI
  // that can no longer load anything. See specs/043-exposed-server.md.
  if (res.status === 401) {
    window.location.reload();
    throw new Error("Signed out");
  }
  if (!res.ok) throw new Error((data && data.error) || res.statusText);
  return data;
}

export const nf = new Intl.NumberFormat();

export function tokens(n) {
  if (n >= 1_000_000) return (n / 1_000_000).toFixed(2) + "M";
  if (n >= 1000) return (n / 1000).toFixed(1) + "k";
  return nf.format(n || 0);
}

// Providers that expose agent identity partition their task totals. Older
// turns and providers without that signal remain in "Unattributed" instead
// of being presented as main-agent work.
export function usageBreakdownRows(stats) {
  if (!stats?.usage_breakdown_turns) return [];
  const value = (input, output) => `${tokens(input)} in · ${tokens(output)} out`;
  const rows = [
    ["Main agent", value(stats.main_input_tokens, stats.main_output_tokens)],
  ];
  const subagentInput = stats.subagent_input_tokens || 0;
  const subagentOutput = stats.subagent_output_tokens || 0;
  const subagentCount = stats.subagent_count || 0;
  if (subagentCount || subagentInput || subagentOutput) {
    rows.push([
      subagentCount ? `Subagents (${nf.format(subagentCount)})` : "Subagents",
      value(subagentInput, subagentOutput),
    ]);
  }
  const unattributedInput = Math.max(
    0,
    (stats.input_tokens || 0) - (stats.main_input_tokens || 0) - subagentInput,
  );
  const unattributedOutput = Math.max(
    0,
    (stats.output_tokens || 0) - (stats.main_output_tokens || 0) - subagentOutput,
  );
  if (unattributedInput || unattributedOutput) {
    rows.push(["Unattributed", value(unattributedInput, unattributedOutput)]);
  }
  return rows;
}

export function duration(ms) {
  if (!ms) return "0s";
  const s = Math.round(ms / 1000);
  if (s < 60) return s + "s";
  const m = Math.floor(s / 60);
  if (m < 60) return `${m}m ${s % 60}s`;
  return `${Math.floor(m / 60)}h ${m % 60}m`;
}

export function ago(ts) {
  if (!ts) return "never";
  const s = Math.round((Date.now() - ts) / 1000);
  if (s < 60) return "just now";
  if (s < 3600) return Math.floor(s / 60) + "m ago";
  if (s < 86400) return Math.floor(s / 3600) + "h ago";
  return Math.floor(s / 86400) + "d ago";
}

// Keep the stats panel stable across browser locales. Times are UTC, matching
// the ISO date source while omitting the T, fractional seconds, and Z for a
// compact `YYYY-MM-DD HH:mm:ss` display.
export function isoDate(ts) {
  if (!ts) return "never";
  return new Date(ts).toISOString().slice(0, 19).replace("T", " ");
}

// A schedule is read off the clock on the wall — "every day at 9" means local
// 9 — so its times are local, in the same YYYY-MM-DD HH:mm:ss layout. Showing
// the next run in UTC beside a local recurrence reads as simply wrong.
export function isoLocal(ts) {
  if (!ts) return "never";
  const d = new Date(ts);
  const p = (n) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ` +
    `${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`;
}

export function cost(usd) {
  return "$" + (usd || 0).toFixed(4);
}
