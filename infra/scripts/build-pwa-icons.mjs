// Rasterize the already approved v20 vector mark; no new brand drawing.
import { chromium } from '../../web/node_modules/playwright-core/index.mjs'
import { readFile, writeFile } from 'node:fs/promises'
import { resolve, join } from 'node:path'

const root = resolve(import.meta.dirname, '../..')
const source = await readFile(join(root, 'designs/20-direct-relay/18-desktop-direct-running.svg'), 'utf8')
const mark = source.match(/<g id="Pinelink mark"[^>]*>(<path[^>]+\/>)/)?.[1]
if (!mark) throw new Error('Approved v20 mark not found')
const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="512" height="512" viewBox="0 0 32 32"><rect width="32" height="32" fill="#FFFFFF"/><g transform="translate(4 3)">${mark}</g></svg>\n`
const output = join(root, 'web/public')
await writeFile(join(output, 'app-icon.svg'), svg)
const browser = await chromium.launch({ headless: true, ...(process.env.CHROMIUM_EXECUTABLE ? { executablePath: process.env.CHROMIUM_EXECUTABLE } : process.platform === 'win32' ? { executablePath: 'C:/Program Files/Google/Chrome/Application/chrome.exe' } : {}) })
try {
  for (const [name, size] of [['pwa-192.png', 192], ['pwa-512.png', 512], ['apple-touch-icon.png', 180], ['favicon.png', 64]]) {
    const page = await browser.newPage({ viewport: { width: size, height: size }, deviceScaleFactor: 1 })
    await page.setContent(`<html><body style="margin:0">${svg.replace('width="512" height="512"', `width="${size}" height="${size}"`)}</body></html>`)
    await page.screenshot({ path: join(output, name), animations: 'disabled' })
    await page.close()
  }
} finally { await browser.close() }
