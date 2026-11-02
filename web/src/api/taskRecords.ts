import { useQuery } from '@tanstack/react-query'
import { ApiError, getFile, getPull, getTasks, getTransfers } from './client'
import type { Task } from './client'

export type TaskRecord = Task & { fileName: string; encryptedMetadata: boolean; peer?: string }

export function useTaskRecords(includeDeleted = false) {
  return useQuery({
    queryKey: ['task-records', includeDeleted],
    queryFn: async (): Promise<TaskRecord[]> => {
      const [{ tasks }, transfers] = await Promise.all([getTasks(undefined, undefined, includeDeleted), getTransfers()])
      return Promise.all(tasks.map(async task => {
        const transfer = transfers.find(item => item.id === task.reference_id || (task.kind === 'UPLOAD' && item.entry_id === task.reference_id))
        let fileName = transfer?.file_name
        if (!fileName && task.kind === 'DOWNLOAD') {
          try { fileName = (await getPull(task.reference_id)).file_name } catch (error) {
            if (!(error instanceof ApiError && error.status === 404)) throw error
          }
        } else if (!fileName && task.kind === 'UPLOAD') {
          try { fileName = (await getFile(task.reference_id)).file_name } catch (error) {
            if (!(error instanceof ApiError && error.status === 404)) throw error
          }
        }
        return { ...task, fileName: fileName === 'encrypted' ? '' : fileName ?? '', encryptedMetadata: fileName === 'encrypted', peer: task.kind === 'DOWNLOAD' ? transfer?.sender_device_id : transfer?.recipient_device_id }
      }))
    },
    refetchInterval: 3000,
  })
}
