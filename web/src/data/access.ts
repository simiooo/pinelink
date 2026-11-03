// Bootstrap sets this before opening any account-scoped durable resources.
// Storage failure must never fall back to an anonymous network client.
export const dataAccess = { managed: import.meta.env.VITE_PINELINK_SERVICE_PROFILE === 'runtime' }
