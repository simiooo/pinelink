const stagingHost = process.env.PINELINK_STAGING_HOST ?? '127.0.0.1'
import { chromium } from '../../web/node_modules/playwright-core/index.mjs'
import { mkdir, writeFile } from 'node:fs/promises'
import { resolve, join } from 'node:path'
import { randomUUID } from 'node:crypto'

const version = process.argv[2]
if (!/^[a-f0-9]{40}$/.test(version ?? '')) throw new Error('A full source version is required')
const root = resolve(import.meta.dirname, '../..')
const output = join(root, '.artifacts', 'concurrency-smoke', version, randomUUID())
await mkdir(output, { recursive: true })
const context = await chromium.launchPersistentContext(join(root, '.artifacts', 'qa-' + randomUUID().slice(0, 8)), {
  executablePath: process.env.CHROMIUM_EXECUTABLE ?? 'C:/Program Files/Google/Chrome/Application/chrome.exe', headless: true,
  ignoreHTTPSErrors: true, locale: 'en-US', viewport: { width: 1280, height: 844 },
  args: ['--ignore-certificate-errors', '--no-proxy-server'],
})
const base = `https://${stagingHost}:18682`
const requests = { registrations: 0, bindings: 0, unauthorized: 0 }
const devices = new Set(), errors = []
context.on('request', request => {
  if (!request.url().includes('/api/v1/')) return
  const device = request.headers()['x-pinelink-device']
  if (device) devices.add(device)
  if (request.url().endsWith('/devices/register')) requests.registrations++
  if (request.url().endsWith('/replication/bind')) requests.bindings++
})
context.on('response', response => {
  if (response.url().includes('/api/v1/') && [401, 403].includes(response.status())) requests.unauthorized++
})
const a = context.pages()[0], b = await context.newPage()
for (const page of [a, b]) page.on('pageerror', e => errors.push(e.name + ': ' + e.message.slice(0, 180)))
let release, entered
const gate = new Promise(resolve => { release = resolve })
const held = new Promise(resolve => { entered = resolve })
let blocked = false
await a.route('**/api/v1/replication/info?nonce=*', async route => {
  if (!blocked) { blocked = true; entered(); await gate }
  await route.continue()
})
try {
  await a.goto(base + '/history', { waitUntil: 'load' })
  await Promise.race([held, new Promise((_, reject) => setTimeout(() => reject(new Error('sync lock was never acquired')), 10000))])
  await b.goto(base + '/history', { waitUntil: 'load' })
  await b.waitForFunction(async () => (await navigator.locks.query()).pending.some(lock => lock.name?.startsWith('sync:')), null, { timeout: 10000 })
  const lock = await b.evaluate(async () => {
    const locks = await navigator.locks.query()
    return { held: locks.held.filter(l => l.name?.startsWith('sync:')).length, pending: locks.pending.filter(l => l.name?.startsWith('sync:')).length }
  })
  release()
  for (const page of [a, b]) {
    await page.getByText('No matching records', { exact: true }).waitFor({ timeout: 30000 })
    const actual = await page.evaluate(async () => (await (await fetch('/version.json')).json()).version)
    if (actual !== version) throw new Error('deployed version mismatch')
  }
  if (devices.size !== 1 || requests.registrations !== 1 || requests.bindings !== 1 || requests.unauthorized || errors.length) throw new Error('multi-tab initialization invariant failed: ' + JSON.stringify({ devices: devices.size, requests, errors }))
  await a.close()
  await b.reload({ waitUntil: 'load' })
  await b.getByText('No matching records', { exact: true }).waitFor({ timeout: 30000 })
  if (devices.size !== 1 || requests.registrations !== 1 || requests.bindings !== 1 || requests.unauthorized || errors.length) throw new Error('surviving tab rotated or rebound the device')
  await b.screenshot({ path: join(output, 'surviving-tab.png'), fullPage: true })
  const result = { version, completed_at: new Date().toISOString(), public_devices: [...devices], requests, lock,
    checks: { simultaneous_first_open: true, waiting_foreground_lock: true, one_identity: true, one_registration: true, one_binding: true, surviving_tab_reload: true, no_authorization_errors: true }, page_errors: errors }
  await writeFile(join(output, 'smoke.json'), JSON.stringify(result, null, 2))
  console.log(JSON.stringify({ evidence: join(output, 'smoke.json'), ...result }, null, 2))
} finally { release(); await context.close() }
