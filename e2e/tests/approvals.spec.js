import { addProject, expect, newTask, openProject, pickModel, sendPrompt, sidebar, test } from "../fixtures.js";

/* A tool approval is server state: every window connected to the server
   shows it, any of them can answer, and the first answer settles it for all.
   See specs/082-tool-approvals.md. */

async function askForApproval(page, command) {
  await addProject(page);
  await openProject(page);
  await newTask(page);
  await pickModel(page);
  await sendPrompt(page, `@approve Bash ${command}`);
  const card = page.getByRole("group", { name: "Tool approval" });
  await expect(card).toContainText("Allow Bash?");
  await expect(card).toContainText(command);
  await expect(page.getByLabel("Agent working")).toContainText("Waiting for approval");
  return card;
}

test("a second window sees the request, answers it, and the first window follows", async ({ page, agenttik }) => {
  const card = await askForApproval(page, "echo from-window-one");

  const other = await page.context().newPage();
  await other.goto(agenttik.url);
  // The sidebar marks the waiting task before its tab is open.
  const waiting = sidebar(other).locator(".task-row").filter({ has: other.getByLabel("Waiting for you") });
  await expect(waiting).toHaveCount(1);
  await waiting.locator(".task-select").click();
  const otherCard = other.getByRole("group", { name: "Tool approval" });
  await expect(otherCard).toContainText("echo from-window-one");

  await otherCard.getByRole("button", { name: "Allow" }).click();
  await expect(otherCard).toHaveCount(0);
  await expect(card).toHaveCount(0);
  await expect(page.getByText("allowed", { exact: true })).toBeVisible();
  await expect(sidebar(page).getByLabel("Waiting for you")).toHaveCount(0);
  await other.close();
});

test("a reloaded window still shows the waiting request and can deny it", async ({ page }) => {
  await askForApproval(page, "rm -rf build");
  await page.reload();
  const card = page.getByRole("group", { name: "Tool approval" });
  await expect(card).toContainText("rm -rf build");
  await card.getByRole("button", { name: "Deny" }).click();
  await expect(card).toHaveCount(0);
  await expect(page.getByText("denied", { exact: true })).toBeVisible();
});

test("stopping the task withdraws its request", async ({ page }) => {
  const card = await askForApproval(page, "sleep 100");
  await sidebar(page).getByRole("button", { name: "Stop task", exact: true }).click();
  await expect(card).toHaveCount(0);
  await expect(page.getByLabel("Agent working")).toHaveCount(0);
});

async function askQuestion(page, line) {
  await addProject(page);
  await openProject(page);
  await newTask(page);
  await pickModel(page);
  await sendPrompt(page, line);
  const card = page.getByRole("group", { name: "Agent question" });
  await expect(card).toBeVisible();
  await expect(page.getByLabel("Agent working")).toContainText("Waiting for your answer");
  return card;
}

test("a question is answered from another window with an option", async ({ page, agenttik }) => {
  const card = await askQuestion(page, "@ask Which color? | Red, Blue");
  await expect(card).toContainText("Which color?");

  const other = await page.context().newPage();
  await other.goto(agenttik.url);
  await sidebar(other).locator(".task-row").filter({ has: other.getByLabel("Waiting for you") }).locator(".task-select").click();
  const otherCard = other.getByRole("group", { name: "Agent question" });
  const send = otherCard.getByRole("button", { name: "Answer" });
  await expect(send).toBeDisabled();
  await otherCard.getByRole("radio", { name: "Blue" }).click();
  await expect(otherCard.getByRole("radio", { name: "Blue" })).toHaveAttribute("aria-checked", "true");
  await send.click();

  await expect(card).toHaveCount(0);
  await expect(otherCard).toHaveCount(0);
  await expect(page.getByText("answer=Blue", { exact: true })).toBeVisible();
  await other.close();
});

test("a typed answer replaces the pick, and a question can be skipped", async ({ page }) => {
  const card = await askQuestion(page, "@ask Which color? | Red, Blue");
  await card.getByRole("radio", { name: "Red" }).click();
  await card.getByLabel("Answer: Which color?").fill("Teal");
  await expect(card.getByRole("radio", { name: "Red" })).toHaveAttribute("aria-checked", "false");
  await card.getByRole("button", { name: "Answer" }).click();
  await expect(page.getByText("answer=Teal", { exact: true })).toBeVisible();

  await sendPrompt(page, "@ask Proceed? | Yes, No");
  const next = page.getByRole("group", { name: "Agent question" });
  await next.getByRole("button", { name: "Skip" }).click();
  await expect(next).toHaveCount(0);
  await expect(page.getByText("declined", { exact: true })).toBeVisible();
});
