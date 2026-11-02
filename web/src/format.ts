import { translate } from './i18n'
import type { Locale } from './i18n'

export function formatBytes(bytes: number, locale: Locale = 'zh-CN') {
  if (!Number.isFinite(bytes) || bytes < 0) return translate(locale, 'sizeUnknown')
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / 1024 / 1024).toFixed(1)} MB`
  return `${(bytes / 1024 / 1024 / 1024).toFixed(1)} GB`
}

export function displayFileName(name: string | undefined, locale: Locale = 'zh-CN') {
  return name === 'encrypted' ? translate(locale, 'encryptedFile') : name
}

export function fileMetadataLabel(name: string | undefined, size: number | undefined, locale: Locale = 'zh-CN') {
  if (name === 'encrypted') return `${translate(locale, 'encryptedNameHint')}${size == null ? '' : ` · ${formatBytes(size, locale)}`}`
  return size == null ? translate(locale, 'fileSizePending') : formatBytes(size, locale)
}

export function formatTime(value: string, locale: Locale = 'zh-CN') {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? translate(locale, 'timeUnknown') : new Intl.DateTimeFormat(locale, { month: 'numeric', day: 'numeric', hour: '2-digit', minute: '2-digit' }).format(date)
}
