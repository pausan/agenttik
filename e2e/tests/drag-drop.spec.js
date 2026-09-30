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

/* Hover shows the destination while every row stays in place until release. */
async function dragAndRelease(page, source, target, cancel = false) {
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
  const tab = await source.getAttribute("role") === "tab";
  const after = tab ? from.x < to.x : from.y < to.y;
  const x = to.x + to.width * (tab ? (after ? 0.8 : 0.2) : 0.5);
  const y = to.y + to.height * (tab ? 0.5 : (after ? 0.8 : 0.2));
  await page.mouse.move(from.x + from.width / 2, from.y + from.height / 2);
  await page.mouse.down();
  await page.mouse.move(from.x + from.width / 2 + 8, from.y + from.height / 2, { steps: 3 });
  await page.mouse.move(x, y, { steps: 5 });
  await expect(source).toHaveCSS("opacity", "1");
  expect(await source.boundingBox()).toEqual(from);
  expect(await target.boundingBox()).toEqual(to);
  await page.mouse.move(x + 1, y);
  const marker = page.locator("[data-drop-position]");
  await expect(marker).toHaveCount(1);
  await expect(target.locator("xpath=ancestor-or-self::*[@data-drop-position]")).toHaveCount(1);
  await expect(source.locator("xpath=ancestor-or-self::*[@data-drop-position]")).toHaveCount(0);
  await expect(marker).toHaveAttribute("data-drop-position", after ? "after" : "before");
  expect(await marker.evaluate((el, isTab) => {
    const style = getComputedStyle(el, "::after");
    return { thickness: isTab ? style.width : style.height, events: style.pointerEvents };
  }, tab)).toEqual({ thickness: "2px", events: "none" });
  if (cancel) await page.keyboard.press("Escape");
  await page.mouse.up();
  await expect(marker).toHaveCount(0);
  if (cancel) {
    expect(await source.boundingBox()).toEqual(from);
    expect(await target.boundingBox()).toEqual(to);
    expect(await page.evaluate(() => window.dropEvents.length)).toBe(0);
    return;
  }

  expect(await page.evaluate(() => ({
    drops: window.dropEvents.length,
    accepted: window.dropEvents[0]?.defaultPrevented || false,
    onSource: window.dragged?.contains(window.dropEvents[0]?.target) || false,
  }))).toEqual({
    drops: 1,
    accepted: true,
    onSource: false,
  });
  await expect(source).toHaveCSS("opacity", "1");
}

test("sidebar projects stay still while dragging and save their order on drop", async ({ page }) => {
  await addProject(page, REPO + "/app");
  await addProject(page, REPO + "/web");
  const rows = sidebar(page).locator('[draggable="true"]');
  const moved = rows.filter({ hasText: REPO + "/web" });
  const orders = [];
  page.on("request", (r) => {
    if (r.method() === "POST" && r.url().endsWith("/api/projects/order")) orders.push(r);
  });
  const saved = page.waitForResponse((r) => r.request().method() === "POST" && r.url().endsWith("/api/projects/order"));

  await dragAndRelease(page, moved, rows.filter({ hasText: REPO + "/app" }));
  expect((await saved).ok()).toBe(true);
  expect(orders).toHaveLength(1);
  await expect(rows.nth(1)).toContainText(REPO + "/web");
  await page.reload();
  await expect(rows.nth(1)).toContainText(REPO + "/web");
});

for (const location of ["sidebar", "project page"]) {
  test(`${location} tasks stay still while dragging and save their order on drop`, async ({ page }) => {
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

    await dragAndRelease(page, rows.filter({ hasText: "second drag task" }), rows.filter({ hasText: "first drag task" }));
    expect((await saved).ok()).toBe(true);
    expect(orders).toHaveLength(1);
    await expect(rows.nth(1)).toContainText("second drag task");
    await page.reload();
    await openProject(page);
    await expect(rows.nth(1)).toContainText("second drag task");
  });
}

test("file tabs stay still while dragging and save their order on drop", async ({ page }) => {
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


test("cancelling a project drag leaves its order unchanged and sends no save", async ({ page }) => {
  await addProject(page, REPO + "/app");
  await addProject(page, REPO + "/web");
  const rows = sidebar(page).locator('[draggable="true"]');
  const orders = [];
  page.on("request", (r) => {
    if (r.method() === "POST" && r.url().endsWith("/api/projects/order")) orders.push(r);
  });
  const before = await rows.allTextContents();
  await dragAndRelease(page, rows.filter({ hasText: REPO + "/web" }), rows.filter({ hasText: REPO + "/app" }), true);
  expect(await rows.allTextContents()).toEqual(before);
  await page.reload();
  expect(await rows.allTextContents()).toEqual(before);
  expect(orders).toHaveLength(0);
});
