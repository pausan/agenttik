import { addProject, expect, test } from "../fixtures.js";

for (const shortcut of ["Control+n", "Control+t"]) {
  test(`${shortcut} starts a task immediately after adding the first project`, async ({ page, agenttik }) => {
    await addProject(page);
    await expect(page.getByRole("button", { name: "New task", exact: true }).first()).toBeVisible();

    await page.keyboard.press(shortcut);

    await expect(page.getByPlaceholder("Ask the agent…")).toBeVisible();
    await expect(page.getByText("Something went wrong", { exact: true })).toHaveCount(0);
    const projects = await (await page.request.get(`${agenttik.url}/api/projects`)).json();
    const sessions = await (await page.request.get(`${agenttik.url}/api/sessions`)).json();
    expect(sessions).toHaveLength(1);
    expect(sessions[0].project_id).toBe(projects[0].id);
  });

  test(`${shortcut} guides users without projects instead of showing an error`, async ({ page, agenttik }) => {
    await expect(page.getByRole("button", { name: "Take the Quick Start Tour", exact: true })).toBeVisible();
    const mutations = [];
    page.on("request", (request) => {
      if (request.method() === "POST") mutations.push(request.url());
    });

    await page.keyboard.press(shortcut);

    await expect(page.getByText("No projects yet", { exact: true })).toBeVisible();
    await expect(page.getByText("Add the folder you want to work in from the Projects panel, or take the Quick Start Tour in Settings → Help.", { exact: true })).toBeVisible();
    await expect(page.getByText("Something went wrong", { exact: true })).toHaveCount(0);
    expect(await (await page.request.get(`${agenttik.url}/api/sessions`)).json()).toEqual([]);
    expect(mutations).toEqual([]);
  });
}
