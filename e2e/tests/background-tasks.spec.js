import { addProject, expect, newTask, openProject, pickModel, sendPrompt, test } from "../fixtures.js";

/* Background work stays in the transcript between the tools that surround
   its first report, through updates, turn completion and reloads. */

test("background tasks stay between surrounding tools after updates and reloads", async ({ page }) => {
  await addProject(page);
  await openProject(page);
  await newTask(page);
  await pickModel(page);
  await sendPrompt(
    page,
    [
      "@tool Read before-task",
      "@background start b1 npm run dev",
      "@tool Read between-tasks",
      "@background start b2 make test",
      "@tool Read after-task",
      "@wait 300",
      "@background end b2 failed exit code 2",
      "@wait 3000",
      "Done.",
    ].join("\n"),
  );

  const tasks = page.getByLabel("Background task", { exact: true });
  const dev = tasks.filter({ hasText: "npm run dev" });
  const tests = tasks.filter({ hasText: "make test" });
  await expect(tasks).toHaveCount(2);
  await expect(dev).toContainText("started");
  await expect(dev).toContainText(/running \ds/);
  await expect(tests).toContainText("failed · exit code 2");

  const checkOrder = async () => {
    const before = page.getByText("Read before-task", { exact: true });
    const between = page.getByText("Read between-tasks", { exact: true });
    const after = page.getByText("Read after-task", { exact: true });
    for (const [left, right] of [[before, dev], [dev, between], [between, tests], [tests, after]]) {
      await expect(left).toBeVisible();
      await expect(right).toBeVisible();
      expect(await left.evaluate((node, next) => !!(node.compareDocumentPosition(next) & Node.DOCUMENT_POSITION_FOLLOWING),
        await right.elementHandle())).toBe(true);
    }
    await expect(page.getByRole("button", { name: /tool calls/ })).toHaveCount(0);
  };
  await checkOrder();

  // Reloading reads the same rows back from the server while the turn runs.
  await page.reload();
  await expect(tasks).toHaveCount(2);
  await checkOrder();

  await expect(page.getByText("Done.", { exact: true })).toBeVisible();
  await expect(page.getByLabel("Agent working")).toHaveCount(0);
  await expect(dev).toContainText("stopped · turn ended");
  await expect(tests).toContainText("failed · exit code 2");
  await checkOrder();
  await page.reload();
  await expect(tasks).toHaveCount(2);
  await expect(dev).toContainText("stopped · turn ended");
  await checkOrder();

  // The provider may reuse a task id in a later turn. It gets a new row.
  await sendPrompt(page, "@background start b2 another test\n@background end b2 completed passed");
  await expect(tasks).toHaveCount(3);
  await expect(tasks.last()).toContainText("completed · passed");
  await expect(tests).toContainText("failed · exit code 2");
  await expect(page.getByLabel("Turn usage")).toHaveCount(2);
  await page.reload();
  await expect(tasks).toHaveCount(3);
  await expect(tasks.last()).toContainText("completed · passed");
});

/* A turn that waits on background tasks takes prompts meanwhile: Send goes
   into it, and a queued prompt's Send now joins it instead of stopping it. */
test("prompts join a turn that waits on background tasks without stopping them", async ({ page }) => {
  await addProject(page);
  await openProject(page);
  await newTask(page);
  await pickModel(page);
  // The task starts after the title is refined, whose refresh would
  // otherwise redraw the sidebar on its own.
  await sendPrompt(page, ["@open", "@wait 1500", "@background start b1 build game A", "Build started."].join("\n"));

  const working = page.getByLabel("Agent working");
  const task = page.getByLabel("Background task", { exact: true });
  await expect(page.getByText("Build started.", { exact: true })).toBeVisible();
  await expect(working).toContainText("Waiting for 1 background task · you can keep writing");
  // The sidebar row says so too, for a task that is not on screen.
  const rowMarker = page.getByLabel("1 background task running · you can keep writing", { exact: true });
  await expect(rowMarker).toBeVisible();

  await sendPrompt(page, ["@tool Read status", "Still building."].join("\n"));
  await expect(page.getByText("Still building.", { exact: true })).toBeVisible();
  await expect(working).toContainText("Waiting for 1 background task");
  await expect(task).toContainText(/running \ds/);

  const prompt = page.getByPlaceholder("Ask the agent…");
  await prompt.fill(["@background end b1 completed built", "All done."].join("\n"));
  await prompt.press("Enter");
  const queued = page.getByLabel("Queued prompt", { exact: true });
  await expect(queued).toHaveCount(1);
  await queued.getByRole("button", { name: "Send now" }).click();
  await expect(page.getByText("All done.", { exact: true })).toBeVisible();
  await expect(working).toHaveCount(0);
  await expect(rowMarker).toHaveCount(0);
  await expect(task).toContainText("completed · built");
  await expect(queued).toHaveCount(0);

  // One turn holds all three prompts, in the order they were answered.
  const order = async () => {
    const texts = ["Build started.", "Still building.", "All done."].map((text) => page.getByText(text, { exact: true }));
    for (const [left, right] of [[task, texts[0]], [texts[0], texts[1]], [texts[1], texts[2]]]) {
      expect(await left.evaluate((node, next) => !!(node.compareDocumentPosition(next) & Node.DOCUMENT_POSITION_FOLLOWING),
        await right.elementHandle())).toBe(true);
    }
  };
  await order();
  await page.reload();
  await expect(page.getByText("All done.", { exact: true })).toBeVisible();
  await order();
});

/* The project page's task list follows a task's background work live, while
   the task itself is not on screen. */
test("the project page's task list shows background work as it changes", async ({ page }) => {
  await addProject(page);
  await openProject(page);
  await newTask(page);
  await pickModel(page);
  await sendPrompt(page, ["@open", "@wait 1500", "@background start b1 build game A", "Build started."].join("\n"));
  await openProject(page);
  const marker = page.locator("main").getByLabel("1 background task running · you can keep writing", { exact: true });
  await expect(marker).toBeVisible();
});
