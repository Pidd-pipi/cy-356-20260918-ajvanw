import { get, post } from '@/utils/request'
import type { UserInfo } from './auth'

export interface ActivePlanInfo {
  id: number
  crop_name: string
  status: string
  status_text: string
  user_id: number
  username: string
}

export interface Plot {
  id: number
  name: string
  code: string
  area: number
  soil_type: string
  sunlight: string
  latitude: number
  longitude: number
  status: string
  adopter_id: number | null
  adopter: UserInfo | null
  description: string
  active_plan: ActivePlanInfo | null
  created_at: string
}

export interface PlotPayload {
  name: string
  code: string
  area: number
  soil_type: string
  sunlight: string
  latitude: number
  longitude: number
  description?: string
}

export function listPlots(params?: Record<string, any>): Promise<{ list: Plot[]; total: number; page: number; page_size: number }> {
  return get('/plots', { params })
}

export function getPlot(id: number): Promise<Plot> {
  return get(`/plots/${id}`)
}

export function createPlot(payload: PlotPayload): Promise<Plot> {
  return post('/plots', payload)
}

export function adoptPlot(id: number): Promise<Plot> {
  return post(`/plots/${id}/adopt`)
}

export function releasePlot(id: number): Promise<Plot> {
  return post(`/plots/${id}/release`)
}
