import { afterEach, expect, it } from 'vitest'
import { freezeForUpdate, holdUpdate, registerUpdateGuard, releaseUpdate, safeToUpdate } from './guard'
import { setGuestWorkPending } from '../guest/continuity'
afterEach(()=>{releaseUpdate();setGuestWorkPending(0);document.body.innerHTML=''})
it('protects bootstrap, durable guest work and registered account operations',()=>{
 const release=holdUpdate();expect(safeToUpdate()).toBe(false);release();expect(safeToUpdate()).toBe(true)
 setGuestWorkPending(1);expect(safeToUpdate()).toBe(false);setGuestWorkPending(0)
 const unregister=registerUpdateGuard(()=>false);expect(safeToUpdate()).toBe(false);unregister();expect(safeToUpdate()).toBe(true)
})
it('keeps unsaved text, selected attachments and pairing screens',()=>{
 document.body.innerHTML='<main><textarea aria-label="Message"></textarea></main>'
 const field=document.querySelector('textarea')!;field.value='unfinished';expect(safeToUpdate()).toBe(false);field.value='';expect(safeToUpdate()).toBe(true)
 document.querySelector('main')!.insertAdjacentHTML('beforeend','<div class="conversation-attachment">draft.pdf</div>');expect(safeToUpdate()).toBe(false)
 document.querySelector('.conversation-attachment')!.remove();document.querySelector('main')!.insertAdjacentHTML('beforeend','<div class="guest-connect"></div>');expect(safeToUpdate()).toBe(false)
})
it('holds the prepared page until a commit reload or explicit cancellation',()=>{
 expect(freezeForUpdate()).toBe(true);expect(safeToUpdate()).toBe(false);expect(freezeForUpdate()).toBe(false)
 releaseUpdate();expect(safeToUpdate()).toBe(true)
})
