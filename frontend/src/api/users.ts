import api from './client'

export type UserPermission =
  | 'dashboard.view'
  | 'projects.view'
  | 'services.view'
  | 'services.manage'
  | 'service_types.view'
  | 'service_types.manage'
  | 'rollout_templates.manage'
  | 'hosts.view'
  | 'hosts.manage'
  | 'services.create'
  | 'services.edit'
  | 'services.delete'
  | 'services.deploy'
  | 'services.start'
  | 'services.stop'
  | 'services.restart'
  | 'services.update_image'
  | 'services.remove_container'
  | 'services.rollback'
  | 'services.batch'
  | 'services.environment_create'
  | 'services.environment_edit'
  | 'services.environment_delete'
  | 'services.rollout_create'
  | 'services.rollout_pause'
  | 'services.rollout_resume'
  | 'services.rollout_retry'
  | 'services.auto_create'
  | 'services.auto_edit'
  | 'services.auto_enable'
  | 'services.auto_disable'
  | 'hosts.create'
  | 'hosts.edit'
  | 'hosts.test'
  | 'hosts.delete'
  | 'service_types.create'
  | 'service_types.edit'
  | 'service_types.delete'

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
