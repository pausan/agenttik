import { addProject, expect, newTask, openProject, pickModel, sendPrompt, test } from "../fixtures.js";

test("a finished turn ends with its time, tokens and cost", async ({ page }) => {
  await addProject(page);
  await openProject(page);
  await newTask(page);
  await pickModel(page);
  await sendPrompt(page, "hello there");

  const summary = page.getByLabel("Turn usage");
  await expect(summary).toHaveText(/^\d+s · 42 in \/ 17 out · <\$0\.01$/);
});
