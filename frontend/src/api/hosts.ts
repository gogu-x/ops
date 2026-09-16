import api from './client'

export interface Host {
  id: string
  name: string
  docker_host: string
  tls_ca: string
  tls_cert: string
  tls_key: string
  note: string
  created_at?: string
  updated_at?: string
}

export interface HostForm {
  name: string
  docker_host: string
  tls_ca: string
  tls_cert: string
  tls_key: string
  note: string
}

export interface DockerInfo {
  version: string
  api_version: string
  os: string
  arch: string
}

export interface HostTestResult {
  host: Host
  docker: DockerInfo
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

  async create(form: HostForm): Promise<Host> {
    const response = await api.post<ApiResponse<Host>>('/hosts', form)
    return response.data.data
  },

  async remove(id: string): Promise<void> {
    await api.delete(`/hosts/${encodeURIComponent(id)}`)
  },

  async test(id: string): Promise<HostTestResult> {
    const response = await api.post<ApiResponse<HostTestResult>>(`/hosts/${encodeURIComponent(id)}/test`)
    return response.data.data
  },
}
