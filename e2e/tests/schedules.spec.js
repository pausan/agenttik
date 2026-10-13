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

for (const kind of ["Every N days", "Selected weekdays"]) {
  test(`${kind} can be created, edited and reloaded`, async ({ page }) => {
    await addProject(page);
    await openProject(page);
    await newTask(page);
    await pickModel(page);
    await page.getByPlaceholder("Ask the agent…").fill("Check the project each morning");
    await page.getByRole("button", { name: "More prompt actions" }).click();
    await page.getByRole("button", { name: "Schedule…" }).click();
    const dialog = page.getByRole("dialog");
    await dialog.getByRole("button", { name: "Repeat every", exact: true }).click();
    await page.getByRole("option", { name: kind, exact: true }).click();
    await dialog.getByLabel("Time of day", { exact: true }).fill("09:30");
    if (kind === "Every N days") {
      const days = dialog.getByRole("spinbutton", { name: "Days between runs" });
      await days.click();
      await days.fill("3");
      await expect(days).toHaveValue("3");
    } else {
      // Empty selections cannot create a schedule.
      for (const day of ["Mon", "Tue", "Wed", "Thu", "Fri"]) {
        await dialog.getByRole("checkbox", { name: day, exact: true }).uncheck();
      }
      await expect(dialog.getByRole("button", { name: "Schedule", exact: true })).toBeDisabled();
      for (const day of ["Mon", "Wed", "Fri"]) {
        await dialog.getByRole("checkbox", { name: day, exact: true }).check();
      }
    }
    await dialog.getByRole("button", { name: "Schedule", exact: true }).click();
    const main = page.locator("main");
    const initialLabel = kind === "Every N days" ? "Every 3 days at 09:30" : "Mon, Wed, Fri at 09:30";
    await expect(main.getByText(initialLabel, { exact: false })).toBeVisible();

    let finalLabel;
    if (kind === "Every N days") {
      const days = main.getByRole("spinbutton", { name: "Days between runs" });
      await days.fill("4");
      await days.press("Tab");
      finalLabel = "Every 4 days at 09:30";
    } else {
      await main.getByRole("checkbox", { name: "Sun", exact: true }).check();
      finalLabel = "Mon, Wed, Fri, Sun at 09:30";
    }
    await expect(main.getByText(finalLabel, { exact: false })).toBeVisible();
    await page.reload();
    await expect(main.getByText(finalLabel, { exact: false })).toBeVisible();
    await expect(main.getByLabel("Time of day", { exact: true })).toHaveValue("09:30");
  });
}
