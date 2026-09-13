import { mkdir, mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";

import { addProject, expect, openProject, sidebar, test } from "../fixtures.js";

test("Tree remembers expansion and supports recursive expansion, arrows and filtering", async ({ page }) => {
  const root = await mkdtemp(join(tmpdir(), "agenttik-tree-"));
  const other = await mkdtemp(join(tmpdir(), "agenttik-tree-other-"));
  try {
    for (const dir of [root, other]) {
      await mkdir(join(dir, "src/nested"), { recursive: true });
      await writeFile(join(dir, "src/nested/a.txt"), "a\n");
      await writeFile(join(dir, "src/nested/b.txt"), "b\n");
    }
    await addProject(page, root);
    await addProject(page, other);
    await openProject(page, root);
    const tree = sidebar(page);
    const projectRows = await tree.locator(".sidebar-project").allTextContents();
    const projectKey = (path) => "Alt+" + String.fromCharCode(65 + projectRows.findIndex((text) => text.includes(path)));
    const showTree = () => tree.getByRole("tab", { name: "Tree", exact: true }).click();
    const row = (name) => tree.getByRole("button", { name, exact: true });
    await showTree();
    await expect(row("src")).toHaveAttribute("aria-expanded", "false");
    await expect(row("nested")).toHaveCount(0);
    await row("src").click();
    await expect(row("nested")).toHaveAttribute("aria-expanded", "false");
    await row("src").click({ button: "right" });
    await page.getByRole("menuitem", { name: "Expand recursively", exact: true }).click();
    await row("a.txt").click();
    await expect(row("a.txt")).toHaveAttribute("aria-current", "true");
    await expect(row("a.txt")).toBeFocused();
    await page.keyboard.press("ArrowDown");
    await expect(row("b.txt")).toBeFocused();
    await expect(row("b.txt")).toHaveAttribute("aria-current", "true");
    await page.keyboard.press("ArrowUp");
    await expect(row("a.txt")).toBeFocused();
    await page.keyboard.press("ArrowLeft");
    await expect(row("nested")).toBeFocused();
    await page.keyboard.press("ArrowLeft");
    await expect(row("nested")).toHaveAttribute("aria-expanded", "false");
    await page.keyboard.press("ArrowRight");
    await expect(row("nested")).toHaveAttribute("aria-expanded", "true");
    await page.keyboard.press("Control+f");
    const filter = tree.getByPlaceholder("Filter files");
    await expect(filter).toBeFocused();
    await filter.fill("b.txt");
    await expect(row("a.txt")).toHaveCount(0);
    await filter.press("Escape");
    await expect(filter).toHaveValue("");
    await expect(row("a.txt")).toBeVisible();

    await tree.getByRole("tab", { name: "Projects", exact: true }).click();
    await openProject(page, other);
    await showTree();
    await expect(row("src")).toHaveAttribute("aria-expanded", "false");
    await tree.getByRole("tab", { name: "Projects", exact: true }).click();
    await openProject(page, root);
    await showTree();
    await expect(row("a.txt")).toBeVisible();
    // Switch projects while the Tree component stays mounted.
    await page.keyboard.press(projectKey(other));
    await expect(row("src")).toHaveAttribute("aria-expanded", "false");
    await page.keyboard.press(projectKey(root));
    await expect(row("a.txt")).toBeVisible();
    await page.reload();
    await showTree();
    await expect(row("a.txt")).toBeVisible();
    await row("src").click();
    await page.reload();
    await showTree();
    await expect(row("src")).toHaveAttribute("aria-expanded", "false");
    await filter.fill("a.txt");
    await expect(row("a.txt")).toBeVisible();
    await filter.press("Escape");
    await expect(row("src")).toHaveAttribute("aria-expanded", "false");
    await filter.fill("a.txt");
    await row("src").click();
    await expect(row("src")).toHaveAttribute("aria-expanded", "false");
    await row("src").click();
    await filter.press("Escape");
    await expect(row("src")).toHaveAttribute("aria-expanded", "true");
    await page.reload();
    await showTree();
    await expect(row("src")).toHaveAttribute("aria-expanded", "true");
  } finally {
    await rm(root, { recursive: true, force: true });
    await rm(other, { recursive: true, force: true });
  }
});
