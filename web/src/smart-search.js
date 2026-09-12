import { reactive } from "vue";
import { api } from "./api.js";

const KEY = "agenttik.smartSearch";
export const smartSearch = reactive({ enabled: false, phase: "idle", percent: 0,
  completed: 0, total: 0, seconds: null, error: "", version: 0 });
let generation = 0;
let indexing;
let refreshRequested = false;
let timer;
let cancelPoll;

function pause() {
  return new Promise((resolve) => {
    const timeout = setTimeout(done, 500);
    function done() { clearTimeout(timeout); cancelPoll = null; resolve(); }
    cancelPoll = done;
  });
}

export function refreshSmartIndex() {
  if (!smartSearch.enabled) return;
  if (indexing) { refreshRequested = true; return indexing; }
  const current = generation;
  const operation = (async () => {
    try {
      smartSearch.error = "";
      let status = await api("POST", "/api/smart-search/index");
      while (current === generation) {
        Object.assign(smartSearch, status);
        if (status.phase === "ready" || status.phase === "error") break;
        await pause();
        if (current !== generation) return;
        status = await api("GET", "/api/smart-search");
      }
    } catch (error) {
      if (current === generation) Object.assign(smartSearch, { error: error.message, phase: "error" });
    } finally {
      if (current === generation) {
        indexing = null;
        if (refreshRequested) {
          refreshRequested = false;
          queueMicrotask(refreshSmartIndex);
        }
      }
    }
  })();
  indexing = operation;
  return operation;
}

export function setSmartSearch(enabled) {
  localStorage.setItem(KEY, enabled ? "1" : "0");
  generation++;
  clearInterval(timer);
  cancelPoll?.();
  indexing = null;
  refreshRequested = false;
  Object.assign(smartSearch, { enabled, phase: enabled ? "loading" : "idle", error: "", percent: 0,
    completed: 0, total: 0, seconds: null });
  if (enabled) {
    refreshSmartIndex();
    timer = setInterval(refreshSmartIndex, 30000);
  }
}

export function initSmartSearch() {
  if (localStorage.getItem(KEY) === "1") setSmartSearch(true);
}

export async function searchTasks(query, ids) {
  const current = generation;
  if (indexing) await indexing;
  if (current !== generation || !smartSearch.enabled) throw new Error("Smart Search stopped");
  if (smartSearch.phase !== "ready") throw new Error(smartSearch.error || "Smart Search is preparing");
  try {
    const scores = await api("POST", "/api/smart-search/query", { query, ids });
    if (current !== generation) throw new Error("Smart Search stopped");
    return scores;
  } catch (error) {
    if (current === generation) Object.assign(smartSearch, { phase: "error", error: error.message });
    throw error;
  }
}
