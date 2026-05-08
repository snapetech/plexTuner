import { api } from './client'

export interface User {
  id: number
  username: string
  role: 'admin' | 'standard' | 'streamer'
  xc_password?: string
  hide_mature: boolean
  stream_limit: number
  epg_days_back: number
  epg_days_fwd: number
  created_at: string
  profile_ids: number[]
}

export interface UserInput {
  username: string
  password?: string
  role: 'admin' | 'standard' | 'streamer'
  xc_password?: string
  hide_mature: boolean
  stream_limit: number
  epg_days_back: number
  epg_days_fwd: number
  profile_ids: number[]
}

export const usersApi = {
  list: () => api.get<User[]>('/api/v2/users'),
  create: (data: UserInput) => api.post<User>('/api/v2/users', data),
  update: (id: number, data: UserInput) => api.patch<User>(`/api/v2/users/${id}`, data),
  delete: (id: number) => api.del(`/api/v2/users/${id}`),
}
