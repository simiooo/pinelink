import { expect, it } from 'vitest'
import { parseConnectionInput } from './connection-input'

it('distinguishes local packages, codes and same-service links without changing tokens', () => {
  const origin = 'https://example.test', token = 'AbCdEf0123456789_-1234'
  expect(() => parseConnectionInput('pl2:abc:', origin)).toThrow()
  expect(parseConnectionInput(' PL2:AbC: ', origin)).toEqual({ kind: 'local', value: 'PL2:AbC:' })
  expect(parseConnectionInput('PL1.aBc_1', origin)).toEqual({ kind: 'local', value: 'PL1.aBc_1' })
  expect(parseConnectionInput('k7m4 8f2q', origin)).toEqual({ kind: 'online', value: 'K7M48F2Q' })
  expect(parseConnectionInput(token, origin)).toEqual({ kind: 'online', value: token })
  expect(parseConnectionInput(`${origin}/connect#${token}`, origin)).toEqual({ kind: 'online', value: token })
  expect(() => parseConnectionInput(`https://other.test/connect#${token}`, origin)).toThrow()
  expect(() => parseConnectionInput(`https://user@example.test/connect#${token}`, origin)).toThrow()
  expect(() => parseConnectionInput('PL2:' + 'x'.repeat(18001), origin)).toThrow()
})
