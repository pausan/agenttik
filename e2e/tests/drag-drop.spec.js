import {
  REPO,
  addProject,
  expect,
  newTask,
  openProject,
  pickModel,
  sendPrompt,
  sidebar,
  test,
} from "../fixtures.js";

/* Reordering moves the source under the cursor. Keep hovering there before
   releasing, so the browser must accept a drop onto the source itself. */
async function dragAndRelease(page, source, target) {
  await page.evaluate(() => {
    window.dropEvents = [];
    document.addEventListener("dragstart", (e) => {
      window.dragged = e.target.closest('[draggable="true"]');
    }, { once: true, capture: true });
    document.addEventListener("drop", (e) => {
      window.dropEvents.push(e);
    }, { once: true, capture: true });
  });

  await source.hover();
  const from = await source.boundingBox();
  const to = await target.boundingBox();
  const x = to.x + to.width / 2;
  const y = to.y + to.height / 2;
  await page.mouse.move(from.x + from.width / 2, from.y + from.height / 2);
  await page.mouse.down();
  await page.mouse.move(from.x + from.width / 2 + 8, from.y + from.height / 2, { steps: 3 });
  await page.mouse.move(x, y, { steps: 5 });
  await expect(source).toHaveCSS("opacity", "1");
  await expect.poll(async () => {
    const box = await source.boundingBox();
    return box.x <= x && x < box.x + box.width && box.y <= y && y < box.y + box.height;
  }).toBe(true);
  await page.mouse.move(x + 1, y);
  await page.mouse.up();

  expect(await page.evaluate(() => ({
    drops: window.dropEvents.length,
    accepted: window.dropEvents[0]?.defaultPrevented || false,
    onSource: window.dragged?.contains(window.dropEvents[0]?.target) || false,
  }))).toEqual({
    drops: 1,
    accepted: true,
    onSource: true,
  });
  await expect(source).toHaveCSS("opacity", "1");
}

test("sidebar projects drop immediately and keep their order after reload", async ({ page }) => {
  await addProject(page, REPO + "/app");
  await addProject(page, REPO + "/web");
  const rows = sidebar(page).locator('[draggable="true"]');
  const moved = rows.filter({ hasText: REPO + "/app" });
  const orders = [];
  page.on("request", (r) => {
    if (r.method() === "POST" && r.url().endsWith("/api/projects/order")) orders.push(r);
  });
  const saved = page.waitForResponse((r) => r.request().method() === "POST" && r.url().endsWith("/api/projects/order"));

  await dragAndRelease(page, moved, rows.filter({ hasText: REPO + "/web" }));
  expect((await saved).ok()).toBe(true);
  expect(orders).toHaveLength(1);
  await expect(rows.nth(1)).toContainText(REPO + "/app");
  await page.reload();
  await expect(rows.nth(1)).toContainText(REPO + "/app");
});

for (const location of ["sidebar", "project page"]) {
  test(`${location} tasks drop immediately and keep their order after reload`, async ({ page }) => {
    await addProject(page);
    for (const title of ["first drag task", "second drag task"]) {
      await openProject(page);
      await newTask(page);
      await pickModel(page);
      await sendPrompt(page, title);
      await expect(sidebar(page).getByText(`Refined: ${title}`, { exact: true })).toBeVisible();
    }
    await openProject(page);
    const panel = location === "sidebar" ? sidebar(page) : page.locator("main");
    const rows = panel.locator('[draggable="true"]').filter({ hasText: "drag task" });
    const orders = [];
    const isOrder = (r) => r.method() === "POST" && /\/sessions\/order$/.test(r.url());
    page.on("request", (r) => {
      if (isOrder(r)) orders.push(r);
    });
    const saved = page.waitForResponse((r) => isOrder(r.request()));

    await dragAndRelease(page, rows.filter({ hasText: "first drag task" }), rows.filter({ hasText: "second drag task" }));
    expect((await saved).ok()).toBe(true);
    expect(orders).toHaveLength(1);
    await expect(rows.nth(1)).toContainText("first drag task");
    await page.reload();
    await openProject(page);
    await expect(rows.nth(1)).toContainText("first drag task");
  });
}

test("file tabs drop immediately and keep their order after reload", async ({ page }) => {
  await addProject(page, REPO + "/e2e");
  await openProject(page, REPO + "/e2e");
  await sidebar(page).getByRole("tab", { name: "Tree" }).click();
  for (const name of ["fixtures.js", "playwright.config.js"]) {
    await sidebar(page).getByRole("button", { name, exact: true }).dblclick();
    await expect(page.getByRole("tab", { name, exact: true })).toBeVisible();
  }
  const rows = page.locator('main [role="tab"][draggable="true"]');
  await dragAndRelease(page, rows.filter({ hasText: "fixtures.js" }), rows.filter({ hasText: "playwright.config.js" }));
  await expect(rows.nth(2)).toHaveAttribute("aria-label", "fixtures.js");
  await expect.poll(() => page.evaluate(() => {
    const tabs = JSON.parse(localStorage.getItem("agenttik.openTabs") || "{}").tabs || [];
    return tabs.filter((tab) => tab.kind === "file").map((tab) => tab.path);
  })).toEqual(["playwright.config.js", "fixtures.js"]);
  await page.reload();
  await expect(rows.nth(2)).toHaveAttribute("aria-label", "fixtures.js");
});
