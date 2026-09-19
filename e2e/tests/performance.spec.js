import { execFileSync } from "node:child_process";
import { join } from "node:path";
import { writeFileSync } from "node:fs";
import { expect, REPO, sidebar, test } from "../fixtures.js";

// Uses only the fixture's disposable database. Run with --workers=1 so the
// before/after numbers do not measure contention from the rest of the suite.
test("large archive interaction benchmark", async ({ page, agenttik }) => {
  test.setTimeout(60_000);
  const project = await (await page.request.post(`${agenttik.url}/api/projects`, {
    data: { path: REPO, name: "Performance project" },
  })).json();
  execFileSync("python3", ["-c", `
import sqlite3, sys, time
db = sqlite3.connect(sys.argv[1])
now = int(time.time() * 1000)
with db:
    db.executemany("INSERT INTO sessions (id, project_id, title, provider, model, created_at, updated_at, last_active_at, done_at) VALUES (?, ?, ?, 'fake', 'fake-quick', ?, ?, ?, ?)",
        [(f'bench-{i}', int(sys.argv[2]), f'Benchmark task {i}', now, now, now-i, now if i else 0) for i in range(5001)])
    db.executemany("INSERT INTO messages (session_id, role, content, created_at) VALUES ('bench-0', ?, ?, ?)",
        [('user' if i % 2 == 0 else 'assistant', f'Message {i}: ' + 'A benchmark paragraph. ' * 30, now) for i in range(200)])
`, join(agenttik.dataDir, "agenttik.db"), String(project.id)]);
  await page.addInitScript((id) => localStorage.setItem("agenttik.openTabs", JSON.stringify({
    tabs: [{ id: `project:${id}`, kind: "project", projectID: id }],
    activeTab: `project:${id}`, activeProjectID: id,
  })), project.id);
  const requests = [];
  let archiveBytes = 0;
  page.on("request", r => { if (r.url().includes("/api/")) requests.push(r.url()); });
  page.on("response", async r => {
    if (r.url().includes("only_done=true")) archiveBytes += (await r.body().catch(() => Buffer.alloc(0))).length;
  });
  await page.reload();
  await expect(page.locator("main").getByText("Benchmark task 0", { exact: true })).toBeVisible();
  const startup = await page.evaluate(() => performance.now());
  await page.waitForTimeout(500); // let deferred baseline history requests finish
  const startupRequests = requests.length;
  const startupArchiveBytes = archiveBytes;
  const open = await sidebar(page).getByText("Benchmark task 0", { exact: true }).evaluate(async el => {
    const start = performance.now();
    el.click();
    while (!document.querySelector('textarea[placeholder="Ask the agent…"]')) await new Promise(requestAnimationFrame);
    await new Promise(requestAnimationFrame);
    return performance.now() - start;
  });
  await page.waitForTimeout(500);
  const typing = await typingLatency(page, page.getByPlaceholder("Ask the agent…"));
  const closeStart = performance.now();
  await page.keyboard.press("Control+w");
  await expect(page.locator("main").getByText("Benchmark task 0", { exact: true })).toBeVisible();
  const close = performance.now() - closeStart;
  console.log(JSON.stringify({ startup, open, typingP95: typing, close, startupRequests, startupArchiveBytes }));
  if (!process.env.PERF_BASELINE) {
    expect(startupArchiveBytes).toBe(0);
    expect(requests.filter(url => new URL(url).pathname === "/api/sessions")
      .every(url => new URL(url).searchParams.get("include_done") === "false")).toBeTruthy();
    expect(requests.filter(url => /\/(stats|metrics)(?:\?|$)/.test(url))).toHaveLength(0);
    const main = page.locator("main");
    await main.getByRole("tab", { name: "Archived", exact: true }).click();
    const archive = main.getByRole("region", { name: "Archived tasks" });
    await expect(archive.getByText("Benchmark task 1", { exact: true })).toBeVisible();
    await expect(archive.getByTitle("Unarchive task")).toHaveCount(25);
    await archive.getByRole("button", { name: "Next", exact: true }).click();
    await expect(archive.getByText("Benchmark task 26", { exact: true })).toBeVisible();
    await archive.getByRole("searchbox", { name: "Search archived titles" }).fill("Benchmark task 4999");
    await expect(archive.getByTitle("Unarchive task")).toHaveCount(1);
    await expect(archive.getByText("Benchmark task 4999", { exact: true })).toBeVisible();
    await archive.getByTitle("Unarchive task").click();
    await expect(archive.getByTitle("Unarchive task")).toHaveCount(0);
    await main.getByRole("tab", { name: "Tasks", exact: true }).click();
    await expect(main.getByText("Benchmark task 4999", { exact: true })).toBeVisible();
    expect(requests.filter(url => url.includes("only_done=true")).every(url => new URL(url).searchParams.get("limit") === "26")).toBeTruthy();
  }
  expect(startup).toBeLessThan(2000);
  expect(typing).toBeLessThan(100);
});

test("file typing and profile switching benchmark", async ({ page, agenttik }) => {
  test.setTimeout(60_000);
  const project = await (await page.request.post(`${agenttik.url}/api/projects`, {
    data: { path: REPO, name: "Performance project" },
  })).json();
  await page.request.post(`${agenttik.url}/api/profiles`, { data: { name: "Work" } });
  await page.addInitScript((id) => {
    if (new URLSearchParams(location.search).has("profile")) return;
    const owner = `project:${id}`;
    const file = `file:${id}:web/src/store.js`;
    localStorage.setItem("agenttik.openTabs", JSON.stringify({ tabs: [
      { id: owner, kind: "project", projectID: id },
      { id: file, kind: "file", projectID: id, owner, path: "web/src/store.js", temp: false },
    ], activeTab: file, activeProjectID: id }));
  }, project.id);
  await page.reload();
  const editor = page.getByRole("textbox", { name: "web/src/store.js", exact: true });
  await expect(editor).toBeVisible();
  const profiler = process.env.PERF_PROFILE ? await page.context().newCDPSession(page) : null;
  if (profiler) {
    await profiler.send("Profiler.enable");
    await profiler.send("Profiler.start");
  }
  const fileTyping = await typingLatency(page, editor);
  if (profiler) {
    const { profile } = await profiler.send("Profiler.stop");
    writeFileSync(process.env.PERF_PROFILE, JSON.stringify(profile));
  }
  const settingsStart = performance.now();
  await sidebar(page).getByRole("button", { name: "Settings", exact: true }).click();
  await expect(page.getByRole("dialog")).toBeVisible();
  const settingsOpen = performance.now() - settingsStart;
  const closeStart = performance.now();
  await page.keyboard.press("Escape");
  await expect(page.getByRole("dialog")).toBeHidden();
  const settingsClose = performance.now() - closeStart;
  await page.getByRole("button", { name: "Profile: Default", exact: true }).click();
  const profileStart = performance.now();
  await page.getByRole("menuitem", { name: "Work", exact: true }).click();
  await expect(page.getByRole("button", { name: "Profile: Work", exact: true })).toBeVisible();
  const profileSwitch = performance.now() - profileStart;
  console.log(JSON.stringify({ fileTypingP95: fileTyping, settingsOpen, settingsClose, profileSwitch }));
});

// Real key events let the browser edit incrementally. Assigning textarea.value
// on every sample measures whole-document replacement instead of typing.
async function typingLatency(page, field) {
  await field.evaluate(el => {
    el.focus();
    el.setSelectionRange(el.value.length, el.value.length);
    window.typingSamples = [];
    let start;
    el.addEventListener("keydown", () => { start = performance.now(); });
    el.addEventListener("input", () => {
      const at = start;
      requestAnimationFrame(() => window.typingSamples.push(performance.now() - at));
    });
  });
  await page.keyboard.type("x".repeat(30), { delay: 20 });
  await page.waitForFunction(() => window.typingSamples.length === 30);
  return page.evaluate(() => window.typingSamples.sort((a, b) => a - b)[28]);
}
