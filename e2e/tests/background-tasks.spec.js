import { addProject, expect, newTask, openProject, pickModel, sendPrompt, test } from "../fixtures.js";

/* What a running turn leaves working behind it is listed under the progress
   line, closed until asked for, and goes with the turn. See
   specs/083-background-tasks.md. */

test("background tasks open under the progress line and go with the turn", async ({ page }) => {
  await addProject(page);
  await openProject(page);
  await newTask(page);
  await pickModel(page);
  await sendPrompt(
    page,
    [
      "@background start b1 npm run dev",
      "@background start b2 make test",
      "@wait 300",
      "@background end b2 failed exit code 2",
      "@wait 3000",
      "Done.",
    ].join("\n"),
  );

  const toggle = page.getByRole("button", { name: "1 background task running" });
  await expect(toggle).toBeVisible();
  const list = page.getByRole("list", { name: "Background tasks" });
  await expect(list).toHaveCount(0);

  await toggle.click();
  await expect(list.getByRole("listitem")).toHaveCount(2);
  const dev = list.getByRole("listitem").filter({ hasText: "npm run dev" });
  await expect(dev).toContainText("started");
  await expect(dev).toContainText(/running \ds/);
  await expect(list.getByRole("listitem").filter({ hasText: "make test" })).toContainText("failed · exit code 2");

  // Reloading reads the list back from the server.
  await page.reload();
  await page.getByRole("button", { name: "1 background task running" }).click();
  await expect(page.getByRole("list", { name: "Background tasks" }).getByRole("listitem")).toHaveCount(2);

  await expect(page.getByText("Done.", { exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: /background task/ })).toHaveCount(0);
});
