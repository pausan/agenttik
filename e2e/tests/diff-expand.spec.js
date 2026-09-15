import { addProject, openProject, sidebar, inspector, test, expect } from "../fixtures.js";

test("diff expands to the app window and restores with its button or Escape", async ({ page }) => {
  await addProject(page);
  await openProject(page);
  await sidebar(page).getByRole("tab", { name: "Tree" }).click();
  await sidebar(page).getByRole("button", { name: "go.mod", exact: true }).click();
  await page.getByRole("tab", { name: "Diff", exact: true }).click();
  const main = page.getByRole("main");
  const original = await main.boundingBox();
  for (const exit of ["button", "escape", "mode"]) {
    await page.getByRole("button", { name: "Expand diff", exact: true }).click();
    await expect(sidebar(page)).toBeHidden();
    await expect(inspector(page)).toBeHidden();
    await expect(page.locator(".main-tab-bar")).toBeHidden();
    const expanded = await main.boundingBox();
    expect(expanded).toEqual({ x: 0, y: 0, ...page.viewportSize() });
    await expect(page.getByRole("button", { name: "Restore diff", exact: true })).toBeVisible();
    if (exit === "button") await page.getByRole("button", { name: "Restore diff", exact: true }).click();
    else if (exit === "escape") await page.keyboard.press("Escape");
    else await page.getByRole("tab", { name: "Edit", exact: true }).click();
    await expect(sidebar(page)).toBeVisible();
    await expect(inspector(page)).toBeVisible();
    await expect(page.locator(".main-tab-bar")).toBeVisible();
    expect(await main.boundingBox()).toEqual(original);
  }
});
