import test from "node:test";
import assert from "node:assert/strict";
import { recordError, readDiagnostics, diagnosticReport, clearDiagnostics } from "./diagnostics.js";

function memory() {
  let value = null;
  return { getItem: () => value, setItem: (_, next) => { value = next; }, removeItem: () => { value = null; } };
}

test("reports exclude arbitrary error and request content and deduplicate errors", () => {
  const storage = memory();
  const error = new TypeError("secret prompt person@example.com");
  error.stack = "TypeError: secret\n at secretFunction (https://private.host/home/person/file.js?token=secret:12:34)";
  recordError(error, { source: "api", method: "POST", resource: "sessions", status: 500, body: "secret" }, storage);
  recordError(error, { source: "notification" }, storage);
  const entries = readDiagnostics(storage);
  assert.equal(entries.length, 1);
  assert.deepEqual(entries[0].trace, ["line 12, column 34"]);
  assert.equal(entries[0].status, 500);
  assert.equal(entries[0].kind, "TypeError");
  assert.doesNotMatch(diagnosticReport(entries), /secret|person|private|file.js/);
});

test("bounded history, clearing, corrupt and unavailable storage", () => {
  const storage = memory();
  for (let i = 0; i < 110; i++) recordError(new Error("private"), {}, storage);
  assert.equal(readDiagnostics(storage).length, 100);
  clearDiagnostics(storage);
  assert.deepEqual(readDiagnostics(storage), []);
  storage.setItem("", "invalid");
  assert.deepEqual(readDiagnostics(storage), []);
  assert.doesNotThrow(() => recordError(new Error(), {}, { getItem() { throw Error(); }, setItem() { throw Error(); } }));
});

test("retains only five launches with errors and revalidates stored fields", () => {
  const storage = memory();
  storage.setItem("", JSON.stringify(Array.from({ length: 8 }, (_, i) => ({ launch: `170000000000${i}-abc`, time: i, kind: "secret", trace: ["secret"], source: "secret" }))));
  recordError(new Error(), {}, storage);
  assert.equal(readDiagnostics(storage).length, 5);
  assert.doesNotMatch(diagnosticReport(readDiagnostics(storage)), /secret/);
});
