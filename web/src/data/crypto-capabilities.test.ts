import {webcrypto} from 'node:crypto'
import {afterEach,expect,it,vi} from 'vitest'
import {assertCryptoContext,newProfile} from './crypto'

afterEach(()=>{vi.restoreAllMocks();vi.unstubAllGlobals()})
it('unsupported crypto asks for a newer browser without falling back to extractable identity keys',async()=>{
 vi.stubGlobal('crypto',webcrypto)
 const generate=vi.spyOn(webcrypto.subtle,'generateKey').mockRejectedValueOnce(new DOMException('Algorithm: Unrecognized name','NotSupportedError'))
 await expect(newProfile()).rejects.toThrow('BROWSER_CRYPTO_UNAVAILABLE')
 expect(generate).toHaveBeenCalledWith('Ed25519',false,['sign','verify'])
})
it('other key-generation failures retain their cause instead of being called unsupported',async()=>{
 vi.stubGlobal('crypto',webcrypto)
 const failure=new DOMException('Operation interrupted','OperationError')
 vi.spyOn(webcrypto.subtle,'generateKey').mockRejectedValueOnce(failure)
 await expect(newProfile()).rejects.toBe(failure)
})
it('an insecure origin is rejected before creating a local identity',()=>{
 vi.stubGlobal('crypto',webcrypto);vi.stubGlobal('isSecureContext',false)
 const generate=vi.spyOn(webcrypto.subtle,'generateKey')
 expect(()=>assertCryptoContext()).toThrow('BROWSER_CONTEXT_UNAVAILABLE')
 expect(generate).not.toHaveBeenCalled()
})
