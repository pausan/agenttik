import { addProject, expect, newTask, openProject, pickModel, sendPrompt, test } from "../fixtures.js";

test("each finished turn ends with its usage and the task's totals so far", async ({ page }) => {
  await addProject(page);
  await openProject(page);
  await newTask(page);
  await pickModel(page);
  await sendPrompt(page, "hello there");

  const summary = page.getByLabel("Turn usage");
  await expect(summary).toHaveText(/^\d+s · 42 in \/ 17 out · <\$0\.01$/);

  await sendPrompt(page, "and again");
  await expect(summary).toHaveCount(2);
  await expect(summary.last()).toHaveText(/^\d+s · 42 in \/ 17 out · <\$0\.01 — total \d+s · 84 in \/ 34 out · <\$0\.01$/);
});
