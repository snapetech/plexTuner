import { api } from './client'

export interface VodStream {
  num?: number
  name: string
  stream_type: string
  stream_id: string | number
  stream_icon?: string
  category_id?: string
  category_name?: string
  container_extension?: string
  direct_source?: string
}

export interface SeriesStream {
  num?: number
  name: string
  series_id: string | number
  cover?: string
  plot?: string
  genre?: string
  category_id?: string
  category_name?: string
  last_modified?: string
  rating?: string
  release_date?: string
}

export interface VodCategory {
  category_id: string
  category_name: string
}

export const vodsApi = {
  movies: () => api.get<VodStream[]>('/api/v2/vods?kind=movies'),
  series: () => api.get<SeriesStream[]>('/api/v2/vods?kind=series'),
  movieCategories: () => api.get<VodCategory[]>('/api/v2/vods?kind=categories'),
  seriesCategories: () => api.get<VodCategory[]>('/api/v2/vods?kind=series-categories'),
}
