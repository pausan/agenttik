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

  // Send now on a queued prompt joins the turn too. A transcript read is
  // slow on a long task, so it lands after the rows streamed meanwhile; held
  // back here, one that replaced the transcript would wipe this prompt's
  // reply. Only the queue may be read back.
  const prompt = page.getByPlaceholder("Ask the agent…");
  const queued = page.getByLabel("Queued prompt", { exact: true });
  await prompt.fill(["@wait 1000", "Any news on the build?"].join("\n"));
  await prompt.press("Enter");
  await expect(queued).toHaveCount(1);
  const transcript = /\/api\/sessions\/[^/?]+$/;
  await page.route(transcript, async (route) => {
    if (route.request().method() !== "GET") return route.continue();
    const response = await route.fetch();
    await new Promise((resolve) => setTimeout(resolve, 1500));
    await route.fulfill({ response });
  });
  await queued.getByRole("button", { name: "Send now" }).click();
  await expect(queued).toHaveCount(0);
  await page.waitForTimeout(3000);
  // The reply is stored only once it ends, well after a read taken on click.
  await expect(page.locator("main").getByText("Any news on the build?", { exact: true })).toBeVisible();
  await expect(page.locator("main").getByText(/@wait 1000\s+Any news on the build\?/)).toBeVisible();
  await expect(working).toContainText("Waiting for 1 background task");
  await page.unroute(transcript);

  await prompt.fill(["@background end b1 completed built", "All done."].join("\n"));
  await prompt.press("Enter");
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

/* A prompt the turn accepted is stored at once. If the task stops before the
   agent gets to it, it stays in the transcript, marked as not processed. */
test("a prompt the agent never got to stays, marked as not processed", async ({ page }) => {
  await addProject(page);
  await openProject(page);
  await newTask(page);
  await pickModel(page);
  await sendPrompt(page, ["@open", "@background start b1 build game A", "Build started."].join("\n"));
  const working = page.getByLabel("Agent working");
  await expect(working).toContainText("Waiting for 1 background task");
  // The agent starts on one prompt and is kept busy by it.
  await sendPrompt(page, ["@wait 20000", "Checked."].join("\n"));
  await expect(working).toContainText("Working · 1 background task");

  const prompt = page.getByPlaceholder("Ask the agent…");
  const queued = page.getByLabel("Queued prompt", { exact: true });
  await prompt.fill("Is game B next?");
  await prompt.press("Enter");
  await queued.getByRole("button", { name: "Send now" }).click();
  await expect(queued).toHaveCount(0);
  const asked = page.locator("[data-message-id]").filter({ hasText: "Is game B next?" });
  await expect(asked).toContainText("Waiting for the agent");

  await page.getByRole("button", { name: "Stop", exact: true }).click();
  await expect(working).toHaveCount(0);
  await expect(asked).toContainText("Not processed");
  await expect(page.locator("[data-message-id]").last()).toContainText("Is game B next?");
  await page.reload();
  await expect(asked).toContainText("Not processed");
});

/* A prompt the agent never got to goes again as it is, without editing it,
   which would cut the transcript below it. Send runs it as the prompt bar
   would — now, or into a turn that takes prompts — and Enqueue queues it.
   Either moves it to the end. */
test("a prompt the agent never processed can be sent or queued again", async ({ page }) => {
  await addProject(page);
  await openProject(page);
  await newTask(page);
  await pickModel(page);
  await sendPrompt(page, ["@wait 20000", "Checked."].join("\n"));
  const working = page.getByLabel("Agent working");
  await expect(working).toBeVisible();
  const prompt = page.getByPlaceholder("Ask the agent…");
  const queued = page.getByLabel("Queued prompt", { exact: true });
  for (const text of ["Is game B next?", "And game C?", "And game D?"]) {
    await prompt.fill(text);
    await prompt.press("Enter");
  }
  await expect(queued).toHaveCount(3);
  await page.getByRole("button", { name: "Stop", exact: true }).click();
  await expect(working).toHaveCount(0);

  const mine = page.locator("[data-message-id]").filter({ has: page.getByText("You", { exact: true }) });
  const [b, c, d] = ["Is game B next?", "And game C?", "And game D?"].map((text) => mine.filter({ hasText: text }));
  for (const asked of [b, c, d]) await expect(asked).toContainText("Not processed");
  const text = (value) => page.getByText(value, { exact: true });
  const before = async (left, right) =>
    expect(await left.evaluate((node, next) => !!(node.compareDocumentPosition(next) & Node.DOCUMENT_POSITION_FOLLOWING),
      await right.elementHandle())).toBe(true);

  // Sent while idle, it runs now, below the prompts still unprocessed.
  await b.getByRole("button", { name: "Send", exact: true }).click();
  await expect(text("Is game B next?")).toHaveCount(2);
  await expect(working).toHaveCount(0);
  await expect(b).not.toContainText("Not processed");
  await before(d, b);

  // Sent into a turn waiting on a background task, it joins that turn.
  await sendPrompt(page, ["@open", "@background start b1 build game A", "Build started."].join("\n"));
  await expect(working).toContainText("Waiting for 1 background task");
  await c.getByRole("button", { name: "Send", exact: true }).click();
  await expect(text("And game C?")).toHaveCount(2);
  await expect(c).not.toContainText("Not processed");
  await expect(working).toContainText("Waiting for 1 background task");
  await before(text("Build started."), c);
  await sendPrompt(page, ["@background end b1 completed built", "All done."].join("\n"));
  await expect(working).toHaveCount(0);

  // While the agent is busy it can only be queued, and runs after.
  await sendPrompt(page, ["@wait 3000", "Built."].join("\n"));
  await expect(working).toBeVisible();
  await expect(d.getByRole("button", { name: "Send", exact: true })).toBeDisabled();
  await d.getByRole("button", { name: "Enqueue", exact: true }).click();
  await expect(queued).toHaveCount(1);
  await expect(queued).toContainText("And game D?");
  await expect(d).toHaveCount(0);
  await expect(text("And game D?")).toHaveCount(2);
  await expect(working).toHaveCount(0);
  await expect(queued).toHaveCount(0);

  const order = async () => {
    await expect(page.getByText("Not processed")).toHaveCount(0);
    await before(b, text("Build started."));
    await before(c, text("All done."));
    await before(text("All done."), text("Built."));
    await before(text("Built."), d);
  };
  await order();
  await page.reload();
  await expect(text("And game D?")).toHaveCount(2);
  await order();
});

/* A turn waiting on background tasks runs on the model it started on. A
   prompt meant for another asks first: it can go on the turn's model, stay
   where it is, or — queued — run on its own by stopping the turn. */
test("a prompt for another model asks before going into the running turn", async ({ page }) => {
  await addProject(page);
  await openProject(page);
  await newTask(page);
  await pickModel(page);
  await sendPrompt(page, ["@open", "@background start b1 build game A", "Build started."].join("\n"));
  const working = page.getByLabel("Agent working");
  await expect(working).toContainText("Waiting for 1 background task");
  await pickModel(page, "Fake Careful");

  const dialog = page.getByRole("dialog", { name: "Send on the running turn's model?" });
  const prompt = page.getByPlaceholder("Ask the agent…");
  const text = (value) => page.getByText(value, { exact: true });
  // Cancelled, nothing is sent and the text stays in the box.
  await sendPrompt(page, "Status of game A?");
  await expect(dialog.getByLabel("Running turn")).toContainText("Fake Quick");
  await expect(dialog.getByLabel("Prompt's model")).toContainText("Fake Careful");
  await expect(dialog.getByRole("button", { name: /^Force send/ })).toHaveCount(0);
  await dialog.getByRole("button", { name: "Cancel" }).click();
  await expect(dialog).toHaveCount(0);
  await expect(prompt).toHaveValue("Status of game A?");
  await expect(page.locator("[data-message-id]").filter({ hasText: "Status of game A?" })).toHaveCount(0);

  // Sent on the turn's model, it joins the turn, which keeps its task.
  await prompt.press("Control+Enter");
  await dialog.getByRole("button", { name: "Send with Fake Quick", exact: true }).click();
  await expect(text("Status of game A?")).toHaveCount(2);
  await expect(working).toContainText("Waiting for 1 background task");

  // Queued, it keeps the task's new model; Send now can stop the turn to run
  // it on that model.
  await prompt.fill("Deploy game A.");
  await prompt.press("Enter");
  const queued = page.getByLabel("Queued prompt", { exact: true });
  await expect(queued).toHaveCount(1);
  const sendNow = queued.getByRole("button", { name: "Send now…" });
  await expect(sendNow).toHaveAttribute("title", /another model/);
  await sendNow.click();
  await expect(dialog.getByLabel("Prompt's model")).toContainText("Fake Careful");
  await dialog.getByRole("button", { name: "Force send with Fake Careful" }).click();
  await expect(queued).toHaveCount(0);
  await expect(text("Deploy game A.")).toHaveCount(2);
  await expect(working).toHaveCount(0);
  await expect(page.getByLabel("Background task", { exact: true })).toContainText("stopped");
});
