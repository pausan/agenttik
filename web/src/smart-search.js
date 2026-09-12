import { reactive } from "vue";
import { api } from "./api.js";

const KEY = "agenttik.smartSearch";
export const smartSearch = reactive({ enabled: false, phase: "idle", percent: 0,
  completed: 0, total: 0, seconds: null, error: "", version: 0 });
let worker;
let sequence = 0;
let generation = 0;
let indexing;
let refreshRequested = false;
let timer;
const requests = new Map();

function stopWorker() {
  worker?.terminate();
  worker = null;
  for (const { reject } of requests.values()) reject(new Error("Smart Search stopped"));
  requests.clear();
}

function request(type, payload) {
  if (!worker) {
    worker = new Worker(new URL("./smart-search.worker.js", import.meta.url), { type: "module" });
    const activeWorker = worker;
    worker.onmessage = ({ data }) => {
      if (worker !== activeWorker) return;
      if (data.progress) { Object.assign(smartSearch, data.progress); return; }
      const pending = requests.get(data.id);
      requests.delete(data.id);
      if (data.error) pending?.reject(new Error(data.error));
      else pending?.resolve(data.result);
    };
    worker.onerror = (event) => {
      if (worker !== activeWorker) return;
      for (const { reject } of requests.values()) reject(new Error(event.message || "Could not start Smart Search"));
      requests.clear();
      stopWorker();
    };
  }
  return new Promise((resolve, reject) => {
    const id = ++sequence;
    requests.set(id, { resolve, reject });
    worker.postMessage({ id, type, ...payload });
  });
}

export function refreshSmartIndex() {
  if (!smartSearch.enabled) return;
  if (indexing) { refreshRequested = true; return indexing; }
  const current = generation;
  const operation = (async () => {
    try {
      smartSearch.error = "";
      // limit=0 is the existing API's unbounded listing, including archived tasks.
      const tasks = await api("GET", "/api/sessions?window=all&include_done=true&limit=0");
      if (current !== generation) return;
      const changed = await request("index", { tasks });
      if (current === generation && changed) smartSearch.version++;
    } catch (error) {
      if (current === generation) {
        smartSearch.error = error.message;
        smartSearch.phase = "error";
      }
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
  stopWorker();
  indexing = null;
  refreshRequested = false;
  Object.assign(smartSearch, { enabled, phase: enabled ? "loading" : "idle", error: "", percent: 0 });
  if (enabled) {
    refreshSmartIndex();
    timer = setInterval(refreshSmartIndex, 30000);
  }
}

export function initSmartSearch() {
  if (localStorage.getItem(KEY) === "1") setSmartSearch(true);
}

export async function searchTasks(query, ids) {
  if (indexing) await indexing;
  if (!smartSearch.enabled || smartSearch.phase !== "ready") throw new Error(smartSearch.error || "Smart Search is preparing");
  const current = generation;
  try {
    return await request("search", { query, ids });
  } catch (error) {
    if (current === generation) Object.assign(smartSearch, { phase: "error", error: error.message });
    throw error;
  }
}
