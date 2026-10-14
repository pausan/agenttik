import { execFileSync } from "node:child_process";
import { join } from "node:path";
import { expect, REPO, sidebar, test } from "../fixtures.js";

async function seed(page, agenttik, count) {
  const project = await (await page.request.post(`${agenttik.url}/api/projects`, {
    data: { path: REPO, name: "Transcript performance" },
  })).json();
  execFileSync("python3", ["-c", `
import sqlite3, sys, time
db = sqlite3.connect(sys.argv[1])
now = int(time.time() * 1000)
count = int(sys.argv[3])
with db:
    db.execute("INSERT INTO sessions (id, project_id, title, provider, model, created_at, updated_at, last_active_at) VALUES ('long-transcript', ?, 'Long conversation', 'fake', 'fake-quick', ?, ?, ?)", (int(sys.argv[2]), now, now, now))
    db.executemany("INSERT INTO turns (id, session_id, model, started_at, ended_at, input_tokens, output_tokens, status) VALUES (?, 'long-transcript', 'fake-quick', ?, ?, 100, 50, 'done')",
        [(i+1, now+i*1000, now+(i+1)*1000) for i in range(count//2)])
    db.executemany("INSERT INTO messages (id, session_id, turn_id, role, content, created_at) VALUES (?, 'long-transcript', ?, ?, ?, ?)",
        [(i+1, i//2+1, 'user' if i%2 == 0 else 'assistant',
          f'Prompt {i}' if i%2 == 0 else f'Reply {i}: A **special** phrase.\\n\\n' + 'A paragraph with readable content. '*30 + '\\n\\n- First item\\n- Second item', now)
         for i in range(count)])
`, join(agenttik.dataDir, "agenttik.db"), String(project.id), String(count)]);
  await page.addInitScript((id) => localStorage.setItem("agenttik.openTabs", JSON.stringify({
    tabs: [{ id: `project:${id}`, kind: "project", projectID: id }],
    activeTab: `project:${id}`, activeProjectID: id,
  })), project.id);
  await page.reload();
  await expect(sidebar(page).getByText("Long conversation", { exact: true })).toBeVisible();
}

test("5,000 messages retain only nearby bodies and open within budget", async ({ page, agenttik }) => {
  test.setTimeout(90_000);
  await seed(page, agenttik, 5000);
  const cdp = await page.context().newCDPSession(page);
  await cdp.send("HeapProfiler.collectGarbage");
  const before = await cdp.send("Runtime.getHeapUsage");
  const open = await sidebar(page).getByText("Long conversation", { exact: true }).evaluate(async el => {
    const start = performance.now();
    window.transcriptOpenedAt = start;
    el.click();
    while (!document.querySelector('[data-message-id="5000"]')) await new Promise(requestAnimationFrame);
    await new Promise(requestAnimationFrame);
    return performance.now() - start;
  });
  const bodies = page.locator('main [data-message-id]');
  if (process.env.PERF_BASELINE) await expect(bodies).toHaveCount(5000, { timeout: 60_000 });
  else {
    await expect(page.locator('[data-transcript-row]')).toHaveCount(5000);
    await expect.poll(() => bodies.count()).toBeLessThan(80);
  }
  await page.waitForTimeout(500);
  const settled = await page.evaluate(() => performance.now() - window.transcriptOpenedAt);
  await cdp.send("HeapProfiler.collectGarbage");
  const heap = await cdp.send("Runtime.getHeapUsage");
  const dom = await cdp.send("Memory.getDOMCounters");
  console.log(JSON.stringify({ openMs: Math.round(open), settledMs: Math.round(settled), bodies: await bodies.count(), domNodes: dom.nodes,
    retainedHeapMB: +((heap.usedSize - before.usedSize) / 1024 / 1024).toFixed(2) }));
  if (process.env.PERF_BASELINE) return;
  expect(open).toBeLessThan(2000);
  expect(heap.usedSize - before.usedSize).toBeLessThan(50 * 1024 * 1024);
  expect(await bodies.count()).toBeLessThan(80);
  const transcript = page.locator('[data-transcript]');
  await expect.poll(() => transcript.evaluate(el => el.scrollHeight - el.clientHeight - el.scrollTop)).toBeLessThan(2);
  await transcript.evaluate(el => { el.scrollTop = 0; });
  await expect(page.locator('[data-message-id="1"]')).toBeVisible();
  await expect(page.locator('[data-message-id="5000"]')).toHaveCount(0);
  await expect.poll(() => bodies.count()).toBeLessThan(80);
  await transcript.evaluate(el => { el.scrollTop = el.scrollHeight; });
  await expect(page.locator('[data-message-id="5000"]')).toBeVisible();
  await expect(page.locator('[data-message-id="1"]')).toHaveCount(0);
});

test("deferred history supports navigation, editing and full rendered-text search", async ({ page, agenttik }) => {
  await seed(page, agenttik, 240);
  await sidebar(page).getByText("Long conversation", { exact: true }).click();
  const transcript = page.locator('[data-transcript]');
  const bodies = page.locator('main [data-message-id]');
  await expect(page.locator('[data-message-id="240"]')).toBeVisible();
  await page.getByRole("button", { name: "Previous human message", exact: true }).click();
  await expect(page.locator('[data-message-id="239"]')).toBeVisible();
  await transcript.evaluate(el => { el.scrollTop = 0; });
  const prompt = page.locator('[data-message-id="1"]');
  await expect(prompt).toBeVisible();
  await page.setViewportSize({ width: 390, height: 844 });
  await expect(prompt).toBeVisible();
  await prompt.getByRole("button", { name: "Edit this prompt" }).click();
  const editor = prompt.getByRole("textbox");
  await editor.fill("Keep this edit while reading elsewhere");
  await page.getByPlaceholder("Ask the agent…").focus();
  await transcript.evaluate(el => { el.scrollTop = el.scrollHeight; });
  await expect(editor).toHaveValue("Keep this edit while reading elsewhere");
  await transcript.evaluate(el => { el.scrollTop = 0; });
  await expect(editor).toBeVisible();
  await prompt.getByRole("button", { name: "Cancel", exact: true }).click();
  await page.getByPlaceholder("Ask the agent…").focus();
  await page.keyboard.press("Control+f");
  const find = page.getByRole("textbox", { name: "Find in current page", exact: true });
  await find.fill("special phrase");
  const status = page.getByRole("search").getByRole("status");
  await expect(status).toHaveText("1 of 120");
  await find.press("Shift+Enter");
  await expect(status).toHaveText("120 of 120");
  await find.press("Escape");
  await expect.poll(() => bodies.count()).toBeLessThan(80);
  await expect(page.locator('[data-message-id="240"]')).toBeVisible();
});

test("expanded tools keep their state until collapsed, then leave the viewport", async ({ page, agenttik }) => {
  await seed(page, agenttik, 240);
  await page.route(/\/api\/sessions\/long-transcript$/, async route => {
    const response = await route.fetch();
    const detail = await response.json();
    for (let i = 230; i < 233; i++) Object.assign(detail.messages[i], { role: "tool", content: `Tool step ${i}` });
    await route.fulfill({ response, json: detail });
  });
  await sidebar(page).getByText("Long conversation", { exact: true }).click();
  const transcript = page.locator('[data-transcript]');
  await expect(page.locator('[data-message-id="240"]')).toBeVisible();
  await transcript.evaluate(el => {
    el.scrollTop += el.querySelector('[data-transcript-row="230"]').getBoundingClientRect().top - el.getBoundingClientRect().top;
  });
  const tools = page.getByRole("button", { name: "3 tool calls", exact: true });
  await tools.click();
  await page.getByPlaceholder("Ask the agent…").focus();
  await transcript.evaluate(el => { el.scrollTop = 0; });
  await expect(page.locator('[data-message-id="231"]')).toHaveCount(1);
  await expect(tools).toHaveAttribute("aria-expanded", "true");
  // Close an offscreen control without scrolling it into view. Its own state
  // changes, while the transcript's rows and visibility set stay unchanged.
  await tools.evaluate(el => el.click());
  await expect(page.locator('[data-message-id="233"]')).toHaveCount(0);
  await expect(tools).toHaveCount(0);
});
