import { expect, openSettings, sidebar, test, REPO } from "../fixtures.js";

test("profiles can be renamed from settings", async ({ page, agenttik }) => {
  await page.request.post(`${agenttik.url}/api/profiles`, { data: { name: "Work" } });
  await page.reload();
  await openSettings(page, "Profiles");
  await page.getByRole("button", { name: "Rename Work", exact: true }).click();
  await page.getByRole("textbox", { name: "Rename Work", exact: true }).fill("Office");
  await page.getByRole("button", { name: "Save", exact: true }).click();
  await expect(page.getByRole("button", { name: "Rename Office", exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Done", exact: true }).click();
  await page.getByRole("button", { name: "Profile: Default", exact: true }).click();
  await expect(page.getByRole("menuitem", { name: "Office", exact: true })).toBeVisible();
});

test("profiles appear only when needed and isolate the same project", async ({ page, agenttik }) => {
  await expect(page.getByRole("button", { name: /^Profile:/ })).toHaveCount(0);
  await page.request.post(`${agenttik.url}/api/projects`, { data: { path: REPO, name: "Personal project" } });
  await page.reload();
  await expect(sidebar(page).getByText("Personal project", { exact: true })).toBeVisible();
  await openSettings(page, "Profiles");
  await page.getByRole("textbox", { name: "Profile name" }).fill("Work");
  await page.getByRole("button", { name: "Add profile", exact: true }).click();
  await expect(page.getByRole("button", { name: "Switch to Work", exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Switch to Work", exact: true }).click();
  await expect(page.getByRole("button", { name: "Profile: Work", exact: true })).toBeVisible();
  await expect(sidebar(page).getByText("Personal project", { exact: true })).toHaveCount(0);
  const id = new URL(page.url()).searchParams.get("profile");
  const project = await (await page.request.post(`${agenttik.url}/api/projects?profile=${id}`, { data: { path: REPO, name: "Work project" } })).json();
  const personal = await (await page.request.get(`${agenttik.url}/api/projects`)).json();
  expect(personal).toHaveLength(1);
  expect(personal[0].name).toBe("Personal project");
  expect(project.name).toBe("Work project");
  await page.reload();
  await expect(sidebar(page).getByText("Work project", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Profile: Work", exact: true }).click();
  await page.getByRole("menuitem", { name: "Default", exact: true }).click();
  await expect(sidebar(page).getByText("Personal project", { exact: true })).toBeVisible();
  await expect(sidebar(page).getByText("Work project", { exact: true })).toHaveCount(0);
  await openSettings(page, "Profiles");
  await page.getByRole("button", { name: "Remove Work", exact: true }).click();
  await page.getByRole("button", { name: "Remove profile", exact: true }).click();
  await expect(page.getByRole("button", { name: "Remove Work", exact: true })).toHaveCount(0);
  await page.keyboard.press("Escape");
  await expect(page.getByRole("button", { name: /^Profile:/ })).toHaveCount(0);
});

async function paletteSwitch(page, name) {
  await page.keyboard.press("Control+Shift+p");
  await page.getByPlaceholder("Command Palette…").fill(`Switch to profile: ${name}`);
  await page.getByText(`Switch to profile: ${name}`, { exact: true }).click();
  await expect(page.getByRole("button", { name: `Profile: ${name}`, exact: true })).toBeVisible();
}

test("command palette refreshes profiles and appearance stays with each profile", async ({ page, agenttik }) => {
  await page.emulateMedia({ colorScheme: "light" });
  await openSettings(page, "Appearance");
  const mode = page.getByRole("button", { name: "Color mode", exact: true });
  await mode.click();
  await page.getByRole("option", { name: "Dark", exact: true }).click();
  await page.keyboard.press("Escape");
  // Added externally after startup: opening the palette must refresh its list.
  await page.request.post(`${agenttik.url}/api/profiles`, { data: { name: "Work" } });
  await paletteSwitch(page, "Work");
  await expect(page.locator("html")).not.toHaveClass(/dark/);
  await openSettings(page, "Appearance");
  await expect(mode).toContainText("System");
  await mode.click();
  await page.getByRole("option", { name: "Light", exact: true }).click();
  await page.keyboard.press("Escape");
  await paletteSwitch(page, "Default");
  await expect(page.locator("html")).toHaveClass(/dark/);
  await openSettings(page, "Appearance");
  await expect(mode).toContainText("Dark");
  await page.keyboard.press("Escape");
  await paletteSwitch(page, "Work");
  await page.reload();
  await openSettings(page, "Appearance");
  await expect(mode).toContainText("Light");
});

test("tree expansion stays with its profile even when project IDs match", async ({ page, agenttik }) => {
  const work = await (await page.request.post(`${agenttik.url}/api/profiles`, { data: { name: "Work" } })).json();
  for (const id of ["default", work.id]) {
    await page.request.post(`${agenttik.url}/api/projects?profile=${id}`, { data: { path: REPO } });
  }
  await page.reload();
  const tree = sidebar(page);
  const showTree = () => tree.getByRole("tab", { name: "Tree", exact: true }).click();
  const folder = tree.getByRole("button", { name: "app", exact: true });
  await showTree();
  await folder.click();
  await expect(folder).toHaveAttribute("aria-expanded", "true");
  await paletteSwitch(page, "Work");
  await showTree();
  await expect(folder).toHaveAttribute("aria-expanded", "false");
  await paletteSwitch(page, "Default");
  await showTree();
  await expect(folder).toHaveAttribute("aria-expanded", "true");
});

test("project options move tasks to another profile", async ({ page, agenttik }) => {
  const work = await (await page.request.post(`${agenttik.url}/api/profiles`, { data: { name: "Work" } })).json();
  const project = await (await page.request.post(`${agenttik.url}/api/projects`, { data: { path: REPO, name: "Moving project" } })).json();
  const task = await (await page.request.post(`${agenttik.url}/api/sessions`, {
    data: { project_id: project.id, provider: "fake", model: "fake-quick", title: "Keep this task" },
  })).json();
  await page.reload();
  await sidebar(page).getByText("Moving project", { exact: true }).click();
  const options = page.locator("aside").filter({ has: page.getByRole("heading", { name: "Workspace", exact: true }) });
  await options.getByRole("tab", { name: "Project Options", exact: true }).click();
  const move = options.getByRole("button", { name: "Move project", exact: true });
  await expect(move).toBeDisabled();
  await options.getByRole("combobox", { name: "Move to profile", exact: true }).click();
  await page.getByRole("option", { name: "Work", exact: true }).click();
  await move.click();
  await expect(sidebar(page).getByText("Moving project", { exact: true })).toHaveCount(0);
  expect((await page.request.get(`${agenttik.url}/api/sessions/${task.id}`)).status()).toBe(404);
  await page.getByRole("button", { name: "Profile: Default", exact: true }).click();
  await page.getByRole("menuitem", { name: "Work", exact: true }).click();
  await expect(sidebar(page).getByText("Moving project", { exact: true })).toBeVisible();
  await expect(sidebar(page).getByText("Keep this task", { exact: true })).toBeVisible();
  const moved = await (await page.request.get(`${agenttik.url}/api/sessions/${task.id}?profile=${work.id}`)).json();
  expect(moved.session.title).toBe("Keep this task");
});

test("a conflicting project move keeps the source visible", async ({ page, agenttik }) => {
  const work = await (await page.request.post(`${agenttik.url}/api/profiles`, { data: { name: "Work" } })).json();
  for (const profile of ["default", work.id]) {
    await page.request.post(`${agenttik.url}/api/projects?profile=${profile}`, { data: { path: REPO, name: "Same folder" } });
  }
  await page.reload();
  await sidebar(page).getByText("Same folder", { exact: true }).click();
  await page.getByRole("tab", { name: "Project Options", exact: true }).click();
  await page.getByRole("combobox", { name: "Move to profile", exact: true }).click();
  await page.getByRole("option", { name: "Work", exact: true }).click();
  await page.getByRole("button", { name: "Move project", exact: true }).click();
  await expect(page.getByText("another project already uses that folder", { exact: true })).toBeVisible();
  await expect(sidebar(page).getByText("Same folder", { exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "Move project", exact: true })).toBeEnabled();
});
