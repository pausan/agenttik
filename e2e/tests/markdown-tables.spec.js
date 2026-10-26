import {
  REPO,
  addProject,
  expect,
  newTask,
  openProject,
  pickModel,
  sendPrompt,
  test,
} from "../fixtures.js";

test("Markdown table columns stay readable and scroll within narrow replies", async ({ page }) => {
  await addProject(page, REPO);
  await openProject(page, REPO);
  await newTask(page);
  await pickModel(page);
  const markdown = [
    "| | AWS SES | Proposed for SMTP | Status |",
    "|---|---|---|---|",
    "| **Opens** | A transparent tracking pixel is added to the HTML before sending. | The same pixel is served from the API. | Supported |",
    "| **Skipping a link** | Add the no-track attribute to the link. | Keep existing templates compatible. | Supported |",
    "| **Accuracy** | Apple Mail preloading and image proxies affect tracking. | Identical limitations apply. | Supported |",
  ].join("\n");
  // The fake provider streams words without line breaks. Supply the table
  // in the stored reply so it reaches the real transcript renderer intact.
  await sendPrompt(page, "Compare tracking options");
  await expect(page.locator(".markdown")).toHaveText("Compare tracking options");
  await page.route(/\/api\/sessions\/[^/]+$/, async (route) => {
    const response = await route.fetch();
    const detail = await response.json();
    for (const message of detail.messages) {
      if (message.role === "assistant") message.content = markdown;
    }
    await route.fulfill({ response, json: detail });
  });
  await page.reload();

  const table = page.locator(".markdown table");
  await expect(table.getByRole("cell", { name: "Accuracy", exact: true })).toBeVisible();

  for (const width of [1440, 860, 390]) {
    await page.setViewportSize({ width, height: 900 });
    const layout = await table.evaluate((el) => {
      const rem = parseFloat(getComputedStyle(document.documentElement).fontSize);
      return {
        widths: [...el.rows].map((row) => [...row.cells].map((cell) => cell.getBoundingClientRect().width / rem)),
        width: el.clientWidth,
        scrollWidth: el.scrollWidth,
        pageWidth: document.documentElement.clientWidth,
        pageScrollWidth: document.documentElement.scrollWidth,
      };
    });
    for (const row of layout.widths) {
      expect(row[0]).toBeGreaterThanOrEqual(10);
      for (const cell of row.slice(1)) expect(cell).toBeGreaterThanOrEqual(8);
    }
    expect(layout.pageScrollWidth).toBeLessThanOrEqual(layout.pageWidth);
    if (width === 390) {
      expect(layout.scrollWidth).toBeGreaterThan(layout.width);
      await table.evaluate((el) => { el.scrollLeft = el.scrollWidth; });
      expect(await table.evaluate((el) => el.scrollLeft)).toBeGreaterThan(0);
    }
  }
});
