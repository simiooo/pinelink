import { MessageChannel } from 'node:worker_threads'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { askClient, coordinateUpdate } from './update-coordinator'
import type { UpdateClient } from './update-coordinator'
beforeEach(() => vi.stubGlobal('MessageChannel', MessageChannel))
afterEach(() => vi.unstubAllGlobals())
const page = (id: string, vote: (phase: string) => boolean | undefined = () => true) => {
  const events: string[] = []
  const client: UpdateClient = { id, postMessage: (raw, ports) => {
    const phase = (raw as { phase: string }).phase; events.push(phase)
    if (ports[0]) (ports[0] as unknown as { postMessage: (v: unknown) => void }).postMessage({ ready: vote(phase) })
  } }
  return { client, events }
}
it('prepares every idle page before a single activation', async () => {
  const a = page('a'), b = page('b'), activate = vi.fn(async () => { expect(a.events).toEqual(['CHECK','PREPARE']); expect(b.events).toEqual(['CHECK','PREPARE']) })
  expect(await coordinateUpdate(async () => [a.client,b.client],activate,'operation')).toBe('applied')
  expect(activate).toHaveBeenCalledOnce()
})
it('leaves a transfer or draft in another page untouched', async () => {
  const a=page('a'),b=page('b',()=>false),activate=vi.fn()
  expect(await coordinateUpdate(async()=>[a.client,b.client],activate,'operation')).toBe('busy')
  expect(a.events).toEqual(['CHECK']);expect(activate).not.toHaveBeenCalled()
})
it('does not replace an unresponsive older page', async () => {
  const old=page('old',()=>undefined),activate=vi.fn()
  expect(await coordinateUpdate(async()=>[old.client],activate,'operation')).toBe('legacy')
  expect(activate).not.toHaveBeenCalled()
  expect(await askClient({id:'unresponsive',postMessage:()=>{}},'CHECK','operation',undefined,5)).toBeUndefined()
})
it('unfreezes pages when a new operation starts between check and prepare', async () => {
  const a=page('a'),b=page('b',phase=>phase!=='PREPARE'),activate=vi.fn()
  expect(await coordinateUpdate(async()=>[a.client,b.client],activate,'operation')).toBe('busy')
  expect(a.events).toEqual(['CHECK','PREPARE','CANCEL']);expect(activate).not.toHaveBeenCalled()
})
it('rechecks the open page set and cancels when a new page appears', async () => {
  const a=page('a'),b=page('b'),activate=vi.fn();let reads=0
  expect(await coordinateUpdate(async()=>++reads===1?[a.client]:[a.client,b.client],activate,'operation')).toBe('busy')
  expect(a.events).toContain('CANCEL');expect(activate).not.toHaveBeenCalled()
})
it('does not activate after a preparation timeout or cancellation', async () => {
  const controller=new AbortController(),a=page('a',phase=>{if(phase==='PREPARE')controller.abort();return true}),activate=vi.fn()
  expect(await coordinateUpdate(async()=>[a.client],activate,'operation',controller.signal)).toBe('error')
  expect(a.events).toContain('CANCEL');expect(activate).not.toHaveBeenCalled()
})
