import { expect, test } from "../fixtures.js";

async function openRepos(page) {
  await page.getByRole("button", { name: "Add project", exact: true }).click();
  const dialog = page.getByRole("dialog");
  await dialog.getByRole("tab", { name: "Multiple Git Repos" }).click();
  await expect(dialog.getByText("Add one or more git repositories", { exact: true })).toBeVisible();
  return dialog;
}

test("typing adds a blank row before validation and broken rows block Add", async ({ page }) => {
  let finish;
  const pending = new Promise((resolve) => { finish = resolve; });
  await page.route("**/api/fs/remote", async (route) => {
    await pending;
    await route.fulfill({ status: 400, json: { error: "Repository unavailable" } });
  });
  const dialog = await openRepos(page);
  const inputs = dialog.getByRole("textbox", { name: /^Repository \d+$/ });
  await inputs.first().fill("https://github.com/org/repo");
  await expect(inputs).toHaveCount(2);
  await expect(inputs.last()).toHaveValue("");
  await expect(dialog.getByRole("button", { name: "Add project", exact: true })).toBeDisabled();
  finish();
  await expect(dialog.getByText("Repository unavailable")).toBeVisible();
  await expect(inputs).toHaveCount(2);
  await expect(dialog.getByRole("button", { name: "Add project", exact: true })).toBeDisabled();
  await inputs.first().fill("");
  await expect(dialog.getByText("Repository unavailable")).toBeHidden();
  await dialog.getByRole("button", { name: "Delete repository", exact: true }).first().click();
  await expect(inputs).toHaveCount(1);
});

test("pasted URLs create separate rows and delete buttons fit at narrow widths", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  const longURL = "https://github.com/org/" + "repository".repeat(30);
  await page.route("**/api/fs/remote", (route) => route.fulfill({ json: { url: route.request().postDataJSON().url } }));
  const dialog = await openRepos(page);
  const inputs = dialog.getByRole("textbox", { name: /^Repository \d+$/ });
  await inputs.first().evaluate((el, text) => {
    const clipboardData = new DataTransfer();
    clipboardData.setData("text/plain", text);
    el.dispatchEvent(new ClipboardEvent("paste", { clipboardData, bubbles: true, cancelable: true }));
  }, longURL + " \n\tgit@gitlab.com:group/repo.git");
  await expect(inputs).toHaveCount(3);
  await expect(inputs.nth(0)).toHaveValue(longURL);
  await expect(inputs.nth(1)).toHaveValue("git@gitlab.com:group/repo.git");
  await expect(inputs.nth(2)).toHaveValue("");
  await expect(dialog.getByRole("button", { name: "Add project", exact: true })).toBeEnabled();
  const bounds = await dialog.boundingBox();
  const input = await inputs.first().boundingBox();
  const button = dialog.getByRole("button", { name: "Delete " + longURL, exact: true });
  const deletion = await button.boundingBox();
  expect(deletion.x).toBeGreaterThanOrEqual(input.x + input.width);
  expect(deletion.x + deletion.width).toBeLessThanOrEqual(bounds.x + bounds.width);
  expect(Math.abs(deletion.y + deletion.height / 2 - input.y - input.height / 2)).toBeLessThan(2);
  await button.click();
  await expect(inputs).toHaveCount(2);
  await expect(inputs.first()).toHaveValue("git@gitlab.com:group/repo.git");
});

for (const failed of [false, true]) {
  test(`repository check and clone stop spinning after ${failed ? "failure" : "success"}`, async ({ page }) => {
    let releaseCheck, releaseClone, releaseProject;
    const checkGate = new Promise((resolve) => { releaseCheck = resolve; });
    const cloneGate = new Promise((resolve) => { releaseClone = resolve; });
    const projectGate = new Promise((resolve) => { releaseProject = resolve; });
    await page.route("**/api/fs/remote", async (route) => {
      await checkGate;
      await route.fulfill({ json: { url: "https://example.com/repo.git" } });
    });
    await page.route("**/api/fs/clone", async (route) => {
      await cloneGate;
      await route.fulfill(failed ? { status: 400, json: { error: "Clone failed" } } : { json: { name: "repo" } });
    });
    await page.route("**/api/projects", async (route) => {
      if (route.request().method() !== "POST") return route.fallback();
      await projectGate;
      await route.fulfill({ status: 400, json: { error: "Project unavailable" } });
    });
    try {
      const dialog = await openRepos(page);
      await dialog.getByRole("textbox", { name: "Repository 1", exact: true }).fill("https://example.com/repo.git");
      await expect(dialog.locator(".animate-spin")).toBeVisible();
      releaseCheck();
      await expect(dialog.locator(".animate-spin")).toHaveCount(0);
      const add = dialog.getByRole("button", { name: "Add project", exact: true });
      await expect(add).toBeEnabled();
      await add.click();
      await expect(dialog.locator(".animate-spin")).toBeVisible();
      releaseClone();
      // A successful clone has finished even while adding the project waits.
      await expect(dialog.locator(".animate-spin")).toHaveCount(0);
      await expect(dialog.getByText("Cloning 1 of 1 repositories", { exact: true })).toBeHidden();
      if (failed) await expect(dialog.getByText("Clone failed", { exact: true })).toBeVisible();
    } finally {
      releaseCheck();
      releaseClone();
      releaseProject();
      await page.unrouteAll({ behavior: "wait" });
    }
  });
}
