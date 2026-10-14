import { addProject, expect, newTask, openProject, pickModel, test } from "../fixtures.js";

async function pasteImages(page, count, size = 2) {
  await page.getByPlaceholder("Ask the agent…").evaluate((box, { count, size }) => {
    const canvas = document.createElement("canvas");
    canvas.width = canvas.height = size;
    const clipboardData = new DataTransfer();
    const ctx = canvas.getContext("2d");
    for (let i = 0; i < count; i++) {
      ctx.fillStyle = i % 2 ? "blue" : "red";
      ctx.fillRect(0, 0, size, size);
      const bytes = Uint8Array.from(atob(canvas.toDataURL("image/png").split(",")[1]), (c) => c.charCodeAt(0));
      clipboardData.items.add(new File([bytes], `image-${i}.png`, { type: "image/png" }));
    }
    box.dispatchEvent(new ClipboardEvent("paste", { clipboardData, bubbles: true, cancelable: true }));
  }, { count, size });
}

test("paste multiple images, remove one, restore draft and require text before sending", async ({ page }) => {
  await addProject(page);
  await openProject(page);
  await newTask(page);
  await pasteImages(page, 2);
  const previews = page.locator(".prompt-bar").getByRole("img", { name: /Attached image/ });
  await expect(previews).toHaveCount(2);
  await page.getByRole("button", { name: "Remove image 1", exact: true }).click();
  await expect(previews).toHaveCount(1);
  await page.reload();
  await expect(previews).toHaveCount(1);
  await expect(page.getByPlaceholder("Ask the agent…")).toHaveValue("");
  await pasteImages(page, 1);
  await expect(previews).toHaveCount(2);
  await expect(page.getByRole("status").filter({ hasText: "Saving images" })).toHaveCount(0);
  await expect(page.locator('button[type="submit"]')).toBeDisabled();
  await page.getByPlaceholder("Ask the agent…").fill("Describe these images");
  const request = page.waitForRequest((r) => r.method() === "POST" && /\/(messages|queue)$/.test(new URL(r.url()).pathname));
  await page.locator('button[type="submit"]').click();
  const sent = (await request).postDataJSON().prompt;
  expect(sent.match(/!\[Attached image\]/g)).toHaveLength(2);
  await expect(previews).toHaveCount(0);
});

test("submitted images stay visible and open a zoomable viewer after reload", async ({ page }) => {
  await addProject(page);
  await openProject(page);
  await newTask(page);
  await pickModel(page);
  const text = "Describe **these** images\nwith this spacing.";
  await page.getByPlaceholder("Ask the agent…").fill(text);
  await pasteImages(page, 2, 1024);
  await expect(page.locator(".prompt-bar").getByRole("img")).toHaveCount(2);
  await expect(page.getByRole("status").filter({ hasText: "Saving images" })).toHaveCount(0);
  await page.getByPlaceholder("Ask the agent…").press("Control+Enter");
  const message = page.locator("[data-human-message]").last();
  const images = message.getByRole("img", { name: /Attached image/ });
  await expect(images).toHaveCount(2);
  await expect.poll(async () => (await images.first().boundingBox()).height).toBe(320);
  await expect(message.getByText(text, { exact: true })).toBeVisible();
  await expect(message).not.toContainText("/api/attachments/");
  await expect(page.getByText("idle", { exact: true })).toBeVisible();
  await page.setViewportSize({ width: 390, height: 844 });
  await page.reload();
  await expect(images).toHaveCount(2);
  await expect.poll(() => images.first().evaluate((img) => img.naturalWidth)).toBe(1024);
  expect((await images.first().boundingBox()).width).toBeLessThan(390);
  const source = await images.nth(1).getAttribute("src");
  const thumbnail = await images.nth(1).boundingBox();
  await message.getByRole("button", { name: "View attached image 2", exact: true }).click();
  const viewer = page.getByRole("dialog", { name: "Attached image", exact: true });
  await expect(viewer).toBeVisible();
  await expect(viewer.getByRole("img")).toHaveAttribute("src", source);
  await expect.poll(() => viewer.getByRole("img").evaluate((img) => img.naturalWidth)).toBe(1024);
  await expect.poll(async () => (await viewer.getByRole("img").boundingBox()).width).toBeGreaterThan(thumbnail.width);
  await viewer.getByRole("button", { name: "Zoom in", exact: true }).click();
  await expect(viewer.getByTitle("Fit to the pane", { exact: true })).not.toHaveText("Fit");
  await page.keyboard.press("Escape");
  await expect(viewer).toBeHidden();
  await expect(images).toHaveCount(2);
});

test("a circled image area opens a thread with a marked image and survives reload", async ({ page }) => {
  await addProject(page);
  await openProject(page);
  await newTask(page);
  await pickModel(page);
  await page.getByPlaceholder("Ask the agent…").fill("Describe this image");
  await pasteImages(page, 1, 1024);
  await expect(page.getByRole("status").filter({ hasText: "Saving images" })).toHaveCount(0);
  await page.getByPlaceholder("Ask the agent…").press("Control+Enter");
  await expect(page.getByLabel("Turn usage")).toHaveCount(1);
  const mainMessages = () => page.locator("[data-message-id]:not([data-threads] [data-message-id])");
  const count = await mainMessages().count();
  await mainMessages().first().getByRole("button", { name: "View attached image 1", exact: true }).click();
  const viewer = page.getByRole("dialog", { name: "Attached image", exact: true });
  await viewer.getByRole("button", { name: "Circle an area" }).click();
  await expect.poll(() => viewer.getByRole("img").evaluate((img) => img.naturalWidth)).toBe(1024);
  const image = await viewer.getByRole("img").boundingBox();
  const ask = viewer.getByRole("button", { name: "Ask in thread" });
  await expect(ask).toBeDisabled();
  // A click alone does not create an empty selection.
  await page.mouse.click(image.x + image.width / 2, image.y + image.height / 2);
  await expect(ask).toBeDisabled();
  // Reverse dragging selects the same region as a forward drag.
  await page.mouse.move(image.x + image.width * 0.75, image.y + image.height * 0.75);
  await page.mouse.down();
  await page.mouse.move(image.x + image.width * 0.25, image.y + image.height * 0.25, { steps: 5 });
  await page.mouse.up();
  await expect(ask).toBeEnabled();
  await expect(viewer.getByLabel("Selected image area")).toBeVisible();
  await page.route("**/api/attachments", (route) => route.fulfill({ status: 503, contentType: "application/json", body: JSON.stringify({ error: "Image save unavailable" }) }), { times: 1 });
  await ask.click();
  await expect(viewer.getByRole("alert")).toHaveText("Image save unavailable");
  await expect(viewer.getByLabel("Selected image area")).toBeVisible();
  await ask.click();
  await expect(viewer).toBeHidden();
  const form = page.getByRole("form", { name: "New thread" });
  await expect(form).toContainText("x 256–768, y 256–768 in the 1024 × 1024 image");
  await expect(form.getByRole("textbox", { name: "Thread question" })).toBeFocused();
  const marked = form.getByRole("img", { name: "Circled image area" });
  await expect(marked).toBeVisible();
  // Check the uploaded pixels, not just the temporary SVG overlay.
  const red = await marked.evaluate(async (img) => {
    await img.decode();
    const canvas = document.createElement("canvas");
    canvas.width = canvas.height = 1024;
    const ctx = canvas.getContext("2d");
    ctx.drawImage(img, 0, 0);
    return Array.from(ctx.getImageData(512, 256, 1, 1).data);
  });
  expect(red).toEqual([239, 68, 68, 255]);
  await form.getByRole("textbox", { name: "Thread question" }).fill("What is in the circle?");
  const request = page.waitForRequest((r) => r.method() === "POST" && /\/threads$/.test(new URL(r.url()).pathname));
  await form.getByRole("button", { name: "Ask", exact: true }).click();
  const body = (await request).postDataJSON();
  expect(body.prompt).toMatch(/^What is in the circle\?\n\n!\[Attached image\]\(\/api\/attachments\//);
  const thread = page.getByRole("region", { name: "Thread: What is in the circle?" });
  await expect(thread).toContainText("Read the attached image using your image-reading tool:");
  await expect(thread.getByLabel("Thread working")).toHaveCount(0);
  await expect(mainMessages()).toHaveCount(count);
  await expect(thread.getByRole("img", { name: "Attached image 1", exact: true })).toBeVisible();
  await thread.getByRole("button", { name: "Close thread" }).click();
  await page.reload();
  await page.getByRole("button", { name: "Open thread: What is in the circle?" }).click();
  await expect(thread.getByRole("img", { name: "Attached image 1", exact: true })).toBeVisible();
  // Threads cannot contain nested threads.
  await thread.getByRole("button", { name: "View attached image 1", exact: true }).click();
  await expect(viewer.getByRole("button", { name: "Circle an area" })).toHaveCount(0);
});

test.describe("touch image areas", () => {
  test.use({ isMobile: true, hasTouch: true });

  test("a finger can circle an image on a phone", async ({ page }) => {
    await addProject(page);
    await openProject(page);
    await newTask(page);
    await pickModel(page);
    await page.getByPlaceholder("Ask the agent…").fill("Describe this image");
    await pasteImages(page, 1, 1024);
    await expect(page.getByRole("status").filter({ hasText: "Saving images" })).toHaveCount(0);
    await page.getByPlaceholder("Ask the agent…").press("Control+Enter");
    await expect(page.getByLabel("Turn usage")).toHaveCount(1);
    await page.setViewportSize({ width: 390, height: 844 });
    await page.getByRole("button", { name: "View attached image 1", exact: true }).tap();
    const viewer = page.getByRole("dialog", { name: "Attached image", exact: true });
    await viewer.getByRole("button", { name: "Circle an area" }).tap();
    await expect.poll(() => viewer.getByRole("img").evaluate((img) => img.naturalWidth)).toBe(1024);
    const picture = await viewer.getByRole("img").boundingBox();
    const client = await page.context().newCDPSession(page);
    const x = picture.x + picture.width * 0.25, y = picture.y + picture.height * 0.25;
    await client.send("Input.dispatchTouchEvent", { type: "touchStart", touchPoints: [{ x, y }] });
    for (let i = 1; i <= 5; i++) {
      await client.send("Input.dispatchTouchEvent", { type: "touchMove", touchPoints: [{ x: x + picture.width * i / 10, y: y + picture.height * i / 10 }] });
    }
    await client.send("Input.dispatchTouchEvent", { type: "touchEnd", touchPoints: [] });
    await expect(viewer.getByRole("button", { name: "Ask in thread" })).toBeEnabled();
    // Sliding off the action cancels it; a subsequent tap should act immediately.
    const ask = await viewer.getByRole("button", { name: "Ask in thread" }).boundingBox();
    await client.send("Input.dispatchTouchEvent", { type: "touchStart", touchPoints: [{ x: ask.x + ask.width / 2, y: ask.y + ask.height / 2 }] });
    await client.send("Input.dispatchTouchEvent", { type: "touchMove", touchPoints: [{ x: ask.x + ask.width + 20, y: ask.y + ask.height / 2 }] });
    await client.send("Input.dispatchTouchEvent", { type: "touchEnd", touchPoints: [] });
    await expect(viewer).toBeVisible();
    await viewer.getByRole("button", { name: "Ask in thread" }).tap();
    const form = page.getByRole("form", { name: "New thread" });
    await expect(form.getByRole("img", { name: "Circled image area" })).toBeVisible();
    await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth)).toBe(390);
    await form.screenshot({ path: test.info().outputPath("image-question-mobile.png") });
  });
});

test("ordinary paste retains the browser default", async ({ page }) => {
  await addProject(page);
  await openProject(page);
  await newTask(page);
  const allowed = await page.getByPlaceholder("Ask the agent…").evaluate((box) => {
    const clipboardData = new DataTransfer();
    clipboardData.setData("text/plain", "ordinary text");
    return box.dispatchEvent(new ClipboardEvent("paste", { clipboardData, bubbles: true, cancelable: true }));
  });
  expect(allowed).toBe(true);
});

test("empty WebKit paste event reads clipboard images into the prompt", async ({ page }) => {
  await addProject(page);
  await openProject(page);
  await newTask(page);
  await page.getByPlaceholder("Ask the agent…").evaluate((box) => {
    const canvas = document.createElement("canvas");
    canvas.width = canvas.height = 2;
    const bytes = Uint8Array.from(atob(canvas.toDataURL("image/png").split(",")[1]), (c) => c.charCodeAt(0));
    Object.defineProperty(navigator.clipboard, "read", { configurable: true, value: async () => [
      new ClipboardItem({ "image/png": new Blob([bytes], { type: "image/png" }) }),
    ] });
    box.dispatchEvent(new ClipboardEvent("paste", { clipboardData: new DataTransfer(), bubbles: true, cancelable: true }));
  });
  const image = page.locator(".prompt-bar").getByRole("img", { name: "Attached image 1" });
  await expect(image).toBeVisible();
  await expect.poll(() => image.evaluate((img) => img.naturalWidth)).toBe(2);
});

test("real Ctrl+V pastes a clipboard image into the prompt", async ({ page, context }) => {
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  await addProject(page);
  await openProject(page);
  await newTask(page);
  await page.evaluate(async () => {
    const canvas = document.createElement("canvas");
    canvas.width = canvas.height = 2;
    const blob = await new Promise((resolve) => canvas.toBlob(resolve, "image/png"));
    await navigator.clipboard.write([new ClipboardItem({ "image/png": blob })]);
  });
  await page.getByPlaceholder("Ask the agent…").press("Control+v");
  await expect(page.locator(".prompt-bar").getByRole("img", { name: "Attached image 1" })).toBeVisible();
});

test("failed submission restores both the text and pasted image", async ({ page }) => {
  await addProject(page);
  await openProject(page);
  await newTask(page);
  await page.getByPlaceholder("Ask the agent…").fill("Look at this");
  await pasteImages(page, 1);
  await expect(page.getByRole("img", { name: "Attached image 1" })).toBeVisible();
  // fail() logs handled API errors as well as showing a toast. Suppress only
  // the refusal deliberately injected below; unexpected errors still fail.
  await page.evaluate(() => {
    const error = console.error.bind(console);
    console.error = (...args) => {
      if (args[0]?.message !== "Submission refused") error(...args);
    };
  });
  await page.route(/\/api\/sessions\/[^/]+\/(messages|queue)$/, async (route) => {
    if (route.request().method() !== "POST") return route.continue();
    await route.fulfill({ status: 400, contentType: "application/json", body: JSON.stringify({ error: "Submission refused" }) });
  });
  await page.locator('button[type="submit"]').click();
  await expect(page.getByText("Submission refused", { exact: true })).toBeVisible();
  await expect(page.getByPlaceholder("Ask the agent…")).toHaveValue("Look at this");
  await expect(page.getByRole("img", { name: "Attached image 1" })).toBeVisible();
});
