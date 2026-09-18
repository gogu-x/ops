import api from './client'

export interface ServiceParam {
  flag: string
  value: string
}

export interface ServiceType {
  id: string
  project_id: string
  host_id: string
  name: string
  created_at?: string
  updated_at?: string
}

export type ServiceTypeForm = Omit<ServiceType, 'id' | 'created_at' | 'updated_at'>

interface ApiResponse<T> {
  ok: boolean
  data: T
  error?: string
}

export const serviceApi = {
  async list(): Promise<ServiceType[]> {
    const response = await api.get<ApiResponse<ServiceType[]>>('/service-types')
    return response.data.data || []
  },

  async create(form: ServiceTypeForm): Promise<ServiceType> {
    const response = await api.post<ApiResponse<ServiceType>>('/service-types', form)
    return response.data.data
  },

  async update(id: string, form: ServiceTypeForm): Promise<ServiceType> {
    const response = await api.put<ApiResponse<ServiceType>>(`/service-types/${encodeURIComponent(id)}`, form)
    return response.data.data
  },

  async remove(id: string): Promise<void> {
    await api.delete(`/service-types/${encodeURIComponent(id)}`)
  },
}
