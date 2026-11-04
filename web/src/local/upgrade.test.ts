import 'fake-indexeddb/auto'
import {webcrypto} from 'node:crypto'
import {Blob as NodeBlob} from 'node:buffer'
import {afterEach,expect,it,vi} from 'vitest'
import {LocalStore} from '../data/store'
import {newProfile} from '../data/crypto'
import {GuestStore} from '../guest/store'
afterEach(()=>vi.unstubAllGlobals())
it('upgrades a populated visitor v4 database without replacing cloud keys, devices, files or checkpoints',async()=>{
 vi.stubGlobal('crypto',webcrypto)
 vi.stubGlobal('Blob',NodeBlob)
 const name='upgrade:'+crypto.randomUUID(),old=new LocalStore(name)
 old.version(3).stores({leases:'id',jobs:'id,created,transfer.id,direction',chunks:'[job+index],job'})
 old.version(4).stores({guestPeers:'pair_id',peerRemovals:'pair,expires'})
 const identity={...await newProfile(),id:'self'}
 await old.profiles.add(identity)
 await old.table('leases').put({id:'self',owner_id:'legacy-owner'})
 await old.table('guestPeers').put({pair_id:'legacy-pair',device_name:'My laptop'})
 await old.table('jobs').put({id:'legacy-transfer',received:65536,phase:'paused',transport:{state:'direct'}})
 await old.table('chunks').put({job:'legacy-transfer',index:0,bytes:new Blob(['ciphertext']),hash:'legacy-hash'})
 old.close()
 const upgraded=new GuestStore(name)
 try{
  expect((await upgraded.profile()).root).toBe(identity.root)
  expect(await upgraded.leases.get('self')).toMatchObject({owner_id:'legacy-owner'})
  expect(await upgraded.guestPeers.get('legacy-pair')).toMatchObject({device_name:'My laptop'})
  expect(await upgraded.jobs.get('legacy-transfer')).toMatchObject({phase:'paused',received:65536,transport:{state:'direct'}})
  expect((await upgraded.chunks.get(['legacy-transfer',0]))?.bytes.size).toBe(10)
  expect(await upgraded.localPeers.count()).toBe(0)
  expect(await upgraded.localJournal.count()).toBe(0)
 }finally{upgraded.close();await upgraded.delete()}
})
