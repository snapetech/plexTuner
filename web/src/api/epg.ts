import { api } from './client'

export interface EPGAccount {
  id: number
  name: string
  source_type: 'xmltv' | 'sd' | 'dummy'
  url?: string
  api_key?: string
  refresh_interval_hrs: number
  refresh_cron?: string
  priority: number
  is_active: boolean
  dummy_config_json?: string
  last_refreshed_at?: string
  created_at: string
}

export interface EPGAccountInput {
  name: string
  source_type: 'xmltv' | 'sd' | 'dummy'
  url?: string
  api_key?: string
  refresh_interval_hrs: number
  refresh_cron?: string
  priority: number
  is_active: boolean
  dummy_config_json?: string
}

export const epgAccountsApi = {
  list: () => api.get<EPGAccount[]>('/api/v2/epg-accounts'),
  get: (id: number) => api.get<EPGAccount>(`/api/v2/epg-accounts/${id}`),
  create: (data: EPGAccountInput) => api.post<EPGAccount>('/api/v2/epg-accounts', data),
  update: (id: number, data: Partial<EPGAccountInput>) =>
    api.patch<EPGAccount>(`/api/v2/epg-accounts/${id}`, data),
  delete: (id: number) => api.del(`/api/v2/epg-accounts/${id}`),
  refresh: (id: number) => api.post(`/api/v2/epg-accounts/${id}/refresh`, {}),
  preview: (config: string, sample: string) =>
    api.post<{ preview: string; note?: string }>('/api/v2/epg-accounts/preview', { config, sample }),
}
