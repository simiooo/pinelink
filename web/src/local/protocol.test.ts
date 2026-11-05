import {Blob as NodeBlob} from 'node:buffer'
import {webcrypto} from 'node:crypto'
import {deflateSync} from 'node:zlib'
import {afterEach,expect,it,vi} from 'vitest'
import {base64,newProfile} from '../data/crypto'
import {comparison,decodePackage,encodePackage,identity,makePackage,signed,validatePackage,verified} from './protocol'
const setup=()=>{vi.stubGlobal('crypto',webcrypto);vi.stubGlobal('Blob',NodeBlob)}
const sdp='v=0\r\nm=application 9 UDP/DTLS/SCTP webrtc-datachannel\r\na=ice-ufrag:abc\r\na=ice-pwd:abcdefghijklmnopqrstuv\r\na=fingerprint:sha-256 '+Array(32).fill('AB').join(':')+'\r\n'
afterEach(()=>vi.unstubAllGlobals())
it('round trips bounded offline packages and binds both keys, SDP and reply into the comparison',async()=>{
 setup();const a=await newProfile(),b=await newProfile(),offer=await makePackage(a,identity(a,'Laptop'),'offer',crypto.randomUUID(),sdp),answer=await makePackage(b,identity(b,'Phone'),'answer',offer.id,sdp,'a'.repeat(64))
 expect(await decodePackage(await encodePackage(offer))).toEqual(offer)
 const legacy='PL1.'+base64(deflateSync(JSON.stringify(offer))).replaceAll('+','-').replaceAll('/','_').replace(/=+$/,'');expect(await decodePackage(legacy)).toEqual(offer)
 expect(await comparison(offer,answer)).toMatch(/^\d{3} \d{3}$/)
 const changed=await makePackage(b,identity(b,'Phone'),'answer',offer.id,sdp,'b'.repeat(64));expect(await comparison(offer,answer)).not.toBe(await comparison(offer,changed))
 await expect(validatePackage({...offer,sdp:sdp.replace('abc','tampered')})).rejects.toThrow('LOCAL_CODE_INVALID')
 await expect(validatePackage({...offer,peer:{...offer.peer,signing_key:b.signingKey}})).rejects.toThrow('LOCAL_CODE_INVALID')
})
it('rejects decompression bombs, media SDP and malformed connection packages',async()=>{
 setup();const p=await newProfile(),offer=await makePackage(p,identity(p,'Laptop'),'offer',crypto.randomUUID(),sdp)
 const z=deflateSync('x'.repeat(1000000)),bomb='PL1.'+base64(z).replaceAll('+','-').replaceAll('/','_').replace(/=+$/,'')
 await expect(decodePackage(bomb)).rejects.toThrow('LOCAL_CODE_INVALID')
 await expect(decodePackage('PL1.notvalid')).rejects.toThrow('LOCAL_CODE_INVALID')
 await expect(validatePackage({...offer,sdp:sdp+'m=audio 9 UDP/TLS/RTP/SAVPF 111\r\n'})).rejects.toThrow('LOCAL_CODE_INVALID')
})
it('domain separates receipts, connection identities and control-state signatures',async()=>{
 setup();const p=await newProfile(),receipt=await signed(p,'receipt',{id:crypto.randomUUID(),root:'a'.repeat(64)})
 expect(await verified(p.signingKey,'receipt',receipt)).toBe(true)
 expect(await verified(p.signingKey,'state',receipt)).toBe(false)
 const tampered={...receipt,root:'b'.repeat(64)};expect(await verified(p.signingKey,'receipt',tampered)).toBe(false)
})
it('binds bounded peer capabilities to the signed connection without advertising them by default',async()=>{
 setup();const p=await newProfile(),legacy=identity(p,'Laptop');expect(legacy.capabilities).toBeUndefined()
 const offered=await makePackage(p,{...legacy,capabilities:['text-v1']},'offer',crypto.randomUUID(),sdp)
 expect((await validatePackage(offered)).peer.capabilities).toEqual(['text-v1'])
 await expect(validatePackage({...offered,peer:{...offered.peer,capabilities:[]}})).rejects.toThrow('LOCAL_CODE_INVALID')
 const duplicate=await makePackage(p,{...legacy,capabilities:['text-v1','text-v1']},'offer',crypto.randomUUID(),sdp)
 await expect(validatePackage(duplicate)).rejects.toThrow('LOCAL_CODE_INVALID')
})
it('signs the service fallback together with the local identity and SDP',async()=>{
 setup();const p=await newProfile(),rendezvous={origin:'https://pinelink.example',token:'a'.repeat(22),expires_at:'2099-01-01T00:00:00Z'}
 const offer=await makePackage(p,identity(p,'Laptop'),'offer',crypto.randomUUID(),sdp,'',rendezvous)
 expect((await decodePackage(await encodePackage(offer))).rendezvous).toEqual(rendezvous)
 await expect(validatePackage({...offer,rendezvous:{...rendezvous,token:'b'.repeat(22)}})).rejects.toThrow('LOCAL_CODE_INVALID')
 await expect(validatePackage({...offer,rendezvous:undefined})).rejects.toThrow('LOCAL_CODE_INVALID')
 const bad=await makePackage(p,identity(p,'Laptop'),'offer',crypto.randomUUID(),sdp,'',{...rendezvous,origin:'https://['})
 await expect(validatePackage(bad)).rejects.toThrow('LOCAL_CODE_INVALID')
 const numeric={...offer,rendezvous:{...rendezvous,token:-1e20 as unknown as string}}
 await expect(validatePackage(numeric)).rejects.toThrow('LOCAL_CODE_INVALID')
})
