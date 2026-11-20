import api from './client'

export type RolloutAction = 'restart' | 'update_image'
export type RolloutStatus = 'queued' | 'running' | 'paused' | 'succeeded' | 'partial' | 'failed'

export interface RolloutGroupPlan {
  key: string
  name: string
  project_id: string
  initial_unavailable: number
  max_unavailable: number
  max_parallel: number
}

export interface RolloutJob {
  id: string
  project_id: string
  operator: string
  action: RolloutAction
  image?: string
  status: RolloutStatus
  pause_reason?: string
  total: number
  completed: number
  succeeded: number
  failed: number
  max_unavailable_per_group: number
  max_parallel: number
  max_parallel_per_group: number
  groups: RolloutGroupPlan[]
  created_at: string
  updated_at: string
}

export interface RolloutItem {
  id: string
  job_id: string
  instance_id: string
  name: string
  host_id: string
  group_key: string
  previous_image: string
  target_image: string
  initially_available: boolean
  status: 'pending' | 'running' | 'health_check' | 'succeeded' | 'failed'
  attempt: number
  error?: string
  started_at?: string
  updated_at: string
  finished_at?: string
}

export interface RolloutAuditEvent {
  id: string
  job_id: string
  instance_id?: string
  operator: string
  type: string
  message: string
  created_at: string
}

export interface RolloutDetail {
  job: RolloutJob
  items: RolloutItem[]
  events: RolloutAuditEvent[]
}

export interface CreateRolloutRequest {
  parameter_values?: Record<string, string>
  instance_ids: string[]
  action: RolloutAction
  image?: string
  max_unavailable_per_group: number
  max_parallel: number
  max_parallel_per_group: number
}

export interface ReleaseStrategy {
  name: string
  maxUnavailable: number
  maxParallel: number
  maxParallelPerGroup: number
}

type StrategyInput = { kind: 'release' } & ReleaseStrategy
export type StrategyTemplate = StrategyInput & { id: string }

interface ApiResponse<T> {
  ok: boolean
  data: T
  error?: string
}

export const rolloutApi = {
  async listTemplates(): Promise<StrategyTemplate[]> {
    const response = await api.get<ApiResponse<StrategyTemplate[]>>('/rollout-templates')
    return response.data.data
  },

  async createTemplate(template: StrategyInput): Promise<StrategyTemplate> {
    const response = await api.post<ApiResponse<StrategyTemplate>>('/rollout-templates', template)
    return response.data.data
  },

  async create(request: CreateRolloutRequest): Promise<RolloutDetail> {
    const response = await api.post<ApiResponse<RolloutDetail>>('/rollouts', request, { timeout: 180000 })
    return response.data.data
  },

  async list(limit = 50): Promise<RolloutJob[]> {
    const response = await api.get<ApiResponse<RolloutJob[]>>('/rollouts', { params: { limit }, timeout: 60000 })
    return response.data.data || []
  },

  async detail(id: string): Promise<RolloutDetail> {
    const response = await api.get<ApiResponse<RolloutDetail>>(`/rollouts/${encodeURIComponent(id)}`, { timeout: 60000 })
    return response.data.data
  },

  async pause(id: string, reason = ''): Promise<void> {
    await api.post(`/rollouts/${encodeURIComponent(id)}/pause`, { reason })
  },

  async resume(id: string): Promise<void> {
    await api.post(`/rollouts/${encodeURIComponent(id)}/resume`)
  },

  async retryFailed(id: string): Promise<void> {
    await api.post(`/rollouts/${encodeURIComponent(id)}/retry-failed`, undefined, { timeout: 240000 })
  },
}
