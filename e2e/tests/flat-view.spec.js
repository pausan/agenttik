import { mkdtemp } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { expect, newTask, pickModel, sendPrompt, sidebar, startServer, test } from "../fixtures.js";

test("the flat view shows every profile's projects and runs work where each lives", async ({ page, agenttik }) => {
  const other = await startServer();
  try {
    await page.request.put(`${other.url}/api/server/name`, { data: { name: "Laptop" } });
    await page.request.put(`${agenttik.url}/api/server/name`, { data: { name: "Desk" } });
    const far = await mkdtemp(join(tmpdir(), "agenttik-flat-far-"));
    await page.request.post(`${other.url}/api/projects`, { data: { path: far, name: "Far side" } });
    const shared = await mkdtemp(join(tmpdir(), "agenttik-flat-shared-"));
    const work = await (await page.request.post(`${agenttik.url}/api/profiles`, { data: { name: "Work" } })).json();
    await page.request.post(`${agenttik.url}/api/projects`, { data: { path: shared, name: "Shared" } });
    await page.request.post(`${agenttik.url}/api/projects?profile=${work.id}`, { data: { path: shared, name: "Shared" } });
    await page.request.post(`${agenttik.url}/api/remotes`, { data: { address: other.url } });
    await page.reload();

    await page.getByRole("button", { name: "Profile: Default", exact: true }).click();
    await page.getByRole("menuitem", { name: "Flat view", exact: true }).click();
    await expect(page.getByRole("button", { name: "Profile: Flat view", exact: true })).toBeVisible();

    // The same project in two profiles is two rows, each naming its place.
    // Default's orchestrator leads, the only one the flat view keeps.
    const side = sidebar(page);
    await expect(side.getByText("Shared", { exact: true })).toHaveCount(2);
    const sources = side.locator("[data-project-source]");
    await expect(sources).toHaveText(["Desk › Default ·", "Desk › Default ·", "Desk › Work ·", "Laptop › Default ·"].map(s => s + " "), { useInnerText: true });

    // A task started on the remote's project runs on that machine.
    await side.getByText("Far side", { exact: true }).click();
    await newTask(page);
    await pickModel(page);
    await sendPrompt(page, "check the far side");
    await expect(page.getByText("Agent", { exact: true })).toBeVisible();
    await expect(page.getByText("idle")).toBeVisible();
    const remoteTasks = await (await page.request.get(`${other.url}/api/sessions?window=all`)).json();
    expect(remoteTasks.map(t => t.status)).toEqual(["idle"]);
    const localTasks = await (await page.request.get(`${agenttik.url}/api/sessions?window=all`)).json();
    expect(localTasks).toEqual([]);
    await page.screenshot({ path: "test-results/flat-view.png" });
  } finally {
    await other.stop();
  }
});

test("a task's unread state and mark are the same in the flat view and its profile", async ({ page, agenttik, context }) => {
  const work = await (await page.request.post(`${agenttik.url}/api/profiles`, { data: { name: "Work" } })).json();
  const project = await (await page.request.post(`${agenttik.url}/api/projects?profile=${work.id}`, {
    data: { path: await mkdtemp(join(tmpdir(), "agenttik-flat-tasks-")), name: "Work project" },
  })).json();
  const task = await (await page.request.post(`${agenttik.url}/api/sessions?profile=${work.id}`, {
    data: { project_id: project.id, provider: "fake", model: "fake-quick", title: "Seen everywhere" },
  })).json();
  await page.goto(`${agenttik.url}/?profile=${work.id}`);
  const flat = await context.newPage();
  await flat.goto(`${agenttik.url}/?profile=all`);
  const inProfile = sidebar(page).locator(".task-row").filter({ hasText: task.title });
  const inFlat = sidebar(flat).locator(".task-row").filter({ hasText: task.title });
  await sidebar(page).getByText("Work project", { exact: true }).click();
  await sidebar(flat).getByText("Work project", { exact: true }).click();
  await expect(inProfile).toBeVisible();
  await expect(inFlat).toBeVisible();

  // Unread set in one window shows in the other, and across a reload.
  await inProfile.click({ button: "right" });
  await page.getByRole("menuitem", { name: "Mark unread", exact: true }).click();
  await expect(inFlat.locator(".task-title")).toHaveClass(/font-bold/);
  await flat.reload();
  await expect(inFlat.locator(".task-title")).toHaveClass(/font-bold/);

  // Reading it in the flat view reads it in the profile.
  await inFlat.click();
  await expect(inFlat.locator(".task-title")).not.toHaveClass(/font-bold/, { timeout: 6000 });
  await expect(inProfile.locator(".task-title")).not.toHaveClass(/font-bold/);
  await page.reload();
  await expect(inProfile.locator(".task-title")).not.toHaveClass(/font-bold/);

  // A mark set in the flat view shows in the profile, and the other way.
  await sidebar(page).getByText("Work project", { exact: true }).click();
  await expect(inProfile).toBeVisible();
  await inFlat.click({ button: "right" });
  await flat.getByRole("menuitem", { name: "Mark", exact: true }).hover();
  await flat.getByRole("menuitemcheckbox", { name: "Star", exact: true }).click();
  await expect(inProfile.locator(".task-flag")).toHaveAttribute("data-flag", "star");
  await inProfile.click({ button: "right" });
  await page.getByRole("menuitem", { name: "Mark", exact: true }).hover();
  await page.getByRole("menuitemcheckbox", { name: "Warning", exact: true }).click();
  await expect(inFlat.locator(".task-flag")).toHaveAttribute("data-flag", "warn");
});

test("a profile's saved unread state joins the shared copy and the flat view's old copy goes", async ({ page, agenttik }) => {
  const work = await (await page.request.post(`${agenttik.url}/api/profiles`, { data: { name: "Work" } })).json();
  const res = await page.request.get(`${agenttik.url}/api/projects?profile=${work.id}`);
  const instance = res.headers()["x-agenttik-instance"];
  const project = await (await page.request.post(`${agenttik.url}/api/projects?profile=${work.id}`, {
    data: { path: await mkdtemp(join(tmpdir(), "agenttik-flat-saved-")), name: "Work project" },
  })).json();
  const create = async (title) => (await page.request.post(`${agenttik.url}/api/sessions?profile=${work.id}`, {
    data: { project_id: project.id, provider: "fake", model: "fake-quick", title },
  })).json();
  const kept = await create("Unread in Work");
  const stale = await create("Unread only in the flat view");
  const old = (scope) => `agenttik.unreadTasks:${instance}:${scope}`;
  await page.evaluate(([workKey, flatKey, keptID, staleID]) => {
    localStorage.setItem(workKey, JSON.stringify({ [keptID]: -1 }));
    localStorage.setItem(flatKey, JSON.stringify({ [staleID]: -1 }));
  }, [old(work.id), old("all"), kept.id, stale.id]);

  await page.goto(`${agenttik.url}/?profile=${work.id}`);
  await sidebar(page).getByText("Work project", { exact: true }).click();
  const row = (title) => sidebar(page).locator(".task-row").filter({ hasText: title }).locator(".task-title");
  await expect(row(kept.title)).toHaveClass(/font-bold/);

  await page.goto(`${agenttik.url}/?profile=all`);
  await sidebar(page).getByText("Work project", { exact: true }).click();
  await expect(row(kept.title)).toHaveClass(/font-bold/);
  await expect(row(stale.title)).not.toHaveClass(/font-bold/);
  expect(await page.evaluate((keys) => keys.map((key) => localStorage.getItem(key)), [old(work.id), old("all")])).toEqual([null, null]);
});

test("on a phone the flat view names each project's place in the drawer", async ({ page, agenttik }) => {
  const other = await startServer();
  try {
    await page.setViewportSize({ width: 390, height: 844 });
    await page.request.put(`${other.url}/api/server/name`, { data: { name: "Laptop" } });
    const far = await mkdtemp(join(tmpdir(), "agenttik-flat-far-"));
    await page.request.post(`${other.url}/api/projects`, { data: { path: far, name: "Far side" } });
    await page.request.post(`${agenttik.url}/api/remotes`, { data: { address: other.url } });
    await page.goto(`${agenttik.url}/?profile=all`);
    await page.getByRole("button", { name: "Projects and tasks", exact: true }).click();
    const drawer = page.getByRole("dialog", { name: "Projects and tasks", exact: true });
    await expect(drawer.getByText("Far side", { exact: true })).toBeVisible();
    await expect(drawer.locator("[data-project-source]").last()).toHaveText("Laptop › Default ·");
    await page.screenshot({ path: "test-results/flat-view-phone.png" });
  } finally {
    await other.stop();
  }
});

test("a machine that stops answering shows a red cross and is tried again when opened", async ({ page, agenttik }) => {
  const other = await startServer();
  await page.request.put(`${other.url}/api/server/name`, { data: { name: "Laptop" } });
  await page.request.post(`${agenttik.url}/api/remotes`, { data: { address: other.url } });
  await other.stop();

  await page.goto(`${agenttik.url}/?profile=all`);
  await page.getByRole("button", { name: "Profile: Flat view", exact: true }).click();
  const far = page.getByRole("menuitem", { name: "Default", exact: true }).last();
  await expect(far.locator("[data-down]")).toBeVisible({ timeout: 10_000 });
  await page.screenshot({ path: "test-results/flat-view-remote-down.png" });

  // Opening it tries the machine again and says what can be done.
  await far.click();
  await expect(page.getByRole("button", { name: "Retry", exact: true })).toBeVisible();
});

test("the flat view keeps its own order and hidden projects", async ({ page, agenttik }) => {
  const work = await (await page.request.post(`${agenttik.url}/api/profiles`, { data: { name: "Work" } })).json();
  for (const name of ["Alpha", "Beta"]) {
    const path = await mkdtemp(join(tmpdir(), "agenttik-flat-order-"));
    await page.request.post(`${agenttik.url}/api/projects`, { data: { path, name } });
  }
  const gamma = await mkdtemp(join(tmpdir(), "agenttik-flat-order-"));
  await page.request.post(`${agenttik.url}/api/projects?profile=${work.id}`, { data: { path: gamma, name: "Gamma" } });
  const ownOrder = async () => (await (await page.request.get(`${agenttik.url}/api/projects`)).json())
    .filter((p) => p.kind !== "orchestrator").map((p) => p.name);
  const before = await ownOrder();
  await page.goto(`${agenttik.url}/?profile=all`);

  const rows = sidebar(page).locator('[draggable="true"]').filter({ hasText: /Alpha|Beta|Gamma/ });
  const names = async () => (await sidebar(page).locator(".sidebar-project span.font-semibold").allTextContents())
    .filter((n) => ["Alpha", "Beta", "Gamma"].includes(n));
  await expect(rows).toHaveCount(3);
  const first = (await names())[0];

  // Work's project moves above Default's first one.
  const moved = rows.filter({ hasText: "Gamma" });
  const target = rows.filter({ hasText: first });
  const saved = page.waitForResponse((r) => r.request().method() === "POST" && r.url().includes("/api/projects/order"));
  const from = await moved.boundingBox();
  const to = await target.boundingBox();
  await page.mouse.move(from.x + from.width / 2, from.y + from.height / 2);
  await page.mouse.down();
  await page.mouse.move(from.x + from.width / 2 + 8, from.y + from.height / 2, { steps: 3 });
  await page.mouse.move(to.x + to.width / 2, to.y + to.height * 0.2, { steps: 5 });
  await page.mouse.move(to.x + to.width / 2 + 1, to.y + to.height * 0.2);
  await page.mouse.up();
  expect((await saved).ok()).toBe(true);
  const mixed = ["Gamma", ...before];
  await expect.poll(names).toEqual(mixed);
  await page.reload();
  await expect.poll(names).toEqual(mixed);
  expect(await ownOrder()).toEqual(before);

  // Hiding from the flat view leaves the project in its profile.
  await sidebar(page).locator(".sidebar-project").filter({ hasText: "Beta" }).getByRole("button", { name: "Hide project" }).click();
  await expect.poll(names).toEqual(mixed.filter((n) => n !== "Beta"));
  expect(await ownOrder()).toEqual(before);
  await sidebar(page).getByRole("button", { name: "Hidden projects" }).click();
  await page.getByRole("option", { name: /Beta/ }).click();
  await expect.poll(names).toEqual(mixed);
});

test("the flat view shows terminals and the screen of each project's machine", async ({ page, agenttik }) => {
  const other = await startServer({ env: { DISPLAY: "invalid-display", WAYLAND_DISPLAY: "" } });
  try {
    await page.request.put(`${other.url}/api/server/name`, { data: { name: "Laptop" } });
    const far = await mkdtemp(join(tmpdir(), "agenttik-flat-far-"));
    await page.request.post(`${other.url}/api/projects`, { data: { path: far, name: "Far side" } });
    const work = await (await page.request.post(`${agenttik.url}/api/profiles`, { data: { name: "Work" } })).json();
    const near = await mkdtemp(join(tmpdir(), "agenttik-flat-near-"));
    await page.request.post(`${agenttik.url}/api/projects?profile=${work.id}`, { data: { path: near, name: "Near side" } });
    await page.request.post(`${agenttik.url}/api/remotes`, { data: { address: other.url } });
    await page.goto(`${agenttik.url}/?profile=all`);

    const side = sidebar(page);
    const screenButton = page.getByRole("button", { name: "View and control this machine's screen" });
    for (const [name, folder] of [["Near side", near], ["Far side", far]]) {
      await side.getByText(name, { exact: true }).click();
      await newTask(page);
      await page.getByRole("button", { name: "Open a terminal in the project folder" }).click();
      const pane = page.locator(".terminal-pane");
      await pane.locator(".xterm-screen").click();
      await page.keyboard.type("pwd");
      await page.keyboard.press("Enter");
      await expect(async () => {
        expect(await pane.innerText()).toContain(folder);
      }).toPass({ timeout: 15_000 });
    }

    // Only the remote's project is on another machine.
    await page.getByRole("tab", { name: "New task" }).click();
    await expect(screenButton).toBeVisible();
    await screenButton.click();
    await expect(page.getByRole("tab", { name: "Screen · Laptop" })).toBeVisible();
    await expect(page.getByText("No screen to show", { exact: true })).toBeVisible();
    await side.getByText("Near side", { exact: true }).click();
    await newTask(page);
    await expect(page.getByRole("button", { name: "Open a terminal in the project folder" })).toBeVisible();
    await expect(screenButton).toBeHidden();
  } finally {
    await other.stop();
  }
});
