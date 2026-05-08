import { api } from './client'

export interface GuideProgramme {
  title: string
  sub_title?: string
  desc?: string
  categories?: string[]
  start: string
  stop: string
  start_unix: number
  stop_unix: number
  icon?: string
}

export interface GuideChannel {
  epg_id: string
  name: string
  icon?: string
  programmes: GuideProgramme[]
}

export interface GuideGridResponse {
  from: string
  to: string
  channels: GuideChannel[]
}

export const guideApi = {
  grid: (from: Date, to: Date) =>
    api.get<GuideGridResponse>(
      `/api/v2/guide?from=${encodeURIComponent(from.toISOString())}&to=${encodeURIComponent(to.toISOString())}`
    ),
}
