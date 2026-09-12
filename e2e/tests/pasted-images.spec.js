import { addProject, expect, newTask, openProject, test } from "../fixtures.js";

async function pasteImages(page, count) {
  await page.getByPlaceholder("Ask the agent…").evaluate((box, count) => {
    const canvas = document.createElement("canvas");
    canvas.width = canvas.height = 2;
    const bytes = Uint8Array.from(atob(canvas.toDataURL("image/png").split(",")[1]), (c) => c.charCodeAt(0));
    const clipboardData = new DataTransfer();
    for (let i = 0; i < count; i++) clipboardData.items.add(new File([bytes], `image-${i}.png`, { type: "image/png" }));
    box.dispatchEvent(new ClipboardEvent("paste", { clipboardData, bubbles: true, cancelable: true }));
  }, count);
}

test("paste multiple images, remove one, restore draft and send images alone", async ({ page }) => {
  await addProject(page);
  await openProject(page);
  await newTask(page);
  await pasteImages(page, 2);
  const previews = page.getByRole("img", { name: /Attached image/ });
  await expect(previews).toHaveCount(2);
  await page.getByRole("button", { name: "Remove image 1", exact: true }).click();
  await expect(previews).toHaveCount(1);
  await page.reload();
  await expect(previews).toHaveCount(1);
  await expect(page.getByPlaceholder("Ask the agent…")).toHaveValue("");
  await pasteImages(page, 1);
  await expect(previews).toHaveCount(2);
  await expect(page.getByRole("status").filter({ hasText: "Saving images" })).toHaveCount(0);
  const request = page.waitForRequest((r) => r.method() === "POST" && /\/(messages|queue)$/.test(new URL(r.url()).pathname));
  await page.locator('button[type="submit"]').click();
  const sent = (await request).postDataJSON().prompt;
  expect(sent.match(/!\[Attached image\]/g)).toHaveLength(2);
  await expect(previews).toHaveCount(0);
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
