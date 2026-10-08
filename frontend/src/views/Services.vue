<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  Plus, Edit, Delete, Box, VideoPlay, VideoPause, RefreshRight, Refresh, Upload,
  Search, MoreFilled, Close, ArrowRight, FolderOpened,
} from '@element-plus/icons-vue'
import { useAuthStore } from '../stores/auth'
import { environmentApi, type Environment, type EnvironmentForm } from '../api/environments'
import { serviceApi, type ServiceType, type ServiceTypeForm } from '../api/services'
import { hostApi, type Host } from '../api/hosts'
import { projectApi, type Project } from '../api/projects'
import { instanceApi, type ServiceInstance, type ServiceInstanceForm, type ContainerDetail, type RestartPolicy } from '../api/instances'
import { eventApi, type InstanceEvent } from '../api/events'
import AppLayout from '../components/AppLayout.vue'

const auth = useAuthStore()
const route = useRoute()
const router = useRouter()
const environments = ref<Environment[]>([])
const types = ref<ServiceType[]>([])
const instances = ref<ServiceInstance[]>([])
const hosts = ref<Host[]>([])
const projects = ref<Project[]>([])
const latestStartedAt = ref<Record<string, string>>({})
const loading = ref(false)
const loadError = ref(false)
const submitting = ref(false)
const canManage = computed(() => auth.hasPermission('services.manage'))
let servicesDisposed = false

// -------- 顶部筛选状态：项目 -> 环境 -> 服务类型 --------
const activeProjectId = ref<string>(typeof route.query.project === 'string' ? route.query.project : '')
const activeEnvironmentId = ref<string>(typeof route.query.environment === 'string' ? route.query.environment : '')
const activeTypeId = ref<string>('')
const keyword = ref('')

function environmentsInProject(projectId: string): Environment[] {
  return environments.value.filter((e) => e.project_id === projectId)
}

function typesInEnvironment(environmentId: string): ServiceType[] {
  return types.value.filter((t) => t.environment_id === environmentId && t.project_id === activeProjectId.value)
}

function countInType(typeId: string): number {
  return instances.value.filter((i) => i.service_type_id === typeId).length
}

const activeProject = computed(() => projects.value.find((p) => p.id === activeProjectId.value) || null)
const activeEnvironment = computed(() => environments.value.find((e) => e.id === activeEnvironmentId.value) || null)
const activeType = computed(() => types.value.find((t) => t.id === activeTypeId.value) || null)

function imageVersion(image?: string | null): string {
  if (!image) return '—'
  const separator = image.lastIndexOf(':')
  return separator > image.lastIndexOf('/') ? image.slice(separator + 1) : image
}

const visibleInstances = computed(() => {
  const allowedTypes = new Set(typesInEnvironment(activeEnvironmentId.value).map((t) => t.id))
  return instances.value.filter((i) => allowedTypes.has(i.service_type_id) && (!activeTypeId.value || i.service_type_id === activeTypeId.value))
})

const environmentInstanceCount = computed(() => typesInEnvironment(activeEnvironmentId.value).reduce((count, type) => count + countInType(type.id), 0))

function selectProject(id: string) {
  activeProjectId.value = id
  onProjectChange()
}

function selectEnvironment(id: string) {
  activeEnvironmentId.value = id
  onEnvironmentChange()
}

const filteredInstances = computed(() => {
  const normalizedKeyword = keyword.value.trim().toLowerCase()
  if (!normalizedKeyword) return visibleInstances.value
  return visibleInstances.value.filter((item) => [item.name, item.image, item.note]
    .some((value) => value?.toLowerCase().includes(normalizedKeyword)))
})

async function loadAll() {
  loading.value = true
  loadError.value = false
  try {
    const [availableEnvironments, serviceTypes, serviceInstances, availableHosts, availableProjects] = await Promise.all([
      environmentApi.list(),
      serviceApi.list(),
      instanceApi.list(),
      hostApi.list(),
      projectApi.list(),
    ])
    environments.value = availableEnvironments
    types.value = serviceTypes
    instances.value = serviceInstances
    hosts.value = availableHosts
    projects.value = availableProjects
    if (!activeProjectId.value || !projects.value.some((p) => p.id === activeProjectId.value)) {
      activeProjectId.value = projects.value[0]?.id || ''
    }
    if (!activeEnvironmentId.value || !environmentsInProject(activeProjectId.value).some((e) => e.id === activeEnvironmentId.value)) {
      activeEnvironmentId.value = environmentsInProject(activeProjectId.value)[0]?.id || ''
    }
    if (activeTypeId.value && !typesInEnvironment(activeEnvironmentId.value).some((t) => t.id === activeTypeId.value)) {
      activeTypeId.value = ''
    }
    if (activeInstanceId.value && !instances.value.some((i) => i.id === activeInstanceId.value)) {
      activeInstanceId.value = ''
    }
  } catch {
    loadError.value = true
    ElMessage.error('数据加载失败')
  } finally {
    loading.value = false
  }
}

// 切换项目时清除上一个项目的环境和实例选择。
function onProjectChange() {
  activeEnvironmentId.value = environmentsInProject(activeProjectId.value)[0]?.id || ''
  onEnvironmentChange()
}

// 环境切换后默认展示该环境下的全部服务。
function onEnvironmentChange() {
  activeTypeId.value = ''
  onTypeChange()
}

// 切换服务类型时关闭当前详情，避免显示其他分组的实例。
function onTypeChange() {
  activeInstanceId.value = ''
  detailVisible.value = false
  keyword.value = ''
}

// -------- 环境表单 --------
const environmentDialogVisible = ref(false)
const editingEnvironmentId = ref('')
const environmentForm = reactive<EnvironmentForm>({ project_id: '', name: '' })
const environmentSubmitting = ref(false)

function openCreateEnvironment() {
  editingEnvironmentId.value = ''
  Object.assign(environmentForm, { project_id: activeProjectId.value || projects.value[0]?.id || '', name: '' })
  environmentDialogVisible.value = true
}

function openEditEnvironment(env: Environment) {
  editingEnvironmentId.value = env.id
  Object.assign(environmentForm, { project_id: env.project_id, name: env.name })
  environmentDialogVisible.value = true
}

async function saveEnvironment() {
  if (!environmentForm.project_id) {
    ElMessage.warning('请先选择所属项目')
    return
  }
  if (!environmentForm.name.trim()) {
    ElMessage.warning('请填写环境名称，例如 beta、dev、dev-1')
    return
  }
  environmentSubmitting.value = true
  try {
    const payload = { ...environmentForm, name: environmentForm.name.trim() }
    if (editingEnvironmentId.value) await environmentApi.update(editingEnvironmentId.value, payload)
    else await environmentApi.create(payload)
    ElMessage.success(editingEnvironmentId.value ? '环境已更新' : '环境已创建')
    environmentDialogVisible.value = false
    await loadAll()
  } catch (error: any) {
    ElMessage.error(error?.response?.data?.error || '保存失败')
  } finally {
    environmentSubmitting.value = false
  }
}

async function removeEnvironment(env: Environment) {
  try {
    await ElMessageBox.confirm(`确定删除环境"${env.name}"吗？删除环境不会自动删除其下的服务类型或实例，但它们将失去所属环境。`, '删除确认', { type: 'warning' })
    await environmentApi.remove(env.id)
    ElMessage.success('环境已删除')
    if (activeEnvironmentId.value === env.id) activeEnvironmentId.value = ''
    await loadAll()
  } catch (error: any) {
    if (error !== 'cancel' && error !== 'close') ElMessage.error(error?.response?.data?.error || '删除失败')
  }
}

// -------- 服务类型表单 --------
const typeDialogVisible = ref(false)
const editingTypeId = ref('')
const typeForm = reactive<ServiceTypeForm>({ project_id: '', environment_id: '', name: '' })

function openCreateType() {
  editingTypeId.value = ''
  Object.assign(typeForm, {
    project_id: activeProjectId.value || projects.value[0]?.id || '',
    environment_id: activeEnvironmentId.value || environmentsInProject(activeProjectId.value)[0]?.id || '',
    name: '',
  })
  typeDialogVisible.value = true
}

function openEditType(type: ServiceType) {
  editingTypeId.value = type.id
  Object.assign(typeForm, { project_id: type.project_id, environment_id: type.environment_id, name: type.name })
  typeDialogVisible.value = true
}

async function saveType() {
  if (!typeForm.project_id) {
    ElMessage.warning('请先选择所属项目')
    return
  }
  if (!typeForm.environment_id) {
    ElMessage.warning('请先选择所属环境')
    return
  }
  if (!typeForm.name.trim()) {
    ElMessage.warning('请填写类型名称')
    return
  }
  submitting.value = true
  try {
    const payload = { ...typeForm, name: typeForm.name.trim() }
    if (editingTypeId.value) await serviceApi.update(editingTypeId.value, payload)
    else await serviceApi.create(payload)
    ElMessage.success(editingTypeId.value ? '服务类型已更新' : '服务类型已创建')
    typeDialogVisible.value = false
    await loadAll()
  } catch (error: any) {
    ElMessage.error(error?.response?.data?.error || '保存失败')
  } finally {
    submitting.value = false
  }
}

async function removeType(type: ServiceType) {
  const relatedCount = countInType(type.id)
  try {
    await ElMessageBox.confirm(
      relatedCount > 0
        ? `类型"${type.name}"下还有 ${relatedCount} 个服务实例，删除类型不会自动删除实例，但这些实例将失去所属类型。是否继续？`
        : `确定删除服务类型"${type.name}"吗？`,
      '删除确认',
      { type: 'warning' },
    )
    await serviceApi.remove(type.id)
    ElMessage.success('服务类型已删除')
    if (activeTypeId.value === type.id) activeTypeId.value = ''
    await loadAll()
  } catch (error: any) {
    if (error !== 'cancel' && error !== 'close') ElMessage.error(error?.response?.data?.error || '删除失败')
  }
}

// -------- 实例表单 --------
const instanceDialogVisible = ref(false)
const editingInstanceId = ref('')
const instanceForm = reactive<ServiceInstanceForm>({ service_type_id: '', host_id: '', name: '', image: '', params: [], env_text: '', network: '', port_mapping: '', restart_policy: 'unless-stopped', note: '' })
const instanceSubmitting = ref(false)
const restartPolicyLabels: Record<RestartPolicy, string> = {
  no: '不自动重启',
  always: '总是重启',
  'unless-stopped': '除非手动停止，否则重启',
  'on-failure': '异常退出时重启',
}
function restartPolicyLabel(policy?: RestartPolicy): string {
  return restartPolicyLabels[policy || 'unless-stopped']
}
function hostBelongsToProject(host: Host, projectId: string): boolean {
  return host.project_id === projectId || host.project_ids?.includes(projectId) === true
}

const hostById = computed(() => new Map(hosts.value.map((host) => [host.id, host])))

const instanceHostOptions = computed(() => {
  const serviceType = types.value.find((item) => item.id === instanceForm.service_type_id)
  if (!serviceType) return []
  return hosts.value.filter((host) => hostBelongsToProject(host, serviceType.project_id))
})

function openCreateInstance() {
  const targetType = activeType.value || typesInEnvironment(activeEnvironmentId.value)[0]
  if (!targetType) return
  editingInstanceId.value = ''
  Object.assign(instanceForm, {
    service_type_id: targetType.id,
    host_id: hosts.value.find((host) => hostBelongsToProject(host, targetType.project_id))?.id || '',
    name: '',
    image: '',
    params: [],
    env_text: activeProject.value?.env_vars || '',
    network: '',
    port_mapping: '',
    restart_policy: 'unless-stopped',
    note: '',
  })
  instanceDialogVisible.value = true
}

function openEditInstance(item: ServiceInstance) {
  editingInstanceId.value = item.id
  Object.assign(instanceForm, {
    service_type_id: item.service_type_id,
    host_id: item.host_id,
    name: item.name,
    image: item.image,
    params: item.params.map((p) => ({ ...p })),
    env_text: item.env_text || '',
    network: item.network || '',
    port_mapping: item.port_mapping || '',
    restart_policy: item.restart_policy || 'unless-stopped',
    note: item.note,
  })
  detailEditMode.value = true
  detailTab.value = 'config'
}

function cancelInstanceEdit() {
  detailEditMode.value = false
  editingInstanceId.value = ''
}

async function saveInstance(fromDetail = false) {
  if (!instanceForm.service_type_id) {
    ElMessage.warning('请选择服务类型')
    return
  }
  if (!instanceForm.host_id) {
    ElMessage.warning('请选择部署主机')
    return
  }
  if (!instanceForm.name.trim()) {
    ElMessage.warning('请填写实例名称，例如 game-1')
    return
  }
  if (!instanceForm.image.trim()) {
    ElMessage.warning('请填写镜像')
    return
  }
  instanceSubmitting.value = true
  try {
    const payload = { ...instanceForm, name: instanceForm.name.trim(), image: instanceForm.image.trim() }
    let saved: ServiceInstance
    if (editingInstanceId.value) saved = await instanceApi.update(editingInstanceId.value, payload)
    else saved = await instanceApi.create(payload)
    const wasEditing = Boolean(editingInstanceId.value)
    ElMessage.success(wasEditing ? '实例已更新' : '服务已部署')
    if (fromDetail) detailEditMode.value = false
    else instanceDialogVisible.value = false
    await loadAll()
    activeInstanceId.value = saved.id
    if (fromDetail) editingInstanceId.value = ''
  } catch (error: any) {
    ElMessage.error(error?.response?.data?.error || '保存失败')
  } finally {
    instanceSubmitting.value = false
  }
}

async function removeInstance(item: ServiceInstance) {
  try {
    await ElMessageBox.confirm(`确定删除实例"${item.name}"吗？`, '删除确认', { type: 'warning' })
    await instanceApi.remove(item.id)
    ElMessage.success('实例已删除')
    if (activeInstanceId.value === item.id) activeInstanceId.value = ''
    await loadAll()
  } catch (error: any) {
    if (error !== 'cancel' && error !== 'close') ElMessage.error(error?.response?.data?.error || '删除失败')
  }
}

// -------- 容器状态展示 + 部署/启停/日志 --------
const statusMeta: Record<string, { text: string; type: 'success' | 'info' | 'warning' | 'danger' }> = {
  running: { text: '运行中', type: 'success' },
  stopped: { text: '已停止', type: 'warning' },
  restarting: { text: '重启中', type: 'warning' },
  not_deployed: { text: '未部署', type: 'info' },
  error: { text: '部署失败', type: 'danger' },
  unknown: { text: '状态未知', type: 'info' },
}

function statusLabel(row: ServiceInstance) {
  return statusMeta[row.status]?.text || '未知'
}

function statusType(row: ServiceInstance) {
  return statusMeta[row.status]?.type || 'info'
}

function isHealthy(row: ServiceInstance) {
  return row.status === 'running'
}

function formatRelativeTime(value?: string) {
  if (!value) return '-'
  const timestamp = new Date(value).getTime()
  if (Number.isNaN(timestamp)) return '-'
  const minutes = Math.max(0, Math.floor((Date.now() - timestamp) / 60000))
  if (minutes < 1) return '刚刚'
  if (minutes < 60) return `${minutes} 分钟`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours} 小时`
  return `${Math.floor(hours / 24)} 天`
}

function runtimeDuration(row: ServiceInstance) {
  if (row.status !== 'running') return '—'
  const match = row.status_text?.match(/^Up\s+(.+?)(?:\s+\(.*\))?$/i)
  if (!match) return '—'
  return match[1]
    .replace(/less than a second/i, '不足 1 秒')
    .replace(/(\d+)\s+years?/gi, '$1 年')
    .replace(/(\d+)\s+weeks?/gi, '$1 周')
    .replace(/(\d+)\s+days?/gi, '$1 天')
    .replace(/(\d+)\s+hours?/gi, '$1 小时')
    .replace(/(\d+)\s+minutes?/gi, '$1 分钟')
    .replace(/(\d+)\s+seconds?/gi, '$1 秒')
}

const startEventTypes = new Set(['deploy_succeeded', 'update_image_succeeded', 'container_started', 'container_restarted'])
let startupEventRequest = 0

async function loadLatestStartedAt() {
  const requestId = ++startupEventRequest
  const targets = [...visibleInstances.value]
  const results: Record<string, string> = {}
  let cursor = 0
  await Promise.all(Array.from({ length: Math.min(4, targets.length) }, async () => {
    while (cursor < targets.length && requestId === startupEventRequest && !servicesDisposed) {
      const item = targets[cursor++]
      try {
        const events = await eventApi.list(item.id, 20)
        const latestStart = events.find((event) => startEventTypes.has(event.type))
        if (latestStart) results[item.id] = latestStart.created_at
      } catch {
        // Keep the timestamp empty when lifecycle history is unavailable.
      }
    }
  }))
  if (requestId === startupEventRequest && !servicesDisposed) latestStartedAt.value = results
}

watch([activeProjectId, activeEnvironmentId, activeTypeId], () => {
  if (!loading.value && projects.value.length) void loadLatestStartedAt()
})

async function refreshAll() {
  await loadAll()
  if (activeInstanceId.value) await loadEvents(activeInstanceId.value)
}

const actionLoadingId = ref('')

async function deployInstance(row: ServiceInstance) {
  actionLoadingId.value = row.id
  try {
    await instanceApi.deploy(row.id)
    ElMessage.success(`实例"${row.name}"已部署并启动`)
    await refreshInstanceStatus(row)
    if (activeInstanceId.value === row.id) { await loadDetail(row.id); await loadEvents(row.id) }
  } catch (error: any) {
    ElMessage.error(error?.response?.data?.error || '部署失败')
  } finally {
    actionLoadingId.value = ''
  }
}

async function startInstance(row: ServiceInstance) {
  actionLoadingId.value = row.id
  try {
    await instanceApi.start(row.id)
    ElMessage.success('已启动')
    await refreshInstanceStatus(row)
    if (activeInstanceId.value === row.id) { await loadDetail(row.id); await loadEvents(row.id) }
  } catch (error: any) {
    ElMessage.error(error?.response?.data?.error || '启动失败')
  } finally {
    actionLoadingId.value = ''
  }
}

async function stopInstance(row: ServiceInstance) {
  try {
    await ElMessageBox.confirm(`确定停止实例"${row.name}"吗？`, '停止确认', { type: 'warning' })
  } catch {
    return
  }
  actionLoadingId.value = row.id
  try {
    await instanceApi.stop(row.id)
    ElMessage.success('已停止')
    await refreshInstanceStatus(row)
    if (activeInstanceId.value === row.id) await loadEvents(row.id)
  } catch (error: any) {
    ElMessage.error(error?.response?.data?.error || '停止失败')
  } finally {
    actionLoadingId.value = ''
  }
}

async function restartInstance(row: ServiceInstance) {
  actionLoadingId.value = row.id
  try {
    await instanceApi.restart(row.id)
    ElMessage.success('已重启')
    await refreshInstanceStatus(row)
    if (activeInstanceId.value === row.id) { await loadDetail(row.id); await loadEvents(row.id) }
  } catch (error: any) {
    ElMessage.error(error?.response?.data?.error || '重启失败')
  } finally {
    actionLoadingId.value = ''
  }
}

async function removeInstanceContainer(row: ServiceInstance) {
  try {
    await ElMessageBox.confirm(`确定移除实例"${row.name}"的容器吗？实例配置会保留，可重新部署。`, '移除确认', { type: 'warning' })
  } catch {
    return
  }
  actionLoadingId.value = row.id
  try {
    await instanceApi.removeContainer(row.id)
    ElMessage.success('容器已移除')
    await refreshInstanceStatus(row)
    if (activeInstanceId.value === row.id) await loadEvents(row.id)
  } catch (error: any) {
    ElMessage.error(error?.response?.data?.error || '移除失败')
  } finally {
    actionLoadingId.value = ''
  }
}

async function refreshInstanceStatus(row: ServiceInstance) {
  try {
    const latest = await instanceApi.status(row.id)
    const index = instances.value.findIndex((i) => i.id === row.id)
    if (index !== -1) instances.value[index] = latest
  } catch {
    const index = instances.value.findIndex((item) => item.id === row.id)
    if (index !== -1) instances.value[index] = { ...instances.value[index], status: 'unknown', status_text: '状态获取失败，请刷新重试' }
  }
}

// -------- 更新镜像 --------
const updateImageDialogVisible = ref(false)
const updateImageSubmitting = ref(false)
const updateImageTarget = ref<ServiceInstance | null>(null)
const updateImageForm = reactive({ image: '' })

function openUpdateImage(row: ServiceInstance) {
  updateImageTarget.value = row
  updateImageForm.image = row.image
  updateImageDialogVisible.value = true
}

async function submitUpdateImage() {
  const target = updateImageTarget.value
  if (!target) return
  const image = updateImageForm.image.trim()
  if (!image) {
    ElMessage.warning('请填写新的镜像标签')
    return
  }
  updateImageSubmitting.value = true
  try {
    await instanceApi.updateImage(target.id, image)
    ElMessage.success(`实例"${target.name}"已更新为镜像 ${image} 并重新部署`)
    updateImageDialogVisible.value = false
    await loadAll()
    if (activeInstanceId.value === target.id) { await loadDetail(target.id); await loadEvents(target.id) }
  } catch (error: any) {
    ElMessage.error(error?.response?.data?.error || '更新镜像失败')
  } finally {
    updateImageSubmitting.value = false
  }
}

let statusPollTimer: ReturnType<typeof setInterval> | null = null
let statusPolling = false

function startStatusPolling() {
  stopStatusPolling()
  statusPollTimer = setInterval(async () => {
    if (statusPolling || loading.value || document.hidden || !visibleInstances.value.length) return
    statusPolling = true
    const targets = [...visibleInstances.value]
    try {
      for (let offset = 0; offset < targets.length && statusPollTimer; offset += 4) {
        await Promise.all(targets.slice(offset, offset + 4).map(refreshInstanceStatus))
      }
    } finally { statusPolling = false }
  }, 10000)
}

function stopStatusPolling() {
  if (statusPollTimer) {
    clearInterval(statusPollTimer)
    statusPollTimer = null
  }
}

// -------- 实例详情抽屉：概览/配置/事件/日志 --------
const detailVisible = ref(false)
const detailEditMode = ref(false)
const logsLoading = ref(false)
const logsContent = ref('')
let logsRequestId = 0

async function loadLogs(instanceId: string) {
  const requestId = ++logsRequestId
  logsLoading.value = true
  logsContent.value = ''
  try {
    const content = await instanceApi.logs(instanceId, 500)
    if (requestId === logsRequestId && activeInstanceId.value === instanceId && detailTab.value === 'logs') {
      logsContent.value = content
    }
  } catch (error: any) {
    if (requestId === logsRequestId && activeInstanceId.value === instanceId && detailTab.value === 'logs') {
      logsContent.value = error?.response?.data?.error || '日志获取失败'
    }
  } finally {
    if (requestId === logsRequestId) logsLoading.value = false
  }
}

function clearLogs() {
  logsRequestId += 1
  logsLoading.value = false
  logsContent.value = ''
}

const activeInstanceId = ref('')
const activeInstance = computed(() => instances.value.find((i) => i.id === activeInstanceId.value) || null)
const detailTab = ref<'overview' | 'config' | 'events' | 'logs'>('overview')

function openInstanceDetails(row: ServiceInstance) {
  activeInstanceId.value = row.id
  detailEditMode.value = false
  detailVisible.value = true
}

async function openInstanceLogs(row: ServiceInstance) {
  openInstanceDetails(row)
  await nextTick()
  detailTab.value = 'logs'
}

function onDetailDrawerClosed() {
  activeInstanceId.value = ''
  detailEditMode.value = false
  editingInstanceId.value = ''
}

const detailData = ref<ContainerDetail | null>(null)
const detailLoading = ref(false)
let detailRequestId = 0

async function loadDetail(instanceId: string) {
  const requestId = ++detailRequestId
  detailLoading.value = true
  detailData.value = null
  try {
    const value = await instanceApi.detail(instanceId)
    if (requestId === detailRequestId && activeInstanceId.value === instanceId) detailData.value = value
  } catch {
    if (requestId === detailRequestId && activeInstanceId.value === instanceId) detailData.value = null
  } finally {
    if (requestId === detailRequestId) detailLoading.value = false
  }
}

const events = ref<InstanceEvent[]>([])
const eventsLoading = ref(false)
let eventsRequestId = 0

async function loadEvents(instanceId: string) {
  const requestId = ++eventsRequestId
  eventsLoading.value = true
  try {
    const value = await eventApi.list(instanceId, 20)
    if (requestId === eventsRequestId && activeInstanceId.value === instanceId) events.value = value
  } catch {
    if (requestId === eventsRequestId && activeInstanceId.value === instanceId) events.value = []
  } finally {
    if (requestId === eventsRequestId) eventsLoading.value = false
  }
}

const eventMeta: Record<string, { text: string; type: 'success' | 'danger' }> = {
  deploy_succeeded: { text: '部署成功', type: 'success' },
  deploy_failed: { text: '部署失败', type: 'danger' },
  update_image_succeeded: { text: '镜像更新成功', type: 'success' },
  update_image_failed: { text: '镜像更新失败', type: 'danger' },
  container_started: { text: '容器启动成功', type: 'success' },
  container_start_failed: { text: '容器启动失败', type: 'danger' },
  container_stopped: { text: '容器已停止', type: 'success' },
  container_stop_failed: { text: '容器停止失败', type: 'danger' },
  container_restarted: { text: '容器重启成功', type: 'success' },
  container_restart_failed: { text: '容器重启失败', type: 'danger' },
  container_removed: { text: '容器已移除', type: 'success' },
  container_remove_failed: { text: '容器移除失败', type: 'danger' },
  health_check_passed: { text: '健康检查通过', type: 'success' },
  health_check_failed: { text: '健康检查失败', type: 'danger' },
}

function eventLabel(event: InstanceEvent) {
  return eventMeta[event.type]?.text || event.type
}

function eventType(event: InstanceEvent) {
  return eventMeta[event.type]?.type || 'success'
}

function formatEventTime(value?: string) {
  if (!value) return ''
  const time = new Date(value)
  if (Number.isNaN(time.getTime())) return ''
  const minutes = Math.max(0, Math.floor((Date.now() - time.getTime()) / 60000))
  if (minutes < 1) return '刚刚'
  if (minutes < 60) return `${minutes} 分钟前`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours} 小时前`
  return `${Math.floor(hours / 24)} 天前`
}

function formatDateTime(value?: string): string {
  if (!value) return '-'
  const time = new Date(value)
  if (Number.isNaN(time.getTime()) || value.startsWith('0001-01-01')) return '-'
  return time.toLocaleString()
}

watch(activeInstanceId, (id) => {
  detailTab.value = 'overview'
  detailData.value = null
  events.value = []
  logsContent.value = ''
  logsLoading.value = false
  logsRequestId++
  if (!id) {
    detailVisible.value = false
    return
  }
  const item = instances.value.find((i) => i.id === id)
  if (item && item.status !== 'not_deployed') loadDetail(id)
  loadEvents(id)
})

watch([activeInstanceId, detailTab], ([id, tab]) => {
  if (id && tab === 'logs') void loadLogs(id)
})

onMounted(() => {
  loadAll().then(() => {
    if (servicesDisposed) return
    startStatusPolling()
    const linked = instances.value.find((item) => item.id === route.query.instance)
    const linkedType = types.value.find((type) => type.id === linked?.service_type_id)
    if (linked && linkedType) {
      activeProjectId.value = linkedType.project_id
      activeEnvironmentId.value = linkedType.environment_id
      activeTypeId.value = ''
      openInstanceDetails(linked)
    }
    void loadLatestStartedAt()
  })
})
onUnmounted(() => {
  servicesDisposed = true
  startupEventRequest++
  stopStatusPolling()
  detailRequestId++
  eventsRequestId++
  logsRequestId++
})
</script>

<template>
  <AppLayout fixed-viewport>
    <div class="services-page" v-loading="loading && !projects.length">
      <aside class="project-rail">
        <div class="rail-heading"><span>我的项目</span><span>{{ projects.length }}</span></div>
        <nav class="project-navigation" aria-label="选择项目">
          <button v-for="project in projects" :key="project.id" class="project-nav-item" :class="{ active: activeProjectId === project.id }" :aria-pressed="activeProjectId === project.id" @click="selectProject(project.id)">
            <span class="project-initial">{{ project.name.slice(0, 1).toUpperCase() }}</span>
            <span class="project-nav-name">{{ project.name }}</span>
            <el-icon v-if="activeProjectId === project.id" class="project-chevron"><ArrowRight /></el-icon>
          </button>
        </nav>
        <div class="rail-footer"><el-button text :icon="FolderOpened" @click="router.push('/projects')">管理项目</el-button></div>
      </aside>
      <section class="service-workspace">
        <div class="workspace-heading">
          <div v-if="activeProject" class="environment-tabs" aria-label="选择环境">
            <button v-for="env in environmentsInProject(activeProjectId)" :key="env.id" :class="{ active: activeEnvironmentId === env.id }" :aria-pressed="activeEnvironmentId === env.id" @click="selectEnvironment(env.id)">{{ env.name }}</button>
          </div>
          <div class="workspace-actions">
            <el-dropdown v-if="canManage && activeProject" trigger="click">
              <el-button text :icon="MoreFilled" aria-label="项目内资源管理">管理</el-button>
              <template #dropdown><el-dropdown-menu>
                <el-dropdown-item @click="openCreateEnvironment">新增环境</el-dropdown-item>
                <el-dropdown-item v-if="activeEnvironment" @click="openEditEnvironment(activeEnvironment)">编辑当前环境</el-dropdown-item>
                <el-dropdown-item v-if="activeEnvironment" @click="removeEnvironment(activeEnvironment)">删除当前环境</el-dropdown-item>
                <el-dropdown-item v-if="activeEnvironment" divided @click="openCreateType">新增服务类型</el-dropdown-item>
                <el-dropdown-item v-if="activeType" @click="openEditType(activeType)">编辑当前服务类型</el-dropdown-item>
                <el-dropdown-item v-if="activeType" @click="removeType(activeType)">删除当前服务类型</el-dropdown-item>
              </el-dropdown-menu></template>
            </el-dropdown>
            <el-button v-if="canManage" type="primary" :icon="Plus" :disabled="!typesInEnvironment(activeEnvironmentId).length" @click="openCreateInstance">部署实例</el-button>
          </div>
        </div>
        <el-alert v-if="loadError" title="数据加载失败，请重试；已有数据可能不是最新状态。" type="error" show-icon :closable="false" class="load-error" />
        <el-empty v-if="!loading && !projects.length" :description="loadError ? '无法获取项目' : '暂无项目'" :image-size="60">
          <el-button v-if="loadError" @click="loadAll">重试</el-button>
          <el-button v-else-if="canManage" type="primary" @click="router.push('/projects')">创建项目</el-button>
        </el-empty>
        <el-empty v-else-if="!loading && activeProjectId && !environmentsInProject(activeProjectId).length" description="当前项目暂无环境" :image-size="60">
          <el-button v-if="canManage" type="primary" :icon="Plus" @click="openCreateEnvironment">添加环境</el-button>
        </el-empty>
        <template v-else-if="activeEnvironmentId">
          <div class="service-filter-bar">
            <div class="type-tabs" aria-label="选择服务类型">
              <button class="type-tab" :class="{ active: !activeTypeId }" :aria-pressed="!activeTypeId" @click="activeTypeId = ''; onTypeChange()">全部 <em>{{ environmentInstanceCount }}</em></button>
              <button v-for="type in typesInEnvironment(activeEnvironmentId)" :key="type.id" class="type-tab" :class="{ active: activeTypeId === type.id }" :aria-pressed="activeTypeId === type.id" @click="activeTypeId = type.id; onTypeChange()">{{ type.name }} <em>{{ countInType(type.id) }}</em></button>
            </div>
            <el-input v-model="keyword" :prefix-icon="Search" clearable placeholder="搜索实例、镜像或备注" aria-label="搜索实例" class="instance-search" />
          </div>
          <el-empty v-if="!typesInEnvironment(activeEnvironmentId).length" description="当前环境暂无服务类型" :image-size="60">
            <el-button v-if="canManage" :icon="Plus" @click="openCreateType">添加服务类型</el-button>
          </el-empty>
          <div v-else class="master-detail" v-loading="loading">
            <section class="instance-panel">
              <div class="mobile-instance-list">
                <article v-for="row in filteredInstances" :key="row.id" class="mobile-instance">
                  <div class="mobile-instance-heading">
                    <button class="instance-name" @click="openInstanceDetails(row)">{{ row.name }}</button>
                    <span class="status-dot-label" :class="`label-${row.status}`"><span class="status-dot"></span>{{ statusLabel(row) }}</span>
                  </div>
                  <div class="instance-secondary" :title="row.image">{{ imageVersion(row.image) }}</div>
                  <div class="mobile-instance-bottom">
                    <span>{{ hostById.get(row.host_id)?.name || '未知主机' }}</span>
                    <div class="row-actions">
                      <el-button v-if="row.status !== 'not_deployed'" link type="primary" @click="openInstanceLogs(row)">日志</el-button>
                      <el-button v-if="canManage && row.status === 'stopped'" link type="primary" :loading="actionLoadingId === row.id" @click="startInstance(row)">启动</el-button>
                      <el-button v-else-if="canManage" link type="primary" @click="openUpdateImage(row)">更新</el-button>
                      <el-button link @click="openInstanceDetails(row)">详情</el-button>
                    </div>
                  </div>
                </article>
                <el-empty v-if="!filteredInstances.length" :description="keyword ? '没有符合筛选条件的实例' : '暂无实例'" :image-size="45" />
              </div>
              <el-table :data="filteredInstances" class="instance-table" height="100%" highlight-current-row :current-row-key="activeInstanceId" row-key="id" @row-click="openInstanceDetails" :empty-text="keyword ? '没有符合筛选条件的实例' : '暂无实例，可点击右上角部署实例'">
                <el-table-column label="实例" min-width="145">
                  <template #default="{ row }"><div class="instance-identity"><span class="instance-box"><el-icon><Box /></el-icon></span><button class="instance-name" :title="row.name" @click.stop="openInstanceDetails(row)">{{ row.name }}</button></div></template>
                </el-table-column>
                <el-table-column label="运行状态" width="112">
                  <template #default="{ row }"><span class="status-dot-label" :class="`label-${row.status}`"><span class="status-dot" :class="`status-${row.status}`"></span>{{ statusLabel(row) }}</span></template>
                </el-table-column>
                <el-table-column label="运行时长" width="105">
                  <template #default="{ row }"><span>{{ runtimeDuration(row) }}</span></template>
                </el-table-column>
                <el-table-column label="镜像号" min-width="205">
                  <template #default="{ row }"><span class="instance-image-cell" :title="row.container_image || row.image">{{ imageVersion(row.container_image || row.image) }}</span></template>
                </el-table-column>
                <el-table-column label="上次启动时间" width="165">
                  <template #default="{ row }"><span>{{ latestStartedAt[row.id] ? formatDateTime(latestStartedAt[row.id]) : '—' }}</span></template>
                </el-table-column>
                <el-table-column label="部署主机" min-width="155">
                  <template #default="{ row }"><div class="instance-host-inline" :title="`${hostById.get(row.host_id)?.name || '未知主机'} · ${hostById.get(row.host_id)?.internal_ip || hostById.get(row.host_id)?.external_ip || '未配置 IP'}`"><span>{{ hostById.get(row.host_id)?.name || '未知主机' }}</span><span class="instance-inline-secondary">{{ hostById.get(row.host_id)?.internal_ip || hostById.get(row.host_id)?.external_ip || '未配置 IP' }}</span></div></template>
                </el-table-column>
                <el-table-column label="操作" width="165" align="right" fixed="right">
                  <template #default="{ row }"><div class="row-actions" @click.stop>
                    <el-button v-if="row.status !== 'not_deployed'" link type="primary" @click="openInstanceLogs(row)">日志</el-button>
                    <el-button v-if="canManage && row.status === 'stopped'" link type="primary" :loading="actionLoadingId === row.id" @click="startInstance(row)">启动</el-button>
                    <el-button v-else-if="canManage" link type="primary" @click="openUpdateImage(row)">更新</el-button>
                    <el-button link :icon="ArrowRight" :aria-label="`查看 ${row.name} 详情`" @click="openInstanceDetails(row)" />
                  </div></template>
                </el-table-column>
              </el-table>
              <div class="instance-list-footer"><span>{{ filteredInstances.length }} 个实例 · {{ filteredInstances.filter(isHealthy).length }} 个运行中</span><el-button text size="small" :icon="Refresh" :loading="loading" @click="refreshAll">刷新</el-button></div>
            </section>

          <!-- 实例详情抽屉，与主机详情使用相同的右侧抽屉交互 -->
          <el-drawer
            v-model="detailVisible"
            direction="rtl"
            size="520px"
            :with-header="false"
            @closed="onDetailDrawerClosed"
          >
            <div v-if="activeInstance" class="detail-panel">
              <div class="detail-header">
                <div class="detail-header-main">
                  <div class="detail-header-icon" :class="`status-${activeInstance.status}`"><el-icon :size="20"><Box /></el-icon></div>
                  <div class="detail-header-info">
                    <div class="detail-title">实例详情</div>
                    <div class="detail-header-name">{{ activeInstance.name }}</div>
                  </div>
                  <el-button class="detail-close-button" circle :icon="Close" aria-label="关闭实例详情" @click="detailVisible = false" />
                </div>
                <div v-if="detailEditMode" class="detail-header-actions detail-edit-actions">
                  <el-button size="small" @click="cancelInstanceEdit">取消</el-button>
                  <el-button size="small" type="primary" :loading="instanceSubmitting" @click="saveInstance(true)">保存</el-button>
                </div>
                <div v-else-if="canManage" class="detail-header-actions">
                  <el-button v-if="activeInstance.status === 'not_deployed' || activeInstance.status === 'error'" size="small" type="primary" :icon="Upload" :loading="actionLoadingId === activeInstance.id" @click="deployInstance(activeInstance)">部署</el-button>
                  <el-button v-if="activeInstance.status === 'stopped'" size="small" type="success" plain :icon="VideoPlay" :loading="actionLoadingId === activeInstance.id" @click="startInstance(activeInstance)">启动</el-button>
                  <el-button v-if="activeInstance.status === 'stopped'" size="small" type="primary" :icon="Upload" :loading="actionLoadingId === activeInstance.id" @click="deployInstance(activeInstance)">重新部署</el-button>
                  <el-button v-if="activeInstance.status === 'running' || activeInstance.status === 'restarting'" size="small" plain :icon="VideoPause" :loading="actionLoadingId === activeInstance.id" @click="stopInstance(activeInstance)">停止</el-button>
                  <el-button v-if="activeInstance.status === 'running'" size="small" plain :icon="RefreshRight" :loading="actionLoadingId === activeInstance.id" @click="restartInstance(activeInstance)">重启</el-button>
                  <el-dropdown trigger="click">
                    <el-button size="small" plain :icon="MoreFilled">更多</el-button>
                    <template #dropdown>
                      <el-dropdown-menu>
                        <el-dropdown-item :icon="Refresh" @click="openUpdateImage(activeInstance)">更新镜像</el-dropdown-item>
                        <el-dropdown-item :icon="Edit" @click="openEditInstance(activeInstance)">编辑配置</el-dropdown-item>
                        <el-dropdown-item v-if="activeInstance.status !== 'not_deployed'" :icon="Delete" @click="removeInstanceContainer(activeInstance)">移除容器</el-dropdown-item>
                        <el-dropdown-item :icon="Delete" divided class="danger-menu-item" @click="removeInstance(activeInstance)">删除实例</el-dropdown-item>
                      </el-dropdown-menu>
                    </template>
                  </el-dropdown>
                </div>
              </div>

              <el-tabs v-model="detailTab" class="detail-tabs">
              <el-tab-pane label="概览" name="overview">
                <div class="detail-scroll">
                  <div class="detail-section-title">实例信息</div>
                  <div class="detail-kv">
                    <div class="detail-kv-row"><span>镜像</span><span class="mono-text">{{ activeInstance.image }}</span></div>
                    <div class="detail-kv-row"><span>网络</span><span>{{ activeInstance.network || '默认网络' }}</span></div>
                    <div class="detail-kv-row"><span>重启策略</span><span>{{ restartPolicyLabel(activeInstance.restart_policy) }}</span></div>
                    <div class="detail-kv-row"><span>端口</span><span class="mono-text">{{ activeInstance.port_mapping || '未映射' }}</span></div>
                    <div class="detail-kv-row"><span>创建时间</span><span>{{ formatDateTime(activeInstance.created_at) }}</span></div>
                  </div>

                  <div v-if="activeInstance.status === 'error' && activeInstance.deploy_error" class="detail-error-banner">
                    部署失败：{{ activeInstance.deploy_error }}
                  </div>

                  <div class="detail-section-title events-title">最近事件</div>
                  <div v-loading="eventsLoading" class="event-timeline">
                    <div v-for="event in events" :key="event.id" class="event-row">
                      <span class="event-dot" :class="`event-${eventType(event)}`"></span>
                      <div class="event-body">
                        <div class="event-message">{{ eventLabel(event) }}</div>
                        <div class="event-time">{{ formatEventTime(event.created_at) }}</div>
                      </div>
                    </div>
                    <div v-if="!eventsLoading && !events.length" class="detail-empty">暂无事件记录</div>
                  </div>
                  <el-link v-if="events.length" type="primary" :underline="false" class="view-all-events" @click="detailTab = 'events'">查看完整事件 →</el-link>
                </div>
              </el-tab-pane>

              <el-tab-pane label="配置" name="config">
                <el-form v-if="detailEditMode" class="detail-scroll instance-inline-editor" label-position="top">
                  <div class="detail-section-title">编辑实例配置</div>
                  <el-form-item label="部署主机" required><el-select v-model="instanceForm.host_id" style="width: 100%" placeholder="选择部署主机"><el-option v-for="host in instanceHostOptions" :key="host.id" :label="host.name" :value="host.id"><span>{{ host.name }}</span><span class="host-option-detail">{{ host.docker_host }}</span></el-option></el-select></el-form-item>
                  <el-form-item label="实例名称" required><el-input v-model="instanceForm.name" placeholder="例如 game-1" /></el-form-item>
                  <el-form-item label="镜像" required><el-input v-model="instanceForm.image" placeholder="镜像名称和标签" /></el-form-item>
                  <el-form-item label="重启策略"><el-select v-model="instanceForm.restart_policy" style="width: 100%"><el-option label="不自动重启" value="no" /><el-option label="总是重启" value="always" /><el-option label="除非手动停止，否则重启" value="unless-stopped" /><el-option label="异常退出时重启" value="on-failure" /></el-select></el-form-item>
                  <el-form-item label="网络"><el-input v-model="instanceForm.network" placeholder="留空使用默认网络" /></el-form-item>
                  <el-form-item label="端口映射"><el-input v-model="instanceForm.port_mapping" placeholder="宿主机:容器，例如 9001:9001" /></el-form-item>
                  <el-form-item label="环境变量 / 启动参数"><el-input v-model="instanceForm.env_text" type="textarea" :rows="10" class="env-vars-editor" placeholder="KEY=VALUE 或每行一个启动参数" /></el-form-item>
                  <el-form-item label="备注"><el-input v-model="instanceForm.note" type="textarea" :rows="3" /></el-form-item>
                </el-form>
                <div v-else class="detail-scroll">
                  <div class="detail-section-title">部署配置</div>
                  <div class="detail-kv">
                    <div class="detail-kv-row"><span>服务类型</span><span>{{ activeType?.name }}</span></div>
                    <div class="detail-kv-row"><span>部署主机</span><span>{{ hosts.find((h) => h.id === activeInstance!.host_id)?.name || activeInstance.host_id }}</span></div>
                    <div class="detail-kv-row"><span>镜像</span><span class="mono-text">{{ activeInstance.image }}</span></div>
                    <div class="detail-kv-row"><span>网络</span><span>{{ activeInstance.network || '默认网络' }}</span></div>
                    <div class="detail-kv-row"><span>端口映射</span><span class="mono-text">{{ activeInstance.port_mapping || '未映射' }}</span></div>
                  </div>

                  <div v-if="activeInstance.params.length" class="detail-section-title config-title">启动参数</div>
                  <div v-if="activeInstance.params.length" class="detail-list">
                    <div v-for="(param, index) in activeInstance.params" :key="index" class="detail-list-row">{{ param.flag }} {{ param.value }}</div>
                  </div>

                  <div class="detail-section-title config-title">环境变量 / 附加参数</div>
                  <pre v-if="activeInstance.env_text" class="detail-code">{{ activeInstance.env_text }}</pre>
                  <div v-else class="detail-empty">未配置</div>

                  <div v-if="activeInstance.note" class="detail-section-title config-title">备注</div>
                  <div v-if="activeInstance.note" class="detail-empty note-text">{{ activeInstance.note }}</div>

                  <el-button class="edit-config-btn" :icon="Edit" @click="openEditInstance(activeInstance)">编辑配置</el-button>
                </div>
              </el-tab-pane>

              <el-tab-pane label="事件" name="events">
                <div class="detail-scroll">
                  <div v-loading="eventsLoading" class="event-timeline full">
                    <div v-for="event in events" :key="event.id" class="event-row">
                      <span class="event-dot" :class="`event-${eventType(event)}`"></span>
                      <div class="event-body">
                        <div class="event-message">{{ eventLabel(event) }}</div>
                        <div v-if="event.message" class="event-detail">{{ event.message }}</div>
                        <div class="event-time">{{ formatDateTime(event.created_at) }}</div>
                      </div>
                    </div>
                    <div v-if="!eventsLoading && !events.length" class="detail-empty">暂无事件记录</div>
                  </div>
                </div>
              </el-tab-pane>

              <el-tab-pane v-if="activeInstance.status !== 'not_deployed'" label="日志" name="logs">
                <div class="detail-scroll detail-logs">
                  <div class="logs-toolbar">
                    <span class="detail-section-title">容器日志</span>
                    <div class="logs-toolbar-actions">
                      <el-button size="small" plain :icon="Delete" title="仅清空当前显示，不删除容器日志" @click="clearLogs">清空</el-button>
                      <el-button size="small" plain :icon="Refresh" :loading="logsLoading" @click="loadLogs(activeInstance.id)">刷新</el-button>
                    </div>
                  </div>
                  <el-input
                    :model-value="logsContent"
                    type="textarea"
                    readonly
                    resize="none"
                    class="logs-viewer logs-viewer-fill"
                    :placeholder="logsLoading ? '加载中...' : '暂无日志'"
                  />
                </div>
              </el-tab-pane>
              </el-tabs>
            </div>
          </el-drawer>
        </div>
        </template>
      </section>
    </div>

    <!-- 环境表单 -->
    <el-dialog v-model="environmentDialogVisible" :title="editingEnvironmentId ? '编辑环境' : '添加环境'" width="480px" destroy-on-close>
      <el-form label-width="100px">
        <el-form-item label="所属项目" required>
          <el-select v-model="environmentForm.project_id" placeholder="请选择项目" style="width: 100%">
            <el-option v-for="project in projects" :key="project.id" :label="project.name" :value="project.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="环境名称" required>
          <el-input v-model="environmentForm.name" placeholder="例如 beta、dev、dev-1、dev-2" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="environmentDialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="environmentSubmitting" @click="saveEnvironment">保存</el-button>
      </template>
    </el-dialog>

    <!-- 服务类型表单 -->
    <el-dialog v-model="typeDialogVisible" :title="editingTypeId ? '编辑服务类型' : '添加服务类型'" width="560px" destroy-on-close>
      <el-form label-width="100px">
        <el-form-item label="所属项目" required>
          <el-select
            v-model="typeForm.project_id"
            placeholder="请选择项目"
            style="width: 100%"
            @change="typeForm.environment_id = ''"
          >
            <el-option v-for="project in projects" :key="project.id" :label="project.name" :value="project.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="所属环境" required>
          <el-select v-model="typeForm.environment_id" placeholder="请选择环境" style="width: 100%">
            <el-option v-for="env in environmentsInProject(typeForm.project_id)" :key="env.id" :label="env.name" :value="env.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="类型名称" required>
          <el-input v-model="typeForm.name" placeholder="例如 game、battle、gate" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="typeDialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="submitting" @click="saveType">保存</el-button>
      </template>
    </el-dialog>

    <!-- 实例表单 -->
    <el-dialog v-model="instanceDialogVisible" :title="editingInstanceId ? '编辑实例' : '部署实例'" width="800px" destroy-on-close>
      <el-form label-width="90px">
        <el-form-item label="服务类型" required>
          <el-select v-model="instanceForm.service_type_id" :disabled="!!editingInstanceId" style="width: 100%" placeholder="选择服务类型">
            <el-option v-for="type in typesInEnvironment(activeEnvironmentId)" :key="type.id" :label="type.name" :value="type.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="部署主机" required>
          <el-select v-model="instanceForm.host_id" placeholder="请选择 Docker 主机" style="width: 100%">
            <el-option v-for="host in instanceHostOptions" :key="host.id" :label="host.name" :value="host.id">
              <span>{{ host.name }}</span>
              <span class="host-option-detail">{{ host.docker_host }}</span>
            </el-option>
          </el-select>
          <div v-if="!instanceHostOptions.length" class="form-hint">
            当前项目还没有绑定部署主机。请先到“项目管理”中编辑项目并绑定主机。
          </div>
        </el-form-item>
        <el-form-item label="实例名称" required>
          <el-input v-model="instanceForm.name" placeholder="例如 game-1、game-2" />
        </el-form-item>
        <el-form-item label="镜像" required>
          <el-input v-model="instanceForm.image" placeholder="例如 gogs-game:v1001-3" />
        </el-form-item>
        <el-form-item label="重启策略">
          <el-select v-model="instanceForm.restart_policy" style="width: 100%">
            <el-option label="不自动重启" value="no" />
            <el-option label="总是重启" value="always" />
            <el-option label="除非手动停止，否则重启" value="unless-stopped" />
            <el-option label="异常退出时重启" value="on-failure" />
          </el-select>
          <div class="form-hint">保存配置后重新部署容器生效。</div>
        </el-form-item>
        <el-form-item label="网络">
          <el-input v-model="instanceForm.network" placeholder="例如 bridge 或自定义网络名称，留空使用默认网络" />
        </el-form-item>
        <el-form-item label="端口映射">
          <el-input v-model="instanceForm.port_mapping" placeholder="宿主机:容器，例如 9001:9001，留空不映射端口" />
        </el-form-item>
        <el-form-item label="环境变量">
          <el-input
            v-model="instanceForm.env_text"
            type="textarea"
            :rows="12"
            class="env-vars-editor"
            placeholder="默认预填项目环境变量模板，可在此自由修改（仅影响当前实例，不影响项目模板）"
          />
          <div class="form-hint">
            支持 KEY=VALUE 环境变量；启动参数请单独一行填写，如 <code>--server-id 1001</code>（会作为容器启动命令参数传给程序，缺少必需参数可能导致服务启动后立即退出、看不到日志）。如需额外端口映射，也可单独一行填写 <code>-p 主机端口:容器端口</code>（如 <code>-p 9001:9001</code>，UDP 加 <code>/udp</code>），会与上方"端口映射"字段共同生效。
          </div>
        </el-form-item>
        <el-form-item label="备注">
          <el-input v-model="instanceForm.note" type="textarea" :rows="3" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="instanceDialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="instanceSubmitting" @click="saveInstance">保存</el-button>
      </template>
    </el-dialog>

    <!-- 更新镜像 -->
    <el-dialog v-model="updateImageDialogVisible" :title="`更新镜像 - ${updateImageTarget?.name || ''}`" width="520px" destroy-on-close>
      <el-form label-width="90px">
        <el-form-item label="当前镜像">
          <span class="current-image-text">{{ updateImageTarget?.image }}</span>
        </el-form-item>
        <el-form-item label="新镜像" required>
          <el-input v-model="updateImageForm.image" placeholder="例如 gogs-game:v1002-5" />
        </el-form-item>
      </el-form>
      <div class="form-hint">
        更新后会拉取新镜像并重新部署该实例的容器（先移除旧容器再按新镜像创建启动），启动参数、环境变量和端口映射保持不变。
      </div>
      <template #footer>
        <el-button @click="updateImageDialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="updateImageSubmitting" @click="submitUpdateImage">更新</el-button>
      </template>
    </el-dialog>

  </AppLayout>
</template>

<style scoped>
.services-page { display: flex; flex: 1; min-width: 0; min-height: 0; overflow: hidden; background: #fff; }
.project-rail { flex: 0 0 204px; width: 204px; display: flex; flex-direction: column; min-height: 0; padding: 25px 12px 16px; border-right: 1px solid var(--ops-border); background: #f7f9fc; }
.rail-heading { display: flex; justify-content: space-between; padding: 0 12px 17px; color: var(--ops-text-secondary); font-size: 12px; }
.project-navigation { min-height: 0; overflow-y: auto; }
.project-nav-item { display: flex; align-items: center; width: 100%; gap: 10px; padding: 11px 10px; margin-bottom: 5px; border: 0; border-radius: 7px; background: transparent; color: var(--ops-text); font: inherit; font-size: 13px; text-align: left; cursor: pointer; }
.project-nav-item:hover { background: #eef2f8; }
.project-nav-item.active { background: var(--ops-primary-light); color: var(--ops-primary); }
.project-initial { display: grid; place-items: center; width: 28px; height: 28px; flex-shrink: 0; border: 1px solid var(--ops-border); border-radius: 6px; background: #fff; font-size: 12px; }
.project-nav-name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.project-chevron { margin-left: auto; flex-shrink: 0; font-size: 11px; }
.rail-footer { margin-top: auto; padding-top: 20px; }
.rail-footer .el-button { color: var(--ops-text-secondary); }
.service-workspace { flex: 1; display: flex; flex-direction: column; min-width: 0; min-height: 0; padding: 26px 30px 12px; }
.workspace-heading { display: flex; align-items: center; gap: 20px; flex-wrap: wrap; margin: -6px -8px 18px; padding: 16px 18px; border: 1px solid #e5ecf8; border-radius: 10px; background: linear-gradient(112deg, #f0f5ff 0%, #f8faff 58%, #f2f7ff 100%); flex-shrink: 0; }
.environment-tabs { display: flex; align-items: center; gap: 3px; padding: 3px; background: var(--ops-bg); border-radius: 6px; max-width: 100%; overflow-x: auto; }
.environment-tabs button { flex-shrink: 0; padding: 5px 12px; background: transparent; border: 0; border-radius: 4px; font: inherit; font-size: 12px; color: var(--ops-text-secondary); cursor: pointer; }
.environment-tabs button.active { background: #fff; color: var(--ops-text); box-shadow: 0 1px 4px #17294512; }
.workspace-actions { display: flex; align-items: center; gap: 8px; margin-left: auto; }
.service-filter-bar { display: flex; align-items: center; gap: 16px; border-bottom: 1px solid var(--ops-border); flex-shrink: 0; }
.type-tabs { display: flex; min-width: 0; gap: 23px; overflow-x: auto; }
.type-tab { display: flex; align-items: center; gap: 7px; flex-shrink: 0; padding: 11px 0 13px; background: transparent; border: 0; border-bottom: 2px solid transparent; color: var(--ops-text-secondary); font: inherit; font-size: 13px; cursor: pointer; }
.type-tab.active { color: var(--ops-primary); border-bottom-color: var(--ops-primary); }
.type-tab em { font-style: normal; font-size: 11px; padding: 1px 6px; background: var(--ops-bg); color: var(--ops-text-secondary); border-radius: 4px; }
.instance-search { width: 205px; margin-left: auto; flex-shrink: 0; }
.instance-search :deep(.el-input__wrapper) { box-shadow: none; }
.master-detail { flex: 1; display: flex; min-height: 0; overflow: hidden; }
.instance-panel { display: flex; flex: 1; min-width: 0; min-height: 0; flex-direction: column; }
.mobile-instance-list { display: none; }
.instance-table { flex: 1; min-height: 0; --el-table-header-bg-color: #fff; --el-table-row-hover-bg-color: #f8faff; }
.instance-table :deep(th.el-table__cell) { font-weight: 400; font-size: 12px; color: var(--ops-text-secondary); padding: 13px 0; }
.instance-table :deep(td.el-table__cell) { padding: 17px 0; }
.instance-table :deep(.el-table__row) { cursor: pointer; }
.instance-table :deep(.current-row td) { background: var(--ops-primary-light); }
.instance-identity { display: flex; align-items: center; gap: 12px; }
.instance-box { display: grid; place-items: center; flex-shrink: 0; width: 34px; height: 36px; background: var(--ops-bg); color: var(--ops-text-secondary); border-radius: 6px; }
.instance-identity-text { min-width: 0; }
.instance-name { display: block; max-width: 100%; padding: 0; border: 0; background: transparent; font: inherit; font-size: 13px; font-weight: 600; color: var(--ops-text); cursor: pointer; text-align: left; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.instance-image-cell { display: block; overflow: hidden; color: var(--ops-text-secondary); font-family: "SFMono-Regular", Consolas, Monaco, monospace; font-size: 12px; text-overflow: ellipsis; white-space: nowrap; }
.status-inline, .instance-host-inline { display: flex; align-items: center; gap: 9px; min-width: 0; white-space: nowrap; }
.instance-host-inline > span:first-child { overflow: hidden; text-overflow: ellipsis; }
.instance-inline-secondary { overflow: hidden; color: var(--ops-text-secondary); font-size: 12px; text-overflow: ellipsis; white-space: nowrap; }
.instance-secondary { margin-top: 3px; font-size: 12px; color: var(--ops-text-secondary); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.status-dot-label { display: inline-flex; align-items: center; gap: 6px; font-size: 12px; padding: 2px 7px; border-radius: 4px; background: var(--ops-bg); color: var(--ops-text-secondary); }
.status-dot-label.label-running { color: var(--ops-success); background: #edf8f4; }
.status-dot-label.label-error { color: var(--ops-danger); background: #fff0f1; }
.status-dot-label.label-restarting { color: var(--ops-warning); background: #fff6e8; }
.status-dot { display: inline-block; width: 5px; height: 5px; border-radius: 50%; background: currentColor; }
.row-actions { display: flex; align-items: center; justify-content: flex-end; gap: 10px; }
.row-actions .el-button { margin-left: 0; }
.instance-list-footer { display: flex; align-items: center; justify-content: space-between; padding: 12px 0 0; font-size: 12px; color: var(--ops-text-secondary); }
.mono-text { font-family: Consolas, monospace; font-size: 12px; color: #52657a; }
.load-error { margin-bottom: 16px; flex-shrink: 0; }

/* -------- 实例详情抽屉 -------- */
:deep(.el-drawer__body) { padding: 0; overflow: hidden; }

.detail-panel {
  display: flex;
  flex-direction: column;
  width: 100%;
  height: 100%;
  flex: 1 1 auto;
  min-width: 0;
  min-height: 0;
  background: #fafcff;
}


.detail-header {
  display: flex;
  flex-direction: column;
  gap: 14px;
  padding: 18px 18px 14px;
  border-bottom: 1px solid var(--ops-border);
}

.detail-header-main { display: flex; align-items: center; gap: 12px; width: 100%; min-width: 0; }

.detail-header-icon {
  display: grid;
  width: 40px;
  height: 40px;
  flex: 0 0 auto;
  place-items: center;
  border-radius: 10px;
  background: var(--ops-primary-light);
  color: var(--ops-primary);
}

.detail-header-icon.status-running { background: #f0f9eb; color: #55a532; }
.detail-header-icon.status-error { background: #fef0f0; color: #f56c6c; }
.detail-header-icon.status-stopped { background: #fdf6ec; color: #e6a23c; }

.detail-header-info { flex: 1; min-width: 0; }
.detail-title { margin-bottom: 4px; color: var(--ops-text-secondary); font-size: 12px; }
.detail-header-name { font-size: 16px; font-weight: 700; color: var(--ops-text); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

.detail-close-button { width: 36px; height: 36px; flex: 0 0 36px; margin-left: auto; border-color: var(--ops-border); color: var(--ops-text-secondary); }
.detail-close-button:hover { border-color: var(--ops-primary); background: var(--ops-primary-light); color: var(--ops-primary); }

.detail-header-actions { display: flex; flex-wrap: wrap; justify-content: flex-end; gap: 6px; width: 100%; }
.detail-edit-actions { justify-content: flex-end; }

.detail-tabs { flex: 1; min-height: 0; display: flex; flex-direction: column; }
.detail-tabs :deep(.el-tabs__header) { margin: 0; padding: 0 18px; }
.detail-tabs :deep(.el-tabs__content) { flex: 1; min-height: 0; overflow-y: auto; }
.detail-tabs :deep(.el-tab-pane) { height: 100%; }

.detail-scroll { min-height: 100%; padding: 16px 18px 20px; box-sizing: border-box; }

.detail-section-title { margin-bottom: 10px; color: var(--ops-text); font-size: 13px; font-weight: 700; }
.detail-section-title.events-title, .detail-section-title.config-title { margin-top: 20px; }

.detail-kv { display: flex; flex-direction: column; gap: 10px; }
.detail-kv-row { display: flex; align-items: flex-start; justify-content: space-between; gap: 12px; font-size: 12.5px; }
.detail-kv-row > span:first-child { flex: 0 0 auto; color: var(--ops-text-secondary); }
.detail-kv-row > span:last-child { min-width: 0; overflow: hidden; text-align: right; text-overflow: ellipsis; white-space: nowrap; color: var(--ops-text); }

.detail-error-banner {
  margin-top: 14px;
  padding: 10px 12px;
  border-radius: 8px;
  background: #fef0f0;
  color: var(--el-color-danger);
  font-size: 12.5px;
  line-height: 1.6;
}

.event-timeline { display: flex; flex-direction: column; gap: 14px; min-height: 40px; }
.event-timeline.full { gap: 18px; }
.event-row { display: flex; gap: 10px; }
.event-dot { flex: 0 0 auto; width: 9px; height: 9px; margin-top: 4px; border-radius: 50%; background: #52c41a; }
.event-dot.event-danger { background: var(--el-color-danger); }
.event-body { min-width: 0; }
.event-message { font-size: 13px; color: var(--ops-text); }
.event-detail { margin-top: 2px; color: var(--ops-text-secondary); font-size: 12px; word-break: break-all; }
.event-time { margin-top: 2px; color: #a0a8b3; font-size: 11px; }

.view-all-events { display: block; margin-top: 14px; font-size: 13px; }

.detail-empty { padding: 6px 0; color: var(--ops-text-secondary); font-size: 12.5px; }
.note-text { line-height: 1.6; }

.detail-list { display: flex; flex-direction: column; gap: 4px; padding: 8px 10px; border-radius: 6px; background: #f6f8fa; }
.detail-list-row { font-family: "SFMono-Regular", Consolas, Monaco, monospace; font-size: 12px; color: var(--ops-text); word-break: break-all; }

.detail-code {
  margin: 0;
  padding: 10px 12px;
  border-radius: 6px;
  background: #f6f8fa;
  font-family: "SFMono-Regular", Consolas, Monaco, monospace;
  font-size: 12px;
  line-height: 1.6;
  white-space: pre-wrap;
  word-break: break-all;
}

.edit-config-btn { width: 100%; margin-top: 20px; }
.instance-inline-editor :deep(.el-form-item) { margin-bottom: 16px; }
.instance-inline-editor :deep(.el-form-item__label) { padding-bottom: 6px; color: #52627a; font-size: 12px; font-weight: 600; }
.instance-inline-editor :deep(.el-input__wrapper), .instance-inline-editor :deep(.el-select__wrapper) { min-height: 36px; border-radius: 7px; }
.instance-inline-editor :deep(.el-textarea__inner) { font-family: "SFMono-Regular", Consolas, Monaco, monospace; font-size: 11px; line-height: 1.55; }

:global(.danger-menu-item) { color: var(--el-color-danger); }

.host-option-detail { margin-left: 10px; color: var(--ops-text-secondary); font-size: 12px; }

.env-vars-editor :deep(textarea) {
  font-family: "SFMono-Regular", Consolas, Monaco, monospace;
  font-size: 13px;
  line-height: 1.6;
  background: #f6f8fa;
}

.form-hint {
  margin-top: 6px;
  font-size: 12px;
  color: var(--ops-text-secondary);
  line-height: 1.6;
}

.form-hint code {
  font-family: "SFMono-Regular", Consolas, Monaco, monospace;
  background: #f6f8fa;
  padding: 1px 4px;
  border-radius: 3px;
}

.current-image-text {
  font-family: "SFMono-Regular", Consolas, Monaco, monospace;
  font-size: 13px;
  color: var(--ops-text-secondary);
}

.logs-viewer :deep(textarea) {
  font-family: "SFMono-Regular", Consolas, Monaco, monospace;
  font-size: 12px;
  line-height: 1.5;
  background: #0d1117;
  color: #c9d1d9;
}

.logs-toolbar { display: flex; align-items: center; justify-content: space-between; gap: 12px; margin-bottom: 12px; }
.logs-toolbar-actions { display: flex; align-items: center; gap: 8px; }
.logs-toolbar .detail-section-title { margin: 0; }
.detail-scroll.detail-logs { display: flex; flex-direction: column; height: 100%; overflow: hidden; }
.logs-viewer-fill { display: flex; flex: 1 1 auto; min-height: 0; }
.logs-viewer-fill :deep(.el-textarea) { height: 100%; }
.logs-viewer-fill :deep(textarea) { height: 100% !important; min-height: 0 !important; overflow: auto; resize: none; }

@media (max-width: 1000px) {
  .project-rail { flex-basis: 170px; width: 170px; }
  .service-workspace { padding: 22px 20px 12px; }
  .workspace-heading { gap: 12px; margin: -4px -4px 16px; padding: 14px; }
  .service-filter-bar { flex-wrap: wrap; gap: 0; }
  .type-tabs { width: 100%; }
  .instance-search { width: 100%; margin: 4px 0; }
}
@media (max-width: 760px) {
  .instance-table { display: none; }
  .mobile-instance-list { display: block; flex: 1; min-height: 0; overflow-y: auto; }
  .mobile-instance { padding: 17px 2px; border-bottom: 1px solid var(--ops-border); }
  .mobile-instance-heading { display: flex; justify-content: space-between; align-items: center; gap: 12px; }
  .mobile-instance-heading .instance-name { overflow-wrap: anywhere; }
  .mobile-instance .instance-secondary { margin-top: 7px; }
  .mobile-instance-bottom { display: flex; justify-content: space-between; align-items: center; margin-top: 9px; gap: 10px; }
  .mobile-instance-bottom > span { color: var(--ops-text-secondary); font-size: 12px; }
  .services-page { flex-direction: column; }
  .project-rail { flex: 0 0 auto; width: 100%; padding: 8px 12px; border-right: 0; border-bottom: 1px solid var(--ops-border); }
  .rail-heading, .rail-footer, .project-initial, .project-chevron { display: none; }
  .project-navigation { display: flex; gap: 6px; overflow-x: auto; }
  .project-nav-item { width: auto; flex-shrink: 0; margin: 0; padding: 8px 12px; }
  .project-nav-name { max-width: 160px; }
  .service-workspace { padding: 16px 14px 10px; }
  .workspace-heading { margin: -2px -2px 10px; padding: 12px; gap: 10px; }
  .workspace-actions { margin-left: 0; }
  .environment-tabs button { padding: 5px 8px; }
  :deep(.el-drawer) { width: min(520px, 100vw) !important; }
}
</style>
