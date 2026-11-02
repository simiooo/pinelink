import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiError, getDevices, getFile, getQuota, getTransfers, IDENTITY, promoteFile, revokeDevice } from '../api/client'
import { Courier } from '../components/Courier'
import { displayFileName, formatBytes, formatTime } from '../format'
import { useI18n } from '../i18n'

export function SettingsPage() {
  const client = useQueryClient()
  const { t, locale } = useI18n()
  const devices = useQuery({ queryKey: ['devices'], queryFn: getDevices })
  const quota = useQuery({ queryKey: ['quota'], queryFn: getQuota })
  const transfers = useQuery({ queryKey: ['transfers'], queryFn: getTransfers })
  const owned = [...new Map((transfers.data ?? []).filter(item => item.sender_owner_id === IDENTITY.owner && item.entry_id).map(item => [item.entry_id!, item])).values()]
  const [entry, setEntry] = useState<string>()
  const selectedEntry = entry ?? owned[0]?.entry_id
  const file = useQuery({ queryKey: ['file', selectedEntry], queryFn: () => getFile(selectedEntry!), enabled: !!selectedEntry, retry: false })
  const current = devices.data?.find(item => item.device_id === IDENTITY.device)
  const [device, setDevice] = useState<string>()
  const selectedDevice = device ?? current?.device_id ?? devices.data?.find(item => item.status === 'ACTIVE')?.device_id
  const [confirm, setConfirm] = useState(false)
  const [notice, setNotice] = useState('')
  const [saved, setSaved] = useState<string>()
  const refresh = () => Promise.all([client.invalidateQueries({ queryKey: ['devices'] }), client.invalidateQueries({ queryKey: ['quota'] }), client.invalidateQueries({ queryKey: ['file'] })])
  const promote = useMutation({
    mutationFn: ({ id, version }: { id: string; version: number }) => promoteFile(id, version),
    onSuccess: (_, target) => { setSaved(target.id); setNotice(t('savedPersistent')) },
    onError: error => setNotice(error instanceof ApiError && [401, 403].includes(error.status) ? t('permissionDenied') : error instanceof ApiError && error.status === 410 ? t('errFileExpired') : error instanceof ApiError && error.code === 'STATE_CONFLICT' ? t('fileConflict') : error instanceof ApiError && error.status === 409 ? t('fileIncompleteOrQuota') : t('storageFailed')),
    onSettled: refresh,
  })
  const revoke = useMutation({
    mutationFn: (id: string) => revokeDevice(id),
    onSuccess: () => { setNotice(t('revokedSuccess')); setConfirm(false) },
    onError: () => setNotice(t('revokeFailed')),
    onSettled: refresh,
  })
  const used = quota.data?.used_bytes ?? 0
  const capacity = quota.data?.quota_bytes ?? 0
  const percentage = capacity > 0 ? Math.min(100, Math.round(used / capacity * 100)) : 0
  return (
    <section aria-labelledby="settings-title">
      <div className="page-heading"><h1 id="settings-title">{t('settingsTitle')}</h1><p>{t('settingsSubtitle')}</p></div>
      {notice && <p className="feedback" role="status">{notice}</p>}
      <h2 className="section-title">{t('thisDevice')}</h2>
      <article className="settings-card current-device">
        <strong>{IDENTITY.device}</strong>
        <span>{current ? t(current.status === 'ACTIVE' ? 'currentDeviceRegistered' : 'currentDeviceRevoked') : t('currentDeviceUnknown')}</span>
        <span>{t('account', { owner: IDENTITY.owner })}</span>
      </article>
      <h2 className="section-title">{t('storageSpace')}</h2>
      <article className="settings-card">
        {quota.isError ? <span>{t('quotaError')}</span> : <>
          <strong>{formatBytes(used, locale)} / {capacity > 0 ? formatBytes(capacity, locale) : t('quotaUnknown')}</strong>
          <div className="progress-track" role="progressbar" aria-label={t('quotaLabel')} aria-valuemin={0} aria-valuemax={100} aria-valuenow={percentage}><span style={{ width: `${percentage}%` }} /></div>
          {percentage >= 80 && <span className="error-note">{t('quotaWarning')}</span>}
          <span>{t('storageNote')}</span>
        </>}
      </article>
      <h2 className="section-title">{t('fileStorage')}</h2>
      {owned.length === 0 ? <div className="state-panel">{t('noOwnedFiles')}</div> : <>
        <label className="request-picker">{t('chooseFileLabel')}<select value={selectedEntry} onChange={event => { setEntry(event.target.value); setSaved(undefined); setNotice('') }}>{owned.map(item => <option key={item.entry_id} value={item.entry_id}>{displayFileName(item.file_name, locale) ?? t('readingFile')}</option>)}</select></label>
        <div className="storage-action">
          {saved === selectedEntry && <Courier action="persist" label={t('persistedCourier')} />}
          <div><strong>{displayFileName(file.data?.file_name, locale) ?? t('readingFile')}</strong><p>{file.isError ? t('fileStorageError') : file.data?.retention_class === 'PERSISTENT' ? t('persistedFile') : file.data?.expires_at ? t('temporaryUntil', { time: formatTime(file.data.expires_at, locale) }) : t('storageAvailable')}</p></div>
          {file.data && file.data.retention_class !== 'PERSISTENT' && <button className="secondary-button" disabled={promote.isPending || file.data.state !== 'AVAILABLE'} onClick={() => promote.mutate({ id: selectedEntry!, version: file.data!.version })}>{t('savePersistent')}</button>}
        </div>
        {(file.isError || promote.isError) && <Courier action="deny" label={t('deniedCourier')} />}
      </>}
      <h2 className="section-title">{t('devicesTitle')}</h2>
      {devices.isPending && <div className="state-panel">{t('loadingDevices')}</div>}
      {devices.isError && <div className="state-panel" role="alert">{t('devicesError')}</div>}
      {devices.data && <label className="request-picker">{t('recipientDevice')}<select value={selectedDevice ?? ''} onChange={event => { setDevice(event.target.value); setConfirm(false); setNotice('') }}>{devices.data.length === 0 && <option value="">{t('noDevices')}</option>}{devices.data.map(item => <option value={item.device_id} key={item.device_id}>{item.device_id} · {item.status === 'ACTIVE' ? t('deviceUsable') : t('currentDeviceRevoked')} · {formatTime(item.last_seen_at, locale)}</option>)}</select></label>}
      {selectedDevice && devices.data?.find(item => item.device_id === selectedDevice)?.status === 'ACTIVE' && <details className="context-menu"><summary className="secondary-button" role="button">{t('manageDevice')}</summary><div><p>{t('revokeHint', { device: selectedDevice })}</p><button className="secondary-button" disabled={revoke.isPending} onClick={() => {
        if (confirm) revoke.mutate(selectedDevice)
        else { setConfirm(true); setNotice(t('revokeHint', { device: selectedDevice })) }
      }}>{t(confirm ? 'confirmRevoke' : 'revokeDevice')}</button></div></details>}
    </section>
  )
}
