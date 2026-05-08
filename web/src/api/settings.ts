import { api } from './client'

export type Settings = Record<string, string>

export interface StreamProfile {
  id: number
  name: string
  type: string
  config_json?: string
  is_default: boolean
  created_at: string
}

export interface StreamProfileInput {
  name: string
  type: string
  config_json?: string
  is_default: boolean
}

export const settingsApi = {
  get: () => api.get<Settings>('/api/v2/settings'),
  patch: (patch: Partial<Settings>) => api.patch<Settings>('/api/v2/settings', patch),
}

export const streamProfilesApi = {
  list: () => api.get<StreamProfile[]>('/api/v2/stream-profiles'),
  create: (data: StreamProfileInput) => api.post<StreamProfile>('/api/v2/stream-profiles', data),
  update: (id: number, data: StreamProfileInput) => api.patch<StreamProfile>(`/api/v2/stream-profiles/${id}`, data),
  delete: (id: number) => api.del(`/api/v2/stream-profiles/${id}`),
}
