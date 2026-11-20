import api from './client'

export interface AutoOpenSettings {
  name: string
  template_id: string
  name_prefix: string
  first_server_id: number
  first_open_at: string
  interval_days: number
  count: number
  first_host_port: number
}

export interface AutoOpenRun {
  server_id: number
  instance_id: string
  instance_name: string
  open_at: string
  finished_at: string
  error?: string
}

export interface AutoOpenPolicy extends AutoOpenSettings {
  id: string
  project_id: string
  environment_id: string
  template_name: string
  enabled: boolean
  running: boolean
  completed: number
  next_open_at: string
  recent_runs: AutoOpenRun[]
  revision: number
}

export interface AutoOpenTemplate {
  id: string
  name: string
  project_id: string
  environment_id: string
  image: string
  parameters: { flag: string; configured: boolean; display_value?: string }[]
}

interface ApiResponse<T> {
  data: T
}

export const autoOpenApi = {
  async list() {
    const response = await api.get<ApiResponse<AutoOpenPolicy[]>>('/auto-open-policies')
    return response.data.data
  },
  async templates() {
    const response = await api.get<ApiResponse<AutoOpenTemplate[]>>('/auto-open-templates')
    return response.data.data
  },
  async save(settings: AutoOpenSettings, current?: AutoOpenPolicy) {
    const response = current
      ? await api.put<ApiResponse<AutoOpenPolicy>>(`/auto-open-policies/${encodeURIComponent(current.id)}`, { ...settings, revision: current.revision })
      : await api.post<ApiResponse<AutoOpenPolicy>>('/auto-open-policies', settings)
    return response.data.data
  },
  async setEnabled(policy: AutoOpenPolicy, enabled: boolean) {
    const response = await api.put<ApiResponse<AutoOpenPolicy>>(`/auto-open-policies/${encodeURIComponent(policy.id)}/enabled`, {
      enabled,
      revision: policy.revision,
    })
    return response.data.data
  },
}
