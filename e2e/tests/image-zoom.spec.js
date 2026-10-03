import { mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { addProject, expect, openProject, sidebar, test } from "../fixtures.js";

test("an image preview zooms by keys, wheel and pinch, and scrolls when larger than its pane", async ({ page }) => {
  const dir = await mkdtemp(join(tmpdir(), "agenttik-image-zoom-"));
  try {
    const png = await page.evaluate(() => {
      const canvas = document.createElement("canvas");
      canvas.width = 3000;
      canvas.height = 2000;
      const ctx = canvas.getContext("2d");
      ctx.fillStyle = "#c33";
      ctx.fillRect(0, 0, 3000, 2000);
      return canvas.toDataURL("image/png").split(",")[1];
    });
    await writeFile(join(dir, "big.png"), Buffer.from(png, "base64"));
    await addProject(page, dir);
    await openProject(page, dir);
    await sidebar(page).getByRole("tab", { name: "Tree" }).click();
    await sidebar(page).getByRole("button", { name: "big.png", exact: true }).click();

    const img = page.getByRole("img", { name: "preview", exact: true });
    const pane = img.locator("..");
    const level = page.getByTitle("Fit to the pane", { exact: true });
    const width = () => img.evaluate((el) => el.getBoundingClientRect().width);
    const scrolls = () => pane.evaluate((el) => el.scrollWidth > el.clientWidth && el.scrollHeight > el.clientHeight);
    await expect(level).toHaveText("Fit");
    await expect.poll(width).toBeLessThan(3000);
    expect(await scrolls()).toBe(false);

    // Keys act from anywhere outside a text field.
    await page.locator("body").click({ position: { x: 1, y: 1 } });
    await page.keyboard.press("Control+0");
    await expect(level).toHaveText("100%");
    await expect.poll(width).toBe(3000);
    expect(await scrolls()).toBe(true);
    await page.keyboard.press("Control+=");
    await expect(level).toHaveText("125%");
    await expect.poll(width).toBe(3750);
    await page.keyboard.press("Control+-");
    await page.keyboard.press("Control+-");
    await expect(level).toHaveText("75%");

    // A field keeps its own keys.
    const filter = sidebar(page).getByRole("textbox").first();
    await filter.focus();
    await page.keyboard.press("Control+0");
    await expect(level).toHaveText("75%");

    await page.getByRole("button", { name: "Zoom in", exact: true }).click();
    await expect(level).toHaveText("100%");
    await level.click();
    await expect(level).toHaveText("Fit");
    const fitted = await width();

    // Ctrl with the wheel zooms about the pointer; the wheel alone does not.
    const box = await pane.boundingBox();
    await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
    await page.mouse.wheel(0, -100);
    await expect(level).toHaveText("Fit");
    await page.keyboard.down("Control");
    await page.mouse.wheel(0, -100);
    await page.keyboard.up("Control");
    await expect.poll(width).toBeGreaterThan(fitted * 1.2);

    // Two fingers spreading apart double the scale.
    await level.click();
    await pane.evaluate((el) => {
      const r = el.getBoundingClientRect();
      const cx = r.left + r.width / 2;
      const cy = r.top + r.height / 2;
      const touches = (d) => [
        new Touch({ identifier: 1, target: el, clientX: cx - d, clientY: cy }),
        new Touch({ identifier: 2, target: el, clientX: cx + d, clientY: cy }),
      ];
      const fire = (type, list) => el.dispatchEvent(new TouchEvent(type, { touches: list, changedTouches: list, bubbles: true }));
      fire("touchstart", touches(50));
      fire("touchmove", touches(100));
      fire("touchend", []);
    });
    await expect.poll(width).toBeCloseTo(fitted * 2, 0);
  } finally {
    await rm(dir, { recursive: true, force: true });
  }
});
