import api from './client'

export type InstanceEventType =
  | 'deploy_succeeded'
  | 'deploy_failed'
  | 'update_image_succeeded'
  | 'update_image_failed'
  | 'container_started'
  | 'container_start_failed'
  | 'container_stopped'
  | 'container_stop_failed'
  | 'container_restarted'
  | 'container_restart_failed'
  | 'container_removed'
  | 'container_remove_failed'
  | 'health_check_passed'
  | 'health_check_failed'

export interface InstanceEvent {
  id: string
  instance_id: string
  type: InstanceEventType | string
  message: string
  created_at: string
}

interface ApiResponse<T> {
  ok: boolean
  data: T
  error?: string
}

export const eventApi = {
  async list(instanceId: string, limit = 20): Promise<InstanceEvent[]> {
    const response = await api.get<ApiResponse<InstanceEvent[]>>(
      `/service-instances/${encodeURIComponent(instanceId)}/events`,
      { params: { limit } },
    )
    return response.data.data || []
  },
}
