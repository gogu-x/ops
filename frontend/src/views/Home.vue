<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { Box, FolderOpened, Grid, Monitor, Refresh, ArrowRight, Warning, CircleCheck } from '@element-plus/icons-vue'
import AppLayout from '../components/AppLayout.vue'
import { useAuthStore } from '../stores/auth'
import { projectApi, type Project } from '../api/projects'
import { environmentApi, type Environment } from '../api/environments'
import { serviceApi, type ServiceType } from '../api/services'
import { hostApi, type Host } from '../api/hosts'
import { instanceApi, type ServiceInstance } from '../api/instances'
import { eventApi, type InstanceEvent } from '../api/events'
import { hostHasProject, needsAttention, normalizedStatus, projectOverview, scopeInstances, statusCounts } from '../utils/dashboard'

const router = useRouter()
const auth = useAuthStore()
const projects = ref<Project[] | null>(null)
const environments = ref<Environment[] | null>(null)
const types = ref<ServiceType[] | null>(null)
const instances = ref<ServiceInstance[] | null>(null)
const hosts = ref<Host[] | null>(null)
const loading = ref(false)
const failures = ref<string[]>([])
const selectedProject = ref('')
const lastUpdated = ref('')
const canReadServices = computed(() => auth.hasPermission('services.view') || auth.hasPermission('projects.view'))
const canReadHosts = computed(() => auth.hasPermission('hosts.view') || canReadServices.value)
const summaryReady = computed(() => projects.value !== null && (!canReadServices.value || (environments.value !== null && types.value !== null && instances.value !== null)))
const scopedProjects = computed(() => (projects.value || []).filter((project) => !selectedProject.value || project.id === selectedProject.value))
const scopedInstances = computed(() => scopeInstances(instances.value || [], types.value || [], selectedProject.value))
const scopedEnvironments = computed(() => (environments.value || []).filter((env) => !selectedProject.value || env.project_id === selectedProject.value))
const scopedTypes = computed(() => (types.value || []).filter((type) => !selectedProject.value || type.project_id === selectedProject.value))
const scopedHosts = computed(() => (hosts.value || []).filter((host) => !selectedProject.value || hostHasProject(host, selectedProject.value)))
const counts = computed(() => statusCounts(scopedInstances.value))
const running = computed(() => counts.value.find((item) => item.key === 'running')!.count)
const attention = computed(() => scopedInstances.value.filter(needsAttention).sort((a, b) => {
  const order = { error: 0, restarting: 1, unknown: 2 }
  return (order[normalizedStatus(a) as keyof typeof order] ?? 3) - (order[normalizedStatus(b) as keyof typeof order] ?? 3)
}))
const overview = computed(() => projectOverview(scopedProjects.value, environments.value || [], types.value || [], instances.value || [], hosts.value || []))
const ringStyle = computed(() => {
  if (!scopedInstances.value.length || !summaryReady.value) return { background: 'var(--ops-border)' }
  let offset = 0
  const segments = counts.value.filter((item) => item.count).map((item) => {
    const start = offset
    offset += item.count / scopedInstances.value.length * 100
    return `${item.color} ${start}% ${offset}%`
  })
  return { background: `conic-gradient(${segments.join(',')})` }
})

function openServices(project = selectedProject.value, instance?: ServiceInstance) {
  const type = types.value?.find((type) => type.id === instance?.service_type_id)
  router.push({ path: '/services', query: { ...(project || type?.project_id ? { project: project || type?.project_id } : {}), ...(instance ? { instance: instance.id } : {}) } })
}
function openProjectRow(row: { id: string }) {
  if (auth.hasPermission('services.view')) openServices(row.id)
  else if (auth.hasPermission('projects.view')) router.push('/projects')
}
function instanceScope(instance: ServiceInstance) {
  const type = types.value?.find((type) => type.id === instance.service_type_id)
  return [projects.value?.find((project) => project.id === type?.project_id)?.name, environments.value?.find((env) => env.id === type?.environment_id)?.name].filter(Boolean).join(' / ') || '资源归属待检查'
}
function attentionReason(instance: ServiceInstance) {
  return instance.deploy_error || instance.status_text || (normalizedStatus(instance) === 'unknown' ? '无法确定当前容器状态' : normalizedStatus(instance) === 'restarting' ? '容器正在重启，请检查运行日志' : '实例运行异常')
}

const recentEvents = ref<(InstanceEvent & { instance: ServiceInstance })[]>([])
const eventsLoading = ref(false)
const eventFailures = ref(0)
const sampledInstances = ref(0)
let eventRequest = 0
let disposed = false
async function loadRecentEvents() {
  const request = ++eventRequest
  recentEvents.value = []
  eventFailures.value = 0
  const targets = summaryReady.value && auth.hasPermission('services.view') ? [...scopedInstances.value].sort((a, b) => (b.updated_at || b.created_at || '').localeCompare(a.updated_at || a.created_at || '')).slice(0, 12) : []
  sampledInstances.value = targets.length
  eventsLoading.value = targets.length > 0
  const result: (InstanceEvent & { instance: ServiceInstance })[] = []
  let cursor = 0
  let failed = 0
  await Promise.all(Array.from({ length: Math.min(3, targets.length) }, async () => {
    while (cursor < targets.length && request === eventRequest && !disposed) {
      const instance = targets[cursor++]
      try { result.push(...(await eventApi.list(instance.id, 5)).map((event) => ({ ...event, instance }))) } catch { failed++ }
    }
  }))
  if (request !== eventRequest || disposed) return
  recentEvents.value = result.sort((a, b) => b.created_at.localeCompare(a.created_at)).slice(0, 5)
  eventFailures.value = failed
  eventsLoading.value = false
}

async function loadDashboard() {
  if (loading.value) return
  loading.value = true
  eventRequest++
  const readServices = canReadServices.value
  const readHosts = canReadHosts.value || auth.hasPermission('projects.view')
  const results = await Promise.allSettled([
    projectApi.list(),
    readServices ? environmentApi.list() : Promise.resolve([]),
    readServices || auth.hasPermission('projects.view') ? serviceApi.list() : Promise.resolve([]),
    readServices ? instanceApi.list() : Promise.resolve([]),
    readHosts ? hostApi.list() : Promise.resolve([]),
  ] as const)
  if (disposed) return
  const [p, e, t, i, h] = results
  projects.value = p.status === 'fulfilled' ? p.value : null
  environments.value = e.status === 'fulfilled' ? e.value : null
  types.value = t.status === 'fulfilled' ? t.value : null
  instances.value = i.status === 'fulfilled' ? i.value : null
  hosts.value = h.status === 'fulfilled' ? h.value : null
  failures.value = ['项目', '环境', '服务类型', '实例', '主机'].filter((_, index) => results[index].status === 'rejected')
  if (projects.value && selectedProject.value && !projects.value.some((project) => project.id === selectedProject.value)) selectedProject.value = ''
  lastUpdated.value = new Date().toLocaleTimeString('zh-CN', { hour12: false })
  await loadRecentEvents()
  if (disposed) return
  loading.value = false
}
const eventLabels: Record<string, string> = { deploy_succeeded: '部署成功', deploy_failed: '部署失败', update_image_succeeded: '镜像更新成功', update_image_failed: '镜像更新失败', container_started: '容器已启动', container_start_failed: '容器启动失败', container_stopped: '容器已停止', container_stop_failed: '容器停止失败', container_restarted: '容器已重启', container_restart_failed: '容器重启失败', container_removed: '容器已移除', container_remove_failed: '容器移除失败', health_check_passed: '健康检查通过', health_check_failed: '健康检查失败' }
function eventTime(value: string) { const date = new Date(value); return Number.isNaN(date.getTime()) ? '—' : date.toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hour12: false }) }
watch(selectedProject, () => { if (!loading.value) void loadRecentEvents() })
onMounted(loadDashboard)
onUnmounted(() => { disposed = true; eventRequest++ })
</script>

<template>
  <AppLayout>
    <div class="dashboard-toolbar"><el-select v-model="selectedProject" placeholder="全部项目" aria-label="筛选项目" :disabled="projects === null"><el-option label="全部项目" value="" /><el-option v-for="project in projects || []" :key="project.id" :value="project.id" :label="project.name" /></el-select><span class="toolbar-caption">{{ selectedProject ? '当前项目运行概况' : '全部可见资源' }}</span><el-button :icon="Refresh" :loading="loading" @click="loadDashboard">刷新</el-button></div>
    <div class="dashboard-content page-content-loading" v-loading="loading">
    <el-alert v-if="failures.length" class="dashboard-error" :title="`${failures.join('、')}数据加载失败，相关统计暂不可用，请刷新重试。`" type="error" :closable="false" show-icon />
    <div class="dashboard-metrics">
      <button class="metric-card" :disabled="!canReadServices" @click="openServices()"><span class="metric-label">服务实例<el-icon><Box /></el-icon></span><strong>{{ canReadServices && summaryReady ? scopedInstances.length : '—' }}<small>个</small></strong><span class="metric-caption"><span v-if="canReadServices && summaryReady" class="success-text">{{ running }} 个运行中</span><span v-else>无服务查看权限</span> · 当前状态</span></button>
      <button class="metric-card" @click="auth.hasPermission('projects.view') ? router.push('/projects') : router.push('/services')"><span class="metric-label">项目 / 环境<el-icon><FolderOpened /></el-icon></span><strong>{{ projects !== null ? scopedProjects.length : '—' }}<small>/ {{ environments !== null && canReadServices ? scopedEnvironments.length : '—' }}</small></strong><span class="metric-caption">按项目组织部署环境</span></button>
      <button class="metric-card" :disabled="!auth.hasPermission('services.view') && !auth.hasPermission('projects.view')" @click="openServices()"><span class="metric-label">服务类型<el-icon><Grid /></el-icon></span><strong>{{ (auth.hasPermission('services.view') || auth.hasPermission('projects.view')) && types !== null ? scopedTypes.length : '—' }}<small>类</small></strong><span class="metric-caption">项目内的服务分类</span></button>
      <button class="metric-card" @click="router.push('/hosts')"><span class="metric-label">部署主机<el-icon><Monitor /></el-icon></span><strong>{{ hosts !== null ? scopedHosts.length : '—' }}<small>台</small></strong><span class="metric-caption">跨项目共享 · 去重统计</span></button>
    </div>
    <div class="dashboard-columns">
      <section class="dashboard-panel"><header><h2>实例运行状态</h2><span>容器状态 · 当前快照</span></header><div v-if="summaryReady && canReadServices" class="status-content"><div class="status-ring" :style="ringStyle" role="img" :aria-label="`${scopedInstances.length} 个实例，${running} 个运行中`"><div><strong>{{ running }}</strong><span>运行中</span></div></div><div class="status-legend"><div v-for="item in counts" :key="item.key"><span><i :style="{ background: item.color }"></i>{{ item.label }}</span><b>{{ item.count }}</b></div></div></div><el-empty v-else :description="loading ? '正在加载状态' : canReadServices ? '状态数据不可用' : '无服务查看权限'" :image-size="55" /></section>
      <section class="dashboard-panel"><header><h2>需要关注 <span v-if="summaryReady && canReadServices && attention.length" class="attention-count">{{ attention.length }}</span></h2><span>错误 / 重启 / 未知</span></header><div v-if="summaryReady && canReadServices && attention.length" class="attention-list"><button v-for="item in attention.slice(0, 3)" :key="item.id" class="attention-item" @click="openServices('', item)"><el-icon><Warning /></el-icon><span class="attention-body"><b>{{ item.name }}</b><span class="attention-reason" :title="attentionReason(item)">{{ attentionReason(item) }}</span><small>{{ instanceScope(item) }}</small></span><el-icon class="attention-arrow"><ArrowRight /></el-icon></button><button v-if="attention.length > 3" class="show-more" @click="openServices()">查看服务列表中的其余 {{ attention.length - 3 }} 项</button></div><div v-else-if="summaryReady && !canReadServices" class="healthy-empty"><el-icon><CircleCheck /></el-icon><strong>无服务查看权限</strong><span>请联系管理员申请服务查看权限</span></div><div v-else-if="summaryReady" class="healthy-empty"><el-icon><CircleCheck /></el-icon><strong>{{ scopedInstances.length ? '暂无需要关注的异常' : '当前范围暂无实例' }}</strong><span>{{ scopedInstances.length ? '容器运行状态不等同于业务健康' : '部署实例后将在这里显示运行情况' }}</span></div><el-empty v-else :description="loading ? '正在检查实例' : '无法检查实例状态'" :image-size="55" /></section>
    </div>
    <section class="dashboard-panel project-overview"><header><h2>项目运行概览</h2><span>项目 → 环境 → 服务实例</span></header><div v-if="summaryReady" class="project-mobile-list">
          <button v-for="row in overview" :key="row.id" class="project-mobile-item" @click="openProjectRow(row)">
          <span class="project-mobile-heading"><b>{{ row.name }}</b><span :class="{ 'has-issues': row.attention }">{{ row.attention ? `${row.attention} 项需关注` : '无异常' }}</span></span>
          <span class="project-mobile-stats"><span><small>环境</small>{{ canReadServices ? row.environments : '—' }}</span><span><small>实例</small>{{ canReadServices ? row.total : '—' }}</span><span><small>运行中</small>{{ canReadServices ? row.running : '—' }}</span><span><small>主机</small>{{ hosts !== null ? row.hosts : '—' }}</span></span>
        </button>
        <el-empty v-if="!overview.length" description="暂无项目" :image-size="45" />
      </div><el-table v-if="summaryReady" :data="overview" @row-click="openProjectRow" class="overview-table" empty-text="暂无项目"><el-table-column label="项目" min-width="180"><template #default="{ row }"><button class="project-name" @click.stop="openProjectRow(row)">{{ row.name }}</button><div class="project-note">{{ row.note || '业务服务项目' }}</div></template></el-table-column><el-table-column label="环境" width="85"><template #default="{ row }">{{ canReadServices ? row.environments : '—' }}</template></el-table-column><el-table-column label="实例" width="85"><template #default="{ row }">{{ canReadServices ? row.total : '—' }}</template></el-table-column><el-table-column label="运行中" min-width="150"><template #default="{ row }"><template v-if="canReadServices"><div class="running-ratio">{{ row.running }} / {{ row.total }}</div><el-progress :percentage="row.percentage" :show-text="false" :stroke-width="5" color="var(--ops-success)" /></template><span v-else>—</span></template></el-table-column><el-table-column label="异常 / 未知" width="130"><template #default="{ row }"><el-tag v-if="canReadServices" :type="row.attention ? 'danger' : 'success'" size="small" effect="light">{{ row.attention ? `${row.attention} 项` : '无异常' }}</el-tag><span v-else>—</span></template></el-table-column><el-table-column label="主机" width="90"><template #default="{ row }">{{ hosts !== null ? row.hosts : '—' }}</template></el-table-column></el-table><el-empty v-else :description="loading ? '正在加载项目' : '项目概览暂不可用'" :image-size="55" /></section>
    <div class="dashboard-bottom">
      <section class="dashboard-panel"><header><h2>最近实例事件</h2><span>{{ sampledInstances ? `最近更新的 ${sampledInstances} 个实例` : '当前范围' }}</span></header><div v-loading="eventsLoading" class="event-list"><button v-for="event in recentEvents" :key="`${event.instance_id}-${event.id}`" class="event-item" @click="openServices('', event.instance)"><i class="event-status" :class="{ failed: event.type.includes('failed') }"></i><span class="event-body"><b>{{ eventLabels[event.type] || event.type }}</b><span>{{ event.instance.name }} · {{ instanceScope(event.instance) }}</span></span><time>{{ eventTime(event.created_at) }}</time></button><el-empty v-if="!recentEvents.length" :description="eventsLoading ? '正在获取事件' : !summaryReady ? '实例数据不可用' : !auth.hasPermission('services.view') ? '无事件查看权限' : eventFailures ? '事件获取失败' : '当前范围暂无实例事件'" :image-size="45" /></div><p v-if="eventFailures" class="event-warning">{{ eventFailures }} 个实例事件获取失败，已显示其余结果。</p></section>
      <section class="dashboard-panel"><header><h2>快捷入口</h2><span>常用运维功能</span></header><div class="quick-actions"><button v-if="auth.hasPermission('services.view')" @click="openServices()"><span><el-icon><Box /></el-icon>{{ auth.hasPermission('services.manage') ? '部署与管理实例' : '查看服务实例' }}</span><el-icon><ArrowRight /></el-icon></button><button v-if="auth.hasPermission('hosts.view')" @click="router.push('/hosts')"><span><el-icon><Monitor /></el-icon>查看主机连接</span><el-icon><ArrowRight /></el-icon></button><button v-if="auth.hasPermission('projects.view')" @click="router.push('/projects')"><span><el-icon><FolderOpened /></el-icon>查看项目</span><el-icon><ArrowRight /></el-icon></button></div></section>
    </div>
    <footer class="dashboard-footer"><span>运行中表示容器状态；采集失败显示为未知。</span><span>{{ lastUpdated ? `最近获取 ${lastUpdated}` : '等待首次加载' }}</span></footer>
    </div>
  </AppLayout>
</template>

<style scoped>
.dashboard-toolbar { display: flex; align-items: center; gap: 12px; margin-bottom: 20px; }
.dashboard-content { min-width: 0; }
.dashboard-content.page-content-loading { min-height: calc(100dvh - 114px); }
.dashboard-content:deep(> .el-loading-mask) { position: fixed !important; inset: 0 0 0 220px; width: auto; height: auto; }
.dashboard-toolbar .el-select { width: 190px; }
.dashboard-toolbar .el-button { margin-left: auto; }
.toolbar-caption { font-size: 12px; color: var(--ops-text-secondary); }
.dashboard-error { margin-bottom: 18px; }
.dashboard-metrics { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 16px; margin-bottom: 20px; }
.metric-card { display: flex; flex-direction: column; text-align: left; padding: 21px 22px; border: 1px solid var(--ops-border); border-radius: 10px; background: #fff; font: inherit; cursor: pointer; transition: border-color .15s; }
.metric-card:hover { border-color: #afc4f5; }
.metric-label { display: flex; align-items: center; justify-content: space-between; width: 100%; color: var(--ops-text-secondary); font-size: 13px; }
.metric-label .el-icon { font-size: 18px; }
.metric-card strong { font-size: 32px; font-weight: 600; color: var(--ops-text); margin: 11px 0 7px; font-variant-numeric: tabular-nums; }
.metric-card strong small { margin-left: 8px; font-size: 14px; font-weight: 400; color: var(--ops-text-secondary); }
.metric-caption { font-size: 12px; color: var(--ops-text-secondary); }
.success-text { color: var(--ops-success); }
.dashboard-columns { display: grid; grid-template-columns: 1.15fr 1fr; gap: 20px; margin-bottom: 20px; }
.dashboard-panel { background: #fff; border: 1px solid var(--ops-border); border-radius: 10px; overflow: hidden; min-width: 0; }
.dashboard-panel header { display: flex; align-items: center; justify-content: space-between; gap: 10px; padding: 17px 21px; border-bottom: 1px solid var(--ops-border); flex-wrap: wrap; }
.dashboard-panel h2 { font-size: 14px; font-weight: 600; margin: 0; }
.dashboard-panel header > span { font-size: 11px; color: var(--ops-text-secondary); }
.status-content { display: flex; align-items: center; justify-content: space-evenly; padding: 25px; gap: 30px; }
.status-ring { display: grid; place-items: center; width: 153px; height: 153px; flex-shrink: 0; border-radius: 50%; }
.status-ring > div { display: flex; flex-direction: column; align-items: center; justify-content: center; width: 123px; height: 123px; border-radius: 50%; background: #fff; }
.status-ring strong { font-size: 30px; font-weight: 600; }
.status-ring span { font-size: 12px; color: var(--ops-text-secondary); margin-top: 2px; }
.status-legend { width: 180px; max-width: 50%; }
.status-legend > div { display: flex; align-items: center; justify-content: space-between; font-size: 12px; padding: 6px 0; }
.status-legend span { display: flex; align-items: center; gap: 9px; }
.status-legend i { width: 7px; height: 7px; border-radius: 50%; }
.status-legend b { font-weight: 500; font-variant-numeric: tabular-nums; }
.attention-count { color: var(--ops-danger); background: #fff0f1; font-size: 11px; padding: 2px 6px; border-radius: 4px; margin-left: 5px; }
.attention-item { display: flex; align-items: flex-start; text-align: left; gap: 11px; width: 100%; border: 0; border-bottom: 1px solid var(--ops-border); background: transparent; padding: 14px 21px; font: inherit; cursor: pointer; }
.attention-item:hover, .event-item:hover { background: #f8faff; }
.attention-item > .el-icon { color: var(--ops-warning); margin-top: 3px; }
.attention-body { display: flex; flex-direction: column; flex: 1; min-width: 0; gap: 4px; }
.attention-body b { font-size: 12px; font-weight: 500; color: var(--ops-text); }
.attention-reason { font-size: 12px; color: var(--ops-text-secondary); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.attention-body small { font-size: 11px; color: var(--ops-text-secondary); }
.attention-item .attention-arrow { color: var(--ops-text-secondary); font-size: 12px; align-self: center; }
.show-more { width: 100%; padding: 10px; font: inherit; font-size: 12px; border: 0; background: #fff; color: var(--ops-primary); cursor: pointer; }
.healthy-empty { min-height: 223px; display: flex; align-items: center; justify-content: center; flex-direction: column; gap: 10px; padding: 20px; }
.healthy-empty .el-icon { color: var(--ops-success); font-size: 29px; }
.healthy-empty strong { font-size: 13px; font-weight: 500; }
.healthy-empty span { font-size: 12px; color: var(--ops-text-secondary); }
.project-overview { margin-bottom: 20px; }
.project-mobile-list { display: none; }
.overview-table { --el-table-header-bg-color: #f8fafd; }
.overview-table :deep(th.el-table__cell) { font-size: 12px; font-weight: 400; padding: 12px; }
.overview-table :deep(td.el-table__cell) { padding: 15px 12px; cursor: pointer; }
.project-name { border: 0; background: transparent; font: inherit; font-size: 13px; color: var(--ops-text); font-weight: 600; text-align: left; padding: 0; cursor: pointer; }
.project-name:hover { color: var(--ops-primary); }
.project-note { color: var(--ops-text-secondary); font-size: 11px; margin-top: 3px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.running-ratio { font-size: 12px; margin-bottom: 5px; }
.overview-table .el-progress { max-width: 140px; }
.dashboard-bottom { display: grid; grid-template-columns: 1.6fr 1fr; gap: 20px; }
.event-list { min-height: 178px; }
.event-item { display: flex; align-items: center; gap: 12px; text-align: left; width: 100%; padding: 14px 21px; background: transparent; border: 0; border-bottom: 1px solid var(--ops-border); font: inherit; cursor: pointer; }
.event-item:last-child { border-bottom: 0; }
.event-status { flex-shrink: 0; width: 6px; height: 6px; border-radius: 50%; background: var(--ops-success); }
.event-status.failed { background: var(--ops-danger); }
.event-body { display: flex; flex-direction: column; gap: 4px; flex: 1; min-width: 0; }
.event-body b { font-size: 12px; font-weight: 500; color: var(--ops-text); }
.event-body span { font-size: 11px; color: var(--ops-text-secondary); overflow-wrap: anywhere; }
.event-item time { font-size: 11px; color: var(--ops-text-secondary); white-space: nowrap; }
.event-warning { padding: 0 21px 14px; margin: 0; color: var(--ops-warning); font-size: 12px; }
.quick-actions { padding: 18px; display: grid; gap: 10px; }
.quick-actions button { display: flex; align-items: center; justify-content: space-between; gap: 10px; padding: 13px 14px; background: #fff; border: 1px solid var(--ops-border); border-radius: 6px; font: inherit; font-size: 12px; color: var(--ops-text); cursor: pointer; }
.quick-actions button:hover { background: var(--ops-primary-light); border-color: #c5d4f6; }
.quick-actions button span { display: flex; align-items: center; gap: 10px; }
.quick-actions .el-icon { color: var(--ops-text-secondary); }
.dashboard-footer { display: flex; justify-content: space-between; flex-wrap: wrap; gap: 8px; color: var(--ops-text-secondary); font-size: 11px; margin-top: 20px; }
@media (max-width: 1000px) { .dashboard-metrics { gap: 12px; } .metric-card { padding: 18px 16px; } .dashboard-columns { grid-template-columns: 1fr 1fr; gap: 16px; } .status-content { padding: 24px 16px; gap: 20px; } .status-ring { width: 124px; height: 124px; } .status-ring > div { width: 100px; height: 100px; } }
@media (max-width: 1000px) and (min-width: 761px) { .dashboard-content:deep(> .el-loading-mask) { left: 64px; } }
@media (max-width: 800px) { .dashboard-metrics { grid-template-columns: repeat(2, minmax(0, 1fr)); } .dashboard-columns, .dashboard-bottom { grid-template-columns: 1fr; } .status-content { justify-content: center; gap: 40px; } }
@media (max-width: 760px) { .dashboard-content.page-content-loading { min-height: calc(100dvh - 220px - env(safe-area-inset-bottom)); } .dashboard-content:deep(> .el-loading-mask) { inset: 58px 0 calc(64px + env(safe-area-inset-bottom)) 0; } }
@media (max-width: 480px) { .toolbar-caption { display: none; } .dashboard-toolbar .el-select { width: 170px; } .metric-card { padding: 15px; } .metric-card strong { font-size: 28px; } .metric-caption { font-size: 11px; } .status-content { gap: 25px; } .dashboard-panel header { padding: 16px; } .event-item { padding: 13px 16px; gap: 8px; } .event-item time { max-width: 66px; white-space: normal; } }
@media (max-width: 640px) {
  .overview-table { display: none; }
  .project-mobile-list { display: block; }
  .project-mobile-item { width: 100%; text-align: left; background: #fff; color: var(--ops-text); border: 0; border-bottom: 1px solid var(--ops-border); padding: 17px; font: inherit; cursor: pointer; }
  .project-mobile-item:last-child { border-bottom: 0; }
  .project-mobile-heading { display: flex; justify-content: space-between; align-items: center; gap: 12px; margin-bottom: 14px; }
  .project-mobile-heading b { font-size: 13px; font-weight: 600; }
  .project-mobile-heading > span { font-size: 11px; color: var(--ops-success); }
  .project-mobile-heading .has-issues { color: var(--ops-danger); }
  .project-mobile-stats { display: grid; grid-template-columns: repeat(4, 1fr); font-size: 15px; }
  .project-mobile-stats small { display: block; color: var(--ops-text-secondary); font-size: 11px; margin-bottom: 4px; }
}
</style>
