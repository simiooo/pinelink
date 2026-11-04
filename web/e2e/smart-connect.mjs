import { chromium } from 'playwright-core'
import { mkdir, writeFile } from 'node:fs/promises'
import assert from 'node:assert/strict'
import { startGuestWebServer } from './guest-server.mjs'

const server = await startGuestWebServer(), browser = await chromium.launch({ ...(process.env.CHROMIUM_PATH ? { executablePath: process.env.CHROMIUM_PATH } : {}), headless: true, args: ['--no-sandbox', '--disable-dev-shm-usage'] })
const errors = [], contexts = [], output = 'e2e-output/smart-connect'
try {
  await mkdir(output, { recursive: true })
  for (const width of [320, 390, 1280]) {
    const context = await browser.newContext({ locale: 'en-US', viewport: { width, height: 844 } }); contexts.push(context)
    const page = await context.newPage(); page.on('pageerror', error => errors.push(error.message))
    await page.goto('http://localhost:5184'); await page.locator('.workspace-add-device').click()
    await page.getByTestId('connection-code').waitFor(); await page.waitForFunction(() => /[0-9A-Z]{4} [0-9A-Z]{4}/.test(document.querySelector('[data-testid=connection-code]')?.textContent ?? ''))
    assert.equal(await page.getByRole('tab', { name: 'Online', exact: true }).count(), 0); assert.equal(await page.getByRole('tab', { name: 'Local', exact: true }).count(), 0)
    assert.equal(await page.getByRole('tab', { name: 'Show code', exact: true }).count(), 1); assert.equal(await page.getByRole('tab', { name: 'Enter code', exact: true }).count(), 1)
    await page.screenshot({ path: `${output}/online-${width}.png` })
    await page.getByRole('tab', { name: 'Enter code', exact: true }).click()
    await page.getByLabel('Connection code', { exact: true }).fill('https://foreign.example/connect#AbCdEf0123456789_-1234')
    await page.getByRole('button', { name: 'Connect', exact: true }).click(); await page.getByRole('alert').filter({ hasText: 'Check the code' }).waitFor()
    assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth))
    if (width === 320) await page.screenshot({ path: `${output}/input-320.png` })
    if (width === 390) { await page.getByRole('button', { name: 'Scan code', exact: true }).click(); await page.getByRole('alert').filter({ hasText: 'Camera unavailable' }).waitFor(); await page.locator('.connection-input').waitFor(); assert.equal(await page.getByRole('textbox', { name: 'Connection code', exact: true }).count(), 1) }
    await context.close()
  }
  assert.deepEqual(errors, []); await writeFile(`${output}/result.json`, JSON.stringify({ passed: true, checks: ['single show/enter tabs', 'online invitation', '320/390/desktop layout', 'foreign links rejected', 'camera failure returns to input'], errors }, null, 2)); console.log('smart connection UI: passed')
} catch (error) { for (const [index, context] of contexts.entries()) for (const page of context.pages()) { await page.screenshot({ path: `${output}/failure-${index}.png` }).catch(() => {}); await writeFile(`${output}/failure-${index}.txt`, await page.locator('body').innerHTML()).catch(() => {}) }; throw error
} finally { await browser.close(); server.closeAllConnections(); await new Promise(resolve => server.close(resolve)) }
