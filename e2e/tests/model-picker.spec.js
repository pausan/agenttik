import { expect, FAKE_MODEL, addProject, modelButton, newTask, openProject, pickModel, test } from "../fixtures.js";

const palette = (page) => page.locator(".model-selection-palette");
const groupRow = (page, name) => palette(page).getByRole("option", { name, exact: true });

/* A catalogue as long-winded as a real API one: the service repeats its own
   name on every model, and the models themselves are a mouthful. */
async function withLongCatalogue(page) {
  await page.route("**/api/providers", async (route) => {
    const response = await route.fetch();
    const providers = await response.json();
    const fake = providers.find((provider) => provider.name === "fake");
    providers.push({
      ...fake,
      name: "fakerouter",
      display_name: "Fake",
      kind: "api",
      models: [
        { id: "fake/one", label: "Fake: Reasoning Extra Large 4.5 Preview" },
        { id: "other/two", label: "Other Vendor: Reasoning Extra Large 4.5" },
      ],
    });
    await route.fulfill({ response, json: providers });
  });
  await page.reload();
}

test("favourites head the picker and the rest collapses to one row per subscription", async ({ page }) => {
  await addProject(page);
  await openProject(page);
  await newTask(page);
  await pickModel(page);
  await page.getByTitle("Effort", { exact: true }).click();
  await page.getByRole("option", { name: "High", exact: true }).click();
  await page.getByRole("button", { name: `Add to favourites: System · ${FAKE_MODEL} · High` }).click();

  await modelButton(page).click();
  // A favourite names the whole combination it would set.
  await expect(palette(page).getByRole("option").first()).toHaveText(`System · Fake · ${FAKE_MODEL} · High`);

  // The rest is one row per subscription, so the favourites stay in view.
  await expect(groupRow(page, "System · Fake")).toBeVisible();
  await expect(palette(page).getByRole("option", { name: "Fake Careful", exact: true })).toHaveCount(0);

  // Searching reaches into the collapsed group without expanding it by hand,
  // and the rows it finds say only the model.
  await page.getByPlaceholder("Search models…").fill("careful");
  const found = palette(page).getByRole("option", { name: "Fake Careful", exact: true });
  await expect(found).toBeVisible();
  await expect(palette(page).getByRole("group", { name: "System · Fake" })).toBeVisible();

  // Expanding does the same by hand, and collapsing puts it back.
  await page.getByPlaceholder("Search models…").fill("");
  await groupRow(page, "System · Fake").click();
  await expect(found).toBeVisible();
  await expect(modelButton(page)).toContainText(FAKE_MODEL); // expanding is not a choice
  await palette(page).getByRole("button", { name: "System · Fake" }).click();
  await expect(found).toHaveCount(0);

  // A reopened picker starts collapsed again.
  await page.keyboard.press("Escape");
  await modelButton(page).click();
  await expect(found).toHaveCount(0);
  await expect(groupRow(page, "System · Fake")).toBeVisible();
});

test("without favourites every subscription stays open", async ({ page }) => {
  await addProject(page);
  await openProject(page);
  await newTask(page);
  await modelButton(page).click();
  await expect(palette(page).getByRole("option", { name: "Fake Careful", exact: true })).toBeVisible();
  await expect(palette(page).getByRole("group", { name: "System · Fake" })).toBeVisible();
});

test("a model name drops the vendor its own connection already says", async ({ page }) => {
  await addProject(page);
  await openProject(page);
  await newTask(page);
  await withLongCatalogue(page);
  await modelButton(page).click();

  const group = palette(page).getByRole("group", { name: "Fake", exact: true });
  await expect(group.getByRole("option", { name: "Reasoning Extra Large 4.5 Preview", exact: true })).toBeVisible();
  // A vendor the connection does not name is what tells the two rows apart,
  // so it stays.
  await expect(group.getByRole("option", { name: "Other Vendor: Reasoning Extra Large 4.5", exact: true })).toBeVisible();
});

test("the edited prompt picker can favourite a model too", async ({ page }) => {
  await addProject(page);
  await openProject(page);
  await newTask(page);
  await pickModel(page);
  await page.getByPlaceholder("Ask the agent…").fill("first prompt");
  await page.keyboard.press("Control+Enter");
  await expect(page.getByText("idle", { exact: true })).toBeVisible();

  await page.getByRole("button", { name: "Edit this prompt" }).click();
  const editor = page.getByRole("form", { name: "Edit prompt" });
  const star = editor.getByRole("button", { name: `Add to favourites: System · ${FAKE_MODEL} · Default` });
  await expect(star).toBeVisible();
  await star.click();

  await editor.getByTitle(/^Change edited prompt model/).click();
  await expect(palette(page).getByRole("option").first()).toHaveText(`System · Fake · ${FAKE_MODEL} · Default`);
});
