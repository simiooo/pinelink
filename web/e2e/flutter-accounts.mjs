import { chromium } from 'playwright-core'
import { createServer, request } from 'node:http'
import { readFile, mkdir } from 'node:fs/promises'
import { resolve, extname } from 'node:path'
import assert from 'node:assert/strict'
const root = resolve('../mobile/build/web'), origin = 'http://localhost:5184', api = 'http://127.0.0.1:8084'
const out = resolve('e2e-output/flutter-accounts'); await mkdir(out, { recursive: true })
const server = createServer(async (req, res) => {
  res.setHeader('Cache-Control', 'no-store'); res.setHeader('Cross-Origin-Opener-Policy', 'same-origin'); res.setHeader('Cross-Origin-Embedder-Policy', 'require-corp')
  const url = new URL(req.url, origin)
  if (url.pathname.startsWith('/api/')) { const target = request(api + url.pathname.slice(4) + url.search, { method: req.method, headers: req.headers }, reply => { res.writeHead(reply.statusCode, reply.headers); reply.pipe(res) }); target.on('error', () => { res.statusCode = 503; res.end() }); req.pipe(target); return }
  try { const path = resolve(root, '.' + decodeURIComponent(url.pathname)); if (!path.startsWith(root + '/') && path !== root) throw Error('path'); const file = extname(path) ? path : resolve(root, 'index.html'); res.setHeader('Content-Type', ({ '.js': 'text/javascript', '.html': 'text/html', '.json': 'application/json', '.wasm': 'application/wasm', '.png': 'image/png' })[extname(file)] ?? 'application/octet-stream'); res.end(await readFile(file)) } catch { res.statusCode = 404; res.end() }
})
await new Promise(r => server.listen(5184, '0.0.0.0', r))
for (let i = 0; i < 100; i++) { try { if ((await fetch(api + '/healthz')).ok) break } catch {} await new Promise(r => setTimeout(r, 150)) }
const browser = await chromium.launch({ executablePath: '/usr/bin/chromium', headless: true, args: ['--no-sandbox', '--disable-dev-shm-usage'] })
const context = await browser.newContext({ locale: 'en-US', viewport: { width: 390, height: 844 } }), page = await context.newPage(), errors = []
page.on('pageerror', e => errors.push(e.message))
const cookieSession = () => context.request.get(origin + '/api/v1/auth/session', { headers: { 'X-Pinelink-Surface': 'app' } })
async function fillField(name, value) { await page.getByRole('textbox', { name, exact: true }).click(); await page.evaluate(() => new Promise(r => requestAnimationFrame(() => requestAnimationFrame(r)))); await page.keyboard.press('ControlOrMeta+A'); await page.keyboard.insertText(value) }
async function until(work) { for (let i = 0; i < 120; i++) { const result = await work(); if (result) return result; await new Promise(r => setTimeout(r, 200)) } throw Error('condition timeout') }
try {
  await page.goto(origin)
  await page.locator('flt-semantics-placeholder').waitFor({ state: 'attached', timeout: 60000 }); await page.locator('flt-semantics-placeholder').evaluate(el => el.click())
  await page.getByRole('button', { name: 'Account', exact: true }).click({ timeout: 60000 })
  await page.getByRole('button', { name: 'Create account', exact: true }).click()
  const email = 'flutter-' + Date.now() + '@example.com', password = 'the amber lake contains 74 quiet lanterns'
  await fillField('Email', email)
  await fillField('Password', password)
  await page.getByRole('button', { name: 'Create account', exact: true }).click()
  const mail = await until(async () => { const value = await fetch('http://127.0.0.1:8085/mail').then(r => r.json()); return value?.to?.[0] === email ? value : undefined })
  const code = mail.text.match(/Code: ([0-9]{8})/)?.[1]; assert.ok(code)
  await fillField('8-digit code', code)
  await page.getByRole('button', { name: 'Continue', exact: true }).click()
  const first = await until(async () => { const response = await cookieSession(); return response.ok() ? response.json() : undefined })
  assert.ok(first.device_id.startsWith('flutter-'))
  assert.equal((await context.request.get(origin + '/api/v1/auth/session')).status(), 401)
  const cookie = (await context.cookies()).find(c => c.name === '__Host-pinelink_flutter_session'); assert.ok(cookie?.secure && cookie.httpOnly)
  await page.getByRole('button', { name: 'Account', exact: true }).click()
  await page.screenshot({ path: out + '/account-mobile.png' })
  await page.getByRole('button', { name: 'Sign out', exact: true }).click()
  await until(async () => (await cookieSession()).status() === 401)
  await page.getByRole('button', { name: 'Sign in', exact: true }).waitFor()
  await fillField('Email', email)
  await fillField('Password', password)
  await page.getByRole('button', { name: 'Sign in', exact: true }).click()
  const second = await until(async () => { const response = await cookieSession(); return response.ok() ? response.json() : undefined })
  assert.equal(second.account.id, first.account.id); assert.equal(second.device_id, first.device_id)
  assert.deepEqual(errors, [])
  console.log(JSON.stringify({ surface: 'Flutter Web', checks: ['real Dart proof/PKCE and email signup', 'actual enrollment and isolated HttpOnly cookie', 'sign-out revokes session', 'password login preserves account device keys'], passed: 4, pageErrors: 0 }))
} catch (error) { await page.screenshot({ path: out + '/failure.png' }).catch(() => {}); console.error(JSON.stringify({ errors, accessibility: await page.locator('body').ariaSnapshot() })); throw error }
finally { await browser.close(); await new Promise(r => server.close(r)) }
