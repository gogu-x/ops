import api from './client'

export interface Host {
  id: string
  project_id: string
  project_ids?: string[]
  name: string
  internal_ip: string
  external_ip: string
  docker_host: string
  tls_ca: string
  tls_cert: string
  tls_key: string
  note: string
  created_at?: string
  updated_at?: string
}

export interface HostForm {
  project_ids?: string[]
  name: string
  internal_ip: string
  external_ip: string
  docker_host: string
  tls_ca: string
  tls_cert: string
  tls_key: string
  note: string
}

interface ApiResponse<T> {
  ok: boolean
  data: T
  error?: string
}

export const hostApi = {
  async list(): Promise<Host[]> {
    const response = await api.get<ApiResponse<Host[]>>('/hosts')
    return response.data.data || []
  },

  async configuration(id: string): Promise<Host> {
    const response = await api.get<ApiResponse<Host>>(`/hosts/${encodeURIComponent(id)}/config`)
    return response.data.data
  },

  async create(form: HostForm): Promise<Host> {
    const response = await api.post<ApiResponse<Host>>('/hosts', form)
    return response.data.data
  },

  async update(id: string, form: HostForm): Promise<Host> {
    const response = await api.put<ApiResponse<Host>>(`/hosts/${encodeURIComponent(id)}`, form)
    return response.data.data
  },

  async remove(id: string): Promise<void> {
    await api.delete(`/hosts/${encodeURIComponent(id)}`)
  },

  async test(id: string): Promise<void> {
    await api.post<ApiResponse<unknown>>(`/hosts/${encodeURIComponent(id)}/test`)
  },

  async testConfiguration(id: string, form: HostForm): Promise<void> {
    await api.post<ApiResponse<unknown>>(`/hosts/${encodeURIComponent(id)}/test-connection`, form)
  },
}
