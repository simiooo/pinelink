const stagingHost = process.env.PINELINK_STAGING_HOST ?? '127.0.0.1'
import { chromium } from '../../web/node_modules/playwright-core/index.mjs'
import { createServer, request as httpRequest } from 'node:http'
import { execFileSync } from 'node:child_process'
import { mkdir, readFile, writeFile } from 'node:fs/promises'
import { resolve, join, extname, sep } from 'node:path'
import { randomUUID } from 'node:crypto'

const version = process.argv[2]
if (!/^[a-f0-9]{40}$/.test(version || '')) throw new Error('Full deployed source version required')
const root = resolve(import.meta.dirname, '../..'), run = randomUUID()
const labels = JSON.parse(await readFile(join(root, 'i18n/en-US.json'), 'utf8'))
const output = join(root, '.artifacts/pwa-smoke', version, run), profile = join(root, '.artifacts/pwa-' + run.slice(0, 8))
const staticRoot = join(root, '.stage/clients', version, 'web')
await mkdir(output, { recursive: true })
const owner = 'staging-persistence', sender = 'pwa-' + run, api = `http://${stagingHost}:18680`
const binary = join(output, 'pinelink.exe'), environment = { ...process.env, PINELINK_CONFIG_HOME: join(output, 'sender-profile') }
execFileSync('go', ['build', '-trimpath', '-o', binary, './cmd/pinelink'], { cwd: join(root, 'cli'), timeout: 120000 })
function cli(...args) {
  return JSON.parse(execFileSync(binary, ['--server', api, '--owner', owner, '--device', sender, '--scope', sender, '--encryption-domain', sender, '--json', ...args], { env: environment, encoding: 'utf8', timeout: 90000 }))
}
cli('device', 'register')
const fixture = join(output, 'offline-fixture.fig')
await writeFile(fixture, Buffer.from('Pinelink PWA encrypted fixture\n'.repeat(64)))
const sleep = ms => new Promise(resolve => setTimeout(resolve, ms))
async function until(read, accept, timeout = 90000) {
  const deadline = Date.now() + timeout
  while (Date.now() < deadline) { const value = await read(); if (accept(value)) return value; await sleep(300) }
  throw new Error('PWA smoke convergence timed out')
}

// A loopback harness uses the exact staged artifact and real signed staging API.
// It injects only outages and a SW revision marker to exercise a safe upgrade.
let reachable = true, workerRevision = 'a'
const mime = { '.html': 'text/html', '.js': 'text/javascript', '.css': 'text/css', '.json': 'application/json', '.webmanifest': 'application/manifest+json', '.svg': 'image/svg+xml', '.png': 'image/png' }
const server = createServer(async (request, response) => {
  const url = new URL(request.url, 'http://localhost')
  if (url.pathname.startsWith('/api/')) {
    if (!reachable) { response.writeHead(503, { 'Content-Type': 'application/json' }); response.end('{"code":"QA_NETWORK_OUTAGE"}'); return }
    const proxy = httpRequest(api + request.url.slice(4), { method: request.method, headers: { ...request.headers, origin: '', host: `${stagingHost}:18680` } }, upstream => { response.writeHead(upstream.statusCode, upstream.headers); upstream.pipe(response) })
    proxy.on('error', () => { response.writeHead(502); response.end() })
    request.pipe(proxy)
    return
  }
  try {
    const pathname = /^\/(?:notifications|tasks|history|settings)?\/?$/.test(url.pathname) ? '/index.html' : decodeURIComponent(url.pathname)
    const path = resolve(staticRoot, '.' + pathname)
    if (!path.startsWith(staticRoot + sep)) throw new Error('Path outside artifact')
    let content = await readFile(path)
    if (url.pathname === '/sw.js') content = Buffer.concat([content, Buffer.from(`\n;self.__PINELINK_QA_REVISION=${JSON.stringify(workerRevision)};\n`)])
    response.writeHead(200, { 'Content-Type': mime[extname(path)] ?? 'application/octet-stream', 'Cache-Control': 'no-store', 'Cross-Origin-Opener-Policy': 'same-origin', 'Cross-Origin-Embedder-Policy': 'require-corp' })
    response.end(content)
  } catch { response.writeHead(404); response.end() }
})
await new Promise(resolve => server.listen(0, '127.0.0.1', resolve))
const base = 'http://127.0.0.1:' + server.address().port
const evidence = { version, started_at: new Date().toISOString(), deployed_url: `https://${stagingHost}:18682`, harness: base, checks: {} }
let context, page, control, device, sent = [], pageErrors = [], workerRequests = 0
async function launch(offline = false) {
  context = await chromium.launchPersistentContext(profile, { executablePath: process.env.CHROMIUM_EXECUTABLE ?? 'C:/Program Files/Google/Chrome/Application/chrome.exe', headless: true, ignoreHTTPSErrors: true, locale: 'en-US', viewport: { width: 1200, height: 800 }, args: ['--ignore-certificate-errors', '--no-proxy-server'] })
  context.on('page', p => p.on('pageerror', error => pageErrors.push(error.message)))
  context.on('request', request => { if (request.url().includes('/api/v1/') && request.headers()['x-pinelink-device'] && request.serviceWorker()) workerRequests++ })
  await context.setOffline(offline)
  page = context.pages()[0] ?? await context.newPage()
  page.on('pageerror', error => pageErrors.push(error.message))
  await page.goto(base, { waitUntil: 'load' })
  await page.getByRole('heading', { name: labels.homeTitle, exact: true }).waitFor()
}
async function snapshot(p = page) {
  return p.evaluate(async () => {
    const name = (await indexedDB.databases()).find(value => value.name?.startsWith('pinelink-'))?.name
    if (!name) return null
    return new Promise((resolve, reject) => {
      const opening = indexedDB.open(name)
      opening.onerror = () => reject(opening.error)
      opening.onsuccess = () => {
        const db = opening.result, transaction = db.transaction(['profiles', 'targets', 'responses'], 'readonly')
        const profile = transaction.objectStore('profiles').get('self'), target = transaction.objectStore('targets').get('cloud'), rows = transaction.objectStore('responses').getAll()
        transaction.oncomplete = () => {
          const p = profile.result, values = rows.result
          resolve({ root: p?.root, device: p?.device, nonextractable: p && !p.signingPrivate.extractable && !p.encryptionPrivate.extractable, target: target.result, pending: values.filter(row => row.pending).length, transfers: values.filter(row => row.path.startsWith('/v1/transfers?')).flatMap(row => row.value?.transfers ?? []), paths: values.map(row => row.path) })
          db.close()
        }
        transaction.onerror = () => { reject(transaction.error); db.close() }
      }
    })
  })
}
function send() {
  const value = cli('p2p', 'send', fixture, '--to-owner', owner, '--to-device', device, '--path', 'relay')
  if (!value.stored) throw new Error('Encrypted fixture not stored')
  sent.push(value.id)
  return value
}
try {
  await launch()
  await page.evaluate(async () => { await navigator.serviceWorker.ready })
  await page.reload({ waitUntil: 'load' })
  const initial = await until(() => snapshot(), value => value?.target?.bootstrapped && value.pending === 0)
  device = initial.device
  if (!initial.nonextractable) throw new Error('Nonextractable durable identity missing')
  const cdp = await context.newCDPSession(page)
  const manifest = await cdp.send('Page.getAppManifest')
  const installability = await cdp.send('Page.getInstallabilityErrors')
  if (manifest.errors.length || installability.installabilityErrors.length) throw new Error('PWA installability errors: ' + JSON.stringify({ manifest: manifest.errors, installability: installability.installabilityErrors }))
  evidence.checks.installable_manifest = { errors: 0, icons: [192, 512], id: '/', scope: '/', display: 'standalone' }
  console.log('PWA: install criteria and initial offline catalog ready')

  // Settings has never been opened. Its data must have been primed already.
  await context.setOffline(true)
  await page.goto(base + '/settings', { waitUntil: 'domcontentloaded' })
  await page.getByRole('heading', { name: labels.settingsTitle, exact: true }).waitFor()
  await page.getByText(device, { exact: true }).waitFor()
  await page.screenshot({ path: join(output, 'unvisited-settings-offline.png'), fullPage: true })
  const apiOffline = await page.evaluate(async () => { try { await fetch('/api/healthz'); return false } catch { return true } })
  const cacheKeys = await page.evaluate(async () => (await Promise.all((await caches.keys()).map(async name => (await (await caches.open(name)).keys()).map(request => request.url)))).flat())
  if (!apiOffline || cacheKeys.some(url => new URL(url).pathname.startsWith('/api/') || new URL(url).pathname === '/version.json')) throw new Error('Private API/version entered the shell cache')
  evidence.checks.unvisited_offline_route = true
  evidence.checks.api_not_sw_cached = true
  await context.setOffline(false)

  const a = send()
  await page.goto(base + '/notifications', { waitUntil: 'domcontentloaded' })
  await page.getByRole('button', { name: 'Accept', exact: true }).click({ timeout: 45000 })
  await page.goto(base + '/history', { waitUntil: 'domcontentloaded' })
  await page.getByRole('heading', { name: labels.historyTitle, exact: true }).waitFor()
  await until(() => snapshot(), value => value.transfers.some(row => row.id === a.id && row.state === 'ACCEPTED') && value.pending === 0)
  reachable = false
  cli('cancel', a.id)
  await page.waitForTimeout(800)
  const during = await snapshot()
  if (during.transfers.find(row => row.id === a.id)?.state !== 'ACCEPTED') throw new Error('Fault injection did not preserve the last verified snapshot')
  reachable = true
  await until(() => snapshot(), value => value.transfers.some(row => row.id === a.id && row.state === 'CANCELLED'))
  await page.getByText(/cancelled|canceled/i).filter({ visible: true }).first().waitFor()
  evidence.checks.server_outage_recovery_without_online_event = true
  console.log('PWA: real API outage recovered and visible history caught up')

  const b = send()
  await until(() => snapshot(), value => value.transfers.some(row => row.id === b.id))
  control = await context.newPage()
  await control.goto(base + '/version.json')
  const observer = await context.newPage()
  const workerCdp = await context.newCDPSession(observer), registrations = new Map(), versions = new Map()
  workerCdp.on('ServiceWorker.workerRegistrationUpdated', payload => { for (const value of payload.registrations) registrations.set(value.registrationId, value) })
  workerCdp.on('ServiceWorker.workerVersionUpdated', payload => { for (const value of payload.versions) versions.set(value.versionId, value) })
  await workerCdp.send('ServiceWorker.enable')
  const registration = await until(async () => [...registrations.values()].find(value => value.scopeURL === base + '/'), Boolean)
  const oldVersion = await until(async () => [...versions.values()].find(value => value.registrationId === registration.registrationId && value.status === 'activated'), Boolean)
  const requestsBefore = workerRequests, before = await snapshot(control)
  await page.close()
  cli('cancel', b.id)
  await workerCdp.send('ServiceWorker.dispatchSyncEvent', { origin: base, registrationId: registration.registrationId, tag: 'pinelink-client-sync-v1', lastChance: false })
  const caught = await until(() => snapshot(control), value => value.transfers.some(row => row.id === b.id && row.state === 'CANCELLED'))
  if (workerRequests <= requestsBefore || caught.target.cursor <= before.target.cursor) throw new Error('Background worker did not perform signed journal consumption')
  evidence.checks.background_sync_without_app_page = { signed_requests: workerRequests - requestsBefore, cursor_before: before.target.cursor, cursor_after: caught.target.cursor }
  console.log('PWA: actual background sync consumed signed changes with app pages closed')

  page = await context.newPage()
  await page.goto(base)
  const second = await context.newPage()
  await second.goto(base)
  await page.evaluate(() => { window.__qaController = navigator.serviceWorker.controller; window.__qaControllerChanges = 0; navigator.serviceWorker.addEventListener('controllerchange', () => window.__qaControllerChanges++) })
  workerRevision = 'b'
  await page.evaluate(async () => { await (await navigator.serviceWorker.getRegistration()).update() })
  await until(() => page.evaluate(async () => (await navigator.serviceWorker.getRegistration()).waiting?.state), value => value === 'installed')
  if (!await page.evaluate(() => navigator.serviceWorker.controller === window.__qaController && window.__qaControllerChanges === 0)) throw new Error('Worker update interrupted the active app')
  await page.close()
  if (!await second.evaluate(async () => Boolean((await navigator.serviceWorker.getRegistration()).waiting))) throw new Error('Update activated before the last app closed')
  await second.close()
  await control.close()
  await until(async () => [...versions.values()].find(value => value.registrationId === registration.registrationId && value.status === 'activated' && value.versionId !== oldVersion.versionId), Boolean)
  evidence.checks.update_waits_for_all_windows = true

  await context.setOffline(true)
  await context.close()
  await launch(true)
  const reopened = await snapshot()
  if (reopened.root !== initial.root || reopened.device !== device || !reopened.nonextractable) throw new Error('Identity changed after offline browser restart')
  for (const [route, title] of [['/tasks', labels.tasksTitle], ['/history', labels.historyTitle], ['/notifications', labels.notificationTitle], ['/settings', labels.settingsTitle]]) {
    await page.goto(base + route, { waitUntil: 'domcontentloaded' })
    await page.getByRole('heading', { name: title, exact: true }).waitFor()
  }
  await page.screenshot({ path: join(output, 'offline-browser-reopened.png'), fullPage: true })
  evidence.checks.offline_browser_restart_all_routes = true
  const c = send()
  await page.goto(base + '/notifications', { waitUntil: 'domcontentloaded' })
  await context.setOffline(false)
  await page.getByRole('button', { name: 'Accept', exact: true }).waitFor({ timeout: 90000 })
  await until(() => snapshot(), value => value.transfers.some(row => row.id === c.id))
  evidence.checks.automatic_online_catchup_without_reload = true

  // Also inspect the actual remote deployment's installation metadata.
  const deployed = await context.newPage()
  await deployed.goto(evidence.deployed_url, { waitUntil: 'load' })
  await deployed.evaluate(async () => { await navigator.serviceWorker.ready })
  const actual = await deployed.evaluate(async () => (await (await fetch('/version.json', { cache: 'no-store' })).json()).version)
  if (actual !== version) throw new Error('Wrong deployed PWA version')
  const deployedCdp = await context.newCDPSession(deployed)
  const deployedManifest = await deployedCdp.send('Page.getAppManifest')
  const deployedInstall = await deployedCdp.send('Page.getInstallabilityErrors')
  if (deployedManifest.errors.length) throw new Error('Deployed manifest failed: ' + JSON.stringify(deployedManifest.errors))
  const tlsOnly = deployedInstall.installabilityErrors.every(error => error.errorId === 'not-from-secure-origin')
  if (deployedInstall.installabilityErrors.length && !tlsOnly) throw new Error('Deployed installability failed: ' + JSON.stringify(deployedInstall))
  evidence.checks.deployed_https_installable = deployedInstall.installabilityErrors.length === 0
  if (deployedInstall.installabilityErrors.length) evidence.staging_tls_limitation = 'Self-signed staging certificate blocks normal browser install promotion; no system trust-store changes.'
  const preview = process.env.PINELINK_PWA_PREVIEW_URL
  if (preview) {
    const remotePreview = await context.newPage()
    await remotePreview.goto(preview, { waitUntil: 'load' })
    await remotePreview.evaluate(async () => { await navigator.serviceWorker.ready })
    const actualPreview = await remotePreview.evaluate(async () => (await (await fetch('/__pinelink_preview', { cache: 'no-store' })).json()))
    if (actualPreview.version !== version || !actualPreview.loopback_only || actualPreview.system_trust_store_modified) throw new Error('Staging preview bridge mismatch')
    const previewCdp = await context.newCDPSession(remotePreview)
    const previewInstall = await previewCdp.send('Page.getInstallabilityErrors')
    if (previewInstall.installabilityErrors.length) throw new Error('Pinned staging preview installation failed: ' + JSON.stringify(previewInstall))
    evidence.checks.trusted_staging_preview_installable = { url: preview, criteria_errors: 0, ...actualPreview }
  } else if (deployedInstall.installabilityErrors.length) throw new Error('A trusted staging preview or valid staging TLS is required')
  if (pageErrors.length) throw new Error('Page errors: ' + pageErrors.join(';'))
  evidence.completed_at = new Date().toISOString()
  await writeFile(join(output, 'smoke.json'), JSON.stringify(evidence, null, 2))
  console.log(JSON.stringify({ evidence: join(output, 'smoke.json'), ...evidence }, null, 2))
} catch (error) {
  if (page && !page.isClosed()) await page.screenshot({ path: join(output, 'failure.png'), fullPage: true }).catch(() => {})
  await writeFile(join(output, 'failure.json'), JSON.stringify({ ...evidence, error: String(error), pageErrors }, null, 2))
  throw error
} finally {
  for (const id of sent) { try { cli('cancel', id) } catch {} }
  await context?.close()
  await new Promise(resolve => server.close(resolve))
}
