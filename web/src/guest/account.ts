import { useEffect, useState } from 'react'
import { authState, sessionExpired } from '../auth/client'
import { clientRuntime } from '../data'
import { ClientRuntime } from '../data/runtime'
import { loadServerPin } from '../data/service-profile'
import { API_URL, IDENTITY } from '../api/client'

// A sign-in window may establish an account while this window keeps sending
// through its visitor identity. Read account data with its own signed profile;
// never replace the visitor runtime or reuse its keys for account operations.
export function useAccountReader() {
  const auth = authState(), account = auth.session?.account ?? auth.offlineAccount
  const [owned, setOwned] = useState<ClientRuntime>()
  let current: ClientRuntime | undefined
  try { current = clientRuntime() } catch { /* An isolated reader can reopen the matching profile. */ }
  const matches = (value?: ClientRuntime) => !!value && value.owner === account?.id && (auth.session ? value.profile.device === auth.session.device_id && value.access.csrf === auth.session.csrf && !value.access.localOnly : value.access.localOnly)
  const reader = matches(current) ? current : matches(owned) ? owned : undefined
  useEffect(() => {
    if (!auth.capabilities.enabled || !account || matches(current)) return
    let live = true, created: ClientRuntime | undefined
    void (async () => {
      const pin = auth.session ? await loadServerPin(API_URL, import.meta.env.VITE_PINELINK_HISTORY_KEY ?? '', import.meta.env.VITE_PINELINK_SERVICE_PROFILE === 'runtime') : ''
      if (!live) return
      created = await ClientRuntime.open(API_URL, account.id, '', pin, undefined, { localOnly: !auth.session, csrf: auth.session?.csrf, onUnauthorized: () => sessionExpired(auth.session?.csrf ?? '') })
      if (!live) { await created.close(); return }
      if (auth.session && created.profile.device !== auth.session.device_id) { await created.close(); return }
      await created.prime(account.id)
      if (live) setOwned(created); else await created.close()
    })().catch(() => { if (created) void created.close() })
    return () => { live = false; if (created) void created.close() }
  }, [auth.capabilities.enabled, account?.id, auth.session?.csrf, auth.session?.device_id, current])
  return { reader, ready: !auth.capabilities.enabled || !!reader, owner: account?.id ?? IDENTITY.owner, device: reader?.profile.device ?? auth.session?.device_id ?? IDENTITY.device }
}
