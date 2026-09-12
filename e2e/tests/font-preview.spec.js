import { copyFile, mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { addProject, expect, openProject, sidebar, test } from "../fixtures.js";

test("fonts render with custom text, size and glyph guides without a text editor", async ({ page }) => {
  const dir = await mkdtemp(join(tmpdir(), "agenttik-fonts-"));
  try {
    for (const ext of ["ttf", "otf", "woff", "woff2"]) {
      await copyFile(new URL(`../font-fixtures/sample.${ext}`, import.meta.url), join(dir, `sample.${ext}`));
    }
    await writeFile(join(dir, "missing.ttf"), "");
    await writeFile(join(dir, "readme.txt"), "An editable file.");
    await addProject(page, dir);
    await openProject(page, dir);
    await sidebar(page).getByRole("tab", { name: "Tree" }).click();

    const textRequests = [];
    page.on("request", (req) => {
      if (/\/file\?.*sample\.(ttf|otf|woff2?)/.test(req.url())) textRequests.push(req.url());
    });
    const sample = page.getByLabel("Font sample", { exact: true });
    const input = page.getByRole("textbox", { name: "Sample text", exact: true });
    const size = page.getByRole("spinbutton", { name: "Size (px)", exact: true });
    for (const ext of ["ttf", "otf", "woff", "woff2"]) {
      await sidebar(page).getByRole("button", { name: `sample.${ext}`, exact: true }).click();
      await expect(sample).toBeVisible();
      await expect(page.getByRole("tab", { name: "Edit", exact: true })).toHaveCount(0);
      await expect(page.getByRole("button", { name: "Save", exact: true })).toHaveCount(0);
      await expect(sample).toHaveCSS("font-size", "48px");
      // Verify the actual face loaded, including its distinctive advance.
      expect(await sample.evaluate((el) => {
        const family = getComputedStyle(el).fontFamily;
        const ctx = document.createElement("canvas").getContext("2d");
        ctx.font = `48px ${family}`;
        return [Array.from(document.fonts).filter((face) => face.family === family.replaceAll('"', '') && face.status === "loaded").length, ctx.measureText("A").width];
      })).toEqual([1, 24]);
    }
    expect(textRequests).toEqual([]);

    const defaultText = await input.inputValue();
    await input.fill("My own text: Aa 123 <b>plain</b>");
    await expect(sample).toHaveText("My own text: Aa 123 <b>plain</b>");
    await expect(sample.locator("b")).toHaveCount(0);
    await size.fill("72");
    await expect(sample).toHaveCSS("font-size", "72px");
    const square = page.getByRole("region", { name: "Sample glyphs" }).locator(".glyph-square").first();
    await expect(square).toHaveCSS("width", "72px");
    await expect(square).toHaveCSS("height", "72px");
    await expect(square.locator("..")).toHaveCSS("padding", "18px");
    await page.getByRole("button", { name: "Reset text", exact: true }).click();
    await expect(input).toHaveValue(defaultText);
    await expect(sample).toHaveText(defaultText);
    await expect(size).toHaveValue("72");

    await size.fill("999");
    await size.blur();
    await expect(size).toHaveValue("160");
    await size.fill("1");
    await size.blur();
    await expect(size).toHaveValue("8");

    // A narrow pane still keeps the controls inside the viewport.
    await page.setViewportSize({ width: 650, height: 900 });
    await expect(input).toBeVisible();
    await expect(size).toBeInViewport();
    await page.setViewportSize({ width: 1440, height: 900 });
    await sidebar(page).getByRole("tab", { name: "Tree" }).click();

    await sidebar(page).getByRole("button", { name: "readme.txt", exact: true }).click();
    await expect(page.getByRole("tab", { name: "Edit", exact: true })).toBeVisible();
    expect(await page.evaluate(() => Array.from(document.fonts).filter((face) => face.family.startsWith("preview-font-")).length)).toBe(0);

    // A missing source reports failure instead of silently showing a fallback.
    await page.route("**/raw?path=missing.ttf", (route) => route.fulfill({ status: 404, body: "Missing font" }));
    await sidebar(page).getByRole("button", { name: "missing.ttf", exact: true }).click();
    await expect(page.getByRole("alert")).toContainText("Cannot preview this font");
    await expect(sample).toHaveCount(0);
    await sidebar(page).getByRole("button", { name: "sample.ttf", exact: true }).click();
    await expect(sample).toBeVisible();
  } finally {
    await rm(dir, { recursive: true, force: true });
  }
});
