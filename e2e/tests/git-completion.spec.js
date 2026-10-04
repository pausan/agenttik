import { addProject, openProject, inspector, test, expect } from "../fixtures.js";

const log = { branch: "main", branches: ["main", "feature"], head: "12345678", operation: "", commits: [] };
const remotes = [{ remote: "origin", branch: "feature", tip: "a".repeat(40) }];

test.beforeEach(async ({ page }) => {
  await page.route("**/api/projects/*/log?*", (route) => route.fulfill({ json: log }));
  await page.route("**/api/projects/*/changes?*", (route) => route.fulfill({
    json: [{ path: "file.txt", status: "MM", staged: true, unstaged: true }],
  }));
  await page.route("**/api/projects/*/branches/*", (route) => route.fulfill({
    json: { deleted: [], remotes, message: "Operation completed." },
  }));
  await addProject(page);
  await openProject(page);
});

// Hold the operation response, then hold every follow-up workspace read.
// Ending the spinner must depend only on the operation's response.
async function holdAction(page, endpoint, success, failed) {
  let releaseAction, releaseReads;
  const actionGate = new Promise((resolve) => { releaseAction = resolve; });
  const readGate = new Promise((resolve) => { releaseReads = resolve; });
  let answered = false;
  let reads = 0;
  await page.route(/\/api\/projects\/[^/]+\/(?:log|changes|tree)(?:\?|$)/, async (route) => {
    if (answered) {
      reads++;
      await readGate;
      await route.fulfill({ status: 503, json: { error: "Refresh unavailable" } });
    } else await route.fallback();
  });
  await page.route(`**/api/projects/*/${endpoint}?*`, async (route) => {
    await actionGate;
    answered = true;
    await route.fulfill(failed ? { status: 400, json: { error: "Git operation failed" } } : success);
  });
  return {
    finish: releaseAction,
    async expectRefresh() { await expect.poll(() => reads).toBeGreaterThan(0); },
    async close() {
      releaseAction();
      releaseReads();
      await page.unrouteAll({ behavior: "wait" });
    },
  };
}

const branchButtons = {
  switch: "Current branch",
  pull: "Pull current branch",
  push: "Push current branch",
  clean: "Clean branches merged into main or master",
  "check-remote": "Clean branches merged into main or master",
  "clean-remote": "Yes, delete all listed branches",
  continue: "Retry solving conflicts",
  abort: "Abort",
};

for (const action of Object.keys(branchButtons)) for (const failed of [false, true]) {
  test(`${action} ends busy state after ${failed ? "failure" : "success"} with workspace reads held`, async ({ page }) => {
    if (["continue", "abort"].includes(action)) {
      await page.route("**/api/projects/*/log?*", (route) => route.fulfill({ json: { ...log, operation: "merge" } }));
      await page.reload();
      await openProject(page);
    }
    const pane = inspector(page);
    await pane.getByRole("tab", { name: "Commits" }).click();
    if (action === "clean-remote") {
      await pane.getByRole("button", { name: branchButtons.clean, exact: true }).click();
      await expect(page.getByRole("dialog", { name: "Delete merged remote branches?" })).toBeVisible();
    }
    const held = await holdAction(page, `branches/${action}`, {
      json: { deleted: [], remotes: [], message: "Operation completed." },
    }, failed);
    try {
      const button = page.getByRole("button", { name: branchButtons[action], exact: true });
      await button.click();
      if (action === "switch") await page.getByRole("option", { name: "feature", exact: true }).click();
      await expect(button).toBeDisabled();
      held.finish();
      await held.expectRefresh();
      await expect(pane.locator(".animate-spin")).toHaveCount(0);
      if (action === "clean" && !failed) {
        await page.getByRole("dialog", { name: "Delete merged remote branches?" })
          .getByRole("button", { name: "No, keep remote branches", exact: true }).click();
      }
      await expect(pane.getByRole("button", { name: ["continue", "abort"].includes(action) ? "Abort" : "Current branch", exact: true })).toBeEnabled();
      if (failed) await expect(pane.getByRole("alert")).toHaveText("Git operation failed");
      else await expect(pane.getByRole("alert")).toBeHidden();
      if (action === "clean-remote") await expect(page.getByRole("dialog", { name: "Delete merged remote branches?" })).toBeHidden();
    } finally {
      await held.close();
    }
  });
}

for (const failed of [false, true]) {
  test(`commit message generation ends busy state after ${failed ? "failure" : "success"}`, async ({ page }) => {
    const pane = inspector(page);
    await pane.getByRole("tab", { name: "Changed", exact: true }).click();
    const message = pane.getByRole("textbox", { name: "Commit message" });
    await message.fill("Old draft");
    const button = pane.getByRole("button", { name: "Generate commit message", exact: true });
    const held = await holdAction(page, "commit-message", { json: { message: "Generated commit" } }, failed);
    try {
      await button.click();
      await expect(button.locator(".animate-spin")).toBeVisible();
      await expect(message).toBeDisabled();
      held.finish();
      await expect(button.locator(".animate-spin")).toHaveCount(0);
      await expect(message).toBeEnabled();
      await expect(message).toHaveValue(failed ? "Old draft" : "Generated commit");
      if (failed) await expect(pane.getByRole("alert")).toHaveText("Git operation failed");
    } finally {
      await held.close();
    }
  });
}

for (const [action, label] of [["stage", "Stage file.txt"], ["stage", "Stage all files"], ["unstage", "Unstage file.txt"], ["unstage", "Unstage all files"], ["commit", "Commit"]]) for (const failed of [false, true]) {
  test(`${label} ends busy state after ${failed ? "failure" : "success"} with workspace reads held`, async ({ page }) => {
    const pane = inspector(page);
    await pane.getByRole("tab", { name: "Changed", exact: true }).click();
    const message = pane.getByRole("textbox", { name: "Commit message" });
    await message.fill("My commit message");
    const held = await holdAction(page, action, { status: 204 }, failed);
    try {
      await pane.getByRole("button", { name: label, exact: true }).click();
      await expect(pane.getByRole("button", { name: "Stage all files", exact: true })).toBeDisabled();
      await expect(pane.locator(".animate-spin")).toBeVisible();
      held.finish();
      await held.expectRefresh();
      await expect(pane.locator(".animate-spin")).toHaveCount(0);
      await expect(pane.getByRole("button", { name: "Stage all files", exact: true })).toBeEnabled();
      await expect(message).toHaveValue(action === "commit" && !failed ? "" : "My commit message");
      if (failed) await expect(pane.getByRole("alert")).toHaveText("Git operation failed");
    } finally {
      await held.close();
    }
  });
}

for (const failed of [false, true]) {
  test(`revert ends busy state after ${failed ? "failure" : "success"} with workspace reads held`, async ({ page }) => {
    if (failed) {
      // fail() logs expected revert errors as well as displaying their toast.
      await page.evaluate(() => {
        const logError = console.error.bind(console);
        console.error = (...args) => {
          if (args[0]?.message === "Git operation failed") return;
          logError(...args);
        };
      });
    }
    const pane = inspector(page);
    await pane.getByRole("tab", { name: "Changed", exact: true }).click();
    await pane.getByRole("button", { name: "Revert changes to file.txt", exact: true }).first().click();
    const dialog = page.getByRole("dialog", { name: "Revert changes", exact: true });
    const button = dialog.getByRole("button", { name: "Revert", exact: true });
    const held = await holdAction(page, "revert", { json: { paths: ["file.txt"] } }, failed);
    try {
      await button.click();
      await expect(button.locator(".animate-spin")).toBeVisible();
      held.finish();
      await held.expectRefresh();
      if (failed) {
        await expect(button.locator(".animate-spin")).toHaveCount(0);
        await expect(button).toBeEnabled();
        await expect(page.getByText("Git operation failed", { exact: true })).toBeVisible();
      } else await expect(dialog).toBeHidden();
    } finally {
      await held.close();
    }
  });
}
