import { api } from './client'

export interface M3UAccount {
  id: number
  name: string
  account_type: 'standard' | 'xtream'
  url: string
  upload_path?: string
  expiration_date?: string
  max_streams: number
  user_agent?: string
  refresh_interval_hrs: number
  refresh_cron?: string
  stale_retention_days: number
  vod_scanning: boolean
  vod_priority: number
  is_active: boolean
  last_refreshed_at?: string
  created_at: string
  stream_count?: number
}

export interface M3UAccountInput {
  name: string
  account_type: 'standard' | 'xtream'
  url: string
  max_streams: number
  user_agent?: string
  refresh_interval_hrs: number
  refresh_cron?: string
  stale_retention_days: number
  vod_scanning: boolean
  is_active: boolean
}

export interface M3UFilter {
  id?: number
  account_id?: number
  field: 'group' | 'name' | 'url'
  pattern: string
  exclude: boolean
  case_sens: boolean
  position?: number
}

export interface M3UGroup {
  id: number
  account_id: number
  name: string
  enabled: boolean
  auto_channel_sync: boolean
  channel_numbering_mode: 'fixed' | 'provider' | 'next'
  start_channel_number?: number
  force_dummy_epg: boolean
  override_group?: string
  name_find_regex?: string
  name_replace?: string
  name_filter_regex?: string
  profile_ids: number[]
  sort_order_mode: 'provider' | 'alpha' | 'fixed'
  stream_profile?: string
  stream_count?: number
  created_at: string
}

export interface M3UAccountProfile {
  id: number
  account_id: number
  name: string
  username?: string
  password?: string
  search_pat?: string
  replace_pat?: string
  max_streams: number
}

export const m3uAccountsApi = {
  list: () => api.get<M3UAccount[]>('/api/v2/m3u-accounts'),
  get: (id: number) => api.get<M3UAccount>(`/api/v2/m3u-accounts/${id}`),
  create: (data: M3UAccountInput) => api.post<M3UAccount>('/api/v2/m3u-accounts', data),
  update: (id: number, data: Partial<M3UAccountInput>) =>
    api.patch<M3UAccount>(`/api/v2/m3u-accounts/${id}`, data),
  delete: (id: number) => api.del(`/api/v2/m3u-accounts/${id}`),
  refresh: (id: number) => api.post(`/api/v2/m3u-accounts/${id}/refresh`, {}),

  listFilters: (id: number) => api.get<M3UFilter[]>(`/api/v2/m3u-accounts/${id}/filters`),
  saveFilters: (id: number, filters: M3UFilter[]) =>
    api.post<M3UFilter[]>(`/api/v2/m3u-accounts/${id}/filters`, filters),

  listGroups: (id: number) => api.get<M3UGroup[]>(`/api/v2/m3u-accounts/${id}/groups`),
  updateGroup: (accountId: number, groupId: number, data: Partial<M3UGroup>) =>
    api.patch<M3UGroup>(`/api/v2/m3u-accounts/${accountId}/groups/${groupId}`, data),

  listProfiles: (id: number) =>
    api.get<M3UAccountProfile[]>(`/api/v2/m3u-accounts/${id}/profiles`),
  createProfile: (id: number, data: Omit<M3UAccountProfile, 'id' | 'account_id'>) =>
    api.post<M3UAccountProfile>(`/api/v2/m3u-accounts/${id}/profiles`, data),
}
