export type ConnectionInput = { kind: 'local' | 'online'; value: string }

// Only the current service's invitation links may be redeemed here. Local
// packages are still fully verified by LocalRuntime before any pairing.
export function parseConnectionInput(raw: string, origin: string): ConnectionInput {
  const value = raw.trim()
  if (!value || value.length > 18000) throw new Error('INVALID_REQUEST')
  if (value.startsWith('PL2:') || value.startsWith('PL1.')) return { kind: 'local', value }
  if (/^[A-Za-z0-9_-]{22}$/.test(value)) return { kind: 'online', value }
  const short = value.toUpperCase().replace(/[ -]/g, '')
  if (/^[A-Z0-9]{8}$/.test(short)) return { kind: 'online', value: short }
  try {
    const link = new URL(value)
    const token = link.hash.slice(1)
    if (link.origin === origin && !link.username && !link.password && link.pathname === '/connect' && !link.search && /^[A-Za-z0-9_-]{22}$/.test(token)) return { kind: 'online', value: token }
  } catch { /* Invalid input is reported below. */ }
  throw new Error('INVALID_REQUEST')
}
