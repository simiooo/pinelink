import { File } from 'lucide-react'

export function FileIcon({ extension }: { extension?: string }) {
  return <div className="file-icon" aria-hidden="true"><File size={22} strokeWidth={1.5} /><span>{extension?.slice(0, 4).toUpperCase()}</span></div>
}
