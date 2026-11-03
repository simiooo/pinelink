import { chromium } from 'playwright-core'
import { spawn } from 'node:child_process'
import { mkdir, writeFile } from 'node:fs/promises'
import assert from 'node:assert/strict'

const out = process.env.MOTION_EVIDENCE || '../.artifacts/motion-runtime-browser'
await mkdir(out, { recursive: true })
const server = spawn(process.execPath, ['node_modules/vite/bin/vite.js', '--host', '127.0.0.1', '--port', '5189'], { stdio: ['ignore', 'ignore', 'pipe'] })
const serverErrors = []; server.stderr.on('data', data => serverErrors.push(String(data)))
const base = 'http://127.0.0.1:5189/test/motion.html', checks = [], errors = []
const measurements = {}
let browser
async function check(name, action) { await action(); checks.push({ name, passed: true }) }
try {
  const deadline = Date.now() + 30000
  while (true) { try { if ((await fetch(base)).ok) break } catch {} if (Date.now() > deadline) throw Error(serverErrors.join('')); await new Promise(resolve => setTimeout(resolve, 100)) }
  browser = await chromium.launch({ executablePath: process.env.CHROMIUM_PATH || '/usr/bin/chromium', headless: true, args: ['--no-sandbox', '--disable-dev-shm-usage'] })
  const context = await browser.newContext({ viewport: { width: 1280, height: 1000 }, reducedMotion: 'no-preference' })
  const page = await context.newPage(); page.on('pageerror', error => errors.push(error.message))
  await page.goto(base); await page.waitForFunction(() => window.motionFixture)
  const fixture = callback => page.evaluate(callback)
  await check('StrictMode setup does not dispatch an action', async () => assert.equal(await fixture(() => window.motionFixture.actions()), 0))
  await check('button press reverses from current pose', async () => {
    const face = page.locator('#open-surface .action-face'); await page.locator('#open-surface').hover(); await page.mouse.down(); await page.waitForTimeout(36)
    const down = await face.evaluate(el => window.motionFixture.owner.current(el).scale); assert(down < 1 && down > .96)
    await page.mouse.move(2, 2); await page.mouse.up(); await page.waitForTimeout(400)
    assert(Math.abs(await face.evaluate(el => window.motionFixture.owner.current(el).scale) - 1) < .001)
  })
  await check('two clicks while an explicit promise is pending dispatch once', async () => {
    await fixture(() => { const button = document.querySelector('#async-action'); button.click(); button.click() })
    assert.equal(await fixture(() => window.motionFixture.actions()), 1); assert.equal(await page.locator('#async-action').getAttribute('aria-busy'), 'true')
    await fixture(() => window.motionFixture.release()); await page.waitForFunction(() => !document.querySelector('#async-action').disabled)
  })
  const sheen = page.locator('.transfer-curve > path').last()
  await check('real running transfer has a clipped 1.8 second sheen', async () => {
    assert.equal(await sheen.getAttribute('mask') !== null, true)
    assert.equal(await sheen.evaluate(el => el.getAnimations()[0]?.effect.getTiming().duration), 1800)
    assert.equal(await page.getByRole('progressbar', { name: 'Confirmed bytes' }).getAttribute('aria-valuenow'), '42')
  })
  await check('pause and late acknowledgement retain a static confirmed boundary', async () => {
    await fixture(() => { window.motionFixture.running(false); window.motionFixture.value(.66) }); await page.waitForTimeout(50)
    assert.equal(await sheen.evaluate(el => el.getAnimations().length), 0); assert.equal(await sheen.evaluate(el => getComputedStyle(el).opacity), '0')
    assert.equal(await page.getByRole('progressbar', { name: 'Confirmed bytes' }).getAttribute('aria-valuenow'), '66')
  })
  await check('live reduced motion stops flow and full motion restores visible flow', async () => {
    await fixture(() => window.motionFixture.running(true))
    await page.emulateMedia({ reducedMotion: 'reduce' }); await page.waitForTimeout(30); assert.equal(await sheen.evaluate(el => el.getAnimations().length), 0)
    await page.emulateMedia({ reducedMotion: 'no-preference' }); await page.waitForTimeout(50)
    assert.equal(await sheen.evaluate(el => getComputedStyle(el).opacity), '1'); assert.equal(await sheen.evaluate(el => el.getAnimations().length), 1)
  })
  await check('100 percent bytes wait for a separate receipt', async () => {
    await fixture(() => window.motionFixture.value(1)); await page.waitForTimeout(160)
    assert.equal(await sheen.evaluate(el => el.getAnimations().length), 0); assert.equal(await page.locator('.graph-receipt').count(), 0)
    await fixture(() => window.motionFixture.complete(true)); await page.waitForTimeout(30)
    assert.equal(await page.locator('.graph-local .graph-receipt').count(), 1)
    await fixture(() => { window.motionFixture.complete(false); window.motionFixture.value(.42) })
  })
  await check('offscreen flow pauses and resumes on intersection', async () => {
    await page.locator('input[aria-label="gamma"]').scrollIntoViewIfNeeded(); await page.evaluate(() => window.scrollTo(0, document.body.scrollHeight));
    // A small viewport guarantees the graph is outside the view even on tall CI screens.
    await page.setViewportSize({ width: 1280, height: 300 }); await page.evaluate(() => window.scrollTo(0, document.body.scrollHeight)); await page.waitForTimeout(100)
    assert.equal(await sheen.evaluate(el => el.getAnimations()[0]?.playState), 'paused')
    await page.setViewportSize({ width: 1280, height: 1000 }); await page.evaluate(() => window.scrollTo(0, 0)); await page.waitForTimeout(100)
    assert.equal(await sheen.evaluate(el => el.getAnimations()[0]?.playState), 'running')
  })
  await check('Radix dialog traps focus, survives a reversal, and restores trigger focus', async () => {
    await page.locator('#open-surface').click(); await page.getByRole('dialog').waitFor(); await page.getByRole('textbox', { name: 'Draft' }).fill('Retained draft')
    await fixture(() => window.motionFixture.open(false)); await page.waitForTimeout(32); await fixture(() => window.motionFixture.open(true)); await page.waitForTimeout(180)
    assert.equal(await page.getByRole('textbox', { name: 'Draft' }).inputValue(), 'Retained draft')
    await page.keyboard.press('Tab'); await page.keyboard.press('Tab'); assert(await page.getByRole('dialog').evaluate(el => el.contains(document.activeElement)))
    await page.keyboard.press('Escape'); await page.waitForTimeout(180); assert.equal(await page.getByRole('dialog').count(), 0)
    assert.equal(await page.locator('#open-surface').evaluate(el => document.activeElement === el), true, await page.evaluate(() => document.activeElement.outerHTML.slice(0, 200)))
    assert.equal(await page.evaluate(() => document.body.style.pointerEvents), '')
  })
  await check('Radix menu supports arrows and escape', async () => {
    await page.getByRole('button', { name: 'More', exact: true }).focus(); await page.keyboard.press('ArrowDown'); await page.getByRole('menu').waitFor()
    await page.waitForFunction(() => document.activeElement?.getAttribute('role') === 'menuitem')
    await page.keyboard.press('Home'); await page.waitForFunction(() => document.activeElement?.textContent === 'History')
    await page.keyboard.press('ArrowDown'); await page.waitForFunction(() => document.activeElement?.textContent === 'Settings')
    await page.keyboard.press('Escape'); await page.waitForTimeout(150)
  })
  await check('tooltip has keyboard and hover access', async () => {
    await page.getByRole('button', { name: 'Info', exact: true }).focus(); await page.getByRole('tooltip').waitFor(); assert.equal(await page.getByRole('tooltip').textContent(), 'More information')
    await page.locator('#open-surface').focus(); await page.waitForTimeout(100)
  })
  await check('keyed filter preserves the surviving editable DOM node', async () => {
    await page.getByRole('textbox', { name: 'alpha', exact: true }).fill('Keep edit')
    await fixture(() => window.motionFixture.rows(['gamma', 'alpha'])); await page.waitForTimeout(220)
    assert.equal(await page.getByRole('textbox', { name: 'alpha', exact: true }).inputValue(), 'Keep edit')
  })
  await check('large list remains bounded under four times CPU throttling', async () => {
    const cdp = await context.newCDPSession(page)
    try {
      await cdp.send('Emulation.setCPUThrottlingRate', { rate: 4 })
      const started = Date.now()
      await fixture(() => window.motionFixture.rows(Array.from({ length: 1000 }, (_, i) => `file-${i}`)))
      await page.waitForFunction(() => document.querySelectorAll('[data-motion-key]').length === 1000)
      measurements.list1000_render_ms_at_4x_cpu = Date.now() - started
      const diagnostic = await fixture(() => window.motionFixture.owner.diagnostics())
      assert(diagnostic.animations <= 6); assert(diagnostic.poses < 30)
      await fixture(() => window.motionFixture.rows(['gamma', 'alpha'])); await page.waitForTimeout(250)
      assert((await fixture(() => window.motionFixture.owner.diagnostics())).poses < 30)
    } finally { await cdp.send('Emulation.setCPUThrottlingRate', { rate: 1 }); await cdp.detach() }
  })
  await check('nested dragleave does not dismiss a valid file target; escape clears it', async () => {
    await page.locator('.dropzone').evaluate(el => { const dataTransfer = new DataTransfer(); dataTransfer.items.add(new File(['x'], 'test.txt')); window.fileDrag = dataTransfer; el.dispatchEvent(new DragEvent('dragenter', { bubbles: true, dataTransfer })); el.querySelector('#drop-child').dispatchEvent(new DragEvent('dragenter', { bubbles: true, dataTransfer })); el.querySelector('#drop-child').dispatchEvent(new DragEvent('dragleave', { bubbles: true, dataTransfer })) })
    assert.equal(await page.locator('.dropzone').getAttribute('data-drop-ready'), 'true'); await page.keyboard.press('Escape'); assert.equal(await page.locator('.dropzone').getAttribute('data-drop-ready'), 'false')
    await page.locator('.dropzone').evaluate(el => el.dispatchEvent(new DragEvent('drop', { bubbles: true, cancelable: true, dataTransfer: window.fileDrag })))
    assert.equal(await fixture(() => window.motionFixture.drops()), 1)
    await page.locator('.dropzone').evaluate(el => { const dataTransfer = new DataTransfer(); dataTransfer.setData('text/plain', 'plain'); el.dispatchEvent(new DragEvent('drop', { bubbles: true, cancelable: true, dataTransfer })) }); assert.equal(await fixture(() => window.motionFixture.drops()), 1)
  })
  await page.screenshot({ path: `${out}/desktop.png`, fullPage: true })
  await page.setViewportSize({ width: 390, height: 844 }); await page.evaluate(() => window.scrollTo(0, 0))
  await check('mobile sheet opens from below, cancels a short drag, and dismisses a long drag', async () => {
    await page.locator('#open-surface').click(); const grip = page.locator('.sheet-drag-target'); await grip.waitFor(); await page.waitForTimeout(250)
    const box = await grip.boundingBox(); await page.mouse.move(box.x + box.width / 2, box.y + 24); await page.mouse.down(); await page.mouse.move(box.x + box.width / 2, box.y + 36, { steps: 8 }); await page.waitForTimeout(100); await page.mouse.up(); await page.waitForTimeout(400)
    assert.equal(await page.getByRole('dialog').count(), 1); await page.screenshot({ path: `${out}/mobile-sheet.png`, fullPage: true })
    const next = await grip.boundingBox(); await page.mouse.move(next.x + next.width / 2, next.y + 24); await page.mouse.down(); await page.mouse.move(next.x + next.width / 2, next.y + 124, { steps: 8 }); await page.mouse.up(); await page.waitForTimeout(220)
    assert.equal(await page.getByRole('dialog').count(), 0); assert.equal(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth), false)
  })
  await check('unmount cancels owned animation records and late asynchronous completion safely', async () => {
    await page.locator('#async-action').click(); await fixture(() => window.unmountMotionFixture()); await fixture(() => window.motionFixture.release()); await page.waitForTimeout(80)
    const diagnostic = await fixture(() => window.motionFixture.owner.diagnostics()); assert.deepEqual(diagnostic, { animations: 0, poses: 0, styles: 0, loops: 0, reduced: false, attached: 0 })
  })
  assert.deepEqual(errors, [])
  const report = { passed: true, browser: browser.version(), checks, measurements, javascript_errors: errors, fixture_only: true, production_route: false }
  await writeFile(`${out}/report.json`, JSON.stringify(report, null, 2) + '\n'); console.log(JSON.stringify(report))
} catch (error) {
  await writeFile(`${out}/failure.json`, JSON.stringify({ passed: false, checks, error: String(error), errors, serverErrors }, null, 2)); throw error
} finally { await browser?.close(); server.kill('SIGTERM') }
