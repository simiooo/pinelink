import {cleanup,render,screen} from '@testing-library/react'
import {afterEach,expect,it,vi} from 'vitest'
import {App} from '../main'
const open=vi.hoisted(()=>vi.fn())
vi.mock('./runtime',()=>({getGuestRuntime:open}))
afterEach(()=>{cleanup();vi.clearAllMocks()})
it('a missing crypto algorithm offers the Android app instead of a storage error',async()=>{
 open.mockRejectedValue(new Error('BROWSER_CRYPTO_UNAVAILABLE'))
 render(<App initialEntries={['/']}/>);
 await screen.findByRole('heading',{name:'请更新浏览器'})
 expect(screen.getByRole('link',{name:'Android 应用'})).toHaveAttribute('href','https://github.com/simiooo/pinelink/releases/tag/v0.1.0-preview.13')
 expect(screen.queryByText('无法在本机保存')).not.toBeInTheDocument()
})
it('a storage failure keeps the storage recovery action',async()=>{
 open.mockRejectedValue(new DOMException('IndexedDB denied','SecurityError'))
 render(<App initialEntries={['/']}/>);
 await screen.findByRole('heading',{name:'无法在本机保存'})
 expect(screen.getByRole('button',{name:'重试'})).toBeInTheDocument()
 expect(screen.queryByRole('link',{name:'Android 应用'})).not.toBeInTheDocument()
})
