export const workspacePaths = new Set(['/', '/files', '/local-files', '/history', '/tasks', '/notifications', '/connect'])
export type FileFilter = 'all' | 'active' | 'saved' | 'sent' | 'issues'
export const routePanel = (path: string) => path === '/connect' ? 'connect' as const : path === '/notifications' ? 'notifications' as const : ['/files', '/local-files', '/history', '/tasks'].includes(path) ? 'files' as const : null
export const routeFilter = (path: string): FileFilter => path === '/tasks' ? 'active' : path === '/local-files' ? 'saved' : 'all'
