import { useEffect, useState } from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryHistory, createRootRoute, createRoute, createRouter, RouterProvider } from '@tanstack/react-router'
import { AppShell } from './components/AppShell'
import { NewTransferPage } from './routes/index'
import { NotificationsPage } from './routes/notifications'
import { TasksPage } from './routes/tasks'
import { HistoryPage } from './routes/history'
import { SettingsPage } from './routes/settings'
import { I18nProvider } from './i18n'
import type { LocalePreference } from './i18n'
import { clientRuntime } from './data'
import { observeCache } from './data/cache-observer'
import './styles/app.css'

const rootRoute = createRootRoute({ component: AppShell })
const indexRoute = createRoute({ getParentRoute: () => rootRoute, path: '/', component: NewTransferPage })
const notificationsRoute = createRoute({ getParentRoute: () => rootRoute, path: '/notifications', component: NotificationsPage })
const tasksRoute = createRoute({ getParentRoute: () => rootRoute, path: '/tasks', component: TasksPage })
const settingsRoute = createRoute({ getParentRoute: () => rootRoute, path: '/settings', component: SettingsPage })
const historyRoute = createRoute({ getParentRoute: () => rootRoute, path: '/history', component: HistoryPage })
const routeTree = rootRoute.addChildren([indexRoute, notificationsRoute, tasksRoute, settingsRoute, historyRoute])

// Browser history installs global listeners at construction. Create it at the
// entry point, outside React render/state initializers that StrictMode replays.
export const createAppRouter = (initialEntries?: string[]) => createRouter({ routeTree, history: initialEntries ? createMemoryHistory({ initialEntries }) : undefined })

export function App({ initialEntries, localePreference, router: suppliedRouter }: { initialEntries?: string[]; localePreference?: LocalePreference; router?: ReturnType<typeof createAppRouter> } = {}) {
  // Queries read local durable data even when the browser reports offline.
  // Mutations must run to reject unavailable controls rather than silently
  // queuing a versioned action in React Query's transient memory.
  const [queryClient] = useState(() => new QueryClient({ defaultOptions: {
    queries: { networkMode: 'always', retry: (attempt) => navigator.onLine !== false && attempt < 2 },
    mutations: { networkMode: 'always' },
  } }))
  const [router] = useState(() => suppliedRouter ?? createAppRouter(initialEntries ?? ['/']))
  useEffect(() => {
    try {
      const runtime = clientRuntime()
      if (runtime) return observeCache(runtime.store, () => { void queryClient.invalidateQueries({ refetchType: 'active' }) })
    } catch { /* Resource queries provide the existing local-storage error states. */ }
  }, [queryClient])
  return (
    <I18nProvider initialPreference={localePreference}>
      <QueryClientProvider client={queryClient}>
        <RouterProvider router={router} />
      </QueryClientProvider>
    </I18nProvider>
  )
}
