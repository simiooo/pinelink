const stagingHost = process.env.PINELINK_STAGING_HOST ?? '127.0.0.1'
// Development review bridge only. Streams the deployed staging client through
// a loopback secure context; it does not replace production HTTPS or trust roots.
import { createServer } from 'node:http'
import { Agent, request as httpsRequest } from 'node:https'
import { X509Certificate } from 'node:crypto'
import { readFile } from 'node:fs/promises'
import { resolve } from 'node:path'

const version = process.argv[2], port = Number(process.env.PINELINK_PREVIEW_PORT ?? 18684)
if (!/^[a-f0-9]{40}$/.test(version || '') || !Number.isInteger(port) || port < 1024 || port > 65535) throw new Error('A source version and valid unprivileged port are required')
const root = resolve(import.meta.dirname, '../..'), upstream = new URL(`https://${stagingHost}:18682`)
const certificate = await readFile(resolve(root, '.stage/client-tls/server.crt'))
const identity = new X509Certificate(certificate)
const agent = new Agent({ ca: certificate, keepAlive: true })
function readVersion() {
  return new Promise((resolve, reject) => {
    const request = httpsRequest(new URL('/version.json', upstream), { agent }, response => {
      const parts = []
      response.on('data', part => parts.push(part))
      response.on('end', () => {
        try {
          const actual = JSON.parse(Buffer.concat(parts).toString()).version
          if (response.statusCode !== 200 || !/^[a-f0-9]{40}$/.test(actual ?? '')) throw new Error('Deployed staging version unavailable')
          resolve(actual)
        } catch (error) { reject(error) }
      })
    })
    request.setTimeout(10000, () => request.destroy(new Error('Version check timed out')))
    request.on('error', reject)
    request.end()
  })
}
if (await readVersion() !== version) throw new Error('Deployed staging version does not match')
const allowedHosts = new Set([`localhost:${port}`, `127.0.0.1:${port}`])
const allowedOrigins = new Set([`http://localhost:${port}`, `http://127.0.0.1:${port}`])
const ordinaryHeaders = new Set(['accept', 'accept-language', 'content-type', 'content-length', 'range', 'if-range', 'if-none-match', 'if-modified-since', 'cache-control', 'pragma'])
const server = createServer(async (request, response) => {
  if (!allowedHosts.has(request.headers.host) || (request.headers.origin && !allowedOrigins.has(request.headers.origin)) || !request.url?.startsWith('/') || request.url.startsWith('//')) {
    response.writeHead(403); response.end(); return
  }
  const incoming = new URL(request.url, 'http://' + request.headers.host)
  if (incoming.pathname === '/__pinelink_preview') {
    try {
      const actual = await readVersion()
      response.writeHead(200, { 'Content-Type': 'application/json', 'Cache-Control': 'no-store' })
      response.end(JSON.stringify({ version: actual, started_for_version: version, upstream: upstream.origin, certificate_sha256: identity.fingerprint256, loopback_only: true, system_trust_store_modified: false }))
    } catch {
      response.writeHead(502, { 'Content-Type': 'application/json', 'Cache-Control': 'no-store' })
      response.end('{"code":"STAGING_UNAVAILABLE"}')
    }
    return
  }
  const target = new URL(upstream)
  target.pathname = incoming.pathname
  target.search = incoming.search
  // Cookies are shared across localhost ports. Never forward another local
  // application's cookies or bearer credentials to the remote staging service.
  const headers = { host: upstream.host }
  for (const [name, value] of Object.entries(request.headers)) {
    if (ordinaryHeaders.has(name) || name.startsWith('x-pinelink-')) headers[name] = value
  }
  const proxy = httpsRequest(target, { method: request.method, headers, agent }, remote => {
    const received = { ...remote.headers }
    for (const name of ['set-cookie', 'connection', 'transfer-encoding', 'www-authenticate', 'proxy-authenticate']) delete received[name]
    response.writeHead(remote.statusCode ?? 502, received)
    remote.on('error', () => response.destroy())
    remote.pipe(response)
  })
  proxy.setTimeout(30000, () => proxy.destroy(new Error('Staging upstream timed out')))
  proxy.on('error', () => {
    if (!response.headersSent) response.writeHead(502, { 'Content-Type': 'application/json', 'Cache-Control': 'no-store' })
    response.end('{"code":"STAGING_UNAVAILABLE"}')
  })
  request.on('aborted', () => proxy.destroy())
  response.on('close', () => { if (!response.writableEnded) proxy.destroy() })
  request.pipe(proxy)
})
server.listen(port, '127.0.0.1', () => console.log(JSON.stringify({ preview: `http://localhost:${port}`, version, upstream: upstream.origin, certificate_sha256: identity.fingerprint256 })))
const stop = () => { server.close(() => { agent.destroy(); process.exit(0) }); setTimeout(() => process.exit(0), 5000).unref() }
process.on('SIGINT', stop)
process.on('SIGTERM', stop)
