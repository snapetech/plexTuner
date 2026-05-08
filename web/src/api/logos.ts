import { api } from './client'
import { boot } from './client'

export interface Logo {
  id: number
  filename: string
  content_type: string
  size_bytes: number
  created_at: string
  url?: string
}

export const logosApi = {
  list: () => api.get<Logo[]>('/api/v2/logos'),
  delete: (id: number) => api.del(`/api/v2/logos/${id}`),
  upload: async (file: File): Promise<Logo> => {
    const form = new FormData()
    form.append('file', file)
    const headers: Record<string, string> = {}
    const csrf = boot().csrf
    if (csrf) headers['X-IPTVTunerr-CSRF'] = csrf
    const res = await fetch('/api/v2/logos', { method: 'POST', headers, body: form })
    if (!res.ok) throw new Error(`Upload failed: ${res.status}`)
    return res.json()
  },
}
