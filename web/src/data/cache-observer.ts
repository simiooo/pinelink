import { liveQuery } from 'dexie'
import type { LocalStore } from './store'

// Observe semantic changes, not read timestamps: refetching an unchanged value
// must not create an invalidation/refetch loop. Dexie also observes other tabs.
export function observeCache(store: LocalStore, changed: () => void) {
  let previous: number | undefined
  const subscription = liveQuery(async () => (await store.target()).cacheRevision ?? 0).subscribe({
    next(revision) {
      const initial = previous === undefined
      const different = previous !== revision
      previous = revision
      if (!initial && different) changed()
    },
    error() { /* Storage errors remain visible through the actual resource query. */ },
  })
  return () => subscription.unsubscribe()
}
