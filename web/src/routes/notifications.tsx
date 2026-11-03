import { NativeSelect } from '../components/ui/native-select'
import { MotionButton, MotionReveal } from '../motion/components'
import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiError, getPendingNotifications, IDENTITY, respondTransfer } from '../api/client'
import { FileIcon } from '../components/FileIcon'
import { displayFileName, fileMetadataLabel } from '../format'
import { useI18n } from '../i18n'

export function NotificationsPage() {
  const client = useQueryClient()
  const { t, locale } = useI18n()
  const [selected, setSelected] = useState<string>()
  const [notice, setNotice] = useState('')
  const pending = useQuery({
    queryKey: ['notifications', IDENTITY.owner, IDENTITY.device],
    queryFn: () => getPendingNotifications(),
    refetchInterval: 5000,
  })
  const mutation = useMutation({
    mutationFn: ({ id, action, version }: { id: string; action: 'accept' | 'reject'; version: number }) => respondTransfer(id, action, IDENTITY.owner, IDENTITY.device, version),
    onSuccess: (_, { action }) => setNotice(t(action === 'accept' ? 'accepted' : 'rejected')),
    onError: error => setNotice(error instanceof ApiError && error.status === 410 ? t('requestExpired') : error instanceof ApiError && error.status === 409 ? t('notificationStateChanged') : t('actionFailed')),
    onSettled: () => Promise.all([
      client.invalidateQueries({ queryKey: ['notifications'] }),
      client.invalidateQueries({ queryKey: ['tasks'] }),
    ]),
  })

  const requests = pending.data?.transfers ?? []
  const first = requests.find(request => request.id === selected) ?? requests[0]
  return (
    <section className="notifications-page" aria-labelledby="notifications-title">
      <div className="page-heading">
        <h1 id="notifications-title">{t('notificationTitle')}</h1>
        <p>{t('notificationSubtitle')}</p>
      </div>
      {pending.isPending && <div className="state-panel" role="status">{t('loadingNotifications')}</div>}
      {pending.isError && <div className="state-panel" role="alert">{t('notificationsError')}</div>}
      {notice && <MotionReveal className="feedback" role="status" identity={notice}>{notice}</MotionReveal>}
      {!pending.isPending && !pending.isError && requests.length > 1 && (
        <label className="request-picker">{t('selectRequest')}
          <NativeSelect value={first?.id} onChange={event => setSelected(event.target.value)}>
            {requests.map(request => <option key={request.id} value={request.id}>{displayFileName(request.file_name, locale) ?? t('unknownFile')} · {request.sender_device_id}</option>)}
          </NativeSelect>
        </label>
      )}
      {first ? (
        <>
          <article className="notification-card">
            <div className="file-summary">
              <FileIcon extension={first.file_name === 'encrypted' ? undefined : first.file_name?.split('.').pop()} />
              <div className="file-summary-copy">
                <strong>{displayFileName(first.file_name, locale) ?? t('unknownFile')}</strong>
                <span>{fileMetadataLabel(first.file_name, first.size_bytes, locale)}</span>
              </div>
            </div>
            <dl className="metadata-list">
              <div><dt>{t('senderDevice')}</dt><dd>{first.sender_device_id}</dd></div>
              <div><dt>{t('senderAccount')}</dt><dd>{first.sender_owner_id}</dd></div>
            </dl>
            <p className="consent-note">{t('consentNote')}</p>
          </article>
          <div className="page-actions">
            <MotionButton className="secondary-button" disabled={mutation.isPending} onClick={() => mutation.mutateAsync({ id: first.id, action: 'reject', version: first.version })}>{t('reject')}</MotionButton>
            <MotionButton className="primary-button" disabled={mutation.isPending} onClick={() => mutation.mutateAsync({ id: first.id, action: 'accept', version: first.version })}>{t(mutation.isPending ? 'confirming' : 'accept')}</MotionButton>
          </div>
          <aside className="privacy-note">
            <p>{t('relayNote')}</p>
            <p>{t('persistentConsent')}</p>
          </aside>
        </>
      ) : !pending.isPending && !pending.isError && (
        <div className="state-panel empty-state">
          <strong>{t('emptyNotifications')}</strong>
          <p>{t('emptyNotificationsHint')}</p>
        </div>
      )}
    </section>
  )
}
