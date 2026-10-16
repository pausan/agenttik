import { test } from "node:test";
import assert from "node:assert/strict";
import { CHUNK, carriesFiles, existing, readDesktopDrop, readDrop, uploadDrop } from "./upload.js";

const fileEntry = (name, file = { name }) => ({ name, isFile: true, file: (ok) => ok(file) });
const dirEntry = (name, children) => ({
  name,
  isFile: false,
  createReader() {
    // Two batches and the empty one that ends them, as real readers do.
    const batches = [children.slice(0, 1), children.slice(1), []];
    return { readEntries: (ok) => ok(batches.shift()) };
  },
});
const item = (entry) => ({ kind: "file", webkitGetAsEntry: () => entry });

test("drops walk folders and keep empty ones", async () => {
  const drop = await readDrop({
    items: [
      item(fileEntry("a.txt")),
      item(dirEntry("src", [fileEntry("x.go"), dirEntry("empty", []), dirEntry("sub", [fileEntry("y.go")])])),
      { kind: "string" },
    ],
  });
  assert.deepEqual(drop.files.map((f) => f.path), ["a.txt", "src/x.go", "src/sub/y.go"]);
  assert.deepEqual(drop.dirs, ["src/empty"]);
});

test("drops without entries fall back to the file list", async () => {
  const file = { name: "plain.txt" };
  assert.deepEqual(await readDrop({ items: [], files: [file] }), { files: [{ path: "plain.txt", file }], dirs: [] });
});

test("existing names the files a drop would replace", () => {
  const files = [{ path: "a.txt" }, { path: "src/b.txt" }];
  assert.deepEqual(existing(files, "", new Set(["a.txt", "b.txt"])), ["a.txt"]);
  assert.deepEqual(existing(files, "in", new Set(["in/src/b.txt", "a.txt"])), ["in/src/b.txt"]);
});

const blob = (size) => ({
  size,
  slice: (from, to) => ({ arrayBuffer: async () => new ArrayBuffer(Math.max(0, Math.min(to, size) - from)) }),
});

test("uploads send folders, then each file in pieces", async (t) => {
  const calls = [];
  t.mock.method(globalThis, "fetch", async (url, init) => {
    calls.push({ url, method: init.method, bytes: init.body?.byteLength });
    return { ok: true, status: 200, headers: new Headers() };
  });
  const seen = [];
  await uploadDrop(7, "dest", {
    files: [{ path: "big.bin", file: blob(CHUNK + 5) }, { path: "empty", file: blob(0) }],
    dirs: ["d/e"],
  }, { replace: true, progress: (n) => seen.push(n) });
  assert.equal(calls[0].url, "/api/projects/7/upload?path=dest%2Fd%2Fe&dir=1");
  const pieces = calls.slice(1).map((c) => {
    const q = new URL(c.url, "http://x").searchParams;
    return [q.get("path"), q.get("offset"), q.get("size"), q.get("replace"), c.bytes];
  });
  assert.deepEqual(pieces.sort(), [
    ["dest/big.bin", "0", String(CHUNK + 5), "1", CHUNK],
    ["dest/big.bin", String(CHUNK), String(CHUNK + 5), "1", 5],
    ["dest/empty", "0", "0", "1", 0],
  ].sort());
  assert.equal(seen.at(-1), CHUNK + 5);
});

test("a failed piece stops the copy and removes the partial file", async (t) => {
  const calls = [];
  t.mock.method(globalThis, "fetch", async (url, init) => {
    calls.push(`${init.method} ${url}`);
    if (init.method === "PUT") {
      return { ok: false, status: 400, statusText: "Bad", headers: new Headers(), json: async () => ({ error: "a.txt already exists" }) };
    }
    return { ok: true, status: 204, headers: new Headers() };
  });
  await assert.rejects(uploadDrop(1, "", { files: [{ path: "a.txt", file: blob(3) }], dirs: [] }), /already exists/);
  assert.match(calls.at(-1), /^DELETE \/api\/projects\/1\/upload\?path=a\.txt&upload=[0-9a-f]{32}$/);
});

test("desktop drags are told apart from the app's own", () => {
  const drag = (...types) => ({ dataTransfer: { types } });
  assert.equal(carriesFiles(drag("Files")), true);
  // WebKitGTK shows a desktop file only as a URI list.
  assert.equal(carriesFiles(drag("text/uri-list", "text/html")), true);
  assert.equal(carriesFiles(drag("text/plain")), false);
  assert.equal(carriesFiles({}), false);
});

test("a native desktop drop is read back from this machine piece by piece", async (t) => {
  const urls = [];
  t.mock.method(globalThis, "fetch", async (url) => {
    urls.push(url);
    if (url === "/api/desktop/drop") {
      const body = JSON.stringify({ files: [{ path: "dir/a b.bin", size: CHUNK + 1 }], dirs: ["dir/empty"] });
      return new Response(body, { headers: { "content-type": "application/json" } });
    }
    return new Response(new Uint8Array(Number(new URL(url, "http://x").searchParams.get("length"))));
  });
  const drop = await readDesktopDrop();
  assert.deepEqual(drop.dirs, ["dir/empty"]);
  const [{ path, file }] = drop.files;
  assert.equal(path, "dir/a b.bin");
  assert.equal(file.size, CHUNK + 1);
  assert.equal((await file.slice(CHUNK, 2 * CHUNK).arrayBuffer()).byteLength, 1);
  assert.equal(urls.at(-1), `/api/desktop/drop/file?path=dir%2Fa%20b.bin&offset=${CHUNK}&length=1`);
});
