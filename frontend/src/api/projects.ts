import api from './client'

export interface Project {
  id: string
  name: string
  note: string
  env_vars: string
  created_at?: string
  updated_at?: string
}

export interface ProjectForm {
  name: string
  note: string
  env_vars: string
}

interface ApiResponse<T> {
  ok: boolean
  data: T
  error?: string
}

export const projectApi = {
  async list(): Promise<Project[]> {
    const response = await api.get<ApiResponse<Project[]>>('/projects')
    return response.data.data || []
  },

  async create(form: ProjectForm): Promise<Project> {
    const response = await api.post<ApiResponse<Project>>('/projects', form)
    return response.data.data
  },

  async update(id: string, form: ProjectForm): Promise<Project> {
    const response = await api.put<ApiResponse<Project>>(`/projects/${encodeURIComponent(id)}`, form)
    return response.data.data
  },

  async updateHosts(id: string, hostIds: string[]): Promise<void> {
    await api.put(`/projects/${encodeURIComponent(id)}/hosts`, { host_ids: hostIds })
  },

  async remove(id: string): Promise<void> {
    await api.delete(`/projects/${encodeURIComponent(id)}`)
  },
}
