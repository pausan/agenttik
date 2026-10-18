import { execFileSync } from "node:child_process";
import { writeFileSync } from "node:fs";
import { mkdir, mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { expect, test } from "../fixtures.js";

// Words whose letters recur, as real names do: a short query then matches
// most paths for its first few keys, the costly case.
const WORDS = ["index", "button", "dialog", "render", "parser", "client", "server", "handler", "config", "utils",
  "string", "stream", "events", "loader", "runtime", "helpers", "context", "options", "request", "response"];
const AREAS = ["components", "utils", "hooks", "services", "models", "views", "tests"];
const PACKAGES = 50;
const PER_AREA = 150; // 50 × 7 × 150 = 52,500 project files
const DEPS = 1000;
const DEP_FILES = 105; // 105,000 git-ignored files under node_modules
const FILES = PACKAGES * (AREAS.length * PER_AREA + 1) + DEPS * DEP_FILES;
const QUERY = "index";
// The old ranking put node_modules first; a baseline run waits for any match.
const BEST = process.env.PERF_BASELINE ? "index" : "packages/pkg-0/src/index.ts";

const word = (i) => WORDS[i % WORDS.length];

async function seed(root) {
  execFileSync("git", ["init", "-q"], { cwd: root });
  await writeFile(join(root, ".gitignore"), "node_modules/\n");
  for (let p = 0; p < PACKAGES; p++) {
    await mkdir(join(root, "packages", `pkg-${p}`, "src"), { recursive: true });
    await writeFile(join(root, "packages", `pkg-${p}`, "src", "index.ts"), "");
    for (const area of AREAS) {
      const dir = join(root, "packages", `pkg-${p}`, "src", area);
      await mkdir(dir, { recursive: true });
      await Promise.all(Array.from({ length: PER_AREA }, (_, i) =>
        writeFile(join(dir, `${word(i)}-${word(i * 7 + 3)}-${i}.ts`), "")));
    }
  }
  for (let d = 0; d < DEPS; d++) {
    const pkg = join(root, "node_modules", `${word(d)}-${word(d * 3 + 1)}-${d}`);
    const dir = join(pkg, "dist", word(d * 5 + 2));
    await mkdir(dir, { recursive: true });
    await Promise.all([
      ...["package.json", "README.md", "LICENSE", "index.js"].map((name) => writeFile(join(pkg, name), "")),
      ...Array.from({ length: DEP_FILES - 4 }, (_, i) => writeFile(join(dir, `${word(i + d)}${word(i * 3)}.js`), "")),
    ]);
  }
}

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

// From a key going down to the next frame painted: how long the window took
// to answer the key. The message posted from the frame runs once it is on
// screen. The time of the last key is kept, for resultsAfter.
async function watchKeys(page) {
  await page.evaluate(() => {
    window.keySamples = [];
    document.addEventListener("keydown", () => {
      const start = (window.lastKey = performance.now());
      requestAnimationFrame(() => {
        const ch = new MessageChannel();
        ch.port1.onmessage = () => window.keySamples.push(performance.now() - start);
        ch.port2.postMessage(0);
      });
    }, true);
  });
}
const takeKeys = (page) => page.evaluate(() => {
  const s = window.keySamples.splice(0).sort((a, b) => a - b);
  return { p50: Math.round(s[Math.floor(s.length / 2)]), p95: Math.round(s[Math.floor(s.length * 0.95)]), max: Math.round(s.at(-1)) };
});

// From the last key to the frame showing text in the first row.
async function resultsAfter(page, text, act) {
  await page.evaluate((text) => {
    window.shown = new Promise((resolve) => {
      const check = () => {
        if (!document.querySelector('[role="option"]')?.textContent.includes(text)) return requestAnimationFrame(check);
        resolve(performance.now() - window.lastKey);
      };
      check();
    });
  }, text);
  await act();
  return Math.round(await page.evaluate(() => window.shown));
}

test("Go to file stays responsive over 157,550 files", async ({ page, agenttik }) => {
  test.setTimeout(300_000);
  const root = await mkdtemp(join(tmpdir(), "file-search-performance-"));
  try {
    await seed(root);
    await page.request.post(`${agenttik.url}/api/projects`, { data: { path: root } });
    await page.reload();
    await page.locator("aside span", { hasText: root }).first().click();
    // The tree has loaded once a file of it can be found.
    await page.keyboard.press("Control+p");
    const dialog = page.getByRole("dialog", { name: "Go to file", exact: true });
    const input = dialog.getByPlaceholder("Go to file…");
    await input.fill("pkg-49/src/views");
    await expect(dialog.getByRole("option").first()).toContainText("packages/pkg-49/src/views/");
    await input.fill("");
    await input.press("Escape");
    await expect(dialog).toBeHidden();

    await watchLongTasks(page);
    await watchKeys(page);
    const profiler = process.env.PERF_PROFILE ? await page.context().newCDPSession(page) : null;
    if (profiler) {
      await profiler.send("Profiler.enable");
      await profiler.send("Profiler.start");
    }

    // Opening shows the unfiltered list.
    await page.keyboard.press("Control+p");
    await expect(input).toBeFocused();
    await expect(dialog.getByRole("option").first()).toBeVisible();
    const open = { ...(await takeKeys(page)), ...(await takeLongTasks(page)) };

    // Typing a query one key at a time: the early keys match most paths.
    const typedMs = await resultsAfter(page, BEST, () => input.pressSequentially(QUERY, { delay: 50 }));
    const typing = { ...(await takeKeys(page)), ...(await takeLongTasks(page)), resultsMs: typedMs };

    // Deleting back to nothing, as fast as keys repeat.
    for (let i = 0; i < QUERY.length; i++) await input.press("Backspace", { delay: 0 });
    await expect(input).toHaveValue("");
    await expect(dialog.getByRole("option").first()).toBeVisible();
    const erasing = { ...(await takeKeys(page)), ...(await takeLongTasks(page)) };

    // The whole query at once, as a paste: no earlier answer to narrow.
    const pastedMs = await resultsAfter(page, BEST, async () => {
      await page.evaluate(() => (window.lastKey = performance.now()));
      await input.fill(QUERY);
    });
    const pasted = { ...(await takeLongTasks(page)), resultsMs: pastedMs };

    // A one-letter query matches nearly everything; walk down the results.
    await input.fill("");
    await input.press("s");
    for (let i = 0; i < 30; i++) await input.press("ArrowDown", { delay: 0 });
    const arrows = { ...(await takeKeys(page)), ...(await takeLongTasks(page)) };

    if (profiler) writeFileSync(process.env.PERF_PROFILE, JSON.stringify((await profiler.send("Profiler.stop")).profile));
    const domNodes = await page.evaluate(() => document.querySelectorAll("*").length);
    console.log(JSON.stringify({ files: FILES, open, typing, erasing, pasted, arrows, domNodes }));
    if (process.env.PERF_BASELINE) return;
    // Only the rows in view are drawn, yet every match can be scrolled to.
    const options = dialog.getByRole("option");
    expect(await options.count()).toBeLessThan(60);
    await dialog.getByRole("listbox").evaluate((el) => (el.scrollTop = el.scrollHeight));
    await expect(options.last()).toContainText("node_modules/");
    const matches = await dialog.getByRole("listbox").evaluate((el) => el.firstElementChild.offsetHeight / 32);
    expect(matches).toBeGreaterThan(50_000);
    await expect(options.last()).toHaveAttribute("id", `go-to-file-${matches - 1}`);
    expect(typing.p95).toBeLessThan(50);
    expect(erasing.p95).toBeLessThan(50);
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});
