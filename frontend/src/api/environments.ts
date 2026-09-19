import api from './client'

export interface Environment {
  id: string
  project_id: string
  name: string
  created_at?: string
  updated_at?: string
}

export type EnvironmentForm = Omit<Environment, 'id' | 'created_at' | 'updated_at'>

interface ApiResponse<T> {
  ok: boolean
  data: T
  error?: string
}

export const environmentApi = {
  async list(): Promise<Environment[]> {
    const response = await api.get<ApiResponse<Environment[]>>('/environments')
    return response.data.data || []
  },

  async create(form: EnvironmentForm): Promise<Environment> {
    const response = await api.post<ApiResponse<Environment>>('/environments', form)
    return response.data.data
  },

  async update(id: string, form: EnvironmentForm): Promise<Environment> {
    const response = await api.put<ApiResponse<Environment>>(`/environments/${encodeURIComponent(id)}`, form)
    return response.data.data
  },

  async remove(id: string): Promise<void> {
    await api.delete(`/environments/${encodeURIComponent(id)}`)
  },
}
