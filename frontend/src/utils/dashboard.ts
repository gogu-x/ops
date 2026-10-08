import type { ServiceInstance, InstanceStatus } from '../api/instances'
import type { Host } from '../api/hosts'
import type { ServiceType } from '../api/services'
import type { Environment } from '../api/environments'
import type { Project } from '../api/projects'

export const statusDefinitions: { key: InstanceStatus; label: string; color: string }[] = [
  { key: 'running', label: '运行中', color: '#269879' },
  { key: 'stopped', label: '已停止', color: '#94a3b8' },
  { key: 'restarting', label: '重启中', color: '#bf8728' },
  { key: 'error', label: '错误', color: '#d65360' },
  { key: 'unknown', label: '状态未知', color: '#b6c2d3' },
  { key: 'not_deployed', label: '未部署', color: '#e1e7f0' },
]

export function normalizedStatus(instance: ServiceInstance): InstanceStatus {
  return statusDefinitions.some((item) => item.key === instance.status) ? instance.status : 'unknown'
}

export function needsAttention(instance: ServiceInstance): boolean {
  return ['error', 'restarting', 'unknown'].includes(normalizedStatus(instance))
}

export function hostHasProject(host: Host, projectId: string): boolean {
  return host.project_id === projectId || host.project_ids?.includes(projectId) === true
}

export function scopeInstances(instances: ServiceInstance[], types: ServiceType[], projectId: string): ServiceInstance[] {
  if (!projectId) return instances
  const ids = new Set(types.filter((type) => type.project_id === projectId).map((type) => type.id))
  return instances.filter((instance) => ids.has(instance.service_type_id))
}

export function statusCounts(instances: ServiceInstance[]) {
  return statusDefinitions.map((item) => ({ ...item, count: instances.filter((instance) => normalizedStatus(instance) === item.key).length }))
}

export function projectOverview(projects: Project[], environments: Environment[], types: ServiceType[], instances: ServiceInstance[], hosts: Host[]) {
  return projects.map((project) => {
    const related = scopeInstances(instances, types, project.id)
    const running = related.filter((instance) => normalizedStatus(instance) === 'running').length
    return {
      ...project,
      environments: environments.filter((environment) => environment.project_id === project.id).length,
      total: related.length,
      running,
      attention: related.filter(needsAttention).length,
      hosts: hosts.filter((host) => hostHasProject(host, project.id)).length,
      percentage: related.length ? Math.round(running / related.length * 100) : 0,
    }
  })
}
