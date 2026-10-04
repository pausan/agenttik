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

for (const selected of [true, false]) {
  test(`completion while away from a profile stays unread (${selected ? "selected" : "unselected"} task)`, async ({ page, agenttik }) => {
    await page.request.post(`${agenttik.url}/api/profiles`, { data: { name: "Work" } });
    const project = await (await page.request.post(`${agenttik.url}/api/projects`, {
      data: { path: REPO, name: "Personal project" },
    })).json();
    const task = await (await page.request.post(`${agenttik.url}/api/sessions`, {
      data: { project_id: project.id, provider: "fake", model: "fake-quick", title: "Finishes while away" },
    })).json();
    await page.reload();
    const row = sidebar(page).locator(".task-row").filter({ hasText: task.title });
    await row.click();
    await expect(page.getByPlaceholder("Ask the agent…")).toBeVisible();
    await page.request.post(`${agenttik.url}/api/sessions/${task.id}/messages`, {
      data: { prompt: "@wait 5000\nFinished while away" },
    });
    await expect(page.getByText("running", { exact: true })).toBeVisible();
    if (!selected) await sidebar(page).getByText("Personal project", { exact: true }).click();
    await paletteSwitch(page, "Work");
    await expect.poll(async () => {
      const detail = await (await page.request.get(`${agenttik.url}/api/sessions/${task.id}`)).json();
      return detail.running;
    }).toBe(false);
    await paletteSwitch(page, "Default");
    await expect(row.locator(".task-title")).toHaveClass(/font-bold/);
    // Leave the restored transcript before its three-second acknowledgement.
    await sidebar(page).getByText("Personal project", { exact: true }).click();
    await page.waitForTimeout(3200);
    await expect(row.locator(".task-title")).toHaveClass(/font-bold/);
    await page.reload();
    await expect(row.locator(".task-title")).toHaveClass(/font-bold/);
    await row.click();
    await expect(row.locator(".task-title")).not.toHaveClass(/font-bold/);
    await page.reload();
    await expect(row.locator(".task-title")).not.toHaveClass(/font-bold/);
  });
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

test("settings reorder profiles and persist the picker order", async ({ page, agenttik }) => {
  await page.request.post(`${agenttik.url}/api/profiles`, { data: { name: "Work" } });
  await openSettings(page, "Profiles");
  const up = page.getByRole("button", { name: /^Move .* up$/ });
  await expect(page.getByRole("button", { name: "Move Default up", exact: true })).toBeDisabled();
  await expect(page.getByRole("button", { name: "Move Work down", exact: true })).toBeDisabled();
  await page.getByRole("button", { name: "Move Work up", exact: true }).click();
  await expect(up.first()).toHaveAttribute("aria-label", "Move Work up");
  await expect(up.first()).toBeDisabled();
  await page.reload();
  await openSettings(page, "Profiles");
  await expect(up.first()).toHaveAttribute("aria-label", "Move Work up");
  await page.getByRole("button", { name: "Move Work down", exact: true }).click();
  await expect(up.first()).toHaveAttribute("aria-label", "Move Default up");
  await page.getByRole("button", { name: "Move Default down", exact: true }).click();
  await expect(up.first()).toHaveAttribute("aria-label", "Move Work up");
  await page.getByRole("button", { name: "Done", exact: true }).click();
  await page.getByRole("button", { name: "Profile: Default", exact: true }).click();
  await expect(page.getByRole("menuitem")).toHaveText(["Work", "Default"]);
});

test("profile shortcuts cycle in saved order in both directions", async ({ page, agenttik }) => {
  const work = await (await page.request.post(`${agenttik.url}/api/profiles`, { data: { name: "Work" } })).json();
  const personal = await (await page.request.post(`${agenttik.url}/api/profiles`, { data: { name: "Personal" } })).json();
  await page.request.put(`${agenttik.url}/api/profiles/order`, { data: { ids: ["default", personal.id, work.id] } });
  await page.reload();
  await expect(page.getByRole("button", { name: "Profile: Default", exact: true })).toBeVisible();
  for (const name of ["Personal", "Work", "Default"]) {
    await page.keyboard.press("Control+Alt+p");
    await expect(page.getByRole("button", { name: `Profile: ${name}`, exact: true })).toBeVisible();
  }
  await page.keyboard.press("Control+Alt+Shift+p");
  await expect(page.getByRole("button", { name: "Profile: Work", exact: true })).toBeVisible();
});

test("working profiles pulse in the footer and menu, then clear when stopped", async ({ page, agenttik }) => {
  const work = await (await page.request.post(`${agenttik.url}/api/profiles`, { data: { name: "Work" } })).json();
  const project = await (await page.request.post(`${agenttik.url}/api/projects?profile=${work.id}`, { data: { path: REPO } })).json();
  const task = await (await page.request.post(`${agenttik.url}/api/sessions?profile=${work.id}`, {
    data: { project_id: project.id, provider: "fake", model: "fake-quick", title: "Working elsewhere" },
  })).json();
  const picker = page.getByRole("button", { name: "Profile: Default", exact: true });
  const cue = picker.getByLabel("Profiles are working");
  await expect(picker).toBeVisible();
  await expect(cue).toHaveCount(0);
  await page.request.post(`${agenttik.url}/api/sessions/${task.id}/messages?profile=${work.id}`, { data: { prompt: "@wait 60000" } });
  await expect(cue).toBeVisible();
  const frames = await cue.evaluate(el => {
    const animation = el.getAnimations()[0];
    animation.pause();
    return [0, 1000, 2000].map(time => {
      animation.currentTime = time;
      const style = getComputedStyle(el);
      return { opacity: Number(style.opacity), scale: new DOMMatrix(style.transform).a, width: el.getBoundingClientRect().width };
    });
  });
  expect(frames[0].scale).toBe(1);
  expect(frames[1].scale).toBeCloseTo(0.6);
  expect(frames[1].opacity).toBe(0.5);
  expect(frames[1].width).toBeLessThan(frames[0].width);
  expect(frames[2]).toEqual(frames[0]);
  await picker.click();
  const item = page.getByRole("menuitem", { name: /Work/ });
  await expect(item.getByLabel("Working", { exact: true })).toBeVisible();
  await page.request.post(`${agenttik.url}/api/sessions/${task.id}/stop?profile=${work.id}`);
  await expect(cue).toHaveCount(0);
  await expect(item.getByLabel("Working", { exact: true })).toHaveCount(0);
});

for (const pending of ["projects", "providers"]) {
  test(`Ctrl+T during profile ${pending} loading opens a prompt when ready`, async ({ page, agenttik }) => {
    const work = await (await page.request.post(`${agenttik.url}/api/profiles`, { data: { name: "Work" } })).json();
    await page.request.post(`${agenttik.url}/api/projects?profile=${work.id}`, { data: { path: REPO } });
    await page.reload();

    let release;
    const gate = new Promise(resolve => { release = resolve; });
    let requested;
    const held = new Promise(resolve => { requested = resolve; });
    await page.route(`**/api/${pending}?*`, async route => {
      if (new URL(route.request().url()).searchParams.get("profile") === work.id) {
        requested();
        await gate;
      }
      await route.continue();
    });
    await page.getByRole("button", { name: "Profile: Default", exact: true }).click();
    await page.getByRole("menuitem", { name: "Work", exact: true }).click();
    await held;
    await expect(page.getByRole("button", { name: "Profile: Work", exact: true })).toBeVisible();
    try {
      await page.keyboard.press("Control+t");
      // Give the shortcut time to run while the required response is held.
      await page.waitForTimeout(200);
      await expect(page.getByText("No projects yet", { exact: true })).toHaveCount(0);
      await expect(page.getByText("Open a project or session first.", { exact: true })).toHaveCount(0);
      await expect(page.getByText(/No agent provider is ready/)).toHaveCount(0);
    } finally {
      release();
    }
    await expect(page.getByPlaceholder("Ask the agent…")).toBeFocused();
    const sessions = await (await page.request.get(`${agenttik.url}/api/sessions?profile=${work.id}&window=all`)).json();
    expect(sessions).toHaveLength(1);
    expect(sessions[0].provider).toBe("fake");
  });
}
