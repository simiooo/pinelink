// Window/OAuth messages share this event channel with worker messages. Accessing
// a foreign WindowProxy's properties can throw before the payload is handled.
export function updateMessageSource(event: Pick<MessageEvent, 'data' | 'source'>, origin: string): ServiceWorker | undefined {
  if (event.data?.type !== 'PINELINK_UPDATE') return
  try {
    const source = event.source as ServiceWorker | null
    if (!source?.scriptURL) return
    const url = new URL(source.scriptURL)
    if (url.origin === origin && url.pathname === '/sw.js') return source
  } catch { /* A foreign window cannot participate in the update protocol. */ }
}

export function pendingUpdate(registration?: Pick<ServiceWorkerRegistration, 'active' | 'waiting'>): ServiceWorker | undefined {
  if (!registration?.active || !registration.waiting || registration.active === registration.waiting) return
  return registration.waiting
}
