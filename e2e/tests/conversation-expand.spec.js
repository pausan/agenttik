import { addProject, expect, inspector, newTask, openProject, pickModel, sendPrompt, sidebar, test } from "../fixtures.js";

test("conversation expands to the window width and restores with its button or Escape", async ({ page }) => {
  await page.setViewportSize({ width: 1600, height: 900 });
  await addProject(page);
  await openProject(page);
  await newTask(page);
  await pickModel(page);
  await sendPrompt(page, "expand task");
  await expect(page.getByText("idle", { exact: true })).toBeVisible();
  const main = page.getByRole("main");
  const text = page.locator("[data-transcript] > .relative");
  const original = await main.boundingBox();
  expect((await text.boundingBox()).width).toBe(860);
  for (const exit of ["button", "escape", "new task"]) {
    await page.getByRole("button", { name: "Expand conversation", exact: true }).click();
    await expect(sidebar(page)).toBeHidden();
    await expect(inspector(page)).toBeHidden();
    await expect(page.locator(".main-tab-bar")).toBeVisible();
    expect(await main.boundingBox()).toEqual({ x: 0, y: 0, ...page.viewportSize() });
    expect((await text.boundingBox()).width).toBeGreaterThan(1500);
    if (exit === "button") await page.getByRole("button", { name: "Restore conversation", exact: true }).click();
    else if (exit === "escape") await page.keyboard.press("Escape");
    else await page.keyboard.press("Control+n");
    await expect(sidebar(page)).toBeVisible();
    await expect(inspector(page)).toBeVisible();
    expect(await main.boundingBox()).toEqual(original);
    if (exit === "new task") break;
    expect((await text.boundingBox()).width).toBe(860);
  }
});
