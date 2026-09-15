/* One machine, two subscriptions per provider: adding one, choosing which a
   new task starts on, swapping a task to the other, and what removing one
   says. See specs/050-subscription-accounts.md.

   The fake provider holds subscriptions the way the real CLIs do — an alias
   and a directory — so all of this runs without a CLI on PATH or an
   allowance to spend. */
import {
  addProject,
  expect,
  modelButton,
  newTask,
  openProject,
  openSettings,
  pickModel,
  sendPrompt,
  test,
} from "../fixtures.js";

const FAKE = "Fake";

/* Every registered provider has a subscription list of its own, and each
   starts with a row called System, so a locator has to name the provider it
   means. */
const pane = (page) => page.getByRole("group", { name: `${FAKE} subscriptions` });

async function addSubscription(page, alias) {
  await pane(page).getByRole("button", { name: `Add a ${FAKE} subscription` }).click();
  await pane(page).getByPlaceholder("Personal").fill(alias);
  await pane(page).getByRole("button", { name: "Add", exact: true }).click();
  await expect(pane(page).getByText(alias, { exact: true })).toBeVisible();
}

test("the machine's own login is the only subscription until one is added", async ({ page }) => {
  await openSettings(page, "Subscriptions");

  const system = pane(page).getByText("System", { exact: true });
  await expect(system).toBeVisible();
  // It is what new tasks use, and it is not something to be removed or renamed.
  await expect(pane(page).getByText("default", { exact: true })).toBeVisible();
  await expect(pane(page).getByRole("button", { name: "Remove System" })).toHaveCount(0);
  await expect(pane(page).getByRole("button", { name: "Rename System" })).toHaveCount(0);
});

test("a second subscription is added, named and made the default", async ({ page }) => {
  await openSettings(page, "Subscriptions");
  await addSubscription(page, "Personal");

  // Adding one does not change what new tasks run on: that is a separate,
  // deliberate choice.
  await expect(
    pane(page).getByRole("button", { name: `Start new ${FAKE} tasks on Personal` }),
  ).toBeVisible();
  await pane(page).getByRole("button", { name: `Start new ${FAKE} tasks on Personal` }).click();
  await expect(
    pane(page).getByRole("button", { name: `Start new ${FAKE} tasks on System` }),
  ).toBeVisible();

  // Renaming keeps it the default; the name is only how it is recognised.
  await pane(page).getByRole("button", { name: "Rename Personal" }).click();
  await pane(page).getByRole("textbox").first().fill("Mine");
  await pane(page).getByRole("button", { name: "Save" }).click();
  await expect(pane(page).getByText("Mine", { exact: true })).toBeVisible();
  await expect(pane(page).getByText("Personal", { exact: true })).toHaveCount(0);
});

test("the model picker names the subscription, and swapping runs the turn on it", async ({
  page,
}) => {
  await openSettings(page, "Subscriptions");
  await addSubscription(page, "Personal");
  await page.getByRole("button", { name: "Done" }).click();

  await addProject(page);
  await openProject(page);
  await newTask(page);

  // With two to choose from, every model entry says which one it would run
  // on, and so does the button once picked.
  await pickModel(page, "System · Fake Quick");
  await expect(modelButton(page)).toContainText("System · Fake Quick");

  await pickModel(page, "Personal · Fake Quick");
  await expect(modelButton(page)).toContainText("Personal · Fake Quick");

  // @account makes the fake provider report the login directory the runner
  // resolved, which is the only way to see from here that the turn really ran
  // on the subscription that was chosen.
  await sendPrompt(page, "@account");
  await expect(page.locator("main")).toContainText("account=", { timeout: 15_000 });
  await expect(page.locator("main")).toContainText("accounts/fake/personal");
});

test("a task on the machine's own login is unchanged by a second subscription existing", async ({
  page,
}) => {
  await openSettings(page, "Subscriptions");
  await addSubscription(page, "Personal");
  await page.getByRole("button", { name: "Done" }).click();

  await addProject(page);
  await openProject(page);
  await newTask(page);
  await pickModel(page, "System · Fake Quick");

  await sendPrompt(page, "@account");
  await expect(page.locator("main")).toContainText("account=system", { timeout: 15_000 });
});

test("removing a subscription says what still runs on it", async ({ page }) => {
  await openSettings(page, "Subscriptions");
  await addSubscription(page, "Personal");
  await page.getByRole("button", { name: "Done" }).click();

  await addProject(page);
  await openProject(page);
  await newTask(page);
  await pickModel(page, "Personal · Fake Quick");
  await sendPrompt(page, "hello");
  await expect(page.locator("main")).toContainText("hello", { timeout: 15_000 });

  await openSettings(page, "Subscriptions");
  await pane(page).getByRole("button", { name: "Remove Personal" }).click();
  // The count is the consequence: those tasks stop until another
  // subscription is chosen for them, so it is said before the removal.
  await expect(pane(page).getByText(/1 task\(s\) and 0 job\(s\) still run on it/)).toBeVisible();
  await pane(page).getByRole("button", { name: "Remove", exact: true }).click();
  await expect(pane(page).getByText("Personal", { exact: true })).toHaveCount(0);
});

test("new tasks preserve the last-used subscription, model and effort", async ({ page }) => {
  await openSettings(page, "Subscriptions");
  await addSubscription(page, "Personal");
  await page.getByRole("button", { name: "Done" }).click();
  await addProject(page);
  await openProject(page);
  await newTask(page);
  await pickModel(page, "Personal · Fake Quick");
  await page.getByTitle("Effort", { exact: true }).click();
  await page.getByRole("option", { name: "High", exact: true }).click();
  await page.getByPlaceholder("Ask the agent…").fill("keep this draft");
  const created = page.waitForRequest((r) => r.method() === "POST" && r.url().endsWith("/api/sessions"));
  await page.keyboard.press("Control+n");
  const body = (await created).postDataJSON();
  expect(body.account_id).toBeGreaterThan(0);
  expect(body.model).toBe("fake-quick");
  expect(body.effort).toBe("high");
  await expect(modelButton(page)).toContainText("Personal · Fake Quick");
  await page.reload();
  await expect(modelButton(page)).toContainText("Personal · Fake Quick");
});

test("OpenCode Go connects without a CLI and keeps its direct preference", async ({ page }) => {
  await openSettings(page, "Subscriptions");
  const go = page.getByRole("group", { name: "OpenCode Go subscriptions" });
  await expect(go.getByText(/OpenCode CLI is optional/)).toBeVisible();
  await go.getByRole("button", { name: "Add a OpenCode Go subscription" }).click();
  await go.getByPlaceholder("Personal").fill("Go Work");
  await go.getByRole("button", { name: "Add", exact: true }).click();
  await go.getByRole("button", { name: "Sign in to Go Work for OpenCode Go" }).click();
  await go.getByLabel("Run with").selectOption("direct");
  await go.getByLabel(/OpenCode Go key/).fill("e2e-not-a-real-key");
  await go.getByRole("button", { name: "Save connection" }).click();
  await expect(go.getByText("Go key saved", { exact: true })).toBeVisible();
  await go.getByRole("button", { name: "Sign in to Go Work for OpenCode Go" }).click();
  await expect(go.getByLabel("Run with")).toHaveValue("direct");
  await expect(go.getByLabel(/OpenCode Go key/)).toHaveValue("");
  await expect(page.getByRole("group", { name: "Claude Code subscriptions" }).getByText(/Claude Code CLI is required/)).toBeVisible();
});
