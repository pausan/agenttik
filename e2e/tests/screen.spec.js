import { spawn, execFileSync } from "node:child_process";

import { addProject, expect, newTask, startServer, test } from "../fixtures.js";

/* The Screen tab against a real X server — see specs/096-screen.md. Each test
   runs its own Xvfb with an xev window, white on the black root, which prints
   every key and button it receives, and an agenttik showing that display.
   The browser says it is on another machine (X-Forwarded-For), which is what
   brings the Screen button out. */

const haveX = ["Xvfb", "xev", "xsel"].every((bin) => {
  try {
    execFileSync("which", [bin], { stdio: "ignore" });
    return true;
  } catch {
    return false;
  }
});

/* Xvfb picks a free display itself and says which on fd 3, so parallel
   tests never share one. */
async function startDisplay() {
  const xvfb = spawn("Xvfb", ["-displayfd", "3", "-screen", "0", "800x600x24", "-nolisten", "tcp"], {
    stdio: ["ignore", "ignore", "ignore", "pipe"],
  });
  const display = await new Promise((resolve, reject) => {
    let out = "";
    xvfb.stdio[3].on("data", (b) => {
      out += String(b);
      if (out.includes("\n")) resolve(":" + out.trim());
    });
    xvfb.on("exit", (code) => reject(new Error(`Xvfb exited with ${code}`)));
  });
  const env = { ...process.env, DISPLAY: display, WAYLAND_DISPLAY: "" };
  const xev = spawn("xev", ["-geometry", "300x200+400+300"], { env });
  let events = "";
  xev.stdout.on("data", (b) => (events += String(b)));
  await expect.poll(() => events).toContain("MapNotify");
  return {
    env,
    events: () => events,
    clipboard: () => execFileSync("xsel", ["-b", "-o"], { env, encoding: "utf8" }),
    copy: (text) => execFileSync("xsel", ["-b", "-i"], { env, input: text }),
    stop() {
      xev.kill();
      xvfb.kill();
    },
  };
}

/* A page on the server, as a browser on another machine sees it. */
async function remotePage(browser, server, init, options = {}) {
  const context = await browser.newContext({
    extraHTTPHeaders: { "X-Forwarded-For": "192.0.2.10" },
    permissions: ["clipboard-read", "clipboard-write"],
    ...options,
  });
  const page = await context.newPage();
  if (init) await page.addInitScript(init);
  const errors = [];
  page.on("pageerror", (e) => errors.push(e.message));
  await page.goto(server.url);
  return { page, context, errors };
}

/* The canvas pixel at a screen position, read back from what was drawn. */
function pixel(page, x, y) {
  return page.getByTestId("screen-canvas").evaluate((canvas, [sx, sy]) => {
    const k = canvas.width / 800;
    return [...canvas.getContext("2d").getImageData(Math.floor(sx * k), Math.floor(sy * k), 1, 1).data].slice(0, 3);
  }, [x, y]);
}

/* Where a screen position is on the page. */
async function at(page, x, y) {
  const box = await page.getByTestId("screen-canvas").boundingBox();
  return [box.x + (x / 800) * box.width, box.y + (y / 600) * box.height];
}

for (const codec of ["vp9", "jpeg"]) {
  test(`screen tab shows and drives another machine (${codec})`, async ({ browser }) => {
    test.skip(!haveX, "needs Xvfb, xev and xsel");
    const x = await startDisplay();
    const server = await startServer({ env: { DISPLAY: x.env.DISPLAY, WAYLAND_DISPLAY: "" } });
    // A browser without WebCodecs gets JPEG bands.
    const { page, context, errors } = await remotePage(browser, server, codec === "jpeg" ? () => { delete window.VideoDecoder; } : undefined);
    try {
      await addProject(page);
      await newTask(page);
      await page.getByRole("button", { name: "View and control this machine's screen" }).click();
      await expect(page.getByText(/Screen sharing is off on/)).toBeVisible();
      await page.getByRole("button", { name: "Turn on screen sharing" }).click();

      await expect(page.getByTestId("screen-summary")).toContainText(`${codec.toUpperCase()} ·`);
      await expect(page.getByTestId("screen-summary")).toContainText("800×600");
      await expect.poll(async () => {
        const inside = await pixel(page, 600, 450);
        const outside = await pixel(page, 50, 50);
        return inside.every((v) => v > 200) && outside.every((v) => v < 60);
      }, { message: "the white xev window is drawn where it is" }).toBe(true);

      // A click and a key reach the xev window on the other end.
      const [px, py] = await at(page, 550, 400);
      await page.mouse.click(px, py, { button: "right" });
      await expect.poll(x.events).toMatch(/ButtonPress[\s\S]*button 3/);
      await page.keyboard.type("q");
      await expect.poll(x.events).toContain("keysym 0x71, q");

      // The other machine's clipboard comes to this one...
      x.copy("copied over there");
      await expect.poll(() => page.evaluate(() => navigator.clipboard.readText())).toBe("copied over there");
      // ...and this one's goes there ahead of the paste keystroke.
      await page.evaluate(() => navigator.clipboard.writeText("pasted from here"));
      await page.keyboard.press("Control+V");
      await expect.poll(x.clipboard).toBe("pasted from here");
      await expect.poll(x.events).toContain("keysym 0x76, v");

      // Full screen, and back.
      await page.getByRole("button", { name: "Full screen" }).click();
      await expect.poll(() => page.evaluate(() => !!document.fullscreenElement)).toBe(true);
      await page.getByRole("button", { name: "Exit full screen" }).click();
      await expect.poll(() => page.evaluate(() => !!document.fullscreenElement)).toBe(false);

      // An unchanged screen sends nothing more.
      await expect.poll(async () => (await page.getByTestId("screen-summary").innerText()).includes(" 0 fps"), { timeout: 10_000 }).toBe(true);
      expect(errors).toEqual([]);
    } finally {
      await context.close();
      await server.stop();
      x.stop();
    }
  });
}

test("a tablet taps where it clicks and brings up its keyboard", async ({ browser }) => {
  test.skip(!haveX, "needs Xvfb, xev and xsel");
  const x = await startDisplay();
  const server = await startServer({ env: { DISPLAY: x.env.DISPLAY, WAYLAND_DISPLAY: "" } });
  const { page, context, errors } = await remotePage(browser, server, undefined, { hasTouch: true, viewport: { width: 1180, height: 820 } });
  try {
    await page.request.put(server.url + "/api/screen", { data: { enabled: true } });
    await addProject(page);
    await newTask(page);
    await page.getByRole("button", { name: "View and control this machine's screen" }).click();
    await expect(page.getByTestId("screen-summary")).toContainText("800×600");

    // Trackpad is the default; touching where you click is the other mode.
    await page.getByRole("button", { name: "Touch as trackpad" }).click();
    const [px, py] = await at(page, 550, 400);
    await page.touchscreen.tap(px, py);
    await expect.poll(x.events).toMatch(/ButtonPress[^\n]*\n[^\n]*root:\(550,400\),\n[^\n]*button 1,/);

    await page.getByRole("button", { name: "Show the keyboard" }).click();
    await expect(page.getByLabel("Keyboard input for the remote screen")).toBeFocused();
    expect(errors).toEqual([]);
  } finally {
    await context.close();
    await server.stop();
    x.stop();
  }
});

test("no Screen button on the machine itself", async ({ page }) => {
  await addProject(page);
  await newTask(page);
  await expect(page.getByRole("button", { name: "Open a terminal in the project folder" })).toBeVisible();
  await expect(page.getByRole("button", { name: "View and control this machine's screen" })).toHaveCount(0);
});
