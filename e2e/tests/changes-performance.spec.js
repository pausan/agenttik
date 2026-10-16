import { execFileSync } from "node:child_process";
import { writeFileSync } from "node:fs";
import { mkdir, mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { expect, inspector, sidebar, test } from "../fixtures.js";

const FILES = 16_000;
const COMMITS = 500;

// Long tasks block input; their sum is how long the window stopped answering.
async function watchLongTasks(page) {
  await page.evaluate(() => {
    window.longTasks = [];
    new PerformanceObserver((list) => window.longTasks.push(...list.getEntries().map((e) => e.duration)))
      .observe({ type: "longtask" });
  });
}
const takeLongTasks = (page) => page.evaluate(() => {
  const tasks = window.longTasks.splice(0);
  return { longestMs: Math.round(Math.max(0, ...tasks)), blockedMs: Math.round(tasks.reduce((a, b) => a + b, 0)) };
});

/* Runs act in the page, or outside it when external is given, and answers
   once done() holds on a painted frame. Measuring in the page keeps
   Playwright's own queries over a large DOM out of the numbers. */
async function timed(page, act, arg, done, external) {
  await page.evaluate(([act, arg, done]) => {
    window.timing = (async () => {
      const start = performance.now();
      new Function("arg", `(${act})(arg)`)(arg);
      while (!new Function(`return (${done})()`)()) await new Promise((r) => setTimeout(r, 10));
      await new Promise(requestAnimationFrame);
      return performance.now() - start;
    })();
  }, [String(act), arg, String(done)]);
  if (external) await external();
  return Math.round(await page.evaluate(() => window.timing));
}

async function seed(root) {
  const git = (...args) => execFileSync("git", args, { cwd: root, encoding: "utf8", maxBuffer: 64 << 20 });
  git("init", "-q", "-b", "main");
  git("config", "user.name", "Test");
  git("config", "user.email", "test@example.com");
  // One file per commit, through fast-import: 500 `git commit`s take seconds.
  let stream = "";
  for (let i = 0; i < COMMITS; i++) {
    const body = `commit ${i}\n`;
    const message = `Commit number ${i} with a subject`;
    stream += `commit refs/heads/main\ncommitter Test <test@example.com> ${1_700_000_000 + i} +0000\n`
      + `data ${message.length}\n${message}\nM 644 inline history/${i}.txt\ndata ${body.length}\n${body}\n`;
  }
  execFileSync("git", ["fast-import", "--quiet"], { cwd: root, input: stream });
  git("checkout", "-q", "main");
  // A build that writes into an unignored folder, as a game's assets can.
  for (let d = 0; d < FILES / 100; d++) {
    const dir = join(root, "build", `chunk-${d}`);
    await mkdir(dir, { recursive: true });
    await Promise.all(Array.from({ length: 100 }, (_, i) => writeFile(join(dir, `asset-${i}.bin`), "x")));
  }
}

test("16,000 changed files and 500 commits stay responsive", async ({ page, agenttik }) => {
  test.setTimeout(300_000);
  const root = await mkdtemp(join(tmpdir(), "changes-performance-"));
  try {
    await seed(root);
    // Through the API: the Add project dialog would time the same opening.
    await page.request.post(`${agenttik.url}/api/projects`, { data: { path: root } });
    await page.reload();
    await watchLongTasks(page);
    const pane = inspector(page);

    const profiler = process.env.PERF_PROFILE ? await page.context().newCDPSession(page) : null;
    if (profiler) {
      await profiler.send("Profiler.enable");
      await profiler.send("Profiler.start");
    }
    const openMs = await timed(page, (root) => {
      [...document.querySelectorAll("aside span")].find((el) => el.textContent === root).click();
    }, root, () => document.querySelector('[data-path="build/chunk-0/asset-0.bin"]'));
    if (profiler) writeFileSync(process.env.PERF_PROFILE, JSON.stringify((await profiler.send("Profiler.stop")).profile));
    await expect(pane.getByText(`Changes ${FILES}`, { exact: true })).toBeVisible();
    const open = await takeLongTasks(page);

    // The watcher answers a change with a full re-read of the list. The name
    // sorts first, so it is among the rows shown.
    const refreshMs = await timed(page, () => {}, null, () => document.querySelector('[data-path="a-new.txt"]'),
      () => writeFile(join(root, "a-new.txt"), "new\n"));
    await expect(pane.getByText(`Changes ${FILES + 1}`, { exact: true })).toBeVisible();
    const refresh = await takeLongTasks(page);

    const dom = await page.evaluate(() => document.querySelectorAll("*").length);
    if (!process.env.PERF_BASELINE) {
      const rows = pane.locator('[data-path^="build/"]:not(button)');
      await expect(rows).toHaveCount(249);
      await pane.getByRole("button", { name: `… See 250 more …`, exact: true }).click();
      await expect(rows).toHaveCount(499);
    }

    const logMs = await timed(page, () => {
      [...document.querySelectorAll('aside [role="tab"]')].find((el) => el.textContent.trim() === "Commits")
        .dispatchEvent(new MouseEvent("mousedown", { bubbles: true, button: 0 }));
    }, null, () => document.querySelector("[data-hash]"));
    await expect(pane.getByText(`Commit number ${COMMITS - 1} with a subject`, { exact: true })).toBeVisible();
    const log = await takeLongTasks(page);
    const graphMs = await timed(page, () => {
      document.querySelector('[aria-label="Show commit graph"]').click();
    }, null, () => document.querySelector("[data-hash] svg rect"));
    const graph = await takeLongTasks(page);
    const filter = pane.getByPlaceholder("Filter commits");
    const filterStart = Date.now();
    await filter.pressSequentially("number 4", { delay: 0 });
    await expect(pane.getByText(`Commit number 0 with a subject`, { exact: true })).toBeHidden();
    const filterMs = Date.now() - filterStart;
    const typing = await takeLongTasks(page);

    console.log(JSON.stringify({
      openMs, open, refreshMs, refresh, domNodes: dom,
      logMs, log, graphMs, graph, filterMs, typing,
    }));
    if (process.env.PERF_BASELINE) return;
    expect(open.longestMs).toBeLessThan(300);
    expect(refresh.longestMs).toBeLessThan(300);
    expect(log.longestMs).toBeLessThan(300);
    expect(graph.longestMs).toBeLessThan(300);
    expect(dom).toBeLessThan(20_000);
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});
