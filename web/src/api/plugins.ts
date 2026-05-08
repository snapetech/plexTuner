import { api } from './client'

export interface Plugin {
  id: number
  name: string
  version?: string
  description?: string
  enabled: boolean
  path: string
  manifest?: string
  created_at: string
}

export interface PluginInput {
  name: string
  version?: string
  description?: string
  path: string
  manifest?: string
  enabled: boolean
}

export const pluginsApi = {
  list: () => api.get<Plugin[]>('/api/v2/plugins'),
  create: (data: PluginInput) => api.post<Plugin>('/api/v2/plugins', data),
  update: (id: number, data: PluginInput) => api.patch<Plugin>(`/api/v2/plugins/${id}`, data),
  enable: (id: number) => api.post<Plugin>(`/api/v2/plugins/${id}/enable`, {}),
  disable: (id: number) => api.post<Plugin>(`/api/v2/plugins/${id}/disable`, {}),
  delete: (id: number) => api.del(`/api/v2/plugins/${id}`),
}
