import { expect, openSettings, startServer, test } from "../fixtures.js";

const nextBinding = page => page.getByTitle("Change: Next profile", { exact: true });
const closeSettings = page => page.getByRole("dialog", { name: "Settings", exact: true })
  .getByRole("button", { name: "Close", exact: true }).click();

test("shortcuts migrate from local Default and share edits and resets across open profiles", async ({ page, context, agenttik }) => {
  const other = await startServer();
  try {
    const work = await (await page.request.post(`${agenttik.url}/api/profiles`, { data: { name: "Work" } })).json();
    const remote = (await (await page.request.post(`${agenttik.url}/api/remotes`, { data: { address: other.url } })).json()).remote;
    const instance = (await page.request.get(`${agenttik.url}/api/projects?profile=${work.id}`)).headers()["x-agenttik-instance"];
    const legacy = { "profile.next": ["F6"], "prompt.send": ["Enter"], "prompt.enqueue": ["Ctrl+Enter"] };
    // Upgrade while an added profile is open. Its old set must not win.
    await page.evaluate(({ legacy, scope }) => {
      localStorage.removeItem("agenttik.client.keys");
      localStorage.setItem("agenttik.keys", JSON.stringify(legacy));
      localStorage.setItem(`agenttik.keys:${scope}`, JSON.stringify({ "profile.next": ["F7"] }));
    }, { legacy, scope: `${instance}:${work.id}` });
    await page.goto(`${agenttik.url}/?profile=${work.id}`);
    await openSettings(page, "Shortcuts");
    await expect(nextBinding(page)).toHaveText("F6");
    expect(await page.evaluate(() => JSON.parse(localStorage.getItem("agenttik.client.keys")))).toEqual(legacy);

    const local = await context.newPage();
    await local.goto(agenttik.url);
    await openSettings(local, "Shortcuts");
    await expect(nextBinding(local)).toHaveText("F6");
    await expect(local.getByTitle("Change: Send", { exact: true })).toHaveText("Enter");
    await nextBinding(page).click();
    await page.keyboard.press("F8");
    await expect(nextBinding(local)).toHaveText("F8");
    await closeSettings(page);
    // The new binding actually switches from Work to the remote Default.
    await page.keyboard.press("F8");
    await expect(page).toHaveURL(new RegExp(`remote=${remote.id}`));
    await openSettings(page, "Shortcuts");
    await expect(nextBinding(page)).toHaveText("F8");
    await page.getByRole("button", { name: "Restore the default for Next profile", exact: true }).click();
    await expect(nextBinding(local)).not.toHaveText("F8");
    await page.getByRole("button", { name: "Restore all defaults", exact: true }).click();
    expect(await page.evaluate(() => JSON.parse(localStorage.getItem("agenttik.client.keys")))).toEqual({});
    // Resetting must not resurrect the old local Default bindings on reload.
    await local.reload();
    await openSettings(local, "Shortcuts");
    await expect(nextBinding(local)).not.toHaveText("F6");
    expect(await local.evaluate(() => JSON.parse(localStorage.getItem("agenttik.keys")))).toEqual(legacy);
    await local.close();

    // General's reset also resets the shared bindings and keeps the new key.
    await nextBinding(page).click();
    await page.keyboard.press("F9");
    await page.getByRole("button", { name: "General", exact: true }).click();
    await page.getByRole("button", { name: "Reset defaults", exact: true }).click();
    const warning = page.getByRole("dialog", { name: "Reset defaults?", exact: true });
    await expect(warning).toContainText("Shortcuts reset across all profiles");
    await warning.getByRole("button", { name: "Reset defaults", exact: true }).click();
    await expect(page.getByRole("status").filter({ hasText: "Defaults restored" })).toBeVisible();
    await page.reload();
    await openSettings(page, "Shortcuts");
    await expect(nextBinding(page)).not.toHaveText("F9");
    expect(await page.evaluate(() => JSON.parse(localStorage.getItem("agenttik.client.keys")))).toEqual({});
  } finally { await other.stop(); }
});

for (const [platform, modifier] of [["Linux", "Control"], ["macOS", "Meta"]]) {
  test(`remote profiles use the ${platform} client's keyboard defaults`, async ({ page, agenttik }) => {
    const other = await startServer();
    try {
      const remote = (await (await page.request.post(`${agenttik.url}/api/remotes`, { data: { address: other.url } })).json()).remote;
      await page.addInitScript(platform => {
        Object.defineProperty(navigator, "userAgentData", { value: { platform }, configurable: true });
      }, platform);
      await page.goto(`${agenttik.url}/?remote=${remote.id}`);
      await expect(page.getByRole("button", { name: /^Profile:/ })).toBeVisible();
      await page.keyboard.press(`${modifier}+Shift+p`);
      await expect(page.getByPlaceholder("Command Palette…")).toBeVisible();
    } finally { await other.stop(); }
  });
}
