import { mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { addProject, expect, openProject, sidebar, test } from "../fixtures.js";

test("file header shows byte size and image dimensions and depth", async ({ page }) => {
  const dir = await mkdtemp(join(tmpdir(), "agenttik-file-info-"));
  try {
    await writeFile(join(dir, "readme.txt"), "x".repeat(32400));
    await writeFile(join(dir, "empty.txt"), "");
    await writeFile(join(dir, "vector.svg"), '<svg xmlns="http://www.w3.org/2000/svg" width="32" height="40"></svg>');
    // A real browser-encoded PNG exercises both metadata and rendering.
    const png = await page.evaluate(() => {
      const canvas = document.createElement("canvas");
      canvas.width = 320;
      canvas.height = 420;
      return canvas.toDataURL("image/png").split(",")[1];
    });
    await writeFile(join(dir, "photo.png"), Buffer.from(png, "base64"));
    await addProject(page, dir);
    await openProject(page, dir);
    await sidebar(page).getByRole("tab", { name: "Tree" }).click();
    const info = page.getByLabel("File information", { exact: true });
    await sidebar(page).getByRole("button", { name: "readme.txt", exact: true }).click();
    await expect(info).toHaveText("(32.4 KB)");
    await sidebar(page).getByRole("button", { name: "photo.png", exact: true }).click();
    await expect(info).toHaveText(/^\(\d+\.\d KB, 320x420 32-bit\)$/);
    await page.setViewportSize({ width: 375, height: 800 });
    await expect(info).toBeInViewport({ ratio: 1 });
    await expect(page.getByRole("tab", { name: "Preview", exact: true })).toBeInViewport({ ratio: 1 });
    await page.setViewportSize({ width: 1440, height: 900 });
    await sidebar(page).getByRole("tab", { name: "Tree" }).click();
    await page.getByRole("tab", { name: "Diff", exact: true }).click();
    await expect(info).toHaveText(/^\(\d+\.\d KB, 320x420 32-bit\)$/);
    await sidebar(page).getByRole("button", { name: "empty.txt", exact: true }).click();
    await expect(info).toHaveText("(0.0 B)");
    await page.getByRole("tab", { name: "Edit", exact: true }).click();
    const editor = page.getByRole("textbox", { name: "empty.txt", exact: true });
    await editor.fill("é🎨");
    await expect(info).toHaveText("(0.0 B)");
    await page.getByRole("button", { name: "Save", exact: true }).click();
    await expect(info).toHaveText("(6.0 B)");
    await sidebar(page).getByRole("button", { name: "vector.svg", exact: true }).click();
    await page.getByRole("tab", { name: "Preview", exact: true }).click();
    await expect(info).toHaveText(/^\(\d+\.\d B, 32x40\)$/);
  } finally {
    await rm(dir, { recursive: true, force: true });
  }
});
