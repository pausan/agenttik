import { mkdtemp } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { expect, openSettings, REPO, sidebar, startServer, test } from "../fixtures.js";

test("remote work pulses in the profile area from local and remote profiles", async ({ page, agenttik }) => {
  const other = await startServer();
  try {
    await page.request.put(`${other.url}/api/server/name`, { data: { name: "Office" } });
    const work = await (await page.request.post(`${other.url}/api/profiles`, { data: { name: "Work" } })).json();
    const project = await (await page.request.post(`${other.url}/api/projects?profile=${work.id}`, { data: { path: REPO } })).json();
    const task = await (await page.request.post(`${other.url}/api/sessions?profile=${work.id}`, {
      data: { project_id: project.id, provider: "fake", model: "fake-quick", title: "Remote work" },
    })).json();
    await page.request.post(`${agenttik.url}/api/remotes`, { data: { address: other.url } });
    await page.reload();
    let picker = page.getByRole("button", { name: "Profile: Default", exact: true });
    const cue = () => picker.getByLabel("Profiles are working");
    await expect(cue()).toHaveCount(0);
    await page.request.post(`${other.url}/api/sessions/${task.id}/messages?profile=${work.id}`, { data: { prompt: "@wait 60000" } });
    await expect(cue()).toBeVisible();
    await expect(cue()).toHaveClass(/status-pulse/);
    await picker.click();
    const item = page.getByRole("menuitem", { name: /^Work\b/ });
    await expect(item.getByLabel("Working", { exact: true })).toBeVisible();
    await item.click();
    picker = page.getByRole("button", { name: "Profile: Office › Work", exact: true });
    // The selected remote profile's work still appears in the footer.
    await expect(cue()).toBeVisible();
    await page.request.post(`${other.url}/api/sessions/${task.id}/stop?profile=${work.id}`);
    await expect(cue()).toHaveCount(0);

    // Work in a different remote profile is visible while staying on Office.
    const defaultProject = await (await page.request.post(`${other.url}/api/projects`, { data: { path: REPO } })).json();
    const defaultTask = await (await page.request.post(`${other.url}/api/sessions`, {
      data: { project_id: defaultProject.id, provider: "fake", model: "fake-quick" },
    })).json();
    await page.request.post(`${other.url}/api/sessions/${defaultTask.id}/messages`, { data: { prompt: "@wait 60000" } });
    await expect(cue()).toBeVisible();
    await page.request.post(`${other.url}/api/sessions/${defaultTask.id}/stop`);
    await expect(cue()).toHaveCount(0);
  } finally {
    await other.stop();
  }
});

test("a remote machine's profiles open through this instance and wait for it when it is gone", async ({ page, agenttik }) => {
  const other = await startServer();
  let stopped = false;
  try {
    await page.request.put(`${other.url}/api/server/name`, { data: { name: "Office machine" } });
    const dir = await mkdtemp(join(tmpdir(), "agenttik-remote-project-"));
    await page.request.post(`${other.url}/api/projects`, { data: { path: dir, name: "Remote project" } });
    await page.request.post(`${other.url}/api/profiles`, { data: { name: "Night shift" } });
    await page.request.post(`${agenttik.url}/api/projects`, { data: { path: dir, name: "Local project" } });
    await page.reload();

    await openSettings(page, "Profiles");
    await page.getByRole("textbox", { name: "Remote address" }).fill(other.url);
    await page.getByRole("button", { name: "Add machine", exact: true }).click();
    await expect(page.getByText("Office machine", { exact: true })).toBeVisible();
    await expect(page.getByRole("button", { name: "Switch to Office machine Night shift", exact: true })).toBeVisible();
    await page.getByRole("button", { name: "Switch to Office machine Default", exact: true }).click();

    await expect(sidebar(page).getByText("Remote project", { exact: true })).toBeVisible();
    await expect(sidebar(page).getByText("Local project", { exact: true })).toHaveCount(0);
    const picker = page.getByRole("button", { name: "Profile: Office machine › Default", exact: true });
    await expect(picker).toBeVisible();
    await expect(picker.locator("[data-remote]")).toBeVisible();
    await picker.click();
    await expect(page.getByRole("menu").getByText("Office machine", { exact: true })).toBeVisible();
    await page.getByRole("menuitem", { name: "Default", exact: true }).first().click();
    await expect(sidebar(page).getByText("Local project", { exact: true })).toBeVisible();

    // Opening the remote again after it stopped asks what to do instead of
    // loading a workspace that cannot answer.
    const remoteURL = new URL(agenttik.url);
    remoteURL.searchParams.set("remote", (await (await page.request.get(`${other.url}/api/version`)).json()).id);
    await other.stop();
    stopped = true;
    await page.goto(remoteURL.href);
    const dialog = page.getByRole("dialog", { name: "Office machine" });
    await expect(dialog.getByText("This machine is not answering.")).toBeVisible();
    await expect(dialog.getByRole("button", { name: "Find on network", exact: true })).toBeVisible();
    await dialog.getByRole("button", { name: "Use a local profile", exact: true }).click();
    await expect(sidebar(page).getByText("Local project", { exact: true })).toBeVisible();
  } finally {
    if (!stopped) await other.stop();
  }
});

test("remote discovery can pause, continue, restart and connect to different machines", async ({ page }) => {
  const office = await startServer();
  const lab = await startServer();
  try {
    const machines = [];
    for (const [server, name] of [[office, "Office"], [lab, "Lab"]]) {
      await page.request.put(`${server.url}/api/server/name`, { data: { name } });
      const version = await (await page.request.get(`${server.url}/api/version`)).json();
      machines.push({ id: version.id, name, address: server.url, version: version.version });
    }
    // Keep discovery deterministic without probing the test runner's LAN.
    // Adding and switching machines still uses the two real remote servers.
    let state = { running: false, machines: [] };
    const requests = [];
    await page.route(/\/api\/remotes\/discovery(?:\?|$)/, async route => {
      const method = route.request().method();
      requests.push(method);
      if (method === "POST") {
        const body = route.request().postDataJSON();
        if (!state.total || body.restart) {
          state = { running: true, complete: false, probed: 0, total: 256, port: body.port, machines: body.restart ? [] : machines };
        } else {
          state.running = true;
          state.probed += 10;
        }
      } else if (method === "DELETE") state.running = false;
      await route.fulfill({ json: state });
    });
    await openSettings(page, "Profiles");
    await page.getByRole("button", { name: "Search for machines", exact: true }).click();
    await expect(page.getByRole("button", { name: "Connect to Office", exact: true })).toBeVisible();
    await page.getByRole("button", { name: "Pause search", exact: true }).click();
    await expect(page.getByRole("status").filter({ hasText: "Search paused" })).toBeVisible();
    await page.getByRole("button", { name: "Continue search", exact: true }).click();
    await expect(page.getByRole("status").filter({ hasText: "10 of 256" })).toBeVisible();

    // Closing Settings pauses and reopening it keeps progress and matches.
    await page.getByRole("dialog", { name: "Settings", exact: true }).getByRole("button", { name: "Close", exact: true }).click();
    await expect.poll(() => state.running).toBe(false);
    await openSettings(page, "Profiles");
    await expect(page.getByRole("button", { name: "Connect to Lab", exact: true })).toBeVisible();
    await page.getByRole("button", { name: "Continue search", exact: true }).click();
    await page.getByRole("button", { name: "Restart search", exact: true }).click();
    await expect(page.getByRole("button", { name: "Connect to Office", exact: true })).toHaveCount(0);
    await expect(page.getByRole("status").filter({ hasText: "0 of 256" })).toBeVisible();
    state.machines = machines;
    await expect(page.getByRole("button", { name: "Connect to Office", exact: true })).toBeVisible();

    // A discovered machine that asks for sign-in must still open its profile
    // after sign-in, rather than leave the user in Settings.
    let officeConnection;
    await page.route(/\/api\/remotes(?:\?|$)/, async route => {
      if (route.request().method() !== "POST" || route.request().postDataJSON().address !== office.url) {
        await route.continue();
        return;
      }
      officeConnection = await (await route.fetch()).json();
      await route.fulfill({ json: { ...officeConnection, status: "signin", remote: { ...officeConnection.remote, profiles: [] } } });
    });
    await page.route(/\/api\/remotes\/[^/]+\/login(?:\?|$)/, async route => {
      expect(route.request().postDataJSON().password).toBe("test password");
      await route.fulfill({ json: officeConnection });
    });
    const added = [];
    page.on("request", request => {
      if (request.method() === "POST" && new URL(request.url()).pathname === "/api/remotes") {
        added.push({ address: request.postDataJSON().address, searching: state.running });
      }
    });
    await page.getByRole("button", { name: "Connect to Office", exact: true }).click();
    const signin = page.getByRole("dialog", { name: "Office", exact: true });
    await signin.getByRole("textbox", { name: "Remote password", exact: true }).fill("test password");
    await signin.getByRole("button", { name: "Sign in", exact: true }).click();
    await expect(page.getByRole("button", { name: "Profile: Office › Default", exact: true })).toBeVisible();
    await openSettings(page, "Profiles");
    await expect(page.getByRole("button", { name: "Continue search", exact: true })).toBeVisible();
    await page.getByRole("button", { name: "Continue search", exact: true }).click();
    await page.getByRole("button", { name: "Connect to Lab", exact: true }).click();
    await expect(page.getByRole("button", { name: "Profile: Lab › Default", exact: true })).toBeVisible();
    expect(added).toEqual([{ address: office.url, searching: false }, { address: lab.url, searching: false }]);
    expect(requests.filter(method => method === "DELETE").length).toBeGreaterThanOrEqual(4);
  } finally {
    await office.stop();
    await lab.stop();
  }
});
