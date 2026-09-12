import {
  FAKE_MODEL,
  addProject,
  expect,
  modelButton,
  newTask,
  openProject,
  pickModel,
  sendPrompt,
  sidebar,
  test,
} from "../fixtures.js";

test("a new task asks nothing and opens ready to prompt", async ({ page }) => {
  await addProject(page);
  await openProject(page);
  await newTask(page);

  await expect(page.getByRole("tab", { name: "New task" })).toBeVisible();
  // The primary action is whichever of Send/Enqueue plain Enter currently
  // performs — see shortcuts.js — so the submit button is found by kind.
  await expect(page.locator('button[type="submit"]')).toBeEnabled();
  await expect(page.getByRole("button", { name: "Stop" })).toBeHidden();
  await expect(page.getByText("idle")).toBeVisible();
  await expect(sidebar(page).getByText("Untitled task")).toBeVisible();
});

test("an empty prompt sends nothing", async ({ page }) => {
  await addProject(page);
  await openProject(page);
  await newTask(page);

  await page.getByPlaceholder("Ask the agent…").fill("   ");
  await page.locator('button[type="submit"]').click();

  await expect(page.getByText("running", { exact: true })).toBeHidden();
  await expect(page.getByText("You", { exact: true })).toHaveCount(0);
});

test("setup lists the providers and whether their CLI is installed", async ({ page }) => {
  await page.getByRole("button", { name: "Settings" }).click();
  await page.getByRole("button", { name: "Models" }).click();

  // Every pane of Settings stays mounted, and the Subscriptions one lists the
  // same providers, so each is asked for as its own group in this list.
  await expect(page.getByRole("group", { name: "Claude Code models" })).toBeVisible();
  await expect(page.getByRole("group", { name: "Codex models" })).toBeVisible();
  // The fake provider spawns no CLI, so it is ready regardless of what is
  // installed on the machine running the suite.
  await expect(page.getByRole("group", { name: "Fake models" })).toBeVisible();
  await expect(page.getByTitle(`Star ${FAKE_MODEL} · low`)).toBeVisible();
  await expect(page.getByText("ready")).not.toHaveCount(0);

  // Every model gets a chip per effort, plus default.
  await expect(page.getByRole("button", { name: "default", exact: true }).first()).toBeVisible();
  await expect(page.getByRole("button", { name: "xhigh", exact: true }).first()).toBeVisible();
});

test("a starred model and effort heads the picker and sets both at once", async ({ page }) => {
  await page.getByRole("button", { name: "Settings" }).click();
  await page.getByRole("button", { name: "Models" }).click();
  await page.getByTitle(`Star ${FAKE_MODEL} · high`).click();
  await expect(page.getByTitle(`Star ${FAKE_MODEL} · high`)).toHaveText("★ high");
  await page.getByRole("button", { name: "Done" }).click();

  await addProject(page);
  await openProject(page);
  await newTask(page);

  await modelButton(page).click();
  await expect(page.getByRole("option").first()).toContainText("★");
  await page.getByRole("option").first().click();

  // Picking the combination set the effort too.
  await expect(page.getByTitle("Effort")).toContainText("high");
  await expect(page.getByText("high", { exact: true })).not.toHaveCount(0);
});

test("changing the model updates the badge and the stats", async ({ page }) => {
  await addProject(page);
  await openProject(page);
  await newTask(page);

  await pickModel(page, "Fake Quick");
  await expect(page.getByText("fake-quick", { exact: true })).toBeVisible();

  await pickModel(page, "Fake Careful");
  await expect(page.getByText("fake-careful", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Task stats" }).click();
  await expect(page.getByLabel("Task statistics").getByText("fake-careful", { exact: true })).toBeVisible();
});

test("sending a prompt streams a reply and updates the stats", async ({ page }) => {
  await addProject(page);
  await openProject(page);
  await newTask(page);
  await pickModel(page);
  await sendPrompt(page, "fix the flaky queue test");

  await expect(page.getByText("Agent", { exact: true })).toBeVisible();
  await expect(page.getByText("idle")).toBeVisible();

  await page.getByRole("button", { name: "Task stats" }).click();
  const stats = page.getByLabel("Task statistics");
  await expect(stats.getByText("$0.0042")).toBeVisible();
  await expect(stats.getByText("fake-quick", { exact: true })).toBeVisible();
  await expect(stats.locator("dt").nth(0)).toHaveText("Agent time");
  await expect(stats.locator("dt").nth(1)).toHaveText("Started");
  await expect(stats.locator("dt").nth(2)).toHaveText("Last message");
  await expect(stats.locator("dd").nth(2)).not.toHaveText("never");
});

test("a task's title is refined shortly after its first prompt", async ({ page }) => {
  await addProject(page);
  await openProject(page);
  await newTask(page);
  await pickModel(page);
  await sendPrompt(page, "fix the flaky queue test");

  // The first line names it at once; the fake provider's small model replaces
  // that placeholder a beat later, with a reply that still names the prompt.
  // See specs/020-task-titles.md.
  await expect(sidebar(page).getByText("Refined: fix the flaky queue test")).toBeVisible();
});

test("a run of tool calls collapses to its latest and expands on click", async ({ page }) => {
  await addProject(page);
  await openProject(page);
  await newTask(page);
  await pickModel(page);
  await sendPrompt(page, "@tool Bash echo hi\n@tool Read a.go\n@tool Read b.go\nAll good.");

  // exact: true throughout — the raw prompt is echoed verbatim in the user's
  // own bubble, which contains each of these as a substring too.
  await expect(page.getByText("All good.", { exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "3 tool calls" })).toBeVisible();
  await expect(page.getByText("Read b.go", { exact: true })).toBeVisible();
  await expect(page.getByText("Bash echo hi", { exact: true })).toHaveCount(0);

  await page.getByRole("button", { name: "3 tool calls" }).click();
  await expect(page.getByText("Bash echo hi", { exact: true })).toBeVisible();
  await expect(page.getByText("Read a.go", { exact: true })).toBeVisible();
});

test("Stop cancels a running turn without recording an error", async ({ page }) => {
  await addProject(page);
  await openProject(page);
  await newTask(page);
  await pickModel(page);
  await sendPrompt(page, "@wait 20000");

  await expect(page.getByText("running", { exact: true })).toBeVisible();
  // exact: true — the sidebar's own "Stop task" button also matches "Stop"
  // as a substring.
  const stop = page.getByRole("button", { name: "Stop", exact: true });
  await expect(stop).toBeVisible();
  await stop.click();

  await expect(stop).toBeHidden();
  await expect(page.getByText("idle")).toBeVisible();
  await expect(page.locator(".text-error")).toHaveCount(0);
});

test("a turn that fails outright keeps its prompt and shows the error", async ({ page }) => {
  await addProject(page);
  await openProject(page);
  await newTask(page);
  await pickModel(page);
  await sendPrompt(page, "@error something broke");

  // exact: true — the raw prompt, echoed verbatim in the user's own bubble,
  // contains the same words.
  await expect(page.getByText("something broke", { exact: true })).toBeVisible();
  await expect(page.getByText("You", { exact: true })).toBeVisible();
  await expect(page.getByText("idle")).toBeVisible();
});

test("a provider outage queues the prompt instead of losing it", async ({ page }) => {
  await addProject(page);
  await openProject(page);
  await newTask(page);
  await pickModel(page);
  await sendPrompt(page, "@error rate limit reached");

  await expect(page.getByText("waiting", { exact: true })).toBeVisible();
  await expect(page.getByText("Waiting for Fake")).toBeVisible();

  // Scoped to the queued bubble itself: the task is also titled from this
  // same raw prompt, and that sidebar/tab label matches the text too.
  const queuedPrompt = page.getByLabel("Queued prompt");
  await expect(queuedPrompt.getByText("@error rate limit reached")).toBeVisible();
  await expect(queuedPrompt.getByText(/retrying in \d/)).toBeVisible();

  // The failed attempt produced nothing, so it is erased rather than kept as
  // a sent-then-failed turn — see specs/045-provider-outage-retry.md.
  await expect(page.getByText("You", { exact: true })).toHaveCount(0);
});

test("a second prompt queues behind a running turn and runs in order", async ({ page }) => {
  await addProject(page);
  await openProject(page);
  await newTask(page);
  await pickModel(page);
  await sendPrompt(page, "@wait 1000");
  await expect(page.getByText("running", { exact: true })).toBeVisible();

  await page.getByPlaceholder("Ask the agent…").fill("second prompt");
  await page.getByRole("button", { name: "More prompt actions" }).click();
  await page.getByRole("dialog").getByRole("button", { name: "Enqueue", exact: true }).click();
  await expect(page.getByText("Queued", { exact: true })).toBeVisible();

  // The first turn finishes, the scheduler starts the second one, and it
  // finishes too.
  await expect(page.getByText("idle")).toBeVisible({ timeout: 10_000 });
  await expect(page.getByText("Queued", { exact: true })).toHaveCount(0);
});

test("Stop keeps an unstarted task's text after reload without replacing its draft", async ({ page }) => {
  await addProject(page);
  await openProject(page);
  await newTask(page);
  await pickModel(page);
  await sendPrompt(page, "@wait 60000");
  await expect(page.getByText("running", { exact: true })).toBeVisible();

  await newTask(page);
  await pickModel(page);
  const prompt = "Keep every detail of this queued request";
  await page.getByPlaceholder("Ask the agent…").fill(prompt);
  await page.getByRole("button", { name: "More prompt actions" }).click();
  await page.getByRole("dialog").getByRole("button", { name: "Enqueue", exact: true }).click();
  await expect(page.getByLabel("Queued prompt").getByText(prompt)).toBeVisible();
  await page.getByPlaceholder("Ask the agent…").fill("An existing draft");

  const row = sidebar(page).locator("[draggable]").filter({ hasText: prompt });
  await row.getByRole("button", { name: "Stop task", exact: true }).click();
  await expect(page.getByLabel("Queued prompt")).toHaveCount(0);
  await expect(page.getByText("You", { exact: true })).toHaveCount(1);
  await expect(page.getByPlaceholder("Ask the agent…")).toHaveValue("An existing draft");
  await page.reload();
  await expect(page.getByText("You", { exact: true })).toHaveCount(1);
  await expect(page.locator(".group.mb-4").getByText(prompt, { exact: true })).toBeVisible();
  await expect(page.getByLabel("Queued prompt")).toHaveCount(0);
});

test("the subscription allowance bars show the provider's reported usage", async ({ page }) => {
  await addProject(page);
  await openProject(page);
  await newTask(page);
  await pickModel(page);

  await page.getByRole("button", { name: "Context and subscription usage" }).click();
  await expect(page.getByText("Fake plan subscription")).toBeVisible();
  await expect(page.getByText("42%")).toBeVisible();
  await expect(page.getByText("7%")).toBeVisible();
});

test("the transcript follows the task, and the prompt bar goes away with it", async ({ page }) => {
  await addProject(page);
  await openProject(page);
  await newTask(page);

  await openProject(page);
  await expect(page.getByPlaceholder("Ask the agent…")).toBeHidden();
  await expect(page.getByText("1 task", { exact: true })).toBeVisible();
});

test("clicking a task in the sidebar leaves the cursor in its prompt box", async ({ page }) => {
  await addProject(page);
  await openProject(page);
  await newTask(page);

  // From the project page: there is no prompt bar until the click mounts one,
  // which is the case the focus request has to survive.
  await openProject(page);
  await sidebar(page).getByText("Untitled task").click();

  const prompt = page.getByPlaceholder("Ask the agent…");
  await expect(prompt).toBeFocused();
  await page.keyboard.type("straight into the box");
  await expect(prompt).toHaveValue("straight into the box");
});

test("Ctrl+W closes the active tab, falling back to the project when a task closes", async ({ page }) => {
  await addProject(page);
  await openProject(page);
  await newTask(page);

  await page.keyboard.press("Control+w");
  await expect(page.getByPlaceholder("Ask the agent…")).toBeHidden();
  await expect(page.getByRole("heading", { name: "agenttik" })).toBeVisible();

  await page.keyboard.press("Control+w");
  await expect(page.getByRole("heading", { name: "agenttik" })).toHaveCount(0);
  await expect(page.getByText("Pick a task, or a project to start one.")).toBeVisible();
});
