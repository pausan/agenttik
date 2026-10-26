import { addProject, expect, newTask, openProject, pickModel, sendPrompt, test } from "../fixtures.js";

test("every transcript message shows when it was written", async ({ page }) => {
  await addProject(page);
  await openProject(page);
  await newTask(page);
  await pickModel(page);
  await sendPrompt(page, "timestamp task");
  await expect(page.getByText("idle", { exact: true })).toBeVisible();
  const times = page.locator("[data-message-id] time");
  await expect.poll(() => times.count()).toBeGreaterThanOrEqual(2);
  for (const time of await times.all()) {
    await expect(time).toHaveText(/^\d\d:\d\d:\d\d$/);
    await expect(time).toHaveAttribute("title", /\d{4}/);
  }
  await page.locator("main").screenshot({ path: test.info().outputPath("message-times.png") });
});
