import { mkdir, mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { addProject, expect, openProject, test } from "../fixtures.js";

test("closing Go to file releases its worker before reopening", async ({ page }) => {
  await addProject(page);
  const workers = () => page.workers().filter((worker) => worker.url().includes("file-search-worker"));
  const dialog = page.getByRole("dialog", { name: "Go to file", exact: true });
  for (let i = 0; i < 5; i++) {
    await page.keyboard.press("Control+p");
    const input = dialog.getByPlaceholder("Go to file…");
    await expect(input).toBeFocused();
    await input.fill("AGENTS.md");
    await expect(dialog.getByRole("option").first()).toContainText("AGENTS.md");
    await expect.poll(() => workers().length).toBe(1);
    await input.press(i % 2 ? "Enter" : "Escape");
    await expect(dialog).toBeHidden();
    await expect.poll(() => workers().length).toBe(0);
  }
});

test("file search opens fuzzy matches in Edit and media in Preview", async ({ page }) => {
  const root = await mkdtemp(join(tmpdir(), "agenttik-file-search-"));
  const other = await mkdtemp(join(tmpdir(), "agenttik-file-other-"));
  try {
    await mkdir(join(root, "src"));
    await writeFile(join(root, "src/readme.md"), "# Search example\n");
    await writeFile(join(root, "picture.png"), Buffer.from("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jRZkAAAAASUVORK5CYII=", "base64"));
    await writeFile(join(other, "other.txt"), "other project");
    await addProject(page, root);
    await addProject(page, other);
    await openProject(page, root);
    const search = async (query, path) => {
      await page.keyboard.press("Control+p");
      const dialog = page.getByRole("dialog", { name: "Go to file", exact: true });
      const input = dialog.getByPlaceholder("Go to file…");
      await expect(input).toBeFocused();
      await input.fill(query);
      await expect(dialog.getByRole("option")).toHaveCount(1);
      await expect(dialog.getByRole("option")).toContainText(path);
      await input.press("Enter");
      await expect(dialog).toBeHidden();
    };
    await search("srdm", "src/readme.md");
    const edit = page.getByRole("tab", { name: "Edit", exact: true });
    const preview = page.getByRole("tab", { name: "Preview", exact: true });
    await expect(edit).toHaveAttribute("aria-selected", "true");
    await preview.click();
    await search("srdm", "src/readme.md");
    await expect(edit).toHaveAttribute("aria-selected", "true");
    await page.getByRole("tab", { name: "Diff", exact: true }).click();
    await search("pctpng", "picture.png");
    await expect(preview).toHaveAttribute("aria-selected", "true");
    await expect(edit).toHaveCount(0);
    await page.keyboard.press("Control+p");
    const input = page.getByPlaceholder("Go to file…");
    await input.fill("other");
    await expect(page.getByText("No files found.", { exact: true })).toBeVisible();
    await input.press("Escape");
    await page.keyboard.press("Control+Shift+p");
    await expect(page.getByRole("dialog", { name: "Command Palette", exact: true })).toBeVisible();
  } finally {
    await rm(root, { recursive: true, force: true });
    await rm(other, { recursive: true, force: true });
  }
});
