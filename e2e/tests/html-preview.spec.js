import { copyFile, mkdir, mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { addProject, expect, openProject, sidebar, test } from "../fixtures.js";

for (const profile of ["default", "separate"]) {
  test(`HTML loads relative images and nested CSS in the ${profile} profile`, async ({ context, agenttik }) => {
    // Browser instrumentation also attempts scripts in sandboxed frames.
    const page = await context.newPage();
    const errors = [];
    page.on("pageerror", error => errors.push(error.message));
    page.on("console", message => {
      if (message.type() === "error" && !message.text().startsWith("Blocked script execution in 'about:srcdoc'")) errors.push(message.text());
    });
    await page.goto(agenttik.url);
    const dir = await mkdtemp(join(tmpdir(), "agenttik-html-"));
    try {
      if (profile !== "default") {
        const created = await (await page.request.post(`${agenttik.url}/api/profiles`, { data: { name: "Preview" } })).json();
        await page.goto(`${agenttik.url}/?profile=${created.id}`);
      }
      await mkdir(join(dir, "styles"));
      await mkdir(join(dir, "images"));
      await copyFile(new URL("../font-fixtures/sample.woff2", import.meta.url), join(dir, "styles", "sample.woff2"));
      const png = await page.evaluate(() => {
        const canvas = document.createElement("canvas");
        canvas.width = 32;
        canvas.height = 24;
        canvas.getContext("2d").fillRect(0, 0, 32, 24);
        return canvas.toDataURL().split(",")[1];
      });
      await writeFile(join(dir, "images", "sample image.png"), Buffer.from(png, "base64"));
      await writeFile(join(dir, "images", "icon.svg"), '<svg xmlns="http://www.w3.org/2000/svg" width="20" height="10"><rect width="20" height="10" fill="blue"/></svg>');
      await writeFile(join(dir, "styles", "theme.css"), '@font-face { font-family: Mockup; src: url("sample.woff2"); } h1 { color: rgb(12, 34, 56); font-family: Mockup; }');
      await writeFile(join(dir, "styles", "main.css"), '@import "theme.css"; .background { width: 32px; height: 24px; background-image: url("../images/sample image.png"); }');
      const html = '<!doctype html><html><head><link rel="stylesheet" href="styles/main.css"><style>body { background: rgb(240, 241, 242); }</style></head><body><h1>Local mockup</h1><img alt="Mockup" src="images/sample%20image.png"><img alt="Icon" src="images/icon.svg"><div class="background"></div><script>document.body.dataset.executed = "yes";</script></body></html>';
      await writeFile(join(dir, "overview.html"), html);
      await addProject(page, dir);
      await openProject(page, dir);
      await sidebar(page).getByRole("tab", { name: "Tree" }).click();
      await sidebar(page).getByRole("button", { name: "overview.html", exact: true }).dblclick();
      await page.getByRole("tab", { name: "Preview", exact: true }).click();
      const iframe = page.locator('iframe[title="overview.html"]');
      await expect(iframe).toHaveAttribute("sandbox", "");
      const frame = iframe.contentFrame();
      await expect(frame.getByRole("heading")).toHaveCSS("color", "rgb(12, 34, 56)");
      expect(await frame.locator("body").evaluate(() => document.compatMode)).toBe("CSS1Compat");
      await expect(frame.locator("body")).toHaveCSS("background-color", "rgb(240, 241, 242)");
      expect(await frame.locator("body").getAttribute("data-executed")).toBeNull();
      expect(await iframe.evaluate(el => el.contentDocument)).toBeNull();
      await expect.poll(() => frame.locator("body").evaluate(() => [...document.fonts].some(font => font.family === "Mockup" && font.status === "loaded"))).toBe(true);
      await expect.poll(() => frame.getByAltText("Mockup", { exact: true }).evaluate(img => [img.naturalWidth, img.naturalHeight])).toEqual([32, 24]);
      await expect.poll(() => frame.getByAltText("Icon", { exact: true }).evaluate(img => img.naturalWidth)).toBe(20);
      const background = await frame.locator(".background").evaluate(el => getComputedStyle(el).backgroundImage.match(/url\("(.*)"\)/)[1]);
      expect(background).toContain("/images/sample%20image.png");
      await expect.poll(() => frame.locator(".background").evaluate(el => {
        const url = getComputedStyle(el).backgroundImage.match(/url\("(.*)"\)/)[1];
        return performance.getEntriesByName(url).some(entry => entry.responseEnd > 0);
      })).toBe(true);
      await page.getByRole("tab", { name: "Edit", exact: true }).click();
      await page.getByRole("textbox", { name: "overview.html", exact: true }).fill(html.replace("Local mockup", "Unsaved mockup"));
      await page.getByRole("tab", { name: "Preview", exact: true }).click();
      await expect(frame.getByRole("heading", { name: "Unsaved mockup" })).toHaveCSS("color", "rgb(12, 34, 56)");
      await expect.poll(() => frame.getByAltText("Mockup", { exact: true }).evaluate(img => img.naturalWidth)).toBe(32);
      // Declared relative bases and incomplete edits must keep the preview usable.
      for (const base of ["./", "http://["]) {
        await page.getByRole("tab", { name: "Edit", exact: true }).click();
        await page.getByRole("textbox", { name: "overview.html", exact: true }).fill(html.replace("<head>", `<head><base href="${base}">`));
        await page.getByRole("tab", { name: "Preview", exact: true }).click();
        await expect(frame.getByRole("heading")).toHaveCSS("color", "rgb(12, 34, 56)");
        await expect.poll(() => frame.getByAltText("Mockup", { exact: true }).evaluate(img => img.naturalWidth)).toBe(32);
      }
      expect(errors).toEqual([]);
    } finally {
      await rm(dir, { recursive: true, force: true });
    }
  });
}
