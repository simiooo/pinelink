import { useQuery } from '@tanstack/react-query'
import { ApiError, getFile, getPull, getTasks, getTransfers, IDENTITY } from './client'
import type { Task } from './client'
import { ClientRuntime, DataError } from '../data/runtime'

export type TaskRecord = Task & { fileName: string; encryptedMetadata: boolean; peer?: string }

export function useTaskRecords(includeDeleted = false, enabled = true, source?: { runtime?: ClientRuntime; owner: string; device: string }) {
  const owner = source?.owner ?? IDENTITY.owner, device = source?.device ?? IDENTITY.device, runtime = source?.runtime
  const query = `owner_id=${encodeURIComponent(owner)}&device_id=${encodeURIComponent(device)}`
  const read = async <T,>(path: string) => { try { return await runtime!.request<T>('GET', path) } catch (error) { if (error instanceof DataError) throw new ApiError(error.status, error.code); throw error } }
  return useQuery({
    queryKey: ['task-records', owner, device, includeDeleted],
    enabled,
    queryFn: async (): Promise<TaskRecord[]> => {
      const [{ tasks }, transfers] = await Promise.all([runtime ? read<{ tasks: Task[] | null }>(`/v1/tasks?${query}&include_deleted=${includeDeleted}`) : getTasks(undefined, undefined, includeDeleted), runtime ? read<{ transfers: Awaited<ReturnType<typeof getTransfers>> | null }>(`/v1/transfers?${query}`).then(value => value.transfers ?? []) : getTransfers()])
      return Promise.all((tasks ?? []).map(async task => {
        const transfer = transfers.find(item => item.id === task.reference_id || (task.kind === 'UPLOAD' && item.entry_id === task.reference_id))
        let fileName = transfer?.file_name
        if (!fileName && task.kind === 'DOWNLOAD') {
          try { fileName = (runtime ? await read<{ file_name?: string }>(`/v1/pulls/${encodeURIComponent(task.reference_id)}?${query}`) : await getPull(task.reference_id)).file_name } catch (error) {
            if (!(error instanceof ApiError && error.status === 404)) throw error
          }
        } else if (!fileName && task.kind === 'UPLOAD') {
          try { fileName = (runtime ? await read<{ file_name?: string }>(`/v1/files/${encodeURIComponent(task.reference_id)}?${query}`) : await getFile(task.reference_id)).file_name } catch (error) {
            if (!(error instanceof ApiError && error.status === 404)) throw error
          }
        }
        return { ...task, fileName: fileName === 'encrypted' ? '' : fileName ?? '', encryptedMetadata: fileName === 'encrypted', peer: task.kind === 'DOWNLOAD' ? transfer?.sender_device_id : transfer?.recipient_device_id }
      }))
    },
    refetchInterval: 3000,
  })
}
