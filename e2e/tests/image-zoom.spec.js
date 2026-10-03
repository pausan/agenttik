import { execFileSync } from "node:child_process";
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
    await expect(pane).not.toHaveCSS("cursor", "grab");
    await expect.poll(width).toBeLessThan(3000);
    expect(await scrolls()).toBe(false);

    // In a browser the chords stay the browser's own page zoom.
    await page.locator("body").click({ position: { x: 1, y: 1 } });
    const prevented = await page.evaluate(() => {
      const e = new KeyboardEvent("keydown", { key: "0", ctrlKey: true, bubbles: true, cancelable: true });
      document.body.dispatchEvent(e);
      return e.defaultPrevented;
    });
    expect(prevented).toBe(false);
    await expect(level).toHaveText("Fit");

    // In the desktop window they zoom the picture, from anywhere outside a text field.
    await page.evaluate(() => { window.runtime = {}; });
    await page.keyboard.press("Control+0");
    await expect(level).toHaveText("100%");
    await expect.poll(width).toBe(3000);
    expect(await scrolls()).toBe(true);

    // Grab the picture in both directions, including outside the pane.
    const scroll = () => pane.evaluate((el) => [el.scrollLeft, el.scrollTop]);
    await pane.evaluate((el) => el.scrollTo(400, 300));
    await expect(pane).toHaveCSS("cursor", "grab");
    expect(await img.getAttribute("draggable")).toBe("false");
    const dragBox = await pane.boundingBox();
    const x = dragBox.x + dragBox.width / 2;
    const y = dragBox.y + dragBox.height / 2;
    await page.mouse.move(x, y);
    await page.mouse.down();
    await expect(pane).toHaveCSS("cursor", "grabbing");
    await page.mouse.move(x - 80, y - 60, { steps: 5 });
    await expect.poll(scroll).toEqual([480, 360]);
    await page.mouse.move(dragBox.x - 20, y - 60, { steps: 5 });
    await expect.poll(async () => (await scroll())[0]).toBeGreaterThan(480);
    await page.mouse.up();
    await expect(pane).toHaveCSS("cursor", "grab");
    const released = await scroll();
    await page.mouse.move(x, y);
    expect(await scroll()).toEqual(released);

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
    await expect(pane).not.toHaveCSS("cursor", "grab");
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

test("a diff of two pictures the same size zooms and scrolls both sides as one", async ({ page }) => {
  const dir = await mkdtemp(join(tmpdir(), "agenttik-image-diff-zoom-"));
  const git = (...args) => execFileSync("git", args, { cwd: dir, encoding: "utf8" });
  const png = (w, h, color) => page.evaluate(([w, h, color]) => {
    const canvas = document.createElement("canvas");
    canvas.width = w;
    canvas.height = h;
    const ctx = canvas.getContext("2d");
    ctx.fillStyle = color;
    ctx.fillRect(0, 0, w, h);
    return canvas.toDataURL("image/png").split(",")[1];
  }, [w, h, color]).then((b) => Buffer.from(b, "base64"));
  try {
    git("init", "-q");
    git("config", "user.name", "Test");
    git("config", "user.email", "test@example.com");
    await writeFile(join(dir, "same.png"), await png(3000, 2000, "#c33"));
    await writeFile(join(dir, "other.png"), await png(3000, 2000, "#c33"));
    git("add", ".");
    git("commit", "-qm", "pictures");
    await writeFile(join(dir, "same.png"), await png(3000, 2000, "#33c"));
    await writeFile(join(dir, "other.png"), await png(2000, 3000, "#33c"));
    await addProject(page, dir);
    await openProject(page, dir);
    await sidebar(page).getByRole("tab", { name: "Tree" }).click();
    await sidebar(page).getByRole("button", { name: "same.png", exact: true }).click();
    await page.getByRole("tab", { name: "Diff", exact: true }).click();

    const left = page.getByRole("img", { name: "HEAD", exact: true });
    const right = page.getByRole("img", { name: "working tree", exact: true });
    const width = (img) => img.evaluate((el) => el.getBoundingClientRect().width);
    const scroll = (img) => img.locator("..").evaluate((el) => [el.scrollLeft, el.scrollTop]);
    const levels = page.getByTitle("Fit to the pane", { exact: true });
    await expect(levels).toHaveCount(2);

    await page.getByRole("button", { name: "Zoom in", exact: true }).last().click();
    await expect(levels).toHaveText([/%$/, /%$/]);
    await expect.poll(() => width(left)).toBeGreaterThan(0);
    expect(await width(left)).toBe(await width(right));
    await levels.first().evaluate((el) => el.click());
    await page.evaluate(() => { window.runtime = {}; });
    await page.keyboard.press("Control+0");
    await expect(levels).toHaveText(["100%", "100%"]);
    await expect.poll(() => width(left)).toBe(3000);
    await expect.poll(() => width(right)).toBe(3000);

    // Scrolling one side scrolls the other to the same spot.
    await right.locator("..").evaluate((el) => el.scrollTo(700, 400));
    await expect.poll(() => scroll(left)).toEqual([700, 400]);
    await left.locator("..").evaluate((el) => el.scrollTo(100, 50));
    await expect.poll(() => scroll(right)).toEqual([100, 50]);

    // Grabbing one side pans both images.
    const dragBox = await left.locator("..").boundingBox();
    const x = dragBox.x + dragBox.width / 2;
    const y = dragBox.y + dragBox.height / 2;
    await page.mouse.move(x, y);
    await page.mouse.down();
    await page.mouse.move(x - 80, y - 60, { steps: 5 });
    await page.mouse.up();
    await expect.poll(() => scroll(left)).toEqual([180, 110]);
    await expect.poll(() => scroll(right)).toEqual([180, 110]);

    // Ctrl with the wheel over one side zooms both.
    const box = await right.locator("..").boundingBox();
    await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
    await page.keyboard.down("Control");
    await page.mouse.wheel(0, 100);
    await page.keyboard.up("Control");
    await expect.poll(() => width(right)).toBeLessThan(3000);
    expect(await width(left)).toBe(await width(right));
    expect(await scroll(left)).toEqual(await scroll(right));

    // Pictures of different sizes stay fitted.
    await sidebar(page).getByRole("button", { name: "other.png", exact: true }).click();
    await page.getByRole("tab", { name: "Diff", exact: true }).click();
    await expect(page.getByText("2000 × 3000")).toBeVisible();
    await expect(levels).toHaveCount(0);
  } finally {
    await rm(dir, { recursive: true, force: true });
  }
});

test.describe("touch image scrolling", () => {
  test.use({ viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true });

  test("one finger pans a zoomed picture in both directions on a phone", async ({ page, agenttik }) => {
    const dir = await mkdtemp(join(tmpdir(), "agenttik-image-touch-"));
    try {
      await writeFile(join(dir, "big.svg"), '<svg xmlns="http://www.w3.org/2000/svg" width="3000" height="2000"><rect width="3000" height="2000" fill="#c33"/></svg>');
      const response = await page.request.post(`${agenttik.url}/api/projects`, { data: { name: "Pictures", path: dir } });
      expect(response.ok()).toBeTruthy();
      await page.reload();
      const projects = page.getByRole("button", { name: "Projects and tasks", exact: true });
      const drawer = page.getByRole("dialog", { name: "Projects and tasks", exact: true });
      await projects.tap();
      await drawer.getByRole("button", { name: /Pictures/ }).tap();
      await expect(drawer).toBeHidden();
      await projects.tap();
      await drawer.getByRole("tab", { name: "Tree" }).tap();
      await drawer.getByRole("button", { name: "big.svg", exact: true }).tap();
      await expect(drawer).toBeHidden();
      await page.getByRole("tab", { name: "Preview", exact: true }).tap();
      const img = page.getByRole("img", { name: "preview", exact: true });
      const pane = img.locator("..");
      const level = page.getByTitle("Fit to the pane", { exact: true });
      await expect(level).toHaveText("Fit");
      for (let i = 0; i < 15 && (await level.textContent()) !== "100%"; i++) {
        await page.getByRole("button", { name: "Zoom in", exact: true }).tap();
      }
      await expect(level).toHaveText("100%");
      await pane.evaluate((el) => el.scrollTo(400, 300));
      const scroll = () => pane.evaluate((el) => [el.scrollLeft, el.scrollTop]);
      await expect.poll(scroll).toEqual([400, 300]);

      // Real touch input lets the browser perform native scrolling.
      const client = await page.context().newCDPSession(page);
      const box = await pane.boundingBox();
      const x = Math.round(box.x + box.width / 2);
      const y = Math.round(box.y + box.height / 2);
      for (const direction of [-1, 1]) {
        await client.send("Input.dispatchTouchEvent", { type: "touchStart", touchPoints: [{ x, y }] });
        const before = await scroll();
        for (let i = 1; i <= 8; i++) {
          await client.send("Input.dispatchTouchEvent", { type: "touchMove", touchPoints: [{ x: x + direction * i * 10, y: y + direction * i * 10 }] });
        }
        await expect.poll(async () => direction * (before[0] - (await scroll())[0])).toBeGreaterThan(40);
        await expect.poll(async () => direction * (before[1] - (await scroll())[1])).toBeGreaterThan(40);
        await client.send("Input.dispatchTouchEvent", { type: "touchEnd", touchPoints: [] });
      }
      await expect(level).toHaveText("100%");
      expect(await page.evaluate(() => window.visualViewport.scale)).toBe(1);
    } finally {
      await rm(dir, { recursive: true, force: true });
    }
  });
});
