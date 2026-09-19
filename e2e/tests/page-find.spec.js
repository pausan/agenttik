import { mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { addProject, expect, openProject, sidebar, test } from "../fixtures.js";

test("find searches unsaved file text, wraps, scrolls, and closes on view changes", async ({ page }) => {
  const dir = await mkdtemp(join(tmpdir(), "agenttik-find-"));
  try {
    await writeFile(join(dir, "find.txt"), "Needle\n" + "filler\n".repeat(200) + "needle");
    await addProject(page, dir);
    await openProject(page, dir);
    await sidebar(page).getByRole("tab", { name: "Tree" }).click();
    await sidebar(page).getByRole("button", { name: "find.txt", exact: true }).click();
    const editor = page.getByRole("textbox", { name: "find.txt", exact: true });
    await editor.click();
    await page.keyboard.press("Control+f");
    const find = page.getByRole("textbox", { name: "Find in current page", exact: true });
    await expect(find).toBeFocused();
    await find.fill("needle");
    const status = page.getByRole("search").getByRole("status");
    await expect(status).toHaveText("1 of 2");
    await find.press("Enter");
    await expect(status).toHaveText("2 of 2");
    expect(await page.locator(".editor").evaluate(el => el.parentElement.scrollTop)).toBeGreaterThan(1000);
    await find.press("Enter");
    await expect(status).toHaveText("1 of 2");
    await find.press("Shift+Enter");
    await expect(status).toHaveText("2 of 2");
    await find.press("ArrowDown");
    await expect(status).toHaveText("1 of 2");
    await find.press("ArrowUp");
    await expect(status).toHaveText("2 of 2");
    await find.press("ArrowUp");
    await expect(status).toHaveText("1 of 2");
    await find.press("ArrowDown");
    await expect(status).toHaveText("2 of 2");
    await expect(find).toBeFocused();
    await expect(find).toHaveValue("needle");
    await find.fill("find.txt");
    await expect(status).toHaveText("No matches");
    await find.fill("not present");
    await expect(status).toHaveText("No matches");
    await find.press("ArrowUp");
    await find.press("ArrowDown");
    await expect(status).toHaveText("No matches");
    await find.press("Escape");
    await expect(find).toBeHidden();
    await expect(editor).toBeFocused();
    await editor.fill("unsaved unique match");
    await page.keyboard.press("Control+f");
    await find.fill("unique");
    await expect(status).toHaveText("1 of 1");
    await page.getByRole("tab", { name: "Diff", exact: true }).click();
    await expect(find).toBeHidden();
  } finally {
    await rm(dir, { recursive: true, force: true });
  }
});

test("page find matches rendered text across inline markup and excludes the sidebar", async ({ page }) => {
  const dir = await mkdtemp(join(tmpdir(), "agenttik-find-page-"));
  try {
    await writeFile(join(dir, "find.md"), "A **special** phrase.\n\nA special phrase.");
    await addProject(page, dir);
    await openProject(page, dir);
    await sidebar(page).getByRole("tab", { name: "Tree" }).click();
    await sidebar(page).getByRole("button", { name: "find.md", exact: true }).click();
    await page.getByRole("tab", { name: "Preview", exact: true }).click();
    await page.keyboard.press("Control+f");
    const find = page.getByRole("textbox", { name: "Find in current page", exact: true });
    await find.fill("special phrase");
    await expect(page.getByRole("search").getByRole("status")).toHaveText("1 of 2");
    // Older desktop webviews use a selection instead of CSS Highlights.
    await page.evaluate(() => Object.defineProperty(CSS, "highlights", { value: undefined, configurable: true }));
    await find.fill("phrase");
    await expect.poll(() => page.evaluate(() => window.getSelection().toString())).toBe("phrase");
    await find.press("Escape");
    await expect.poll(() => page.evaluate(() => window.getSelection().toString())).toBe("");
    await page.keyboard.press("Control+f");
    await find.fill("Projects");
    await expect(page.getByRole("search").getByRole("status")).toHaveText("No matches");
  } finally {
    await rm(dir, { recursive: true, force: true });
  }
});

test("editor overlay follows wrapped text, trailing lines and viewport resizing", async ({ page }) => {
  const dir = await mkdtemp(join(tmpdir(), "agenttik-editor-layout-"));
  try {
    const source = 'const text = "' + "wrapped text ".repeat(60) + '";\n\n';
    await writeFile(join(dir, "wrap.js"), source);
    await addProject(page, dir);
    await openProject(page, dir);
    await sidebar(page).getByRole("tab", { name: "Tree" }).click();
    await sidebar(page).getByRole("button", { name: "wrap.js", exact: true }).click();
    const editor = page.getByRole("textbox", { name: "wrap.js", exact: true });
    const aligned = async () => {
      const sizes = await editor.evaluate(el => ({
        field: el.getBoundingClientRect().height,
        layer: el.previousElementSibling.getBoundingClientRect().height,
        extra: el.scrollHeight - el.clientHeight,
      }));
      expect(Math.abs(sizes.field - sizes.layer)).toBeLessThan(2);
      expect(sizes.extra).toBeLessThan(2);
    };
    await expect(editor).toHaveValue(source);
    await aligned();
    await editor.focus();
    await editor.press("Control+End");
    await editor.press("Enter");
    await editor.press("Tab");
    await expect(editor).toHaveValue(source + "\n  ");
    await aligned();
    await editor.press("Control+z");
    await expect(editor).toHaveValue(source + "\n");
    await editor.press("Control+y");
    await expect(editor).toHaveValue(source + "\n  ");
    await page.setViewportSize({ width: 800, height: 700 });
    await aligned();
    await editor.fill("");
    await aligned();
  } finally {
    await rm(dir, { recursive: true, force: true });
  }
});
