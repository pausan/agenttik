import { addProject, expect, newTask, openProject, openSettings, pickModel, sendPrompt, sidebar, test } from "../fixtures.js";

async function expectSolidSurface(surface) {
  await expect(surface).toHaveCSS("opacity", "1");
  const background = await surface.evaluate((el) => getComputedStyle(el).backgroundColor);
  expect(background).not.toBe("transparent");
  expect(background).not.toMatch(/rgba\([^)]*,\s*0(?:\.\d+)?\)|\/\s*(?:0(?:\.\d+)?|\d{1,2}%)\s*\)/);
}

test("task tooltip covers the pane boundary", async ({ page, agenttik }, testInfo) => {
  await addProject(page);
  await openProject(page);
  await newTask(page);
  await pickModel(page);
  await sendPrompt(page, "A long opening prompt that makes the task tooltip cross the sidebar scrollbar and pane resize handle.");
  const [session] = await (await page.request.get(`${agenttik.url}/api/sessions`)).json();
  for (let i = 0; i < 35; i++) {
    const response = await page.request.post(`${agenttik.url}/api/sessions`, {
      data: { project_id: session.project_id, provider: "fake", model: "fake-quick", title: `Overflow task ${i}` },
    });
    expect(response.ok()).toBe(true);
  }
  await page.reload();
  const scroller = sidebar(page).locator(".overflow-auto").first();
  await expect.poll(() => scroller.evaluate((el) => el.scrollHeight > el.clientHeight)).toBe(true);
  const row = sidebar(page).locator(".task-select").filter({ hasText: "A long opening prompt" });
  await row.hover();
  // Reka's accessible tooltip is a hidden copy inside the visible surface.
  const surface = page.locator('[data-slot="content"][data-state="delayed-open"]');
  await expect(surface).toBeVisible();
  await expectSolidSurface(surface);
  const handle = page.getByRole("separator", { name: "Resize the left panel" });
  const boundary = await handle.boundingBox();
  await expect.poll(() => surface.evaluate((el, x) => {
    const box = el.getBoundingClientRect();
    return box.left < x && box.right > x && el.contains(document.elementFromPoint(x, box.top + box.height / 2));
  }, boundary.x)).toBe(true);
  await page.screenshot({ path: testInfo.outputPath("tooltip.png") });
});

test("Settings covers pane boundaries and its select menu stays above the dialog", async ({ page }, testInfo) => {
  await page.setViewportSize({ width: 1000, height: 700 });
  await openSettings(page, "Appearance");
  const dialog = page.getByRole("dialog");
  await expectSolidSurface(dialog);
  const boundary = await page.getByRole("separator", { name: "Resize the left panel", includeHidden: true }).boundingBox();
  expect(await dialog.evaluate((el, x) => {
    const box = el.getBoundingClientRect();
    return box.left < x && box.right > x && el.contains(document.elementFromPoint(x, box.top + box.height / 2));
  }, boundary.x)).toBe(true);
  await dialog.getByRole("button", { name: "Color mode", exact: true }).click();
  const option = page.getByRole("option", { name: "Dark", exact: true });
  await expect(option).toBeVisible();
  await expectSolidSurface(page.locator('[data-slot="content"]').filter({ has: option }));
  await page.screenshot({ path: testInfo.outputPath("settings-menu.png") });
  await option.click();
  await expect(page.locator("html")).toHaveClass(/dark/);
  await expectSolidSurface(dialog);
});
