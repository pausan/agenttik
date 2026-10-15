import { mkdir, mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";

import { addProject, expect, openProject, sidebar, startServer, test } from "../fixtures.js";

/* Drops files the way the desktop does: a DataTransfer carrying File objects.
   Chromium gives a page-made transfer no entries to walk, so this goes
   through the plain file list; folders are covered by upload.test.js. */
async function dropFiles(page, target, files) {
  const transfer = await page.evaluateHandle((files) => {
    const dt = new DataTransfer();
    for (const { name, text, size } of files) {
      dt.items.add(new File([text ?? new Uint8Array(size).fill(7)], name));
    }
    return dt;
  }, files);
  await target.dispatchEvent("dragover", { dataTransfer: transfer });
  await target.dispatchEvent("drop", { dataTransfer: transfer });
}

const BIG = 5 * 1024 * 1024 + 3;

test("files dropped on the Tree are copied into the folder under the pointer", async ({ page }) => {
  const root = await mkdtemp(join(tmpdir(), "agenttik-drop-"));
  try {
    await mkdir(join(root, "src"));
    await writeFile(join(root, "src/a.txt"), "old\n");
    await writeFile(join(root, "top.txt"), "top\n");
    await addProject(page, root);
    await openProject(page, root);
    const tree = sidebar(page);
    await tree.getByRole("tab", { name: "Tree", exact: true }).click();
    const row = (name) => tree.getByRole("button", { name, exact: true });

    await dropFiles(page, row("src"), [{ name: "new.txt", text: "hello\n" }, { name: "big.bin", size: BIG }]);
    await expect(row("new.txt")).toBeVisible();
    await expect(row("big.bin")).toBeVisible();
    expect(await readFile(join(root, "src/new.txt"), "utf8")).toBe("hello\n");
    expect((await readFile(join(root, "src/big.bin"))).length).toBe(BIG);

    // Beside a file at the top is the top of the project.
    await dropFiles(page, row("top.txt"), [{ name: "beside.txt", text: "beside\n" }]);
    await expect(row("beside.txt")).toBeVisible();
    expect(await readFile(join(root, "beside.txt"), "utf8")).toBe("beside\n");

    // An existing file is replaced only once that is confirmed.
    await dropFiles(page, row("src"), [{ name: "a.txt", text: "new\n" }]);
    const dialog = page.getByRole("dialog");
    await expect(dialog.getByText("src/a.txt")).toBeVisible();
    await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
    expect(await readFile(join(root, "src/a.txt"), "utf8")).toBe("old\n");
    await dropFiles(page, row("src"), [{ name: "a.txt", text: "new\n" }]);
    await dialog.getByRole("button", { name: "Replace", exact: true }).click();
    await expect.poll(() => readFile(join(root, "src/a.txt"), "utf8")).toBe("new\n");
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});

test("files dropped on a remote machine's Tree are copied to that machine", async ({ page, agenttik }) => {
  const other = await startServer();
  const root = await mkdtemp(join(tmpdir(), "agenttik-drop-remote-"));
  try {
    await writeFile(join(root, "readme.txt"), "remote\n");
    // Only the remote knows this project, so a file landing in it went there.
    await page.request.post(`${other.url}/api/projects`, { data: { path: root } });
    const remote = (await (await page.request.post(`${agenttik.url}/api/remotes`, { data: { address: other.url } })).json()).remote;
    await page.goto(`${agenttik.url}/?remote=${remote.id}`);
    await openProject(page, root);
    const tree = sidebar(page);
    await tree.getByRole("tab", { name: "Tree", exact: true }).click();
    const row = (name) => tree.getByRole("button", { name, exact: true });

    await dropFiles(page, row("readme.txt"), [{ name: "far.bin", size: BIG }]);
    await expect(row("far.bin")).toBeVisible();
    expect((await readFile(join(root, "far.bin"))).length).toBe(BIG);
    const local = await (await page.request.get(`${agenttik.url}/api/projects`)).json();
    expect(JSON.stringify(local)).not.toContain(root);
  } finally {
    await other.stop();
    await rm(root, { recursive: true, force: true });
  }
});
