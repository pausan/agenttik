import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { addProject, expect, openProject, sidebar, test } from "../fixtures.js";

const png = Buffer.from("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jRZkAAAAASUVORK5CYII=", "base64");

test("binary tabs page, edit, undo and save exact bytes", async ({ page }) => {
  const dir = await mkdtemp(join(tmpdir(), "agenttik-hex-"));
  const original = Buffer.alloc(1027, 65);
  original[0] = 0;
  original[512] = 255;
  try {
    await writeFile(join(dir, "data.bin"), original);
    await writeFile(join(dir, "other.txt"), "other\n");
    await addProject(page, dir);
    await openProject(page, dir);
    const tree = sidebar(page);
    await tree.getByRole("tab", { name: "Tree" }).click();
    await tree.getByRole("button", { name: "data.bin", exact: true }).click();
    await expect(page.getByRole("tab", { name: "Hex", exact: true })).toHaveAttribute("aria-selected", "true");
    await expect(page.getByRole("tab", { name: "Edit", exact: true })).toHaveCount(0);
    const view = page.getByLabel("Hex editor", { exact: true });
    await expect(view.getByRole("button", { name: /^Byte / })).toHaveCount(512);
    await view.getByRole("button", { name: "Next page" }).click();
    await expect(view.getByRole("button", { name: "Byte 00000200: ff", exact: true })).toHaveAttribute("aria-pressed", "true");
    await view.getByLabel("Replace bytes").fill("0g");
    await view.getByRole("button", { name: "Apply bytes" }).click();
    await expect(view.getByRole("alert")).toContainText("complete hex bytes");
    await expect(page.getByRole("button", { name: "Save", exact: true })).toBeDisabled();
    await view.getByLabel("Replace bytes").fill("00 FE 80");
    await view.getByRole("button", { name: "Apply bytes" }).click();
    await expect(view.getByRole("button", { name: "Byte 00000200: 00", exact: true })).toBeVisible();
    await expect(page.getByLabel("Unsaved changes", { exact: true })).toBeVisible();
    await view.getByRole("button", { name: "Undo", exact: true }).click();
    await expect(view.getByRole("button", { name: "Byte 00000200: ff", exact: true })).toBeVisible();
    await expect(page.getByLabel("Unsaved changes", { exact: true })).toHaveCount(0);
    await view.getByRole("button", { name: "Redo", exact: true }).click();
    await tree.getByRole("button", { name: "other.txt", exact: true }).click();
    await tree.getByRole("button", { name: "data.bin", exact: true }).click();
    await expect(view.getByRole("button", { name: "Byte 00000200: 00", exact: true })).toBeVisible();
    await page.keyboard.press("Control+s");
    await expect(page.getByLabel("Unsaved changes", { exact: true })).toHaveCount(0);
    const expected = Buffer.from(original);
    expected.set([0, 254, 128], 512);
    expect(await readFile(join(dir, "data.bin"))).toEqual(expected);
    await view.getByLabel("Offset (hex)").fill("402");
    await view.getByRole("button", { name: "Go", exact: true }).click();
    await expect(view.getByRole("button", { name: /^Byte / })).toHaveCount(3);
    await view.getByLabel("Replace bytes").fill("FF 00");
    await view.getByRole("button", { name: "Apply bytes" }).click();
    await expect(view.getByRole("alert")).toContainText("fit in the file");
    await view.getByLabel("Replace bytes").fill("FF");
    await view.getByRole("button", { name: "Apply bytes" }).click();
    await page.getByRole("button", { name: "Save", exact: true }).click();
    await expect(page.getByLabel("Unsaved changes", { exact: true })).toHaveCount(0);
    expected[1026] = 255;
    expect(await readFile(join(dir, "data.bin"))).toEqual(expected);
  } finally {
    await rm(dir, { recursive: true, force: true });
  }
});

test("large binaries are bounded and read-only; image previews also offer Hex", async ({ page }) => {
  const dir = await mkdtemp(join(tmpdir(), "agenttik-hex-large-"));
  try {
    await writeFile(join(dir, "large.bin"), Buffer.alloc(3 * 1024 * 1024, 128));
    await writeFile(join(dir, "picture.png"), png);
    await addProject(page, dir);
    await openProject(page, dir);
    const tree = sidebar(page);
    await tree.getByRole("tab", { name: "Tree" }).click();
    await tree.getByRole("button", { name: "large.bin", exact: true }).click();
    const view = page.getByLabel("Hex editor", { exact: true });
    await expect(view.getByText(/Showing the first .* bytes/)).toBeVisible();
    await expect(view.getByRole("button", { name: /^Byte / })).toHaveCount(512);
    await expect(page.getByRole("button", { name: "Save", exact: true })).toHaveCount(0);
    await expect(view.getByLabel("Replace bytes")).toHaveCount(0);
    await view.getByLabel("Offset (hex)").fill("1fffff");
    await view.getByRole("button", { name: "Go", exact: true }).click();
    await expect(view.getByRole("button", { name: "Byte 001fffff: 80", exact: true })).toBeVisible();
    await view.getByLabel("Offset (hex)").fill("200000");
    await view.getByRole("button", { name: "Go", exact: true }).click();
    await expect(view.getByRole("alert")).toContainText("outside the loaded bytes");
    await tree.getByRole("button", { name: "picture.png", exact: true }).click();
    await expect(page.getByRole("tab", { name: "Preview", exact: true })).toHaveAttribute("aria-selected", "true");
    await page.getByRole("tab", { name: "Hex", exact: true }).click();
    await expect(view.getByRole("button", { name: "Byte 00000000: 89", exact: true })).toBeVisible();
    await expect(page.getByRole("button", { name: "Save", exact: true })).toBeDisabled();
    await page.getByRole("tab", { name: "Preview", exact: true }).click();
    await expect(page.locator("main img")).toBeVisible();
  } finally {
    await rm(dir, { recursive: true, force: true });
  }
});
