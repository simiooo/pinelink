import 'fake-indexeddb/auto'
import { webcrypto } from 'node:crypto'
import { StrictMode } from 'react'
import { afterEach, expect, it, vi } from 'vitest'
import { cleanup, render, screen } from '@testing-library/react'
import { App } from '../main'
import { IDENTITY } from '../api/client'
import { ClientRuntime } from './runtime'
import { LocalStore } from './store'
import { onlineManager } from '@tanstack/react-query'

let active: ClientRuntime | undefined
vi.mock('./index', () => ({ clientRuntime: () => active }))
afterEach(async () => { cleanup(); onlineManager.setOnline(true); if (active) { const db=active.store; await active.close(); await db.delete(); active=undefined } })

it('reopens the actual history page offline using durable API records', async () => {
  Object.defineProperty(globalThis, 'crypto', { value: webcrypto, configurable: true })
  Object.defineProperty(navigator, 'onLine', { value: false, configurable: true })
  onlineManager.setOnline(false)
  const db=new LocalStore('app-'+crypto.randomUUID())
  active=await ClientRuntime.open('https://unit.invalid',IDENTITY.owner,'','',db)
  IDENTITY.device=active.profile.device
  await db.patchTarget({ bound:'API_ONLY' })
  const identity=`owner_id=${IDENTITY.owner}&device_id=${IDENTITY.device}`
  await db.cacheResponse(`/v1/tasks?${identity}&include_deleted=true`,{ tasks:[{ id:'task',kind:'DOWNLOAD',reference_id:'pull',state:'COMPLETED',version:2,progress_bytes:50,total_bytes:50,updated_at:'2026-11-01T15:00:00Z' }] },1)
  await db.cacheResponse(`/v1/transfers?${identity}`,{transfers:[]},1)
  await db.cacheResponse(`/v1/pulls/pull?${identity}`,{ id:'pull',task_id:'task',file_name:'Design.fig',size_bytes:50 },1)
  render(<StrictMode><App initialEntries={['/history']} localePreference="en-US" /></StrictMode>)
  expect(await screen.findByText('Design.fig')).toBeInTheDocument()
  const root=active.profile.root
  cleanup(); await active.close()
  active=await ClientRuntime.open('https://unit.invalid',IDENTITY.owner,'','',new LocalStore(db.name))
  expect(active.profile.root).toBe(root)
  render(<StrictMode><App initialEntries={['/history']} localePreference="en-US" /></StrictMode>)
  expect(await screen.findByText('Design.fig')).toBeInTheDocument()
})
