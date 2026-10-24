import { addProject, expect, newTask, openSettings, test } from "../fixtures.js";

test.use({ hasTouch: true });

async function fakeMicrophone(page) {
  await page.addInitScript(() => {
    window.microphoneStops = 0;
    Object.defineProperty(navigator, "mediaDevices", { configurable: true, value: {
      getUserMedia: async () => ({ getTracks: () => [{ stop() { window.microphoneStops++; } }] }),
    } });
    window.AudioContext = class {
      sampleRate = 48000;
      state = "running";
      resume() { return Promise.resolve(); }
      close() { this.state = "closed"; return Promise.resolve(); }
      createMediaStreamSource() { return { connect() {}, disconnect() {} }; }
      createGain() { return { gain: { value: 1 }, connect() {}, disconnect() {} }; }
      createScriptProcessor() {
        return { connect() { setTimeout(() => this.onaudioprocess?.({ inputBuffer: { getChannelData: () => new Float32Array(4096).fill(0.1) } }), 0); }, disconnect() {} };
      }
    };
  });
  await page.reload();
}

test("first dictation shows download spinner, shares both microphones and appends one transcript", async ({ page }) => {
  await fakeMicrophone(page);
  await addProject(page); await newTask(page);
  const draft = page.getByPlaceholder("Ask the agent…");
  await draft.fill("Please review:");
  let releaseDownload;
  const download = new Promise((resolve) => { releaseDownload = resolve; });
  await page.route("**/api/speech/prepare", async (route) => {
    expect(route.request().postDataJSON()).toEqual({ model: "whistle" });
    await download;
    await route.fulfill({ status: 204 });
  });
  let transcriptions = 0;
  await page.route("**/api/speech/transcribe", async (route) => {
    transcriptions++;
    const body = route.request().postDataJSON();
    expect(body.model).toBe("whistle");
    expect(Buffer.from(body.pcm, "base64").length).toBeGreaterThan(100);
    await route.fulfill({ json: { text: "El código de autenticación.", warning: "" } });
  });
  const center = page.locator("[data-transcript]");
  await center.getByRole("button", { name: "Dictate a prompt", exact: true }).click();
  const downloading = center.getByRole("button", { name: "Downloading speech model…", exact: true });
  await expect(downloading).toHaveAttribute("aria-busy", "true");
  await expect(downloading.locator(".animate-spin")).toBeVisible();
  releaseDownload();
  await expect(center.getByRole("button", { name: "Stop recording", exact: true })).toHaveAttribute("aria-pressed", "true");
  await page.locator(".prompt-bar").getByRole("button", { name: "Stop recording", exact: true }).click();
  await expect(draft).toHaveValue("Please review:\nEl código de autenticación.");
  expect(transcriptions).toBe(1);
  expect(await page.evaluate(() => window.microphoneStops)).toBe(1);
  await expect(page.locator("[data-transcript] .transcript-row")).toHaveCount(0);
});

test("General speech preferences persist without downloading a model", async ({ page }) => {
  let downloads = 0;
  await page.route("**/api/speech/prepare", (route) => { downloads++; return route.fulfill({ status: 204 }); });
  await openSettings(page);
  const model = page.getByRole("combobox", { name: "Speech recognition model", exact: true });
  await expect(model).toContainText("Whistle");
  await model.click();
  await page.getByRole("option", { name: /Whisper Small/ }).click();
  await expect(model).toContainText("Whisper Small");
  await page.getByRole("switch", { name: "Correct transcription", exact: true }).click();
  await expect(page.getByText("Correction model", { exact: true })).toBeVisible();
  await page.reload(); await openSettings(page);
  await expect(model).toContainText("Whisper Small");
  await expect(page.getByRole("switch", { name: "Correct transcription", exact: true })).toBeChecked();
  expect(downloads).toBe(0);
});

test("empty conversation microphone is a large reachable phone target", async ({ page }) => {
  await fakeMicrophone(page); await addProject(page); await newTask(page);
  await page.setViewportSize({ width: 390, height: 844 });
  await page.route("**/api/speech/prepare", (route) => route.fulfill({ status: 204 }));
  await page.route("**/api/speech/transcribe", (route) => route.fulfill({ json: { text: "Hola desde el móvil." } }));
  const center = page.locator("[data-transcript]");
  const mic = center.getByRole("button", { name: "Dictate a prompt", exact: true });
  await expect(mic).toBeVisible();
  const box = await mic.boundingBox();
  expect(box.width).toBeGreaterThanOrEqual(100);
  expect(box.x).toBeGreaterThanOrEqual(0);
  expect(box.x + box.width).toBeLessThanOrEqual(390);
  expect(box.y + box.height).toBeLessThanOrEqual(844);
  await mic.tap();
  await center.getByRole("button", { name: "Stop recording", exact: true }).tap();
  await expect(page.getByPlaceholder("Ask the agent…")).toHaveValue("Hola desde el móvil.");
});
