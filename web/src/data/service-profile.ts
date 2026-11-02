import Dexie from 'dexie'
type PublicProfile = { version: number; api_url: string; history_public_key: string }
type Pin = { server: string; key: string }

function validKey(value: unknown): value is string {
  if (typeof value !== 'string' || !/^[A-Za-z0-9+/]{43}=$/.test(value)) return false
  try { return atob(value).length === 32 && btoa(atob(value)) === value } catch { return false }
}

// The public configuration is delivered by the same trusted origin as the app.
// Once pinned, a changed key must fail verification, never silently replace it.
export async function loadServerPin(server: string, configured = '', required = false): Promise<string> {
  if (configured || !required) return configured
  const absolute = new URL(server, location.href).href.replace(/\/$/, '')
  if (new URL(absolute).origin !== location.origin) throw new Error('SERVICE_PROFILE_ORIGIN_MISMATCH')
  const database = new Dexie('pinelink.service-profiles')
  database.version(1).stores({ pins: 'server' })
  const pins = database.table<Pin, string>('pins')
  const saved = async () => {
    const value = await pins.get(absolute)
    if (value && !validKey(value.key)) throw new Error('INVALID_SERVICE_PIN')
    return value?.key
  }
  try {
    const existing = await saved()
    if (existing) return existing
    const controller = new AbortController(), timer = setTimeout(() => controller.abort(), 6000)
    let profile: PublicProfile
    try {
      const response = await fetch('/service-config.json', { credentials: 'same-origin', cache: 'no-store', redirect: 'error', signal: controller.signal })
      if (!response.ok || !response.headers.get('content-type')?.includes('application/json')) throw new Error('SERVICE_PROFILE_UNAVAILABLE')
      profile = await response.json() as PublicProfile
    } finally { clearTimeout(timer) }
    if (profile.version !== 1 || !validKey(profile.history_public_key) ||
        new URL(profile.api_url, location.href).href.replace(/\/$/, '') !== absolute) throw new Error('INVALID_SERVICE_PROFILE')
    // IndexedDB serializes competing tabs and service workers, including clients
    // without Web Locks. Both contexts must retain the same first trusted key.
    return await database.transaction('rw', pins, async () => {
      const winner = await saved()
      if (winner) return winner
      await pins.add({ server: absolute, key: profile.history_public_key })
      return profile.history_public_key
    })
  } finally { database.close() }
}
