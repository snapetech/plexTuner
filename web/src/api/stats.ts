import { api } from './client'

export interface ActiveStreamState {
  request_id: string
  channel_id: string
  guide_name?: string
  guide_number?: string
  client_ua?: string
  started_at: string
  duration_ms: number
  cancelable: boolean
  cancel_requested?: boolean
}

export interface ActiveStreamsReport {
  generated_at: string
  in_use: number
  tuner_limit: number
  active: ActiveStreamState[]
}

export interface SystemEvent {
  id: number
  at: string
  level: 'info' | 'warn' | 'error'
  source?: string
  message: string
  detail?: string
}

export interface EventHook {
  id: number
  name: string
  event_types: string[]
  kind: 'webhook' | 'script'
  target: string
  enabled: boolean
  created_at: string
}

export interface EventHookInput {
  name: string
  event_types: string[]
  kind: 'webhook' | 'script'
  target: string
  enabled: boolean
}

export const statsApi = {
  activeStreams: () => api.get<ActiveStreamsReport>('/api/v2/stats/active-streams'),
  stopStream: (requestId: string) =>
    api.post('/api/v2/stats/stream-stop', { request_id: requestId }),
  systemEvents: (opts?: { level?: string; source?: string; limit?: number }) => {
    const params = new URLSearchParams()
    if (opts?.level) params.set('level', opts.level)
    if (opts?.source) params.set('source', opts.source)
    if (opts?.limit) params.set('limit', String(opts.limit))
    const qs = params.toString()
    return api.get<SystemEvent[]>(`/api/v2/stats/system-events${qs ? '?' + qs : ''}`)
  },
}

export const connectionsApi = {
  list: () => api.get<EventHook[]>('/api/v2/connections'),
  create: (data: EventHookInput) => api.post<EventHook>('/api/v2/connections', data),
  update: (id: number, data: EventHookInput) =>
    api.patch<EventHook>(`/api/v2/connections/${id}`, data),
  delete: (id: number) => api.del(`/api/v2/connections/${id}`),
}
