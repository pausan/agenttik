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
