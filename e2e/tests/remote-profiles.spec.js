import { mkdtemp } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { expect, openSettings, sidebar, startServer, test } from "../fixtures.js";

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
