/* Adds this repository as a project with three tasks left running and one
   idle task to stream into, then prints the idle task's id. */
import { chromium } from "../../e2e/node_modules/playwright/index.mjs";
import { addProject, newTask, openProject, pickModel, sendPrompt } from "../../e2e/fixtures.js";

const sessions = () => fetch(`${process.argv[2]}/api/sessions?window=all&include_done=true`).then((r) => r.json());
const browser = await chromium.launch();
const page = await browser.newPage();
await page.goto(process.argv[2]);
await addProject(page);
await openProject(page);
for (const prompt of ["@wait 3600000", "@wait 3600000", "@wait 3600000", "streamtask"]) {
  await newTask(page);
  await pickModel(page);
  await sendPrompt(page, prompt);
  if (prompt !== "streamtask") await page.getByText("running", { exact: true }).first().waitFor();
}
let stream;
while (!(stream = (await sessions()).find((s) => s.prompt === "streamtask" && s.status === "idle"))) await page.waitForTimeout(100);
await browser.close();
console.log(stream.id);
