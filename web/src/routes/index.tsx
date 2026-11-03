import { MotionButton } from '../motion/components'
import { MotionDropzone } from '../motion/dropzone'
import { useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { getTasks, IDENTITY } from '../api/client'
import { formatBytes } from '../format'
import { SendPreparation } from '../components/SendPreparation'
import { useI18n } from '../i18n'

export function NewTransferPage() {
  const inputRef = useRef<HTMLInputElement>(null)
  const [file, setFile] = useState<File | null>(null)
  const [preparing, setPreparing] = useState(false)
  const { t, locale } = useI18n()
  const tasks = useQuery({ queryKey: ['tasks', IDENTITY.owner, IDENTITY.device], queryFn: () => getTasks(IDENTITY.owner, IDENTITY.device) })

  const choose = (selected?: File) => {
    if (selected) setFile(selected)
    else inputRef.current?.click()
  }

  if (file && preparing) return <SendPreparation name={file.name} size={file.size} />

  return (
    <section className="transfer-page" aria-labelledby="transfer-title">
      <div className="page-heading">
        <h1 id="transfer-title">{t('homeTitle')}</h1>
        <p>{t('homeSubtitle')}</p>
      </div>
      <MotionDropzone onFile={selected => choose(selected)}>
        <div className="upload-icon" aria-hidden="true">↓</div>
        <div className="drop-copy">
          <strong>{file ? file.name : t('dropTitle')}</strong>
          <span>{file ? `${formatBytes(file.size, locale)} · ${t('fileReady')}` : t('dropHint')}</span>
        </div>
        <input ref={inputRef} type="file" hidden onChange={event => choose(event.target.files?.[0])} />
        <MotionButton className="primary-button" onClick={() => file ? setPreparing(true) : choose()}>{t(file ? 'continue' : 'chooseFile')}</MotionButton>
        <small>{t('fileSupport')}</small>
      </MotionDropzone>
      <aside className="privacy-note">
        <p>{t('relayNote')}</p>
        <p>{t('persistenceNote')}</p>
      </aside>
      <section className="recent-section" aria-labelledby="recent-title">
        <h2 id="recent-title">{t('recentTasks')}</h2>
        <div className="recent-empty">
          {tasks.isPending && t('loadingTasks')}
          {tasks.isError && t('tasksError')}
          {tasks.data && tasks.data.tasks.length === 0 && t('noTasks')}
          {tasks.data && tasks.data.tasks.length > 0 && t('taskCount', { count: tasks.data.tasks.length })}
        </div>
      </section>
    </section>
  )
}
