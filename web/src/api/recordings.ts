import { api } from './client'

export interface Recording {
  id: number
  channel_id?: number
  channel_name?: string
  title: string
  start_at: string
  end_at: string
  recurring: boolean
  rule_id?: number
  status: 'scheduled' | 'recording' | 'done' | 'failed'
  file_path?: string
  created_at: string
}

export interface RecordingInput {
  channel_id?: number
  title: string
  start_at: string
  end_at: string
  recurring?: boolean
}

export interface RecordingRule {
  id: number
  channel_id?: number
  channel_name?: string
  title: string
  days: number[]
  start_time: string
  end_time: string
  start_date?: string
  end_date?: string
  is_active: boolean
  created_at: string
}

export interface RecordingRuleInput {
  channel_id?: number
  title: string
  days: number[]
  start_time: string
  end_time: string
  start_date?: string
  end_date?: string
  is_active: boolean
}

export const recordingsApi = {
  list: (status?: string) =>
    api.get<Recording[]>(`/api/v2/recordings${status ? `?status=${status}` : ''}`),
  create: (data: RecordingInput) => api.post<Recording>('/api/v2/recordings', data),
  delete: (id: number) => api.del(`/api/v2/recordings/${id}`),

  listRules: () => api.get<RecordingRule[]>('/api/v2/recording-rules'),
  createRule: (data: RecordingRuleInput) =>
    api.post<RecordingRule>('/api/v2/recording-rules', data),
  updateRule: (id: number, data: RecordingRuleInput) =>
    api.patch<RecordingRule>(`/api/v2/recording-rules/${id}`, data),
  deleteRule: (id: number) => api.del(`/api/v2/recording-rules/${id}`),
}
