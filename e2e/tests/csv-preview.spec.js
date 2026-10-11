import { mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { addProject, expect, openProject, sidebar, test } from "../fixtures.js";

test("CSV previews detect separators, retain every column and cycle column sorting", async ({ page, agenttik }) => {
  const dir = await mkdtemp(join(tmpdir(), "agenttik-csv-"));
  try {
    await writeFile(join(dir, "data.csv"), 'Name;Count;Note\nAmy;2;"two\nlines"\nZoe;10;"commas, colons: and <b>text</b>"\nBob;2;plain\n');
    await writeFile(join(dir, "empty.csv"), "");
    await page.goto(agenttik.url);
    await addProject(page, dir);
    await openProject(page, dir);
    await sidebar(page).getByRole("tab", { name: "Tree" }).click();
    await sidebar(page).getByRole("button", { name: "data.csv", exact: true }).dblclick();
    await page.getByRole("tab", { name: "Preview", exact: true }).click();
    const table = page.getByRole("table", { name: "CSV preview" });
    const names = table.locator("tbody tr td:first-child");
    await expect(table.getByRole("columnheader")).toHaveCount(3);
    await expect(names).toHaveText(["Amy", "Zoe", "Bob"]);
    await expect(table.getByRole("cell", { name: "commas, colons: and <b>text</b>", exact: true })).toBeVisible();
    await expect(table.locator("b")).toHaveCount(0);
    await expect(table.locator("tbody tr").first().locator("td").nth(2)).toHaveText("two\nlines");

    const count = table.getByRole("columnheader", { name: "Count" });
    await count.getByRole("button").click();
    await expect(count).toHaveAttribute("aria-sort", "ascending");
    await expect(names).toHaveText(["Amy", "Bob", "Zoe"]);
    await count.getByRole("button").click();
    await expect(count).toHaveAttribute("aria-sort", "descending");
    await expect(names).toHaveText(["Zoe", "Amy", "Bob"]);
    await count.getByRole("button").click();
    await expect(count).toHaveAttribute("aria-sort", "none");
    await expect(names).toHaveText(["Amy", "Zoe", "Bob"]);
    await table.getByRole("columnheader", { name: "Name" }).getByRole("button").click();
    await expect(names).toHaveText(["Amy", "Bob", "Zoe"]);

    // Preview follows unsaved text, including a changed separator and width.
    await page.getByRole("tab", { name: "Edit", exact: true }).click();
    await page.getByRole("textbox", { name: "data.csv", exact: true }).fill("Name\tCount\nZoe\t10\nAmy\t2\nBob\t3\textra");
    await page.getByRole("tab", { name: "Preview", exact: true }).click();
    await expect(table.getByRole("columnheader")).toHaveCount(3);
    await expect(table.getByRole("columnheader", { name: "Column 3" })).toBeVisible();
    await expect(names).toHaveText(["Zoe", "Amy", "Bob"]);
    await expect(table.locator("tbody tr").first().getByRole("cell")).toHaveCount(3);
    await expect(table.getByRole("cell", { name: "extra", exact: true })).toBeVisible();

    // A wide colon-separated file keeps its last column reachable and sortable.
    await page.getByRole("tab", { name: "Edit", exact: true }).click();
    const wide = ["Field ", "Z", "A"].map(prefix => Array.from({ length: 30 }, (_, i) => `${prefix}${i + 1}`).join(":"));
    await page.getByRole("textbox", { name: "data.csv", exact: true }).fill(wide.join("\n"));
    await page.getByRole("tab", { name: "Preview", exact: true }).click();
    await expect(table.getByRole("columnheader")).toHaveCount(30);
    expect(await table.evaluate(table => {
      let pane = table.parentElement;
      while (pane && getComputedStyle(pane).overflowX !== "auto") pane = pane.parentElement;
      return pane && pane.scrollWidth > pane.clientWidth;
    })).toBe(true);
    await table.getByRole("columnheader", { name: "Field 30", exact: true }).getByRole("button").click();
    await expect(table.locator("tbody tr td:last-child")).toHaveText(["A30", "Z30"]);

    await sidebar(page).getByRole("button", { name: "empty.csv", exact: true }).dblclick();
    await page.getByRole("tab", { name: "Preview", exact: true }).click();
    await expect(page.getByText("Empty CSV file.", { exact: true })).toBeVisible();
  } finally {
    await rm(dir, { recursive: true, force: true });
  }
});

test("large CSV previews page all rows and sort the whole file", async ({ page, agenttik }) => {
  const dir = await mkdtemp(join(tmpdir(), "agenttik-csv-pages-"));
  try {
    await writeFile(join(dir, "large.tsv"), "Name\tCount\n" + Array.from({ length: 405 }, (_, i) => `row${i}\t${405 - i}`).join("\n"));
    await page.goto(agenttik.url);
    await addProject(page, dir);
    await openProject(page, dir);
    await sidebar(page).getByRole("tab", { name: "Tree" }).click();
    await sidebar(page).getByRole("button", { name: "large.tsv", exact: true }).dblclick();
    await page.getByRole("tab", { name: "Preview", exact: true }).click();
    const table = page.getByRole("table", { name: "CSV preview" });
    const first = table.locator("tbody tr").first().locator("td").first();
    await expect(table.locator("tbody tr")).toHaveCount(200);
    await expect(first).toHaveText("row0");
    await page.getByRole("button", { name: "Next page", exact: true }).click();
    await expect(first).toHaveText("row200");
    await page.getByRole("button", { name: "Next page", exact: true }).click();
    await expect(table.locator("tbody tr")).toHaveCount(5);
    await expect(first).toHaveText("row400");
    await expect(page.getByRole("button", { name: "Next page", exact: true })).toBeDisabled();
    await page.getByRole("button", { name: "Previous page", exact: true }).click();
    await table.getByRole("columnheader", { name: "Count" }).getByRole("button").click();
    await expect(first).toHaveText("row404");
    await expect(page.getByText("Rows 1–200 of 405", { exact: true })).toBeVisible();
  } finally {
    await rm(dir, { recursive: true, force: true });
  }
});
