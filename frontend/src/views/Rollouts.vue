<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import AppLayout from '../components/AppLayout.vue'
import { useAuthStore } from '../stores/auth'
import { projectApi, type Project } from '../api/projects'
import { environmentApi, type Environment } from '../api/environments'
import { serviceApi, type ServiceType } from '../api/services'
import { hostApi, type Host } from '../api/hosts'
import { instanceApi, type ServiceInstance } from '../api/instances'
import { rolloutApi, type RolloutAction, type RolloutDetail, type RolloutJob } from '../api/rollouts'

type ProbeType = ServiceType['readiness_probe']['type']

interface RolloutTargetGroup {
  key: string
  name: string
  instances: ServiceInstance[]
}

const auth = useAuthStore()
const canManage = computed(() => auth.hasPermission('services.manage'))
const loading = ref(false)
const loadError = ref('')
const projects = ref<Project[]>([])
const environments = ref<Environment[]>([])
const serviceTypes = ref<ServiceType[]>([])
const hosts = ref<Host[]>([])
const instances = ref<ServiceInstance[]>([])
const selectedProjectID = ref(sessionStorage.getItem('ops_rollout_project_id') || '')
const selectedEnvironmentID = ref('')
const selectedTypeID = ref('')
const keyword = ref('')
const selectedIDs = ref<string[]>([])
const submitting = ref(false)
const probeSubmitting = ref(false)
const probeDialogVisible = ref(false)
const editingProbeTypeID = ref('')
const rolloutDetail = ref<RolloutDetail | null>(null)
const rolloutHistory = ref<RolloutJob[]>([])
const pollError = ref('')
const historyLoading = ref(false)
const rolloutForm = reactive({
  action: 'update_image' as RolloutAction,
  image: '',
  max_unavailable_per_group: 1,
  max_parallel: 5,
  max_parallel_per_group: 2,
  health_timeout_seconds: 300,
})
const probeForm = reactive<{ type: ProbeType; port: number; path: string }>({ type: '', port: 0, path: '/health' })
const storageKey = 'ops_rollout_job_id'
let pollTimer: ReturnType<typeof setInterval> | null = null
let pollBusy = false
let disposed = false

const typesByID = computed(() => {
  const result = new Map<string, ServiceType>()
  for (const item of serviceTypes.value) result.set(item.id, item)
  return result
})
const hostsByID = computed(() => {
  const result = new Map<string, Host>()
  for (const item of hosts.value) result.set(item.id, item)
  return result
})
const projectEnvironments = computed(() => environments.value.filter((item) => item.project_id === selectedProjectID.value))
const projectTypes = computed(() => serviceTypes.value.filter((item) => item.project_id === selectedProjectID.value))
const activeTask = computed(() => rolloutDetail.value?.job || null)
const taskInProgress = computed(() => Boolean(activeTask.value && ['queued', 'running', 'paused'].includes(activeTask.value.status)))
const selectedSet = computed(() => new Set(selectedIDs.value))
const selectedInstances = computed(() => instances.value.filter((item) => selectedSet.value.has(item.id)))
const projectInstances = computed(() => {
  const projectTypeIDs = new Set(projectTypes.value.map((item) => item.id))
  return instances.value.filter((item) => projectTypeIDs.has(item.service_type_id))
})
const filteredInstances = computed(() => {
  const query = keyword.value.trim().toLocaleLowerCase()
  return projectInstances.value.filter((item) => {
    const type = typesByID.value.get(item.service_type_id)
    const host = hostsByID.value.get(item.host_id)
    if (selectedEnvironmentID.value && type?.environment_id !== selectedEnvironmentID.value) return false
    if (selectedTypeID.value && item.service_type_id !== selectedTypeID.value) return false
    if (!query) return true
    const searchable = [item.name, item.image, item.note, item.rollout_group, type?.name, host?.name, host?.region, host?.zone]
    return searchable.some((value) => value?.toLocaleLowerCase().includes(query))
  }).sort((left, right) => rolloutGroup(left).name.localeCompare(rolloutGroup(right).name) || left.name.localeCompare(right.name))
})
const targetGroups = computed<RolloutTargetGroup[]>(() => {
  const grouped = new Map<string, RolloutTargetGroup>()
  for (const item of filteredInstances.value) {
    const group = rolloutGroup(item)
    let bucket = grouped.get(group.key)
    if (!bucket) {
      bucket = { key: group.key, name: group.name, instances: [] }
      grouped.set(group.key, bucket)
    }
    bucket.instances.push(item)
  }
  return [...grouped.values()].sort((left, right) => left.name.localeCompare(right.name))
})
const historyForProject = computed(() => selectedProjectID.value
  ? rolloutHistory.value.filter((job) => job.project_id === selectedProjectID.value)
  : rolloutHistory.value)
const selectedServiceTypes = computed(() => {
  const ids = new Set(selectedInstances.value.map((item) => item.service_type_id))
  return [...ids].map((id) => typesByID.value.get(id)).filter((item): item is ServiceType => Boolean(item))
})
const missingProbeTypes = computed(() => selectedServiceTypes.value.filter((item) => !isProbeConfigured(item.readiness_probe)))

watch(selectedProjectID, (id) => {
  sessionStorage.setItem('ops_rollout_project_id', id)
  selectedEnvironmentID.value = ''
  selectedTypeID.value = ''
  selectedIDs.value = []
})

watch(selectedEnvironmentID, () => { selectedTypeID.value = '' })
watch(() => rolloutForm.max_parallel, (value) => {
  if (rolloutForm.max_parallel_per_group > value) rolloutForm.max_parallel_per_group = value
})

function rolloutGroup(item: ServiceInstance): { key: string; name: string } {
  const type = typesByID.value.get(item.service_type_id)
  const host = hostsByID.value.get(item.host_id)
  const region = host?.region?.trim() || '未标注机房'
  const zone = host?.zone?.trim() || item.host_id
  const serviceGroup = item.rollout_group?.trim() || type?.name || '未知服务类型'
  const key = JSON.stringify([type?.project_id || selectedProjectID.value, type?.environment_id || '', item.service_type_id, region, zone, serviceGroup])
  const zoneName = host?.zone?.trim() || host?.name || item.host_id
  const typeName = type?.name || '未知服务类型'
  return { key, name: [region, zoneName, typeName, serviceGroup].join(' / ') }
}

function isProbeConfigured(probe?: ServiceType['readiness_probe']) {
  if (!probe) return false
  if (probe.type === 'docker') return true
  if (probe.type === 'tcp') return probe.port >= 1 && probe.port <= 65535
  if (probe.type === 'http') return probe.port >= 1 && probe.port <= 65535 && (!probe.path || probe.path.startsWith('/'))
  return false
}

function probeSummary(probe?: ServiceType['readiness_probe']) {
  if (!probe?.type) return '未配置'
  if (probe.type === 'docker') return 'Docker HEALTHCHECK'
  if (probe.type === 'tcp') return `TCP :${probe.port}`
  if (probe.type === 'http') return `HTTP :${probe.port}${probe.path || '/health'}`
  return '无效配置'
}

function toggleInstance(id: string, event: Event) {
  const checked = (event.target as HTMLInputElement).checked
  const selected = new Set(selectedIDs.value)
  if (checked && !selected.has(id) && selected.size >= 10000) {
    ElMessage.warning('单个滚动发布任务最多选择 10000 个实例')
    return
  }
  if (checked) selected.add(id)
  else selected.delete(id)
  selectedIDs.value = [...selected]
}

function toggleGroup(group: RolloutTargetGroup, checked: boolean) {
  const selected = new Set(selectedIDs.value)
  const pendingCount = group.instances.filter((item) => checked ? !selected.has(item.id) : selected.has(item.id)).length
  if (checked && selected.size + pendingCount > 10000) {
    ElMessage.warning('单个滚动发布任务最多选择 10000 个实例')
    return
  }
  for (const item of group.instances) {
    if (checked) selected.add(item.id)
    else selected.delete(item.id)
  }
  selectedIDs.value = [...selected]
}

function groupSelection(group: RolloutTargetGroup) {
  const selectedCount = group.instances.filter((item) => selectedSet.value.has(item.id)).length
  return { selectedCount, allSelected: group.instances.length > 0 && selectedCount === group.instances.length, indeterminate: selectedCount > 0 && selectedCount < group.instances.length }
}

function selectFiltered() {
  const selected = new Set(selectedIDs.value)
  const newTargets = filteredInstances.value.filter((item) => !selected.has(item.id))
  if (selected.size + newTargets.length > 10000) {
    ElMessage.warning('单个滚动发布任务最多选择 10000 个实例')
    return
  }
  for (const item of newTargets) selected.add(item.id)
  selectedIDs.value = [...selected]
}

function clearSelection() {
  selectedIDs.value = []
}

function statusLabel(status: string) {
  return ({ running: '运行中', stopped: '已停止', error: '异常', unknown: '未知', not_deployed: '未部署', restarting: '重启中' } as Record<string, string>)[status] || status || '未知'
}

function openProbeEditor(type: ServiceType) {
  editingProbeTypeID.value = type.id
  probeForm.type = type.readiness_probe?.type || ''
  probeForm.port = type.readiness_probe?.port || 0
  probeForm.path = type.readiness_probe?.path || '/health'
  probeDialogVisible.value = true
}

async function saveProbe() {
  const type = typesByID.value.get(editingProbeTypeID.value)
  if (!type) return
  if ((probeForm.type === 'tcp' || probeForm.type === 'http') && (!Number.isInteger(probeForm.port) || probeForm.port < 1 || probeForm.port > 65535)) {
    ElMessage.warning('端口必须在 1 到 65535 之间')
    return
  }
  if (probeForm.type === 'http' && probeForm.path && !probeForm.path.startsWith('/')) {
    ElMessage.warning('HTTP 路径必须以 / 开头')
    return
  }
  probeSubmitting.value = true
  try {
    const updated = await serviceApi.update(type.id, {
      project_id: type.project_id,
      environment_id: type.environment_id,
      name: type.name,
      readiness_probe: { type: probeForm.type, port: probeForm.port, path: probeForm.type === 'http' ? (probeForm.path || '/health') : '' },
    })
    serviceTypes.value = serviceTypes.value.map((item) => item.id === updated.id ? updated : item)
    probeDialogVisible.value = false
    ElMessage.success('就绪检查配置已保存')
  } catch (error: any) {
    ElMessage.error(error?.response?.data?.error || '保存就绪检查配置失败')
  } finally {
    probeSubmitting.value = false
  }
}

async function editInstanceGroup(item: ServiceInstance) {
  let value: string
  try {
    const result = await ElMessageBox.prompt('输入用于分波的服组标签；留空时按服务类型分组。', '设置服组标签', {
      inputValue: item.rollout_group || '',
      confirmButtonText: '保存',
      cancelButtonText: '取消',
      inputPlaceholder: '例如 shard-a、world-1',
    })
    value = result.value.trim()
  } catch {
    return
  }
  try {
    await instanceApi.update(item.id, {
      service_type_id: item.service_type_id,
      host_id: item.host_id,
      name: item.name,
      rollout_group: value,
      image: item.image,
      params: item.params || [],
      env_text: item.env_text || '',
      network: item.network || '',
      port_mapping: item.port_mapping || '',
      restart_policy: item.restart_policy || 'unless-stopped',
      note: item.note || '',
    })
    instances.value = instances.value.map((current) => current.id === item.id ? { ...current, rollout_group: value } : current)
    ElMessage.success('服组标签已保存')
  } catch (error: any) {
    ElMessage.error(error?.response?.data?.error || '保存服组标签失败')
  }
}

async function loadData() {
  loading.value = true
  loadError.value = ''
  try {
    const [projectData, environmentData, typeData, instanceData, hostData] = await Promise.all([
      projectApi.list(),
      environmentApi.list(),
      serviceApi.list(),
      instanceApi.list(),
      auth.hasPermission('hosts.view') ? hostApi.list().catch(() => []) : Promise.resolve([] as Host[]),
    ])
    projects.value = projectData
    environments.value = environmentData
    serviceTypes.value = typeData
    instances.value = instanceData
    hosts.value = hostData
    if (!projects.value.some((item) => item.id === selectedProjectID.value)) selectedProjectID.value = projects.value[0]?.id || ''
  } catch (error: any) {
    loadError.value = error?.response?.data?.error || '读取服务数据失败'
  } finally {
    loading.value = false
  }
}

async function refreshHistory() {
  historyLoading.value = true
  try {
    rolloutHistory.value = await rolloutApi.list(100)
  } catch (error: any) {
    ElMessage.error(error?.response?.data?.error || '读取滚动发布历史失败')
  } finally {
    historyLoading.value = false
  }
}

function taskStatusLabel(status: string) {
  return ({ queued: '排队中', running: '运行中', paused: '已暂停', succeeded: '成功', partial: '部分失败', failed: '失败', pending: '等待中', health_check: '健康检查' } as Record<string, string>)[status] || status
}

function taskProgress(detail: RolloutDetail) {
  return detail.job.total ? Math.round((detail.job.completed / detail.job.total) * 100) : 0
}

function stopPolling() {
  if (pollTimer) clearInterval(pollTimer)
  pollTimer = null
}

function startPolling(id: string) {
  stopPolling()
  sessionStorage.setItem(storageKey, id)
  const poll = async () => {
    if (pollBusy || disposed) return
    pollBusy = true
    try {
      const detail = await rolloutApi.detail(id)
      rolloutDetail.value = detail
      pollError.value = ''
      if (!['queued', 'running'].includes(detail.job.status)) {
        stopPolling()
        await refreshHistory()
        await loadData()
      }
    } catch (error: any) {
      pollError.value = error?.response?.data?.error || '读取任务进度失败，将自动重试'
      if (error?.response?.status === 404) stopPolling()
    } finally {
      pollBusy = false
    }
  }
  void poll()
  pollTimer = setInterval(() => void poll(), 2500)
}

async function loadTask(id: string, poll = true) {
  try {
    const detail = await rolloutApi.detail(id)
    rolloutDetail.value = detail
    sessionStorage.setItem(storageKey, id)
    if (poll && ['queued', 'running'].includes(detail.job.status)) startPolling(id)
    else stopPolling()
  } catch (error: any) {
    ElMessage.error(error?.response?.data?.error || '读取滚动发布任务失败')
  }
}

async function loadInitialTask() {
  await refreshHistory()
  const savedID = sessionStorage.getItem(storageKey)
  const active = rolloutHistory.value.find((job) => ['queued', 'running', 'paused'].includes(job.status))
  const targetID = active?.id || ((savedID && rolloutHistory.value.some((job) => job.id === savedID)) ? savedID : undefined)
  if (targetID) await loadTask(targetID)
}

async function controlTask(action: 'pause' | 'resume' | 'retry') {
  const job = activeTask.value
  if (!job) return
  try {
    if (action === 'pause') await rolloutApi.pause(job.id, '操作员暂停')
    else if (action === 'resume') await rolloutApi.resume(job.id)
    else await rolloutApi.retryFailed(job.id)
    await loadTask(job.id, true)
    ElMessage.success(action === 'pause' ? '已请求暂停；当前派发中的实例会先结束本步' : action === 'resume' ? '任务已续跑' : '失败项已重新排队')
  } catch (error: any) {
    ElMessage.error(error?.response?.data?.error || '任务控制失败')
  }
}

async function createRollout() {
  if (!canManage.value || submitting.value || taskInProgress.value) return
  if (!selectedInstances.value.length) {
    ElMessage.warning('请先选择目标实例')
    return
  }
  if (selectedInstances.value.length > 10000) {
    ElMessage.warning('单个滚动发布任务最多包含 10000 个实例')
    return
  }
  if (missingProbeTypes.value.length) {
    ElMessage.warning(`请先为这些服务类型配置就绪检查：${missingProbeTypes.value.map((item) => item.name).join('、')}`)
    return
  }
  const image = rolloutForm.action === 'update_image' ? rolloutForm.image.trim() : undefined
  if (rolloutForm.action === 'update_image' && !image) {
    ElMessage.warning('请填写目标镜像')
    return
  }
  try {
    await ElMessageBox.confirm(
      `将为项目 ${projects.value.find((item) => item.id === selectedProjectID.value)?.name || ''} 创建滚动${rolloutForm.action === 'restart' ? '重启' : `镜像更新到 ${image}`}任务，目标 ${selectedInstances.value.length} 个实例。每组最多不可用 ${rolloutForm.max_unavailable_per_group} 台，全局并发 ${rolloutForm.max_parallel} 台；就绪检查连续通过后推进，失败时自动暂停。`,
      '确认创建滚动发布',
      { type: 'warning', confirmButtonText: '创建任务' },
    )
  } catch {
    return
  }
  submitting.value = true
  try {
    const detail = await rolloutApi.create({
      instance_ids: selectedInstances.value.map((item) => item.id),
      action: rolloutForm.action,
      ...(image ? { image } : {}),
      max_unavailable_per_group: rolloutForm.max_unavailable_per_group,
      max_parallel: rolloutForm.max_parallel,
      max_parallel_per_group: rolloutForm.max_parallel_per_group,
      health_timeout_seconds: rolloutForm.health_timeout_seconds,
    })
    rolloutDetail.value = detail
    selectedIDs.value = []
    rolloutForm.image = ''
    ElMessage.success('滚动发布任务已创建，后台将持久化推进')
    startPolling(detail.job.id)
    await refreshHistory()
  } catch (error: any) {
    ElMessage.error(error?.response?.data?.error || '创建滚动发布任务失败')
  } finally {
    submitting.value = false
  }
}

async function selectHistoryJob(job: RolloutJob) {
  await loadTask(job.id, ['queued', 'running'].includes(job.status))
}

onMounted(async () => {
  await loadData()
  await loadInitialTask()
})

onUnmounted(() => {
  disposed = true
  stopPolling()
})
</script>

<template>
  <AppLayout>
    <div class="rollouts-page" v-loading="loading">
      <header class="rollouts-header">
        <div>
          <div class="rollouts-breadcrumb">服务 <span>/</span> 滚动发布</div>
          <h1>滚动发布</h1>
          <p>按机房、可用区和服组分波，健康检查通过后自动推进；任务状态和审计记录保存在 Ops 数据库。</p>
        </div>
        <el-button plain :loading="loading" @click="loadData">刷新服务数据</el-button>
      </header>

      <el-alert v-if="loadError" :title="loadError" type="error" show-icon :closable="false" />

      <section class="rollout-filter-bar">
        <el-select v-model="selectedProjectID" placeholder="选择项目" aria-label="筛选项目" style="min-width: 190px">
          <el-option v-for="project in projects" :key="project.id" :label="project.name" :value="project.id" />
        </el-select>
        <el-select v-model="selectedEnvironmentID" clearable placeholder="全部环境" aria-label="筛选环境" style="min-width: 150px">
          <el-option v-for="environment in projectEnvironments" :key="environment.id" :label="environment.name" :value="environment.id" />
        </el-select>
        <el-select v-model="selectedTypeID" clearable placeholder="全部服务类型" aria-label="筛选服务类型" style="min-width: 170px">
          <el-option v-for="type in projectTypes.filter((item) => !selectedEnvironmentID || item.environment_id === selectedEnvironmentID)" :key="type.id" :label="type.name" :value="type.id" />
        </el-select>
        <el-input v-model="keyword" clearable placeholder="搜索实例、服组、主机或镜像" aria-label="搜索目标实例" style="max-width: 300px" />
      </section>

      <div class="rollouts-grid">
        <section class="rollout-card rollout-targets">
          <header class="rollout-card-header">
            <div><h2>发布目标</h2><p>按实际发布波次分组；已选择 {{ selectedInstances.length }} / 10000 台</p></div>
            <div class="target-actions">
              <el-button size="small" plain :disabled="!filteredInstances.length || taskInProgress" @click="selectFiltered">全选筛选结果</el-button>
              <el-button size="small" text :disabled="!selectedIDs.length" @click="clearSelection">清除选择</el-button>
            </div>
          </header>
          <div class="target-group-list">
            <article v-for="group in targetGroups" :key="group.key" class="target-group">
              <header class="target-group-header">
                <el-checkbox
                  :model-value="groupSelection(group).allSelected"
                  :indeterminate="groupSelection(group).indeterminate"
                  :disabled="!canManage || taskInProgress"
                  @change="toggleGroup(group, Boolean($event))"
                >{{ group.name }}</el-checkbox>
                <span class="target-group-count">{{ groupSelection(group).selectedCount }}/{{ group.instances.length }} 已选</span>
              </header>
              <div v-for="item in group.instances" :key="item.id" class="target-instance-row">
                <input
                  type="checkbox"
                  :checked="selectedSet.has(item.id)"
                  :disabled="!canManage || taskInProgress"
                  :aria-label="`选择实例 ${item.name}`"
                  @change="toggleInstance(item.id, $event)"
                />
                <div class="target-instance-main"><b>{{ item.name }}</b><small>{{ typesByID.get(item.service_type_id)?.name || '未知服务类型' }} · {{ hostsByID.get(item.host_id)?.name || item.host_id }}</small></div>
                <code>{{ item.image }}</code>
                <el-tag size="small" :type="item.status === 'running' ? 'success' : item.status === 'error' ? 'danger' : 'info'">{{ statusLabel(item.status) }}</el-tag>
                <el-button v-if="canManage" text size="small" @click="editInstanceGroup(item)">{{ item.rollout_group || '设置服组' }}</el-button>
              </div>
            </article>
            <el-empty v-if="!targetGroups.length" :description="selectedProjectID ? '当前筛选条件下没有服务实例' : '请选择项目'" :image-size="66" />
          </div>
        </section>

        <aside class="rollout-sidebar">
          <section class="rollout-card rollout-strategy">
            <header class="rollout-card-header"><div><h2>发布策略</h2><p>最大不可用数按每个分组独立计算</p></div></header>
            <el-form label-position="top" class="strategy-form">
              <el-form-item label="操作">
                <el-select v-model="rolloutForm.action" style="width: 100%">
                  <el-option label="滚动重启当前镜像" value="restart" />
                  <el-option label="滚动更新镜像" value="update_image" />
                </el-select>
              </el-form-item>
              <el-form-item v-if="rolloutForm.action === 'update_image'" label="目标镜像">
                <el-input v-model="rolloutForm.image" placeholder="例如 game:v1024-a1b2c3" />
              </el-form-item>
              <div class="strategy-grid">
                <el-form-item label="每组最大不可用数"><el-input-number v-model="rolloutForm.max_unavailable_per_group" :min="1" :max="1000" /></el-form-item>
                <el-form-item label="全局并发"><el-input-number v-model="rolloutForm.max_parallel" :min="1" :max="50" /></el-form-item>
                <el-form-item label="每组并发"><el-input-number v-model="rolloutForm.max_parallel_per_group" :min="1" :max="rolloutForm.max_parallel" /></el-form-item>
                <el-form-item label="健康检查超时（秒）"><el-input-number v-model="rolloutForm.health_timeout_seconds" :min="10" :max="1800" :step="30" /></el-form-item>
              </div>
            </el-form>
            <div v-if="missingProbeTypes.length" class="probe-warning">以下服务类型尚未设置就绪检查：{{ missingProbeTypes.map((item) => item.name).join('、') }}</div>
            <el-button class="create-rollout-button" type="primary" :loading="submitting" :disabled="!canManage || !selectedInstances.length || selectedInstances.length > 10000 || taskInProgress || (rolloutForm.action === 'update_image' && !rolloutForm.image.trim()) || missingProbeTypes.length > 0" @click="createRollout">
              创建滚动发布 · {{ selectedInstances.length }} 台
            </el-button>
          </section>

          <section class="rollout-card probe-config-card">
            <header class="rollout-card-header"><div><h2>就绪检查</h2><p>推进下一波前连续通过两次</p></div></header>
            <div v-for="type in projectTypes" :key="type.id" class="probe-config-row">
              <div><b>{{ type.name }}</b><small>{{ projectEnvironments.find((item) => item.id === type.environment_id)?.name || '环境未知' }} · {{ probeSummary(type.readiness_probe) }}</small></div>
              <el-button v-if="canManage" size="small" text type="primary" @click="openProbeEditor(type)">配置</el-button>
            </div>
            <el-empty v-if="!projectTypes.length" description="当前项目暂无服务类型" :image-size="50" />
          </section>

          <section v-if="rolloutDetail" class="rollout-card rollout-task-card">
            <header class="rollout-card-header">
              <div><h2>{{ rolloutDetail.job.action === 'restart' ? '滚动重启' : '滚动更新镜像' }} · {{ taskStatusLabel(rolloutDetail.job.status) }}</h2><p>{{ rolloutDetail.job.operator }} · {{ new Date(rolloutDetail.job.created_at).toLocaleString() }}</p></div>
              <div class="task-controls">
                <el-button v-if="['queued', 'running'].includes(rolloutDetail.job.status) && canManage" size="small" @click="controlTask('pause')">暂停</el-button>
                <el-button v-if="rolloutDetail.job.status === 'paused' && canManage" size="small" type="primary" @click="controlTask('resume')">续跑</el-button>
                <el-button v-if="rolloutDetail.job.failed > 0 && ['paused', 'partial', 'failed'].includes(rolloutDetail.job.status) && canManage" size="small" type="warning" @click="controlTask('retry')">只重试失败项</el-button>
              </div>
            </header>
            <div class="task-progress-row"><el-progress :percentage="taskProgress(rolloutDetail)" :status="rolloutDetail.job.status === 'failed' ? 'exception' : rolloutDetail.job.status === 'succeeded' ? 'success' : undefined" /><span>{{ rolloutDetail.job.completed }}/{{ rolloutDetail.job.total }} · 成功 {{ rolloutDetail.job.succeeded }} · 失败 {{ rolloutDetail.job.failed }}</span></div>
            <div v-if="rolloutDetail.job.pause_reason" class="task-message">{{ rolloutDetail.job.pause_reason }}</div>
            <div v-if="pollError" class="task-message">{{ pollError }}</div>
            <div class="task-groups"><el-tag v-for="group in rolloutDetail.job.groups" :key="group.key" size="small" effect="plain">{{ group.name }} · 基线不可用 {{ group.initial_unavailable }}/{{ group.max_unavailable }}</el-tag></div>
            <div class="task-items">
              <div v-for="item in rolloutDetail.items" :key="item.id" class="task-item-row">
                <span class="task-status-dot" :class="`status-${item.status}`"></span><b>{{ item.name }}</b><span>{{ taskStatusLabel(item.status) }}</span><small v-if="item.error">{{ item.error }}</small>
              </div>
            </div>
            <details v-if="rolloutDetail.events.length" class="task-audit">
              <summary>审计记录（{{ rolloutDetail.events.length }}）</summary>
              <div v-for="event in rolloutDetail.events.slice(-30).reverse()" :key="event.id"><time>{{ new Date(event.created_at).toLocaleString() }}</time><b>{{ event.operator }}</b><span>{{ event.message }}</span></div>
            </details>
          </section>

          <section class="rollout-card rollout-history-card">
            <header class="rollout-card-header"><div><h2>任务历史</h2><p>最多显示最近 100 个任务</p></div><el-button size="small" text :loading="historyLoading" @click="refreshHistory">刷新</el-button></header>
            <button v-for="job in historyForProject" :key="job.id" class="history-job-row" :class="{ selected: rolloutDetail?.job.id === job.id }" @click="selectHistoryJob(job)">
              <span><b>{{ job.action === 'restart' ? '滚动重启' : '滚动更新镜像' }}</b><small>{{ job.image || projects.find((item) => item.id === job.project_id)?.name || job.project_id }} · {{ new Date(job.created_at).toLocaleString() }}</small></span>
              <span><el-tag size="small" :type="job.status === 'succeeded' ? 'success' : job.status === 'paused' || job.status === 'partial' ? 'warning' : job.status === 'failed' ? 'danger' : 'info'">{{ taskStatusLabel(job.status) }}</el-tag><small>{{ job.completed }}/{{ job.total }}</small></span>
            </button>
            <el-empty v-if="!historyLoading && !historyForProject.length" description="暂无滚动发布任务" :image-size="50" />
          </section>
        </aside>
      </div>

      <el-dialog v-model="probeDialogVisible" title="配置服务就绪检查" width="480px">
        <el-form label-width="115px">
          <el-form-item label="检查方式">
            <el-select v-model="probeForm.type" style="width: 100%">
              <el-option label="未配置" value="" />
              <el-option label="Docker HEALTHCHECK" value="docker" />
              <el-option label="TCP 端口连通" value="tcp" />
              <el-option label="HTTP 健康接口" value="http" />
            </el-select>
          </el-form-item>
          <el-form-item v-if="probeForm.type === 'tcp' || probeForm.type === 'http'" label="主机发布端口">
            <el-input-number v-model="probeForm.port" :min="1" :max="65535" />
          </el-form-item>
          <el-form-item v-if="probeForm.type === 'http'" label="HTTP 路径">
            <el-input v-model="probeForm.path" placeholder="/health" />
          </el-form-item>
        </el-form>
        <p class="probe-dialog-hint">TCP/HTTP 检查由 Ops 服务访问主机配置的内网 IP 和发布端口。</p>
        <template #footer><el-button @click="probeDialogVisible = false">取消</el-button><el-button type="primary" :loading="probeSubmitting" @click="saveProbe">保存配置</el-button></template>
      </el-dialog>
    </div>
  </AppLayout>
</template>

<style scoped>
.rollouts-page { width: 100%; min-width: 0; padding: 26px 30px 34px; box-sizing: border-box; }
.rollouts-header { display: flex; align-items: flex-start; justify-content: space-between; gap: 18px; margin-bottom: 20px; }
.rollouts-breadcrumb { color: var(--ops-text-secondary); font-size: 12px; }
.rollouts-breadcrumb span { padding: 0 6px; color: #a7b2c1; }
.rollouts-header h1 { margin: 7px 0 5px; font-size: 23px; font-weight: 650; color: var(--ops-text); }
.rollouts-header p { margin: 0; color: var(--ops-text-secondary); font-size: 13px; line-height: 1.5; }
.rollout-filter-bar { display: flex; flex-wrap: wrap; gap: 10px; margin: 18px 0; padding: 14px; border: 1px solid var(--ops-border); border-radius: 10px; background: #fff; }
.rollouts-grid { display: grid; grid-template-columns: minmax(0, 1.45fr) minmax(360px, .85fr); align-items: start; gap: 16px; }
.rollout-sidebar { display: flex; flex-direction: column; gap: 14px; min-width: 0; }
.rollout-card { min-width: 0; overflow: hidden; border: 1px solid var(--ops-border); border-radius: 10px; background: #fff; box-shadow: 0 2px 8px #1d2e4608; }
.rollout-card-header { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 15px 17px; border-bottom: 1px solid #edf0f4; }
.rollout-card-header h2 { margin: 0; color: var(--ops-text); font-size: 14px; font-weight: 650; }
.rollout-card-header p { margin: 4px 0 0; color: var(--ops-text-secondary); font-size: 11px; }
.target-actions { display: flex; align-items: center; flex-shrink: 0; }
.target-group-list { max-height: calc(100vh - 300px); min-height: 260px; overflow: auto; padding: 4px 14px 14px; }
.target-group { margin-top: 10px; border: 1px solid #e9edf3; border-radius: 8px; overflow: hidden; }
.target-group-header { display: flex; align-items: center; gap: 10px; padding: 10px 12px; background: #f8faff; }
.target-group-header .el-checkbox { flex: 1; min-width: 0; }
.target-group-header .el-checkbox :deep(.el-checkbox__label) { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.target-group-count { flex-shrink: 0; color: var(--ops-text-secondary); font-size: 11px; }
.target-instance-row input { accent-color: var(--ops-primary); }
.target-instance-row { display: grid; grid-template-columns: 18px minmax(130px, 1fr) minmax(100px, .75fr) auto auto; align-items: center; gap: 9px; min-height: 42px; padding: 5px 12px; border-top: 1px solid #f0f2f5; }
.target-instance-main { display: flex; min-width: 0; flex-direction: column; gap: 3px; }
.target-instance-main b { overflow: hidden; color: var(--ops-text); font-size: 12px; text-overflow: ellipsis; white-space: nowrap; }
.target-instance-main small, .target-instance-row code { overflow: hidden; color: var(--ops-text-secondary); font-size: 10px; text-overflow: ellipsis; white-space: nowrap; }
.target-instance-row code { font-family: Consolas, monospace; }
.rollout-strategy { padding-bottom: 14px; }
.strategy-form { padding: 14px 16px 0; }
.strategy-form :deep(.el-form-item) { margin-bottom: 13px; }
.strategy-form :deep(.el-form-item__label) { padding-bottom: 4px; color: var(--ops-text-secondary); font-size: 11px; }
.strategy-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 0 12px; }
.strategy-grid .el-input-number { width: 100%; }
.probe-warning { margin: 0 16px 12px; color: var(--ops-warning); font-size: 11px; line-height: 1.5; }
.create-rollout-button { width: calc(100% - 32px); margin: 0 16px; }
.probe-config-row { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 10px 16px; border-bottom: 1px solid #f0f2f5; }
.probe-config-row:last-child { border-bottom: 0; }
.probe-config-row > div { display: flex; min-width: 0; flex-direction: column; gap: 4px; }
.probe-config-row b { color: var(--ops-text); font-size: 12px; }
.probe-config-row small { color: var(--ops-text-secondary); font-size: 10px; }
.rollout-task-card { padding-bottom: 12px; }
.rollout-task-card .rollout-card-header { align-items: flex-start; }
.rollout-task-card .rollout-card-header p { max-width: 220px; }
.task-controls { display: flex; flex-wrap: wrap; justify-content: flex-end; gap: 5px; }
.task-progress-row { display: flex; align-items: center; gap: 12px; padding: 12px 16px 8px; }
.task-progress-row .el-progress { flex: 1; min-width: 0; }
.task-progress-row > span { flex-shrink: 0; color: var(--ops-text-secondary); font-size: 10px; }
.task-message { margin: 3px 16px 8px; color: var(--ops-warning); font-size: 11px; }
.task-groups { display: flex; flex-wrap: wrap; gap: 5px; padding: 0 16px 9px; }
.task-items { max-height: 250px; overflow: auto; margin: 0 16px; border-top: 1px solid #edf0f4; }
.task-item-row { display: grid; grid-template-columns: 7px minmax(80px, 1fr) auto; align-items: center; gap: 7px; padding: 6px 1px; border-bottom: 1px solid #f3f5f7; color: var(--ops-text-secondary); font-size: 10px; }
.task-item-row b { overflow: hidden; color: var(--ops-text); text-overflow: ellipsis; white-space: nowrap; }
.task-item-row small { grid-column: 2 / -1; color: var(--ops-danger); overflow-wrap: anywhere; }
.task-status-dot { width: 7px; height: 7px; border-radius: 50%; background: #b7c0cd; }
.task-status-dot.status-running, .task-status-dot.status-health_check { background: var(--ops-primary); }
.task-status-dot.status-succeeded { background: var(--ops-success); }
.task-status-dot.status-failed { background: var(--ops-danger); }
.task-audit { margin: 10px 16px 0; padding: 9px; border: 1px solid var(--ops-border); border-radius: 7px; font-size: 10px; }
.task-audit summary { cursor: pointer; color: var(--ops-primary); font-weight: 600; }
.task-audit > div { display: grid; grid-template-columns: 115px 78px minmax(0, 1fr); gap: 6px; padding-top: 7px; color: var(--ops-text-secondary); }
.task-audit time { font-variant-numeric: tabular-nums; }
.rollout-history-card { padding-bottom: 4px; }
.history-job-row { display: flex; width: 100%; align-items: center; justify-content: space-between; gap: 12px; padding: 10px 15px; border: 0; border-bottom: 1px solid #f0f2f5; background: #fff; color: var(--ops-text); text-align: left; cursor: pointer; }
.history-job-row:hover, .history-job-row.selected { background: #f5f8ff; }
.history-job-row > span { display: flex; min-width: 0; flex-direction: column; gap: 4px; }
.history-job-row > span:last-child { align-items: flex-end; flex-shrink: 0; }
.history-job-row b { font-size: 11px; }
.history-job-row small { max-width: 240px; overflow: hidden; color: var(--ops-text-secondary); font-size: 9px; text-overflow: ellipsis; white-space: nowrap; }
.probe-dialog-hint { margin: 0; color: var(--ops-text-secondary); font-size: 11px; }
@media (max-width: 1050px) { .rollouts-grid { grid-template-columns: 1fr; } .target-group-list { max-height: 55vh; } }
@media (max-width: 700px) {
  .rollouts-page { padding: 18px 12px 28px; }
  .rollouts-header { flex-direction: column; }
  .rollout-filter-bar > * { width: 100%; max-width: none !important; }
  .target-instance-row { grid-template-columns: 18px minmax(100px, 1fr) auto; }
  .target-instance-row code { display: none; }
  .target-instance-row > .el-button { grid-column: 2 / -1; justify-self: start; }
  .target-actions { flex-wrap: wrap; justify-content: flex-end; }
  .task-audit > div { grid-template-columns: 1fr; gap: 2px; }
}
</style>
