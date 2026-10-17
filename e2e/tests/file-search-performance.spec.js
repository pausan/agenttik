import { execFileSync } from "node:child_process";
import { writeFileSync } from "node:fs";
import { mkdir, mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { expect, test } from "../fixtures.js";

const PACKAGES = 50;
const AREAS = ["components", "utils", "hooks", "services", "models", "views", "tests"];
const NAMES = ["button", "dialog", "user", "account", "table", "chart", "form", "input", "menu", "modal"];
const PER_NAME = 15;
const DEPS = 300;
const DEP_FILES = 350;
// The project's own files, then a git-ignored node_modules with long paths,
// which the listing carries too.
const FILES = PACKAGES * AREAS.length * NAMES.length * PER_NAME + DEPS * DEP_FILES; // 157,500
const QUERY = "pkg7compbutton";

async function seed(root) {
  execFileSync("git", ["init", "-q"], { cwd: root });
  await writeFile(join(root, ".gitignore"), "node_modules/\n");
  for (let p = 0; p < PACKAGES; p++) {
    for (const area of AREAS) {
      const dir = join(root, "packages", `pkg-${p}`, "src", area);
      await mkdir(dir, { recursive: true });
      const names = NAMES.flatMap((name) => Array.from({ length: PER_NAME }, (_, i) => `${name}-${i}.ts`));
      await Promise.all(names.map((name) => writeFile(join(dir, name), "")));
    }
  }
  for (let d = 0; d < DEPS; d++) {
    const dir = join(root, "node_modules", ".pnpm", `@scope+library-${d}@1.${d}.0`, "node_modules", "@scope", `library-${d}`, "dist", "esm");
    await mkdir(dir, { recursive: true });
    await Promise.all(Array.from({ length: DEP_FILES }, (_, i) => writeFile(join(dir, `${NAMES[i % NAMES.length]}-${i}.js`), "")));
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

// From a key going down to the frame after the list answered it, painted:
// the message posted from the frame runs once the frame is on screen.
async function watchKeys(page) {
  await page.evaluate(() => {
    window.keySamples = [];
    document.addEventListener("keydown", () => {
      const start = performance.now();
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

test("Go to file stays responsive over 157,500 files", async ({ page, agenttik }) => {
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
    await input.fill("pkg-49/src/views/modal-14");
    await expect(dialog.getByRole("option").first()).toContainText("packages/pkg-49/src/views/modal-14.ts");
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

    // Typing a query one key at a time: the early keys match nearly every path.
    await input.pressSequentially(QUERY, { delay: 50 });
    await expect(dialog.getByRole("option").first()).toContainText("packages/pkg-7/src/components/button-");
    const typing = { ...(await takeKeys(page)), ...(await takeLongTasks(page)) };

    // Deleting back to nothing, as fast as keys repeat.
    for (let i = 0; i < QUERY.length; i++) await input.press("Backspace", { delay: 0 });
    await expect(input).toHaveValue("");
    await expect(dialog.getByRole("option").first()).toBeVisible();
    const erasing = { ...(await takeKeys(page)), ...(await takeLongTasks(page)) };

    // A one-letter query matches nearly everything; walk down the results.
    await input.press("s");
    for (let i = 0; i < 30; i++) await input.press("ArrowDown", { delay: 0 });
    const arrows = { ...(await takeKeys(page)), ...(await takeLongTasks(page)) };

    if (profiler) {
      writeFileSync(process.env.PERF_PROFILE, JSON.stringify((await profiler.send("Profiler.stop")).profile));
    }
    const domNodes = await page.evaluate(() => document.querySelectorAll("*").length);
    console.log(JSON.stringify({ files: FILES, open, typing, erasing, arrows, domNodes }));
    if (process.env.PERF_BASELINE) return;
    // Only the rows in view are drawn, yet every match can be scrolled to.
    const options = dialog.getByRole("option");
    expect(await options.count()).toBeLessThan(60);
    await dialog.getByRole("listbox").evaluate((el) => (el.scrollTop = el.scrollHeight));
    await expect(options.last()).toContainText("node_modules/");
    expect(Number((await options.last().getAttribute("id")).split("-").pop())).toBeGreaterThan(100_000);
    expect(typing.p95).toBeLessThan(50);
    expect(erasing.p95).toBeLessThan(50);
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});
