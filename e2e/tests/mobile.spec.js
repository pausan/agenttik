import { join } from "node:path";
import { expect, modelButton, REPO, test } from "../fixtures.js";

// Use the real server and fake provider. Project creation and every action
// under test go through the same API and controls as a remote phone.
async function seed(page, agenttik) {
  const projects = [];
  for (const [name, path] of [["Website", REPO], ["API service", join(REPO, "web")]]) {
    const response = await page.request.post(`${agenttik.url}/api/projects`, { data: { name, path } });
    expect(response.ok()).toBeTruthy();
    projects.push(await response.json());
  }
  await page.reload();
  return projects;
}

const projectsButton = (page) => page.getByRole("button", { name: "Projects and tasks", exact: true });
const workspaceButton = (page) => page.getByRole("button", { name: "Workspace", exact: true });
const projectDrawer = (page) => page.getByRole("dialog", { name: "Projects and tasks", exact: true });
const workspaceDrawer = (page) => page.getByRole("dialog", { name: "Workspace", exact: true });
const prompt = (page) => page.getByPlaceholder("Ask the agent…");

async function selectProject(page, name) {
  await projectsButton(page).tap();
  await projectDrawer(page).getByRole("button", { name: new RegExp(name) }).tap();
  await expect(projectDrawer(page)).toBeHidden();
  await expect(page.locator("main").getByRole("heading", { name, exact: true })).toBeVisible();
}

async function expectInViewport(page, locator) {
  await expect(locator).toBeVisible();
  const box = await locator.boundingBox();
  const { width, height } = page.viewportSize();
  expect(box.x).toBeGreaterThanOrEqual(0);
  expect(box.y).toBeGreaterThanOrEqual(0);
  expect(box.x + box.width).toBeLessThanOrEqual(width + 1);
  expect(box.y + box.height).toBeLessThanOrEqual(height + 1);
}

async function expectPageFits(page) {
  const size = await page.evaluate(() => ({
    width: document.documentElement.scrollWidth,
    height: document.documentElement.scrollHeight,
    viewportWidth: innerWidth,
    viewportHeight: innerHeight,
  }));
  expect(size.width).toBe(size.viewportWidth);
  expect(size.height).toBe(size.viewportHeight);
}

test.describe("phone", () => {
  test.use({ viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true });

  test("switch projects, create a task, send and stop with touch controls", async ({ page, agenttik }) => {
    const [, apiProject] = await seed(page, agenttik);
    await selectProject(page, "API service");
    await expect(page.getByPlaceholder("Filter tasks")).not.toBeFocused();
    await expect(page.getByRole("separator")).toHaveCount(0);

    await page.getByRole("button", { name: "New task", exact: true }).tap();
    await expect(prompt(page)).toBeVisible();
    await expect(prompt(page)).not.toBeFocused();
    await modelButton(page).tap();
    await page.getByRole("option", { name: "Fake Quick" }).tap();
    await prompt(page).fill("Hello from my phone");
    await page.getByRole("button", { name: "More prompt actions" }).tap();
    await page.getByRole("button", { name: "Send", exact: true }).tap();
    await expect(page.locator(".markdown")).toContainText("Hello from my phone");

    const sessions = await (await page.request.get(`${agenttik.url}/api/sessions`)).json();
    expect(sessions).toHaveLength(1);
    expect(sessions[0].project_id).toBe(apiProject.id);

    await prompt(page).fill("@wait 30000\nHold this task");
    await page.locator('.prompt-bar button[type="submit"]').tap();
    await expect(page.getByRole("button", { name: "Stop", exact: true })).toBeVisible();
    await expectInViewport(page, page.getByRole("button", { name: "Stop", exact: true }));
    await page.getByRole("button", { name: "Stop", exact: true }).tap();
    await expect(page.getByRole("button", { name: "Stop", exact: true })).toBeHidden();
    await expectPageFits(page);
  });

  test("task selections dismiss the drawer and preserve each project's draft", async ({ page, agenttik }) => {
    await seed(page, agenttik);
    await selectProject(page, "Website");
    await page.getByRole("button", { name: "New task", exact: true }).tap();
    await prompt(page).fill("Website draft");
    await selectProject(page, "API service");
    await page.getByRole("button", { name: "New task", exact: true }).tap();
    await expect(prompt(page)).toHaveValue("");
    await prompt(page).fill("API draft");

    await selectProject(page, "Website");
    await projectsButton(page).tap();
    await projectDrawer(page).getByRole("button", { name: "Untitled task", exact: true }).tap();
    await expect(projectDrawer(page)).toBeHidden();
    await expect(prompt(page)).toHaveValue("Website draft");
    // Reselecting the current task must close the drawer even though neither
    // the active project nor the active tab changes.
    await projectsButton(page).tap();
    await projectDrawer(page).getByRole("button", { name: "Untitled task", exact: true }).tap();
    await expect(projectDrawer(page)).toBeHidden();
    await expect(prompt(page)).toHaveValue("Website draft");
  });

  test("workspace and project drawers dismiss, restore focus and open files", async ({ page, agenttik }) => {
    await seed(page, agenttik);
    await selectProject(page, "Website");
    await workspaceButton(page).tap();
    await expect(workspaceDrawer(page).getByRole("tab", { name: "Project Options" })).toBeVisible();
    await workspaceDrawer(page).getByRole("tab", { name: "Commits" }).tap();
    await page.keyboard.press("Escape");
    await expect(workspaceDrawer(page)).toBeHidden();
    await expect(workspaceButton(page)).toBeFocused();
    await workspaceButton(page).tap();
    await expect(workspaceDrawer(page).getByRole("tab", { name: "Commits" })).toHaveAttribute("data-state", "active");
    await workspaceDrawer(page).getByRole("button", { name: "Close", exact: true }).tap();

    await projectsButton(page).tap();
    await projectDrawer(page).getByRole("tab", { name: "Tree" }).tap();
    await projectDrawer(page).getByRole("button", { name: "go.mod", exact: true }).tap();
    await expect(projectDrawer(page)).toBeHidden();
    await expect(page.getByRole("tab", { name: /go\.mod/ })).toBeVisible();
    await expect(page.getByText("module github.com/pausan/agenttik")).toBeVisible();
    // The transparent editor and its highlighted copy must still wrap at the
    // same places after phone fields are enlarged to avoid focus zoom.
    const fonts = await page.locator(".editor").evaluate((editor) =>
      [editor.querySelector("pre"), editor.querySelector("textarea")].map((el) => getComputedStyle(el).font),
    );
    expect(fonts[0]).toBe(fonts[1]);
    await projectsButton(page).tap();
    await page.touchscreen.tap(385, 400);
    await expect(projectDrawer(page)).toBeHidden();
    await expect(projectsButton(page)).toBeFocused();
    await expectPageFits(page);
  });

  for (const width of [320, 390, 767]) {
    test(`controls and popovers fit a ${width}px phone, including a short viewport`, async ({ page, agenttik }) => {
      await page.setViewportSize({ width, height: 740 });
      await seed(page, agenttik);
      await selectProject(page, "Website");
      await page.getByRole("button", { name: "New task", exact: true }).tap();
      await expect(prompt(page)).toBeVisible();
      // Subscription aliases can make a model name much longer than the
      // phone. Keep the real picker, but give its model a long display label.
      await page.route("**/api/providers", async (route) => {
        const response = await route.fetch();
        const providers = await response.json();
        const fake = providers.find((provider) => provider.name === "fake");
        fake.models.find((model) => model.id === "fake-quick").label = "Fake Quick with a very long model and subscription label";
        await route.fulfill({ response, json: providers });
      });
      await page.reload();
      await expect(modelButton(page)).toContainText("very long");
      const controls = [
        modelButton(page),
        page.getByTitle("Effort", { exact: true }),
        page.locator('.prompt-bar button[type="submit"]'),
        page.getByRole("button", { name: "More prompt actions" }),
      ];
      for (const control of controls) await expectInViewport(page, control);
      await expectPageFits(page);
      await modelButton(page).tap();
      await expectInViewport(page, page.getByPlaceholder("Search models…"));
      await page.getByRole("option", { name: "Fake Quick" }).tap();
      await page.getByRole("button", { name: /Main context.*and subscription usage/ }).tap();
      await expectInViewport(page, page.locator(".usage-details"));
      await expect(page.getByText("Main context", { exact: true })).toBeVisible();
      await page.keyboard.press("Escape");
      await page.setViewportSize({ width, height: 420 });
      await prompt(page).tap();
      for (const control of controls) await expectInViewport(page, control);
      await expectInViewport(page, prompt(page));
      await expectPageFits(page);
    });
  }

  test("add a project and reach settings from the phone drawer", async ({ page }) => {
    await projectsButton(page).tap();
    await projectDrawer(page).getByRole("button", { name: "Add project", exact: true }).tap();
    const add = page.getByRole("dialog", { name: "Add project", exact: true });
    await expect(add).toBeVisible();
    await expect(projectDrawer(page)).toBeHidden();
    const field = add.getByPlaceholder("~/code/myproject");
    await expect(field).toHaveValue(/.+/);
    await field.fill(REPO);
    await add.getByRole("button", { name: "Add project", exact: true }).tap();
    await expect(add).toBeHidden();
    await selectProject(page, "agenttik");
    await projectsButton(page).tap();
    await projectDrawer(page).getByRole("button", { name: "Settings", exact: true }).tap();
    const settings = page.getByRole("dialog", { name: "Settings", exact: true });
    await expect(settings).toBeVisible();
    await expectInViewport(page, settings);
    await settings.getByRole("button", { name: "Models", exact: true }).tap();
    await expect(settings.getByRole("group", { name: "System · Fake models", exact: true })).toBeVisible();
    await settings.getByRole("button", { name: "Done", exact: true }).tap();
    await expect(settings).toBeHidden();
    await expectPageFits(page);
  });
});

test("tablet and desktop keep the three panels and saved widths after phone navigation", async ({ page, agenttik }) => {
  await seed(page, agenttik);
  await page.locator("aside").first().getByRole("button", { name: /Website/ }).click();
  await page.getByRole("separator", { name: "Resize the left panel" }).press("ArrowRight");
  for (const width of [1440, 1024, 768]) {
    await page.setViewportSize({ width, height: 1024 });
    await expect(projectsButton(page)).toHaveCount(0);
    await expect(page.getByRole("separator")).toHaveCount(2);
    await expect(page.getByRole("separator", { name: "Resize the left panel" })).toHaveAttribute("aria-valuenow", "288");
    await expect(page.locator("aside").last().getByRole("tab", { name: "Project Options" })).toBeVisible();
  }
  await page.setViewportSize({ width: 390, height: 844 });
  await projectsButton(page).click();
  await expect(projectDrawer(page)).toBeVisible();
  await page.setViewportSize({ width: 1440, height: 900 });
  await expect(projectDrawer(page)).toBeHidden();
  await expect(page.getByRole("separator", { name: "Resize the left panel" })).toHaveAttribute("aria-valuenow", "288");
  await page.getByRole("button", { name: "New task", exact: true }).first().click();
  await expect(prompt(page)).toBeFocused();
});
