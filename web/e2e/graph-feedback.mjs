// Run against Vite in an isolated browser: PINELINK_TEST_ORIGIN=http://127.0.0.1:5173 node e2e/graph-feedback.mjs
import { chromium } from 'playwright-core'
import assert from 'node:assert/strict'
const origin = process.env.PINELINK_TEST_ORIGIN || 'http://127.0.0.1:5173'
const browser = await chromium.launch({ headless: true, executablePath: process.env.CHROMIUM_EXECUTABLE || '/usr/bin/chromium', args: ['--no-sandbox'] })
try {
 for (const width of [320, 390, 768, 1280]) {
  const page = await browser.newPage({ viewport: { width, height: 844 } })
  const fixture = origin + '/e2e/fixtures/graph-feedback.html'
  await page.goto(fixture)
  const peer = page.locator('.guest-peer .guest-node'); await peer.waitFor(); await page.waitForTimeout(300)
  const before = await peer.boundingBox(); await peer.click(); const after = await peer.boundingBox()
  assert.deepEqual([after.x, after.y, after.width, after.height], [before.x, before.y, before.width, before.height])
  const animation = await page.evaluate(() => {
   const rect = document.querySelector('.guest-peer .guest-node').getBoundingClientRect()
   const center = r => [r.x + r.width / 2, r.y + r.height / 2]
   const actions = [...document.querySelectorAll('.guest-node-action')], animations = actions.map(a => a.getAnimations()[0])
   animations.forEach(a => { a.pause(); a.currentTime = 0 }); const first = actions.map(a => center(a.getBoundingClientRect()))
   animations.forEach(a => a.currentTime = 200); const last = actions.map(a => center(a.getBoundingClientRect()))
   return { node: center(rect), first, last }
  })
  assert.equal(animation.first.length, 4)
  for (const [x, y] of animation.first) assert.ok(Math.abs(x - animation.node[0]) < 1 && Math.abs(y - animation.node[1]) < 1)
  assert.ok(animation.last[0][0] < animation.node[0] && animation.last[1][0] > animation.node[0] && animation.last[2][0] < animation.node[0] && animation.last[3][0] > animation.node[0])
  for (const route of ['direct', 'relay']) {
   await page.goto(fixture + '?' + route); await page.locator('.guest-curve').first().waitFor(); await page.waitForTimeout(300)
   const geometry = await page.evaluate(() => {
    const svg = document.querySelector('.guest-curve[data-ledger=delivery]'), path = svg.querySelector('path[stroke="#0866FF"]')
    const point = path.getPointAtLength(path.getTotalLength()), p = new DOMPoint(point.x, point.y).matrixTransform(path.getScreenCTM())
    const rect = document.querySelector('.guest-peer .guest-node').getBoundingClientRect()
    return { x: p.x, y: p.y, cx: rect.x + rect.width / 2, cy: rect.y + rect.height / 2, blue: svg.dataset.blue, display: getComputedStyle(document.querySelector('.guest-peers')).display }
   })
   assert.ok(Math.abs(geometry.x - geometry.cx) < 1 && Math.abs(geometry.y - geometry.cy) < 1, JSON.stringify(geometry))
   assert.equal(geometry.blue, '1'); assert.equal(geometry.display, 'flex')
  }
  await page.goto(fixture + '?direct'); await page.locator('.guest-curve').waitFor(); await page.waitForTimeout(300)
  const points = await page.evaluate(() => {
   const p = document.querySelector('.guest-curve path[stroke="#0866FF"]'), nodes = [...document.querySelectorAll('.guest-node')].map(e => e.getBoundingClientRect()), out = []
   for (let i = 1; i < 100; i++) { const z = p.getPointAtLength(p.getTotalLength() * i / 100), q = new DOMPoint(z.x, z.y).matrixTransform(p.getScreenCTM()); if (!nodes.some(r => q.x >= r.left - 3 && q.x <= r.right + 3 && q.y >= r.top - 3 && q.y <= r.bottom + 3)) out.push([q.x, q.y]) }
   return out
  })
  const screenshot = await page.screenshot()
  const missing = await page.evaluate(async ({ data, points }) => {
   const im = new Image(); im.src = 'data:image/png;base64,' + data; await im.decode(); const canvas = document.createElement('canvas'); canvas.width = im.width; canvas.height = im.height
   const ctx = canvas.getContext('2d'); ctx.drawImage(im, 0, 0); const pixels = ctx.getImageData(0, 0, canvas.width, canvas.height).data
   return points.filter(([x, y]) => { for (let dy = -2; dy <= 2; dy++) for (let dx = -2; dx <= 2; dx++) { const i = (Math.round(y + dy) * canvas.width + Math.round(x + dx)) * 4; if (pixels[i] < 90 && pixels[i + 1] < 160 && pixels[i + 2] > 190) return false } return true })
  }, { data: screenshot.toString('base64'), points })
  assert.ok(points.length > 50); assert.deepEqual(missing, []); await page.close()
  console.log(width + 'px: stable node, radial actions, aligned routes and complete blue curve')
 }
 const page = await browser.newPage({ viewport: { width: 390, height: 844 }, reducedMotion: 'reduce' }); await page.goto(origin + '/e2e/fixtures/graph-feedback.html'); await page.locator('.guest-peer .guest-node').click(); assert.equal(await page.locator('.guest-node-action').first().evaluate(e => e.getAnimations().length), 0); await page.close()
 console.log('Reduced motion: no action entrance animation')
} finally { await browser.close() }
