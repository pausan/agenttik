import {
  addProject,
  expect,
  newTask,
  openProject,
  pickModel,
  sidebar,
  test,
} from "../fixtures.js";

/* Schedule the prompt in the box: the Send menu's third action, then the
   dialog's own Schedule button. It leaves a job behind and opens its page. */
async function schedulePrompt(page, text) {
  await page.getByPlaceholder("Ask the agent…").fill(text);
  await page.getByRole("button", { name: "More prompt actions" }).click();
  await page.getByRole("button", { name: "Schedule…" }).click();
  await page.getByRole("button", { name: "Schedule", exact: true }).click();
}

test("an archived job waits on the project page and comes back from it", async ({ page }) => {
  await addProject(page);
  await openProject(page);
  await newTask(page);
  await pickModel(page);
  await schedulePrompt(page, "water the plants");

  // The job names itself from the prompt, and a refined name replaces the
  // placeholder a moment later — so rows are matched on the prompt's words.
  const job = /water the plants/;
  await expect(sidebar(page).getByText(job)).toBeVisible();

  // Only a paused job can be archived: one hidden while it kept starting
  // tasks is the one state nothing in the UI would explain.
  await page.getByRole("button", { name: "Pause", exact: true }).click();
  await page.getByTitle("Archive schedule").click();
  await expect(sidebar(page).getByText(job)).toBeHidden();

  // It waits in the project page's Jobs tab, where the restore icon is the
  // way back. Paused on the way out, paused on the way in.
  await openProject(page);
  await page.getByRole("tab", { name: "Jobs" }).click();
  const row = page.locator("main").getByText(job);
  await expect(row).toBeVisible();
  await expect(page.getByTitle("Unarchive schedule")).toBeVisible();
  await expect(page.getByTitle("Pause schedule")).toHaveCount(0);

  await page.getByTitle("Unarchive schedule").click();
  await expect(sidebar(page).getByText(job)).toBeVisible();
  await expect(page.getByTitle("Resume schedule")).not.toHaveCount(0);
});
