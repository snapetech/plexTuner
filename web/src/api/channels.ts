import { api } from './client'

export interface Channel {
  id: number
  name: string
  channel_number: string
  group_id?: number
  group_name: string
  stream_profile: string
  logo_id?: number
  tvg_id: string
  gracenote_id: string
  epg_id?: number
  epg_name: string
  user_level: string
  mature: boolean
  enabled: boolean
  sort_order: number
  streams?: Stream[]
  created_at: string
  updated_at: string
}

export interface Stream {
  id: number
  channel_id?: number
  m3u_account?: number
  m3u_name: string
  url: string
  name: string
  position: number
  stale: boolean
  stats?: StreamStats
  created_at: string
}

export interface StreamStats {
  resolution?: string
  fps?: number
  video_codec?: string
  audio_codec?: string
  bitrate_kbps?: number
}

export interface ChannelProfile {
  id: number
  name: string
  created_at: string
}

export interface ChannelGroup {
  id: number
  name: string
  sort_order: number
  created_at: string
}

export interface ChannelListResponse {
  total: number
  channels: Channel[]
}

export interface StreamListResponse {
  total: number
  streams: Stream[]
}

export interface ChannelListOpts {
  search?: string
  group_id?: number
  profile_id?: number
  only_empty?: boolean
  page?: number
  per_page?: number
}

export interface StreamListOpts {
  search?: string
  account_id?: number
  unassigned?: boolean
  hide_stale?: boolean
  page?: number
  per_page?: number
}

function toQS(opts: Record<string, unknown>): string {
  const params = new URLSearchParams()
  for (const [k, v] of Object.entries(opts)) {
    if (v !== undefined && v !== null && v !== '' && v !== false) {
      params.set(k, String(v))
    }
  }
  const s = params.toString()
  return s ? '?' + s : ''
}

export const channelsApi = {
  list: (opts: ChannelListOpts = {}) =>
    api.get<ChannelListResponse>('/api/v2/channels' + toQS(opts as Record<string, unknown>)),

  get: (id: number) =>
    api.get<Channel>(`/api/v2/channels/${id}`),

  create: (ch: Partial<Channel>) =>
    api.post<Channel>('/api/v2/channels', ch),

  update: (id: number, ch: Partial<Channel>) =>
    api.patch<Channel>(`/api/v2/channels/${id}`, ch),

  delete: (id: number) =>
    api.del<void>(`/api/v2/channels/${id}`),

  reorder: (ids: number[]) =>
    api.post<void>('/api/v2/channels/reorder', ids),

  bulk: (ids: number[], update: Partial<Channel> & { clear_epg?: boolean }) =>
    api.post<void>('/api/v2/channels/bulk', { ids, update }),

  addStream: (channelId: number, stream: Partial<Stream>) =>
    api.post<Stream>(`/api/v2/channels/${channelId}/streams`, stream),
}

export const streamsApi = {
  list: (opts: StreamListOpts = {}) =>
    api.get<StreamListResponse>('/api/v2/streams' + toQS(opts as Record<string, unknown>)),

  create: (st: Partial<Stream>) =>
    api.post<Stream>('/api/v2/streams', st),

  delete: (id: number) =>
    api.del<void>(`/api/v2/streams/${id}`),

  assign: (id: number, channelId: number) =>
    api.post<void>(`/api/v2/streams/${id}/assign`, { channel_id: channelId }),
}

export const profilesApi = {
  list: () => api.get<ChannelProfile[]>('/api/v2/channel-profiles'),
  create: (name: string) => api.post<ChannelProfile>('/api/v2/channel-profiles', { name }),
  rename: (id: number, name: string) => api.patch<void>(`/api/v2/channel-profiles/${id}`, { name }),
  delete: (id: number) => api.del<void>(`/api/v2/channel-profiles/${id}`),
  duplicate: (id: number, name: string) =>
    api.post<ChannelProfile>(`/api/v2/channel-profiles/${id}/duplicate`, { name }),
}

export const groupsApi = {
  list: () => api.get<ChannelGroup[]>('/api/v2/channel-groups'),
  create: (name: string) => api.post<ChannelGroup>('/api/v2/channel-groups', { name }),
  update: (id: number, name: string) => api.patch<void>(`/api/v2/channel-groups/${id}`, { name }),
  delete: (id: number) => api.del<void>(`/api/v2/channel-groups/${id}`),
}

export interface AutoMatchOpts {
  channel_ids?: number[]
  ignore_prefixes?: string[]
  ignore_suffixes?: string[]
  ignore_strings?: string[]
}

export interface AutoMatchResult {
  matched: number
  skipped: number
  total: number
}

export const autoMatchApi = {
  run: (opts: AutoMatchOpts) =>
    api.post<AutoMatchResult>('/api/v2/channels/automatch', opts),
}
