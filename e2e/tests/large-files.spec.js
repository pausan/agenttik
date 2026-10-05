import { mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { addProject, expect, openProject, sidebar, test } from "../fixtures.js";

test("large files open and type within a bounded frame budget", async ({ page }) => {
  test.setTimeout(60_000);
  const dir = await mkdtemp(join(tmpdir(), "agenttik-large-files-"));
  try {
    for (const size of [12_000, 30_000]) {
      const source = Array.from({ length: size }, (_, i) => `const value${i} = ${i};`).join("\n");
      await writeFile(join(dir, `large-${size}.js`), source);
    }
    await addProject(page, dir);
    await openProject(page, dir);
    await sidebar(page).getByRole("tab", { name: "Tree" }).click();
    for (const size of [12_000, 30_000]) {
      const path = `large-${size}.js`;
      const open = await sidebar(page).getByRole("button", { name: path, exact: true }).evaluate(async (el, path) => {
        const start = performance.now();
        el.click();
        while (!document.querySelector(`textarea[aria-label="${path}"]`)) await new Promise(requestAnimationFrame);
        await new Promise(requestAnimationFrame);
        return performance.now() - start;
      }, path);
      const editor = page.getByRole("textbox", { name: path, exact: true });
      await editor.evaluate(el => {
        el.focus();
        el.setSelectionRange(el.value.length, el.value.length);
        window.largeFileSamples = [];
        window.largeTypingController?.abort();
        window.largeTypingController = new AbortController();
        const options = { signal: window.largeTypingController.signal };
        let start;
        el.addEventListener("keydown", () => { start = performance.now(); }, options);
        el.addEventListener("input", () => {
          const at = start;
          requestAnimationFrame(() => window.largeFileSamples.push(performance.now() - at));
        }, options);
      });
      await page.keyboard.type("x".repeat(30), { delay: 20 });
      await page.waitForFunction(() => window.largeFileSamples.length === 30);
      const p95 = await page.evaluate(() => window.largeFileSamples.sort((a, b) => a - b)[28]);
      console.log(JSON.stringify({ lines: size, open, typingP95: p95 }));
      if (process.env.PERF_BASELINE) continue;
      expect(open).toBeLessThan(1000);
      expect(p95).toBeLessThan(50);
      await expect(page.locator(".editor pre")).toHaveCount(0);
      await editor.press("Control+z");
      await expect(editor).toHaveValue(/x{29}$/);
      await editor.press("Control+y");
      await expect(editor).toHaveValue(/x{30}$/);
      await page.keyboard.press("Control+f");
      const find = page.getByRole("search", { name: "Find in current page" });
      await find.getByRole("textbox").fill(`value${size - 10} =`);
      await expect(find.getByRole("status")).toHaveText("1 of 1");
      expect(await editor.evaluate(el => el.scrollTop)).toBeGreaterThan(100_000);
      await find.getByRole("button", { name: "Close find" }).click();
      await page.getByRole("button", { name: "Save", exact: true }).click();
      await expect(page.getByLabel("Unsaved changes", { exact: true })).toHaveCount(0);
    }
  } finally {
    await rm(dir, { recursive: true, force: true });
  }
});

test("large editors retain scroll positions and reveal linked lines", async ({ page }) => {
  test.skip(!!process.env.PERF_BASELINE, "Requires the native large editor.");
  const dir = await mkdtemp(join(tmpdir(), "agenttik-large-navigation-"));
  try {
    await writeFile(join(dir, "large.js"), Array.from({ length: 12_000 }, (_, i) => `const value${i} = ${i};`).join("\n"));
    await writeFile(join(dir, "links.md"), "[Deep line](large.js:11990)\n");
    await addProject(page, dir);
    await openProject(page, dir);
    const tree = sidebar(page);
    const tab = (name) => page.locator("main").getByRole("tab", { name, exact: true });
    await tree.getByRole("tab", { name: "Tree" }).click();
    await tree.getByRole("button", { name: "large.js", exact: true }).dblclick();
    const editor = page.getByRole("textbox", { name: "large.js", exact: true });
    await editor.evaluate(el => { el.scrollTop = 5000; });
    await tree.getByRole("button", { name: "links.md", exact: true }).dblclick();
    await tab("large.js").click();
    await expect.poll(() => editor.evaluate(el => el.scrollTop)).toBe(5000);
    await tab("Diff").click();
    await tab("Edit").click();
    await expect.poll(() => editor.evaluate(el => el.scrollTop)).toBe(5000);
    await tab("links.md").click();
    await tab("Preview").click();
    await page.getByRole("button", { name: "Deep line", exact: true }).click();
    await expect.poll(() => editor.evaluate(el => el.value.slice(el.selectionStart, el.selectionEnd))).toBe("const value11989 = 11989;");
    expect(await editor.evaluate(el => el.scrollTop)).toBeGreaterThan(100_000);
  } finally {
    await rm(dir, { recursive: true, force: true });
  }
});

test("truncated text stays read-only when Tab is pressed", async ({ page }) => {
  test.skip(!!process.env.PERF_BASELINE, "Requires the read-only keyboard guard.");
  const dir = await mkdtemp(join(tmpdir(), "agenttik-large-readonly-"));
  try {
    await writeFile(join(dir, "readonly.txt"), "line content\n".repeat(170_000));
    await addProject(page, dir);
    await openProject(page, dir);
    await sidebar(page).getByRole("tab", { name: "Tree" }).click();
    await sidebar(page).getByRole("button", { name: "readonly.txt", exact: true }).click();
    const editor = page.getByRole("textbox", { name: "readonly.txt", exact: true });
    await expect(editor).toHaveAttribute("readonly", "");
    const before = await editor.inputValue();
    await editor.focus();
    await editor.press("Tab");
    expect(await editor.inputValue()).toBe(before);
    await expect(page.getByRole("button", { name: "Save", exact: true })).toHaveCount(0);
  } finally {
    await rm(dir, { recursive: true, force: true });
  }
});
