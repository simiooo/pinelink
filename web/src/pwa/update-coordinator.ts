export type UpdateClient = { id: string; postMessage: (message: unknown, ports: Transferable[]) => void }
export type UpdateResult = 'applied' | 'busy' | 'legacy' | 'error'
export function askClient(client: UpdateClient, phase: 'CHECK' | 'PREPARE', session: string, signal?: AbortSignal, timeout = 2000): Promise<boolean | undefined> {
  return new Promise(resolve => {
    const channel = new MessageChannel()
    const aborted = () => finish()
    const finish = (value?: boolean) => { clearTimeout(timer); signal?.removeEventListener('abort', aborted); channel.port1.close(); channel.port2.close(); resolve(value) }
    const timer = setTimeout(() => finish(), timeout)
    channel.port1.onmessage = event => finish(typeof event.data?.ready === 'boolean' ? event.data.ready : undefined)
    signal?.addEventListener('abort', aborted, { once: true })
    if (signal?.aborted) { finish(); return }
    try { client.postMessage({ type: 'PINELINK_UPDATE', phase, session }, [channel.port2]) } catch { finish() }
  })
}
export async function coordinateUpdate(getClients: () => Promise<UpdateClient[]>, activate: () => Promise<unknown>, session: string, signal: AbortSignal = new AbortController().signal): Promise<UpdateResult> {
  let clients: UpdateClient[] = []
  const read = () => Promise.race([getClients(), new Promise<never>((_, reject) => { if (signal.aborted) reject(signal.reason); else signal.addEventListener('abort', () => reject(signal.reason), { once: true }) })])
  let prepared = false, applied = false
  try {
    clients = await read()
    if (!clients.length) return 'legacy'
    const checked = await Promise.all(clients.map(client => askClient(client, 'CHECK', session, signal)))
    signal.throwIfAborted()
    if (checked.some(value => value === undefined)) return 'legacy'
    if (checked.some(value => !value)) return 'busy'
    prepared = true
    const votes = await Promise.all(clients.map(client => askClient(client, 'PREPARE', session, signal)))
    signal.throwIfAborted()
    if (votes.some(value => value !== true)) return 'busy'
    const current = await read(), ids = new Set(clients.map(client => client.id))
    if (current.some(client => !ids.has(client.id))) return 'busy'
    signal.throwIfAborted(); await activate(); applied = true
    return 'applied'
  } catch { return 'error' }
  finally { if (prepared && !applied) for (const client of clients) { try { client.postMessage({ type: 'PINELINK_UPDATE', phase: 'CANCEL', session }, []) } catch { /* A closed page cannot be reloaded. */ } } }
}
