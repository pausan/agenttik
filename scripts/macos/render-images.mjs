// Renders the committed macOS images: the 1024px app icon from the web logo,
// and the disk image background at 1x and 2x. Run after changing either:
//   npm ci --prefix e2e && node scripts/macos/render-images.mjs
import { readFileSync } from 'node:fs'
import { createRequire } from 'node:module'

const here = new URL('.', import.meta.url)
const { chromium } = createRequire(new URL('../../e2e/package.json', import.meta.url))('@playwright/test')
const logo = readFileSync(new URL('../../web/public/agenttik.svg', import.meta.url), 'utf8')

// macOS icons draw an 824px plate centred in a 1024px canvas, with a soft
// shadow, so the icon matches the size of the system's own.
const icon = `<body style="margin:0;width:1024px;height:1024px;display:grid;place-items:center">
  <div style="width:824px;height:824px;filter:drop-shadow(0 10px 14px rgb(0 0 0 / .3))">
    ${logo.replace('<svg ', '<svg style="width:100%;height:100%;display:block" ')}
  </div></body>`

// The disk image window is 660x400 points: the app on the left, Applications
// on the right, and a chevron between them (see make-ds-store.py).
const background = `<body style="margin:0"><svg width="660" height="400" viewBox="0 0 660 400">
  <rect width="660" height="400" fill="#f0f0f5"/>
  <path d="M322 162 341 181 322 200" fill="none" stroke="#2c2c34" stroke-width="6"
        stroke-linecap="round" stroke-linejoin="round"/>
</svg></body>`

const browser = await chromium.launch()
for (const [html, width, height, scale, file] of [
  [icon, 1024, 1024, 1, 'icon.png'],
  [background, 660, 400, 1, 'dmg-background.png'],
  [background, 660, 400, 2, 'dmg-background@2x.png'],
]) {
  const page = await browser.newPage({ viewport: { width, height }, deviceScaleFactor: scale })
  await page.setContent(html)
  await page.screenshot({ path: new URL(file, here).pathname, omitBackground: true })
  await page.close()
}
await browser.close()
