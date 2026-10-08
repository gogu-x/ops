import api from './client'

export type UserPermission = 'projects.view' | 'services.view' | 'services.manage' | 'hosts.view' | 'hosts.manage'

export interface ManagedUser {
  id: string
  username: string
  role: 'admin' | 'user'
  project_ids: string[]
  permissions: UserPermission[]
  disabled: boolean
  created_at: string
}

export interface UserAccessForm {
  username: string
  role: 'admin' | 'user'
  project_ids: string[]
  permissions: UserPermission[]
  disabled: boolean
}

interface ApiResponse<T> { ok: boolean; data: T; error?: string }

export const userApi = {
  async list(): Promise<ManagedUser[]> {
    const { data } = await api.get<ApiResponse<ManagedUser[]>>('/users')
    return data.data || []
  },
  async create(form: UserAccessForm & { password: string }): Promise<ManagedUser> {
    const { data } = await api.post<ApiResponse<ManagedUser>>('/users', form)
    return data.data
  },
  async update(id: string, form: UserAccessForm): Promise<ManagedUser> {
    const { data } = await api.put<ApiResponse<ManagedUser>>(`/users/${encodeURIComponent(id)}`, form)
    return data.data
  },
  async resetPassword(id: string, password: string): Promise<void> {
    await api.put(`/users/${encodeURIComponent(id)}/password`, { password })
  },
}
