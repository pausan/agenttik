import { ref } from "vue";

const KEY = "agenttik.diagnostics.v1";
const launch = `${Date.now()}-${Math.random().toString(36).slice(2, 10)}`;
const sources = new Set(["api", "notification", "vue", "runtime", "promise", "startup"]);
const resources = new Set(["providers", "projects", "sessions", "settings", "subscriptions", "schedules", "profiles", "version", "files", "stars", "repositories"]);
const kinds = new Set(["Error", "TypeError", "RangeError", "SyntaxError", "ReferenceError", "URIError", "AbortError"]);
const seen = new WeakSet();
export const diagnosticRevision = ref(0);

// Only fixed categories and numeric locations cross the privacy boundary.
// Never copy messages, bodies, URLs, function names, or arbitrary properties.
function safeEntry(entry) {
  return {
    time: Number.isFinite(entry.time) ? entry.time : 0,
    launch: /^\d{13}-[a-z0-9]{1,10}$/.test(entry.launch) ? entry.launch : "unknown",
    source: sources.has(entry.source) ? entry.source : "runtime",
    kind: kinds.has(entry.kind) ? entry.kind : "Error",
    method: ["GET", "POST", "PUT", "PATCH", "DELETE"].includes(entry.method) ? entry.method : "",
    resource: resources.has(entry.resource) ? entry.resource : "other",
    status: Number.isInteger(entry.status) && entry.status >= 100 && entry.status <= 599 ? entry.status : 0,
    trace: Array.isArray(entry.trace) ? entry.trace.filter(x => /^line \d{1,8}, column \d{1,8}$/.test(x)).slice(0, 12) : [],
  };
}

export function readDiagnostics(storage) {
  try {
    const entries = JSON.parse(storage.getItem(KEY) || "[]");
    return Array.isArray(entries) ? entries.slice(0, 100).filter(x => x && typeof x === "object").map(safeEntry) : [];
  } catch { return []; }
}

export function recordError(error, context, storage) {
  try {
    if (error && typeof error === "object") {
      if (seen.has(error)) return;
      seen.add(error);
    }
    const trace = typeof error?.stack === "string" ? error.stack.split("\n").slice(1, 13).flatMap(line => {
      const match = line.match(/:(\d{1,8}):(\d{1,8})\)?$/);
      return match ? [`line ${match[1]}, column ${match[2]}`] : [];
    }) : [];
    const entry = safeEntry({ ...context, kind: error?.name, trace, time: Date.now(), launch });
    const entries = [entry, ...readDiagnostics(storage)].slice(0, 100);
    const launches = [...new Set(entries.map(x => x.launch))].slice(0, 5);
    storage.setItem(KEY, JSON.stringify(entries.filter(x => launches.includes(x.launch))));
    diagnosticRevision.value++;
  } catch { /* Diagnostics must never interfere with the failing operation. */ }
}

export function clearDiagnostics(storage) {
  storage.removeItem(KEY);
  diagnosticRevision.value++;
}

export function diagnosticReport(entries) {
  return JSON.stringify({ format: "agenttik-errors-v1", errors: entries.map(safeEntry) }, null, 2);
}
