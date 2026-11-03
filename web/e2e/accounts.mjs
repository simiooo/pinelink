import { chromium } from 'playwright-core'
import { createServer, request } from 'node:http'
import { readFile, mkdir } from 'node:fs/promises'
import { resolve, extname } from 'node:path'
import assert from 'node:assert/strict'

const root = resolve('dist'), out = resolve('e2e-output/accounts')
await mkdir(out, { recursive: true })
const origin = 'http://localhost:5184', base = 'http://127.0.0.1:8084'
const server = createServer(async (req, res) => {
  res.setHeader('Cache-Control', 'no-store'); res.setHeader('Referrer-Policy', 'no-referrer')
  const url = new URL(req.url, origin)
  if (url.pathname.startsWith('/api/') || (url.pathname === '/' && url.searchParams.has('state'))) {
    const path = url.pathname === '/' ? '/v1/auth/google/callback' + url.search : url.pathname.slice(4) + url.search
    const target = request(base + path, { method: req.method, headers: req.headers }, reply => { res.writeHead(reply.statusCode, reply.headers); reply.pipe(res) })
    target.on('error', () => { res.statusCode = 503; res.end() }); req.pipe(target); return
  }
  if (url.pathname === '/service-config.json') {
    try { const info = await fetch(base + '/v1/replication/info?nonce=' + 'A'.repeat(43)).then(r => r.json()); res.setHeader('Content-Type', 'application/json'); res.end(JSON.stringify({ version: 1, api_url: '/api', history_public_key: info.public_key })) } catch { res.statusCode = 503; res.end() }
    return
  }
  try {
    const path = resolve(root, '.' + decodeURIComponent(url.pathname)); if (!path.startsWith(root + '/') && path !== root) throw Error('path')
    const file = extname(path) ? path : resolve(root, 'index.html')
    const mime = { '.js': 'text/javascript', '.html': 'text/html', '.css': 'text/css', '.json': 'application/json', '.png': 'image/png', '.ttf': 'font/ttf', '.webmanifest': 'application/manifest+json' }
    res.setHeader('Content-Type', mime[extname(file)] ?? 'application/octet-stream'); res.end(await readFile(file))
  } catch { res.statusCode = 404; res.end() }
})
await new Promise(r => server.listen(5184, '0.0.0.0', r))
for (let i = 0; i < 100; i++) { try { if ((await fetch(base + '/healthz')).ok) break } catch {} if (i === 99) throw Error('fixture unavailable'); await new Promise(r => setTimeout(r, 200)) }
const browser = await chromium.launch({ executablePath: process.env.CHROMIUM_PATH ?? '/usr/bin/chromium', headless: true, args: ['--no-sandbox', '--disable-dev-shm-usage'] })
const failures = [], results = []
const context = await browser.newContext({ locale: 'en-US', viewport: { width: 1280, height: 900 } }), page = await context.newPage()
page.on('pageerror', e => failures.push(e.message))
const password = 'the violet river carries 47 small stones'
const nextPassword = 'a copper kettle holds 93 quiet clouds'
const email = 'person-' + Date.now() + '@example.com'
async function until(predicate) { for (let i = 0; i < 100; i++) { const value = await predicate(); if (value) return value; await new Promise(r => setTimeout(r, 150)) } throw Error('condition timeout') }
async function mailFor(address) {
  return until(async () => { const mail = await fetch('http://127.0.0.1:8085/mail').then(r => r.json()); if (mail?.to?.[0] !== address) return; const code = mail.text.match(/Code: (\d{8})/)?.[1], link = mail.text.match(/http:\/\/localhost:5184\/a\/[A-Za-z0-9_-]{22}/)?.[0]; return code && link ? { code, link } : undefined })
}
const submit = (p, name) => p.locator('form').getByRole('button', { name, exact: true }).click()
try {
  let mutations = 0; page.on('request', req => { if (req.method() === 'POST' && /\/auth\/requests(?:$|\/)/.test(req.url())) mutations++ })
  await page.goto(origin + '/login'); await page.getByRole('heading', { name: 'Sign in', exact: true }).waitFor()
  await page.getByRole('button', { name: 'Continue with Google', exact: true }).waitFor()
  assert.equal(mutations, 0, 'mount started an authentication mutation')
  assert.equal(await page.getByRole('button', { name: 'Continue with Google', exact: true }).evaluate(el => !!el.closest('form')), false)
  await page.screenshot({ path: out + '/login-desktop.png' }); results.push('independent Google button; no mount mutation')
  await page.goto(origin + '/register'); await page.getByLabel('Email', { exact: true }).fill(email); await page.getByLabel('New password', { exact: true }).fill(password)
  await submit(page, 'Create account'); await page.waitForURL('**/verify')
  const mail = await mailFor(email); await page.getByText('Check your email', { exact: true }).waitFor()
  const parsed = new URL(mail.link); const scan = await context.request.get(mail.link); assert.equal(scan.status(), 200)
  await page.getByLabel('8-digit code', { exact: true }).fill(mail.code); await submit(page, 'Continue'); await page.waitForURL(origin + '/', { timeout: 15000 })
  const initial = await context.request.get(origin + '/api/v1/auth/session').then(r => r.json()); assert.ok(initial.account.id.startsWith('acct-'))
  const cookie = (await context.cookies()).find(c => c.name === '__Host-pinelink_session'); assert.ok(cookie?.httpOnly && cookie.secure && cookie.sameSite === 'Lax')
  assert.equal(await page.evaluate(() => document.cookie.includes('pinelink_session')), false)
  await until(async () => (await page.evaluate(() => indexedDB.databases())).filter(d => d.name?.startsWith('pinelink-') && !d.name?.startsWith('pinelink-auth')).length)
  results.push('real password signup, scanner-safe short link, code, HttpOnly session, account enrollment')
  assert.equal(parsed.pathname.length, 25)
  const denied = await context.request.post(origin + '/api/v1/auth/logout', { headers: { Origin: origin }, data: {} }); assert.equal(denied.status(), 403)
  results.push('logout requires CSRF')
  await page.goto(origin + '/account'); await page.getByRole('button', { name: 'Sign out', exact: true }).click(); await page.waitForURL('**/login')
  await page.getByLabel('Email', { exact: true }).fill(email); await page.getByLabel('Password', { exact: true }).fill(password); await submit(page, 'Sign in'); await page.waitForURL(origin + '/')
  const signed = await context.request.get(origin + '/api/v1/auth/session').then(r => r.json()); assert.equal(signed.account.id, initial.account.id); assert.equal(signed.device_id, initial.device_id)
  results.push('sign-out and sign-in preserve service/account device keys')
  await page.goto(origin + '/forgot'); await page.getByLabel('Email', { exact: true }).fill(email); await submit(page, 'Send code'); await page.waitForURL('**/verify')
  const resetMail = await until(async () => { const m = await mailFor(email); return m.code !== mail.code ? m : undefined })
  await page.getByLabel('8-digit code', { exact: true }).fill(resetMail.code); await submit(page, 'Continue'); await page.waitForURL('**/reset')
  await page.getByLabel('New password', { exact: true }).fill(nextPassword); await submit(page, 'Reset password'); await page.getByRole('heading', { name: 'Password updated', exact: true }).waitFor()
  assert.equal((await context.request.get(origin + '/api/v1/auth/session')).status(), 401)
  results.push('password recovery revokes the previous session')
  await page.goto(origin + '/login'); await page.getByLabel('Email', { exact: true }).fill(email); await page.getByLabel('Password', { exact: true }).fill(nextPassword); await submit(page, 'Sign in'); await page.waitForURL(origin + '/')
  const recovered = await context.request.get(origin + '/api/v1/auth/session').then(r => r.json()); assert.equal(recovered.account.id, initial.account.id)
  const otherEmail = 'other-' + Date.now() + '@example.com'; const other = await browser.newContext({ locale: 'en-US', viewport: { width: 390, height: 844 } }), second = await other.newPage()
  await second.goto(origin + '/register'); await second.getByLabel('Email', { exact: true }).fill(otherEmail); await second.getByLabel('New password', { exact: true }).fill(password); await submit(second, 'Create account'); await second.waitForURL('**/verify'); const otherMail = await mailFor(otherEmail)
  const landing = await browser.newContext({ locale: 'en-US', viewport: { width: 390, height: 844 } }), third = await landing.newPage(); await third.goto(otherMail.link); await third.waitForURL('**/verify-link'); await third.getByRole('heading', { name: 'Finish on this device', exact: true }).waitFor()
  assert.equal((await other.request.get(origin + '/api/v1/auth/session')).status(), 401)
  await third.getByLabel('New password', { exact: true }).fill(nextPassword); await submit(third, 'Continue'); await third.waitForURL(origin + '/')
  const otherAccount = await landing.request.get(origin + '/api/v1/auth/session').then(r => r.json()); assert.notEqual(otherAccount.account.id, initial.account.id); assert.equal((await other.request.get(origin + '/api/v1/auth/session')).status(), 401)
  results.push('cross-device link uses a new password and leaves original requester signed out')
  await second.goto(origin + '/login'); await second.getByRole('heading', { name: 'Sign in', exact: true }).waitFor(); await second.getByLabel('Password', { exact: true }).waitFor(); await second.screenshot({ path: out + '/login-mobile.png' }); assert.equal(await second.evaluate(() => document.documentElement.scrollWidth > innerWidth), false)
  await second.setViewportSize({ width: 320, height: 740 }); await second.getByRole('button', { name: 'Continue with Google', exact: true }).waitFor(); assert.equal(await second.evaluate(() => document.documentElement.scrollWidth > innerWidth), false); results.push('390px and 320px mobile layout')
  await other.close(); await landing.close()
  await page.goto(origin + '/tasks'); await page.getByRole('heading', { name: 'Current task', exact: true }).waitFor(); await until(() => page.locator('.empty-state, .state-panel').count())
  await until(() => page.evaluate(async () => !!navigator.serviceWorker.controller || !!(await navigator.serviceWorker.getRegistration())?.active))
  const cachedUrls = await page.evaluate(async () => (await Promise.all((await caches.keys()).map(async key => (await (await caches.open(key)).keys()).map(r => r.url)))).flat())
  assert.ok(!cachedUrls.some(url => /\/api\/|\/a\/|\/auth\//.test(new URL(url).pathname)))
  await context.setOffline(true); await page.reload(); await page.getByRole('heading', { name: 'Current task', exact: true }).waitFor(); await context.setOffline(false)
  results.push('offline local route and auth responses excluded from navigation caches')
  assert.deepEqual(failures, [])
  console.log(JSON.stringify({ passed: results.length, checks: results, pageErrors: failures.length }))
} catch (error) { await page.screenshot({ path: out + '/failure.png' }).catch(() => {}); throw error }
finally { await browser.close(); await new Promise(r => server.close(r)) }
