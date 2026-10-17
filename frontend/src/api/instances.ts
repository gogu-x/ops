import api from './client'
import type { ServiceParam } from './services'

export type InstanceStatus = 'not_deployed' | 'running' | 'stopped' | 'restarting' | 'error' | 'unknown'
export type RestartPolicy = 'no' | 'always' | 'unless-stopped' | 'on-failure'

export interface ServiceInstance {
  id: string
  service_type_id: string
  host_id: string
  name: string
  image: string
  params: ServiceParam[]
  env_text: string
  network: string
  port_mapping: string
  restart_policy: RestartPolicy
  note: string
  created_at?: string
  updated_at?: string

  // Runtime-only fields reflecting the live Docker container state.
  container_id?: string
  status: InstanceStatus
  status_text?: string
  container_image?: string
  started_at?: string
  deploy_error?: string
}

export type ServiceInstanceForm = Omit<
  ServiceInstance,
  'id' | 'created_at' | 'updated_at' | 'container_id' | 'status' | 'status_text' | 'container_image' | 'started_at' | 'deploy_error'
>

export interface ContainerDetail {
  container_id: string
  name: string
  image: string
  status: InstanceStatus
  state: string
  running: boolean
  restart_count: number
  started_at: string
  finished_at: string
  exit_code: number
  error?: string
  cmd: string[]
  env: string[]
  labels: Record<string, string>
  network_mode: string
  restart_policy: string
  ip_address: string
  ports: string[]
  mounts: string[]
  created: string
}

export interface ContainerResourceMetrics {
  cpu_percent: number
  memory_usage: number
  memory_limit: number
  memory_percent: number
  network_rx_bytes: number
  network_tx_bytes: number
  block_read_bytes: number
  block_write_bytes: number
  processes: number
  sampled_at: string
}

export interface ContainerResourceHistory {
  latest: ContainerResourceMetrics | null
  samples: ContainerResourceMetrics[]
}

export interface InstanceDeployment {
  id: string
  instance_id: string
  operation: 'deploy' | 'update_image' | 'rollback' | string
  image: string
  previous_image?: string
  status: 'succeeded' | 'failed' | string
  operator?: string
  message?: string
  created_at: string
}

export type BatchAction = 'deploy' | 'start' | 'stop' | 'restart' | 'update_image'

export interface ServiceInstanceOperationItem {
  instance_id: string
  name: string
  status: 'pending' | 'running' | 'succeeded' | 'failed'
  error?: string
}

export interface ServiceInstanceOperation {
  id: string
  action: BatchAction
  status: 'running' | 'succeeded' | 'partial' | 'failed'
  total: number
  completed: number
  succeeded: number
  failed: number
  items: ServiceInstanceOperationItem[]
  created_at: string
  updated_at: string
}

interface ApiResponse<T> {
  ok: boolean
  data: T
  error?: string
}

export const instanceApi = {
  async list(): Promise<ServiceInstance[]> {
    const response = await api.get<ApiResponse<ServiceInstance[]>>('/service-instances')
    return response.data.data || []
  },

  async create(form: ServiceInstanceForm): Promise<ServiceInstance> {
    const response = await api.post<ApiResponse<ServiceInstance>>('/service-instances', form)
    return response.data.data
  },

  async update(id: string, form: ServiceInstanceForm): Promise<ServiceInstance> {
    const response = await api.put<ApiResponse<ServiceInstance>>(`/service-instances/${encodeURIComponent(id)}`, form)
    return response.data.data
  },

  async remove(id: string): Promise<void> {
    await api.delete(`/service-instances/${encodeURIComponent(id)}`)
  },

  async status(id: string): Promise<ServiceInstance> {
    const response = await api.get<ApiResponse<ServiceInstance>>(`/service-instances/${encodeURIComponent(id)}/status`)
    return response.data.data
  },

  async detail(id: string): Promise<ContainerDetail> {
    const response = await api.get<ApiResponse<ContainerDetail>>(`/service-instances/${encodeURIComponent(id)}/detail`)
    return response.data.data
  },

  async resources(id: string, limit = 60): Promise<ContainerResourceHistory> {
    const response = await api.get<ApiResponse<ContainerResourceHistory>>(`/service-instances/${encodeURIComponent(id)}/resources`, { params: { limit } })
    return response.data.data
  },

  async logs(id: string, tail = 200): Promise<string> {
    const response = await api.get<ApiResponse<string>>(`/service-instances/${encodeURIComponent(id)}/logs`, { params: { tail } })
    return response.data.data || ''
  },

  async deploy(id: string): Promise<ServiceInstance> {
    const response = await api.post<ApiResponse<ServiceInstance>>(`/service-instances/${encodeURIComponent(id)}/deploy`, undefined, { timeout: 90000 })
    return response.data.data
  },

  async updateImage(id: string, image: string): Promise<ServiceInstance> {
    const response = await api.post<ApiResponse<ServiceInstance>>(
      `/service-instances/${encodeURIComponent(id)}/update-image`,
      { image },
      { timeout: 120000 },
    )
    return response.data.data
  },

  async start(id: string): Promise<void> {
    await api.post(`/service-instances/${encodeURIComponent(id)}/start`)
  },

  async stop(id: string): Promise<void> {
    await api.post(`/service-instances/${encodeURIComponent(id)}/stop`)
  },

  async restart(id: string): Promise<void> {
    // Restart now redeploys the container (removes + recreates) so edited
    // config (image, env vars, params) takes effect, so it can take longer
    // than a plain `docker restart`.
    await api.post(`/service-instances/${encodeURIComponent(id)}/restart`, undefined, { timeout: 90000 })
  },

  async removeContainer(id: string): Promise<void> {
    await api.delete(`/service-instances/${encodeURIComponent(id)}/container`)
  },

  async deployments(id: string, limit = 30): Promise<InstanceDeployment[]> {
    const response = await api.get<ApiResponse<InstanceDeployment[]>>(`/service-instances/${encodeURIComponent(id)}/deployments`, { params: { limit } })
    return response.data.data || []
  },

  async rollback(id: string, deploymentId: string): Promise<ServiceInstance> {
    const response = await api.post<ApiResponse<ServiceInstance>>(
      `/service-instances/${encodeURIComponent(id)}/deployments/${encodeURIComponent(deploymentId)}/rollback`,
      undefined,
      { timeout: 120000 },
    )
    return response.data.data
  },

  async startBatchOperation(instanceIds: string[], action: BatchAction, image?: string): Promise<ServiceInstanceOperation> {
    const response = await api.post<ApiResponse<ServiceInstanceOperation>>('/service-instances/batch-operations', {
      instance_ids: instanceIds,
      action,
      ...(image ? { image } : {}),
    })
    return response.data.data
  },

  async operation(id: string): Promise<ServiceInstanceOperation> {
    const response = await api.get<ApiResponse<ServiceInstanceOperation>>(`/service-instance-operations/${encodeURIComponent(id)}`)
    return response.data.data
  },
}
