import { apiURL } from "./api.js";

/* Files dropped on the Tree from the desktop are sent to the server as bytes,
   a piece at a time, rather than as paths for it to copy: the window may be
   on another machine's project, and the request is forwarded there like any
   other. See specs/095-tree-file-drop.md. */

// The server refuses more than this per request; see uploadChunkBytes.
export const CHUNK = 2 << 20;
// Files sent at once. A folder of many small files is mostly round trips, and
// a remote one is two of them per piece.
const PARALLEL = 4;

/* readDrop lists what was dropped, folders walked, as paths relative to the
   drop. The items have to be read while the drop event is still running, so
   the entries are taken first and walked after. */
export async function readDrop(transfer) {
  const entries = [...(transfer?.items || [])]
    .filter((item) => item.kind === "file")
    .map((item) => item.webkitGetAsEntry?.())
    .filter(Boolean);
  const out = { files: [], dirs: [] };
  if (!entries.length) {
    for (const file of transfer?.files || []) out.files.push({ path: file.name, file });
    return out;
  }
  const walk = async (entry, path) => {
    if (entry.isFile) {
      out.files.push({ path, file: await new Promise((ok, no) => entry.file(ok, no)) });
      return;
    }
    const children = await readAll(entry.createReader());
    // Only an empty folder needs making on its own; the others come with
    // the files in them.
    if (!children.length) out.dirs.push(path);
    for (const child of children) await walk(child, `${path}/${child.name}`);
  };
  for (const entry of entries) await walk(entry, entry.name);
  return out;
}

// A reader hands out a folder's entries in batches until one comes back empty.
async function readAll(reader) {
  const all = [];
  for (;;) {
    const batch = await new Promise((ok, no) => reader.readEntries(ok, no));
    if (!batch.length) return all;
    all.push(...batch);
  }
}

/* existing names the dropped files that would land on a file already listed
   in the project. */
export function existing(files, parent, listed) {
  return files.map((f) => join(parent, f.path)).filter((path) => listed.has(path));
}

export const join = (parent, path) => (parent ? `${parent}/${path}` : path);

/* uploadDrop copies the dropped files and folders under parent, calling
   progress with the bytes sent so far. A failure or an abort stops the rest
   and cleans up the files left half-copied. */
export async function uploadDrop(projectID, parent, { files, dirs }, { replace = false, signal, progress = () => {} } = {}) {
  const base = `/api/projects/${projectID}/upload`;
  for (const dir of dirs) await send("PUT", `${base}?path=${encodeURIComponent(join(parent, dir))}&dir=1`, undefined, signal);
  let sent = 0;
  let next = 0;
  const one = async () => {
    while (next < files.length && !signal?.aborted) {
      const { path, file } = files[next++];
      const query = `path=${encodeURIComponent(join(parent, path))}&upload=${token()}`;
      try {
        let offset = 0;
        do {
          // Bytes rather than the Blob: Linux WebKit can crash in Wails' URI
          // handler when a request body is a Blob.
          const body = await file.slice(offset, offset + CHUNK).arrayBuffer();
          await send("PUT", `${base}?${query}&offset=${offset}&size=${file.size}${replace ? "&replace=1" : ""}`, body, signal);
          offset += body.byteLength;
          sent += body.byteLength;
          progress(sent);
        } while (offset < file.size);
      } catch (error) {
        next = files.length;
        send("DELETE", `${base}?${query}`).catch(() => {});
        throw error;
      }
    }
  };
  await Promise.all(Array.from({ length: Math.min(PARALLEL, files.length) }, one));
  if (signal?.aborted) throw signal.reason;
}

async function send(method, path, body, signal) {
  const res = await fetch(apiURL(path), {
    method,
    body,
    signal,
    headers: body ? { "Content-Type": "application/octet-stream" } : undefined,
  });
  // A remote's own sign-in lapsing is answered by its dialog, not a reload.
  if (res.status === 401 && !res.headers.get("X-Agenttik-Remote-Auth")) {
    window.location.reload();
    throw new Error("Signed out");
  }
  if (!res.ok) throw new Error((await res.json().catch(() => null))?.error || res.statusText);
}

// Hex, from getRandomValues: randomUUID is missing outside secure contexts,
// which is what a window on an exposed http:// address is.
function token() {
  return [...crypto.getRandomValues(new Uint8Array(16))].map((b) => b.toString(16).padStart(2, "0")).join("");
}
