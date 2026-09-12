import { AutoModel, AutoTokenizer, env, mean_pooling } from "@huggingface/transformers";
import wasmURL from "../node_modules/onnxruntime-web/dist/ort-wasm-simd-threaded.asyncify.wasm?url";
import wasmModuleURL from "../node_modules/onnxruntime-web/dist/ort-wasm-simd-threaded.asyncify.mjs?url";
import { dot, secondsLeft, taskText } from "./task-search.js";

const MODEL = "hotchpotch/bekko-embedding-v1-a8m";
const REVISION = "c721113d59a1d91b447450324f51c4b3332c924a";
env.allowLocalModels = false;
// Tokenizer discovery in Transformers.js omits the revision option. Pin the
// URL template too, so discovery reads the same cache entries when offline.
env.remotePathTemplate = `{model}/resolve/${REVISION}/`;
env.backends.onnx.wasm.numThreads = 1;
env.backends.onnx.wasm.proxy = false;
env.backends.onnx.wasm.wasmPaths = { wasm: wasmURL, mjs: wasmModuleURL };
let embedder;
let db;
let vectors = new Map();
let lastQuery = "";
let lastVector;

function progress(state) { self.postMessage({ progress: state }); }

async function database() {
  if (db) return db;
  db = await new Promise((resolve, reject) => {
    const request = indexedDB.open(`agenttik.bekko.${REVISION}`, 1);
    request.onupgradeneeded = () => request.result.createObjectStore("tasks", { keyPath: "id" });
    request.onsuccess = () => resolve(request.result);
    request.onerror = () => reject(request.error);
  });
  return db;
}

async function cache(mode, action) {
  const connection = await database();
  return new Promise((resolve, reject) => {
    const tx = connection.transaction("tasks", mode);
    const request = action(tx.objectStore("tasks"));
    tx.oncomplete = () => resolve(request?.result);
    tx.onerror = () => reject(tx.error);
    tx.onabort = () => reject(tx.error || new Error("Embedding cache write cancelled"));
  });
}

async function load() {
  if (embedder) return;
  progress({ phase: "download", percent: 0 });
  const options = {
    revision: REVISION, device: "wasm", dtype: "fp32",
    progress_callback(event) {
      if (event.status === "progress" && event.file === "onnx/model.onnx") {
        progress({ phase: "download", percent: Math.min(99, event.progress || 0) });
      }
    },
  };
  // Load the tokenizer before the model so download progress tracks the large
  // ONNX file once tokenizer preparation is complete.
  const tokenizer = await AutoTokenizer.from_pretrained(MODEL, options);
  const model = await AutoModel.from_pretrained(MODEL, options);
  embedder = { tokenizer, model };
  progress({ phase: "download", percent: 100 });
}

async function embed(text) {
  // Pass truncation explicitly; the tokenizer's default limit is read-only.
  const inputs = embedder.tokenizer(text, { padding: true, truncation: true, max_length: 512 });
  const outputs = await embedder.model(inputs);
  const pooled = mean_pooling(outputs.last_hidden_state, inputs.attention_mask);
  const normalized = pooled.normalize(2, -1);
  const vector = new Float32Array(normalized.data);
  normalized.dispose();
  pooled.dispose();
  for (const tensor of Object.values(outputs)) tensor.dispose?.();
  for (const tensor of Object.values(inputs)) tensor.dispose?.();
  if (vector.length !== 384 || vector.some((v) => !Number.isFinite(v))) {
    throw new Error("Bekko returned an invalid embedding");
  }
  return vector;
}

async function index(tasks) {
  await load();
  const saved = new Map((await cache("readonly", (s) => s.getAll())).map((v) => [v.id, v]));
  const pending = tasks.filter((task) => saved.get(task.id)?.text !== taskText(task));
  const changed = pending.length > 0 || vectors.size !== tasks.length || tasks.some((task) => !vectors.has(task.id));
  const started = performance.now();
  if (pending.length || !vectors.size) {
    progress({ phase: "index", completed: 0, total: pending.length, seconds: null, percent: 0 });
  }
  for (let i = 0; i < pending.length; i++) {
    const task = pending[i];
    const text = taskText(task);
    const entry = { id: task.id, text, vector: await embed(text) };
    await cache("readwrite", (s) => s.put(entry));
    saved.set(task.id, entry);
    progress({ phase: "index", completed: i + 1, total: pending.length,
      seconds: secondsLeft(i + 1, pending.length, performance.now() - started),
      percent: (i + 1) / pending.length * 100 });
  }
  vectors = new Map(tasks.map((task) => [task.id, saved.get(task.id).vector]));
  await cache("readwrite", (s) => {
    for (const id of saved.keys()) if (!vectors.has(id)) s.delete(id);
  });
  progress({ phase: "ready", percent: 100, completed: tasks.length, total: tasks.length, seconds: 0 });
  return changed;
}

// Serialize inference: an ONNX session must not run indexing and a query concurrently.
let queue = Promise.resolve();
let latestSearch;
self.onmessage = ({ data }) => {
  if (data.type === "search") latestSearch = data.id;
  queue = queue.then(async () => {
    try {
      let result;
      if (data.type === "index") result = await index(data.tasks);
      else if (data.type === "search") {
        if (data.id !== latestSearch) {
          self.postMessage({ id: data.id, result: {} });
          return;
        }
        if (data.query !== lastQuery) {
          lastVector = await embed(data.query);
          lastQuery = data.query;
        }
        result = Object.fromEntries(data.ids.filter((id) => vectors.has(id))
          .map((id) => [id, dot(lastVector, vectors.get(id))]));
      }
      self.postMessage({ id: data.id, result });
    } catch (error) {
      self.postMessage({ id: data.id, error: error.message || String(error) });
    }
  });
};
