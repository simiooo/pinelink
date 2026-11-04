import 'fake-indexeddb/auto'
import {webcrypto} from 'node:crypto'
import {afterEach,expect,it,vi} from 'vitest'
import {newProfile} from '../data/crypto'
import type {DeviceProfile} from '../data/crypto'
import {GuestStore} from '../guest/store'
import type {GuestJob} from '../guest/store'
import {suite} from '../guest/crypto'
import {LocalRuntime} from './runtime'
import {signed} from './protocol'
const stores:GuestStore[]=[],runtimes:LocalRuntime[]=[]
const open=async()=>{vi.stubGlobal('crypto',webcrypto);vi.stubGlobal('fetch',vi.fn(()=>{throw Error('Center must not be used')}));const s=new GuestStore('local-test:'+crypto.randomUUID());stores.push(s);const r=await LocalRuntime.open(s,async()=>{});runtimes.push(r);return r}
afterEach(async()=>{for(const r of runtimes.splice(0))await r.close();for(const s of stores.splice(0)){s.close();await s.delete()};vi.unstubAllGlobals()})
async function job(r:LocalRuntime,direction:'receive'|'send',peer:DeviceProfile){
 const id='local:'+crypto.randomUUID(),sender=direction==='send'?r.profile:peer,recipient=direction==='receive'?r.profile:peer
 const state=await signed(recipient,'state',{id,sender:sender.root,recipient:recipient.root,root:'a'.repeat(64),size:131072,name:'File.bin',plain_size:131008,state:'ACCEPTED' as const,version:2,revision:1,received:65536,paused:false})
 const j:GuestJob={id,created:Date.now(),direction,authority:'LOCAL_DEVICE',peer:{pair_id:'local:'+peer.root,root:peer.root,owner_id:peer.root,device_id:peer.device,device_name:'Peer',signing_key:peer.signingKey,encryption_key:peer.encryptionKey,expires_at:'9999-01-01T00:00:00Z',authority:'LOCAL_DEVICE'},transfer:{id,state:'ACCEPTED',version:2,file_name:'File.bin',content_root:state.root,size_bytes:state.size,encryption_suite:suite,sender_owner_id:state.sender,sender_device_id:sender.device,recipient_owner_id:state.recipient,recipient_device_id:recipient.device,received_bytes:65536,expires_at:new Date(Date.now()+600000).toISOString()},received:65536,name:'File.bin',plainSize:131008,phase:'running',local:{expires:Date.now()+600000,state}}
 await r.store.jobs.add(j);return j
}
it('persists a separate local identity without obtaining any center identity or lease',async()=>{
 const r=await open(),cloud=await r.store.profile();expect(cloud.root).not.toBe(r.profile.root);expect(await r.store.leases.count()).toBe(0)
 const reopened=await LocalRuntime.open(r.store,async()=>{});runtimes.push(reopened);expect(reopened.profile.root).toBe(r.profile.root);expect(fetch).not.toHaveBeenCalled()
})
it('locally pauses and cancels a receiver after disconnect, atomically preserving its signed journal and checkpoint',async()=>{
 const r=await open(),peer=await newProfile(),j=await job(r,'receive',peer)
 await r.action(j.id,'pause');const paused=await r.store.jobs.get(j.id);expect(paused).toMatchObject({phase:'paused',received:65536,local:{state:{version:3,paused:true}}});expect(await r.store.localJournal.count()).toBe(1)
 await expect(r.action(j.id,'resume')).rejects.toThrow('LOCAL_RECONNECT_REQUIRED')
 await r.action(j.id,'cancel');expect(await r.store.jobs.get(j.id)).toMatchObject({phase:'stopped',local:{state:{state:'CANCELLED',version:4,received:65536}}});expect(await r.store.localJournal.count()).toBe(2);expect(fetch).not.toHaveBeenCalled()
})
it('a disconnected sender records a desired pause without inventing a receiver acknowledgement',async()=>{
 const r=await open(),j=await job(r,'send',await newProfile());await r.action(j.id,'pause');expect(await r.store.jobs.get(j.id)).toMatchObject({phase:'paused',local:{intent:'pause',state:{version:2,paused:false,received:65536}}});expect(await r.store.localJournal.count()).toBe(0)
})
it('an active tab lease fences conflicting receiver mutations',async()=>{
 const r=await open(),j=await job(r,'receive',await newProfile());await r.store.localLeases.put({id:j.peer.root!,owner:'other-tab',until:Date.now()+15000});await expect(r.action(j.id,'cancel')).rejects.toThrow('LOCAL_OTHER_TAB');expect((await r.store.jobs.get(j.id))?.phase).toBe('running');expect(await r.store.localJournal.count()).toBe(0)
})
