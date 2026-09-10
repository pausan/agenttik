import { dirname } from "node:path";

import { REPO, expect, test } from "../fixtures.js";

const PARENT = dirname(REPO);

/* The picker walks one directory at a time. What is typed after the last "/"
   never reaches the server: it fuzzy-filters the listing already in hand. */

const field = (page) => page.getByPlaceholder("~/code/myproject");
const rows = (page) => page.getByRole("dialog").locator("div.h-\\[190px\\] button");

test.beforeEach(async ({ page }) => {
  await page.getByRole("button", { name: "Add project" }).click();
  await expect(field(page)).toHaveValue(/.+/);
});

test("it opens on a folder, with a trailing slash ready for the next keystroke", async ({ page }) => {
  await expect(field(page)).toHaveValue(/\/$/);
  await expect(rows(page)).not.toHaveCount(0);
});

test("the tail after the last slash narrows the listing", async ({ page }) => {
  await field(page).fill(PARENT + "/");
  const all = await rows(page).count();

  await field(page).fill(PARENT + "/agentt");
  await expect(rows(page)).toHaveCount(1);
  await expect(rows(page).first()).toContainText("agenttik");
  expect(all).toBeGreaterThan(1);
});

test("letters matched out of order still match, best first, and are highlighted", async ({ page }) => {
  await field(page).fill(PARENT + "/att"); // a…tt, a subsequence of agenttik
  await expect(rows(page).first()).toContainText("agenttik");
  await expect(page.getByRole("dialog").locator(".text-primary").first()).toHaveText("a");
});

test("Enter takes the best match and walks into it", async ({ page }) => {
  await field(page).fill(PARENT + "/agentt");
  await expect(rows(page)).toHaveCount(1);

  await field(page).press("Enter");
  await expect(field(page)).toHaveValue(REPO + "/");
});

test("clicking a folder walks into it and the breadcrumb walks back up", async ({ page }) => {
  await field(page).fill(PARENT + "/");
  await rows(page).filter({ hasText: "agenttik" }).first().click();
  await expect(field(page)).toHaveValue(REPO + "/");

  await page.getByRole("dialog").getByRole("button", { name: "github", exact: true }).click();
  await expect(field(page)).toHaveValue(PARENT + "/");
});

test("dot folders stay hidden until asked for", async ({ page }) => {
  await field(page).fill(REPO + "/");
  await expect(rows(page).filter({ hasText: ".git" })).toHaveCount(0);

  await page.getByRole("dialog").getByText("Show hidden folders").click();
  await expect(rows(page).filter({ hasText: ".git" })).not.toHaveCount(0);
});

test("a folder that does not resolve is left alone rather than flagged", async ({ page }) => {
  await field(page).fill("/no/such/folder");
  await page.waitForTimeout(400); // let the debounced lookup run and fail

  await expect(field(page)).toHaveValue("/no/such/folder");
  await expect(page.getByRole("dialog").getByText(/No folder matches|No folders here/)).toBeHidden();
});
