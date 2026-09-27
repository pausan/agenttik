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

for (const width of [1440, 390]) {
  test(`editor and preview expand without losing edits at ${width}px`, async ({ page }) => {
    // Open the file through the desktop tree before checking either layout.
    await page.setViewportSize({ width: 1440, height: 900 });
    await addProject(page);
    await openProject(page);
    await sidebar(page).getByRole("tab", { name: "Tree" }).click();
    await sidebar(page).getByRole("button", { name: "AGENTS.md", exact: true }).click();
    const editor = page.getByRole("textbox", { name: "AGENTS.md", exact: true });
    await editor.fill("# Unsaved fullscreen preview\n\nSome text.\n");
    await page.setViewportSize({ width, height: 900 });
    const main = page.getByRole("main");
    const original = await main.boundingBox();
    for (const mode of ["editor", "preview"]) {
      if (mode === "preview") await page.getByRole("tab", { name: "Preview", exact: true }).click();
      const markdown = main.locator(".markdown");
      async function checkWidth() {
        const sizes = await markdown.evaluate((el) => ({
          width: el.getBoundingClientRect().width,
          available: el.parentElement.clientWidth,
          left: getComputedStyle(el).paddingLeft,
          right: getComputedStyle(el).paddingRight,
        }));
        expect(sizes.width).toBe(sizes.available);
        expect(sizes.left).toBe("32px");
        expect(sizes.right).toBe("32px");
      }
      if (mode === "preview") await checkWidth();
      for (const exit of ["button", "escape"]) {
        await page.getByRole("button", { name: `Expand ${mode}`, exact: true }).click();
        await expect(page.locator(".main-tab-bar")).toBeHidden();
        expect(await main.boundingBox()).toEqual({ x: 0, y: 0, ...page.viewportSize() });
        if (mode === "preview") {
          await expect(markdown.getByRole("heading", { name: "Unsaved fullscreen preview" })).toBeVisible();
          await checkWidth();
        } else await expect(editor).toHaveValue("# Unsaved fullscreen preview\n\nSome text.\n");
        if (exit === "button") await page.getByRole("button", { name: `Restore ${mode}`, exact: true }).click();
        else await page.keyboard.press("Escape");
        await expect(page.locator(".main-tab-bar")).toBeVisible();
        expect(await main.boundingBox()).toEqual(original);
      }
    }
    await page.getByRole("tab", { name: "Edit", exact: true }).click();
    await expect(editor).toHaveValue("# Unsaved fullscreen preview\n\nSome text.\n");
  });
}
