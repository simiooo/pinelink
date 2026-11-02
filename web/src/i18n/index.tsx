import { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react'
import type { ReactNode } from 'react'
import zh from '../../../i18n/zh-CN.json'
import en from '../../../i18n/en-US.json'

export type Locale = 'zh-CN' | 'en-US'
export type LocalePreference = Locale | 'system'
export type MessageKey = keyof typeof zh
export type TranslateParams = Record<string, string | number>
export type Translate = (key: MessageKey, params?: TranslateParams) => string

const messages: Record<Locale, Record<MessageKey, string>> = { 'zh-CN': zh, 'en-US': en }

export function translate(locale: Locale, key: MessageKey, params: TranslateParams = {}) {
  return messages[locale][key].replace(/\{(\w+)\}/g, (_, name: string) => String(params[name] ?? `{${name}}`))
}

type I18nValue = { locale: Locale; preference: LocalePreference; setPreference: (preference: LocalePreference) => void; t: Translate }
const I18nContext = createContext<I18nValue | null>(null)

function systemLocale(): Locale {
  return typeof navigator !== 'undefined' && navigator.languages.some(language => language.toLowerCase().startsWith('zh')) ? 'zh-CN' : 'en-US'
}

function storedPreference(): LocalePreference {
  try {
    const value = localStorage.getItem('pinelink.locale')
    return value === 'zh-CN' || value === 'en-US' || value === 'system' ? value : 'system'
  } catch {
    return 'system'
  }
}

export function I18nProvider({ children, initialPreference }: { children: ReactNode; initialPreference?: LocalePreference }) {
  const [preference, setPreferenceState] = useState<LocalePreference>(() => initialPreference ?? storedPreference())
  const [system, setSystem] = useState<Locale>(systemLocale)
  const locale = preference === 'system' ? system : preference

  useEffect(() => {
    const update = () => setSystem(systemLocale())
    window.addEventListener('languagechange', update)
    return () => window.removeEventListener('languagechange', update)
  }, [])

  useEffect(() => { document.documentElement.lang = locale }, [locale])

  const setPreference = useCallback((value: LocalePreference) => {
    setPreferenceState(value)
    try { localStorage.setItem('pinelink.locale', value) } catch { /* Storage is optional in restricted browser modes. */ }
  }, [])

  const t = useCallback<Translate>((key, params) => translate(locale, key, params), [locale])
  const value = useMemo(() => ({ locale, preference, setPreference, t }), [locale, preference, setPreference, t])
  return <I18nContext.Provider value={value}>{children}</I18nContext.Provider>
}

export function useI18n() {
  const value = useContext(I18nContext)
  if (!value) throw new Error('useI18n must be used within I18nProvider')
  return value
}
