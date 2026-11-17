<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus, Refresh } from '@element-plus/icons-vue'
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
const createVisible = ref(false)
const detailVisible = ref(false)
const rolloutDetail = ref<RolloutDetail | null>(null)
const rolloutHistory = ref<RolloutJob[]>([])
const historyLoading = ref(false)
const pollError = ref('')
let refreshTimer: ReturnType<typeof setInterval> | undefined
const rolloutForm = reactive({
  action: 'update_image' as RolloutAction,
  image: '',
  max_unavailable_per_group: 1,
  max_parallel: 5,
  max_parallel_per_group: 2,
  health_timeout_seconds: 300,
})
const probeForm = reactive<{ type: ProbeType; port: number; path: string }>({ type: '', port: 0, path: '/health' })

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
    if (!selectedTypeID.value || item.service_type_id !== selectedTypeID.value) return false
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
watch(selectedTypeID, () => { selectedIDs.value = [] })
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

async function refreshHistory(silent = false) {
  if (!silent) historyLoading.value = true
  try {
    rolloutHistory.value = await rolloutApi.list(100)
  } catch (error: any) {
    if (!silent) ElMessage.error(error?.response?.data?.error || '读取滚动发布历史失败')
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

async function loadTask(id: string) {
  try { rolloutDetail.value = await rolloutApi.detail(id); pollError.value = '' }
  catch (error: any) { pollError.value = error?.response?.data?.error || '读取任务详情失败' }
}
async function showTask(job: RolloutJob) {
  rolloutDetail.value = null
  detailVisible.value = true
  await loadTask(job.id)
}
async function controlTask(action: 'pause' | 'resume' | 'retry') {
  const job = rolloutDetail.value?.job
  if (!job) return
  try {
    if (action === 'pause') await rolloutApi.pause(job.id, '操作员暂停')
    else if (action === 'resume') await rolloutApi.resume(job.id)
    else await rolloutApi.retryFailed(job.id)
    await Promise.all([loadTask(job.id), refreshHistory()])
    ElMessage.success('任务操作已提交')
  } catch (error: any) { ElMessage.error(error?.response?.data?.error || '任务操作失败') }
}

async function createRollout() {
  if (!canManage.value || submitting.value) return
  if (new Set(selectedInstances.value.map((item) => item.service_type_id)).size > 1) {
    ElMessage.warning('一个滚动发布任务只能选择同一服务类型')
    return
  }
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
    await rolloutApi.create({
      instance_ids: selectedInstances.value.map((item) => item.id),
      action: rolloutForm.action,
      ...(image ? { image } : {}),
      max_unavailable_per_group: rolloutForm.max_unavailable_per_group,
      max_parallel: rolloutForm.max_parallel,
      max_parallel_per_group: rolloutForm.max_parallel_per_group,
      health_timeout_seconds: rolloutForm.health_timeout_seconds,
    })
    selectedIDs.value = []
    rolloutForm.image = ''
    createVisible.value = false
    ElMessage.success('滚动发布任务已创建，后台将持久化推进')
    await refreshHistory()
  } catch (error: any) {
    ElMessage.error(error?.response?.data?.error || '创建滚动发布任务失败')
  } finally {
    submitting.value = false
  }
}

onMounted(async () => {
  await Promise.all([loadData(), refreshHistory()])
  refreshTimer = setInterval(() => {
    if (rolloutHistory.value.some(job => ['queued', 'running'].includes(job.status))) void refreshHistory(true)
    if (detailVisible.value && rolloutDetail.value && ['queued', 'running'].includes(rolloutDetail.value.job.status)) void loadTask(rolloutDetail.value.job.id)
  }, 5000)
})
onUnmounted(() => { if (refreshTimer) clearInterval(refreshTimer) })
</script>

<template>
  <AppLayout>
    <div class="page-content-loading rollouts-page" v-loading="loading">
      <div class="page-heading page-heading-modern page-heading-actions-only">
        <el-button :icon="Refresh" :loading="historyLoading" @click="refreshHistory()">刷新</el-button>
        <el-button v-if="canManage" type="primary" :icon="Plus" @click="createVisible = true">创建任务</el-button>
      </div>
      <el-alert v-if="loadError" :title="loadError" type="error" show-icon :closable="false" />
      <section class="task-list">
        <header class="task-list-header"><div><h2>任务列表</h2><p>最近 100 个任务</p></div></header>
        <el-table :data="rolloutHistory" class="rollout-table" v-loading="historyLoading" empty-text="暂无滚动发布任务" @row-click="showTask">
          <el-table-column label="任务类型" min-width="140"><template #default="{ row }">{{ row.action === 'restart' ? '滚动重启' : '滚动更新镜像' }}</template></el-table-column>
          <el-table-column label="项目" min-width="120"><template #default="{ row }">{{ projects.find(item => item.id === row.project_id)?.name || row.project_id }}</template></el-table-column>
          <el-table-column prop="image" label="目标镜像" min-width="200" show-overflow-tooltip />
          <el-table-column label="状态" width="110"><template #default="{ row }"><el-tag size="small" :type="row.status === 'succeeded' ? 'success' : row.status === 'failed' ? 'danger' : ['paused', 'partial'].includes(row.status) ? 'warning' : 'info'">{{ taskStatusLabel(row.status) }}</el-tag></template></el-table-column>
          <el-table-column label="进度" width="110"><template #default="{ row }">{{ row.completed }} / {{ row.total }}</template></el-table-column>
          <el-table-column prop="operator" label="操作人" width="110" />
          <el-table-column label="创建时间" min-width="180"><template #default="{ row }">{{ new Date(row.created_at).toLocaleString() }}</template></el-table-column>
          <el-table-column label="操作" width="90"><template #default="{ row }"><el-button text type="primary" @click.stop="showTask(row)">详情</el-button></template></el-table-column>
        </el-table>
      </section>
      <el-dialog v-model="createVisible" title="创建滚动发布任务" width="min(1400px, 94vw)" class="create-dialog" destroy-on-close>
        <div class="create-dialog-content">
      <section class="rollout-filter-bar">
        <el-select v-model="selectedProjectID" placeholder="选择项目" aria-label="筛选项目" style="width: 190px">
          <el-option v-for="project in projects" :key="project.id" :label="project.name" :value="project.id" />
        </el-select>
        <el-select v-model="selectedEnvironmentID" clearable placeholder="全部环境" aria-label="筛选环境" style="width: 150px">
          <el-option v-for="environment in projectEnvironments" :key="environment.id" :label="environment.name" :value="environment.id" />
        </el-select>
        <el-select v-model="selectedTypeID" clearable placeholder="请选择服务类型" aria-label="选择服务类型" style="width: 170px">
          <el-option v-for="type in projectTypes.filter((item) => !selectedEnvironmentID || item.environment_id === selectedEnvironmentID)" :key="type.id" :label="type.name" :value="type.id" />
        </el-select>
        <el-input v-model="keyword" clearable placeholder="搜索实例、服组、主机或镜像" aria-label="搜索目标实例" style="max-width: 300px" />
      </section>

      <div class="rollouts-grid">
        <section class="rollout-card rollout-targets">
          <header class="rollout-card-header">
            <div><h2>发布目标</h2><p>每个任务只能选择一种服务类型；已选择 {{ selectedInstances.length }} / 10000 台</p></div>
            <div class="target-actions">
              <el-button size="small" plain :disabled="!filteredInstances.length" @click="selectFiltered">全选筛选结果</el-button>
              <el-button size="small" text :disabled="!selectedIDs.length" @click="clearSelection">清除选择</el-button>
            </div>
          </header>
          <div class="target-group-list">
            <article v-for="group in targetGroups" :key="group.key" class="target-group">
              <header class="target-group-header">
                <el-checkbox
                  :model-value="groupSelection(group).allSelected"
                  :indeterminate="groupSelection(group).indeterminate"
                  :disabled="!canManage"
                  @change="toggleGroup(group, Boolean($event))"
                >{{ group.name }}</el-checkbox>
                <span class="target-group-count">{{ groupSelection(group).selectedCount }}/{{ group.instances.length }} 已选</span>
              </header>
              <div v-for="item in group.instances" :key="item.id" class="target-instance-row">
                <input
                  type="checkbox"
                  :checked="selectedSet.has(item.id)"
                  :disabled="!canManage"
                  :aria-label="`选择实例 ${item.name}`"
                  @change="toggleInstance(item.id, $event)"
                />
                <div class="target-instance-main"><b>{{ item.name }}</b><small>{{ typesByID.get(item.service_type_id)?.name || '未知服务类型' }} · {{ hostsByID.get(item.host_id)?.name || item.host_id }}</small></div>
                <code>{{ item.image }}</code>
                <el-tag size="small" :type="item.status === 'running' ? 'success' : item.status === 'error' ? 'danger' : 'info'">{{ statusLabel(item.status) }}</el-tag>
                <el-button v-if="canManage" text size="small" @click="editInstanceGroup(item)">{{ item.rollout_group || '设置服组' }}</el-button>
              </div>
            </article>
            <el-empty v-if="!targetGroups.length" :description="!selectedProjectID ? '请选择项目' : !selectedTypeID ? '请先选择服务类型' : '当前筛选条件下没有服务实例'" :image-size="66" />
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
            <el-button class="create-rollout-button" type="primary" :loading="submitting" :disabled="!canManage || !selectedInstances.length || selectedInstances.length > 10000 || (rolloutForm.action === 'update_image' && !rolloutForm.image.trim()) || missingProbeTypes.length > 0" @click="createRollout">
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

        </aside>
      </div>

        </div>
      </el-dialog>
      <el-dialog v-model="detailVisible" title="任务详情" width="min(800px, 94vw)">
        <el-alert v-if="pollError" :title="pollError" type="error" :closable="false" />
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

      </el-dialog>
      <el-dialog v-model="probeDialogVisible" append-to-body title="配置服务就绪检查" width="480px">
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
.rollouts-page { width: 100%; min-width: 0; }
.task-list { overflow: hidden; border: 1px solid var(--ops-border); border-radius: 12px; background: #fff; box-shadow: 0 2px 8px rgba(28, 47, 78, .035); }
.task-list-header { display: flex; align-items: center; padding: 18px 20px; border-bottom: 1px solid var(--ops-border); }
.task-list-header h2 { margin: 0; color: var(--ops-text); font-size: 15px; font-weight: 600; }
.task-list-header p { margin: 4px 0 0; color: var(--ops-text-secondary); font-size: 12px; }
.rollout-table { --el-table-header-bg-color: #fff; --el-table-row-hover-bg-color: #f8faff; }
.rollout-table :deep(th.el-table__cell) { padding: 13px 0; color: var(--ops-text-secondary); font-size: 12px; font-weight: 400; }
.rollout-table :deep(td.el-table__cell) { padding: 17px 0; }
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
.task-list :deep(.el-table__row) { cursor: pointer; }
.create-dialog-content { max-height: 75vh; overflow: auto; }
.create-dialog-content .rollout-filter-bar { margin-top: 0; }
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
.probe-dialog-hint { margin: 0; color: var(--ops-text-secondary); font-size: 11px; }
@media (max-width: 1050px) { .rollouts-grid { grid-template-columns: 1fr; } .target-group-list { max-height: 55vh; } }
@media (max-width: 700px) {
  .rollout-filter-bar > * { width: 100%; max-width: none !important; }
  .target-instance-row { grid-template-columns: 18px minmax(100px, 1fr) auto; }
  .target-instance-row code { display: none; }
  .target-instance-row > .el-button { grid-column: 2 / -1; justify-self: start; }
  .target-actions { flex-wrap: wrap; justify-content: flex-end; }
}
</style>
