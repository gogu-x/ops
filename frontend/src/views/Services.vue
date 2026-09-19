<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  Plus, Edit, Delete, Box, VideoPlay, VideoPause, RefreshRight, Refresh, Upload,
  Search, ArrowDown, MoreFilled, Close,
} from '@element-plus/icons-vue'
import { useAuthStore } from '../stores/auth'
import { environmentApi, type Environment, type EnvironmentForm } from '../api/environments'
import { serviceApi, type ServiceType, type ServiceTypeForm } from '../api/services'
import { hostApi, type Host } from '../api/hosts'
import { projectApi, type Project } from '../api/projects'
import { instanceApi, type ServiceInstance, type ServiceInstanceForm, type ContainerDetail } from '../api/instances'
import { eventApi, type InstanceEvent } from '../api/events'
import AppLayout from '../components/AppLayout.vue'

const auth = useAuthStore()
const environments = ref<Environment[]>([])
const types = ref<ServiceType[]>([])
const instances = ref<ServiceInstance[]>([])
const hosts = ref<Host[]>([])
const projects = ref<Project[]>([])
const loading = ref(false)
const submitting = ref(false)
const canManage = computed(() => auth.user?.role === 'admin')

// -------- 顶部筛选状态：项目 -> 环境 -> 服务类型 --------
const activeProjectId = ref<string>('')
const activeEnvironmentId = ref<string>('')
const activeTypeId = ref<string>('')
const keyword = ref('')

function environmentsInProject(projectId: string): Environment[] {
  return environments.value.filter((e) => e.project_id === projectId)
}

function typesInEnvironment(environmentId: string): ServiceType[] {
  return types.value.filter((t) => t.environment_id === environmentId)
}

function countInType(typeId: string): number {
  return instances.value.filter((i) => i.service_type_id === typeId).length
}

const activeProject = computed(() => projects.value.find((p) => p.id === activeProjectId.value) || null)
const activeEnvironment = computed(() => environments.value.find((e) => e.id === activeEnvironmentId.value) || null)
const activeType = computed(() => types.value.find((t) => t.id === activeTypeId.value) || null)

const visibleInstances = computed(() => {
  if (!activeTypeId.value) return []
  return instances.value.filter((i) => i.service_type_id === activeTypeId.value)
})

const filteredInstances = computed(() => {
  const normalizedKeyword = keyword.value.trim().toLowerCase()
  if (!normalizedKeyword) return visibleInstances.value
  return visibleInstances.value.filter((item) => [item.name, item.image, item.note]
    .some((value) => value?.toLowerCase().includes(normalizedKeyword)))
})

async function loadAll() {
  loading.value = true
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
    if (!activeTypeId.value || !typesInEnvironment(activeEnvironmentId.value).some((t) => t.id === activeTypeId.value)) {
      activeTypeId.value = typesInEnvironment(activeEnvironmentId.value)[0]?.id || ''
    }
    if (activeInstanceId.value && !instances.value.some((i) => i.id === activeInstanceId.value)) {
      activeInstanceId.value = ''
    }
  } catch {
    ElMessage.error('数据加载失败')
  } finally {
    loading.value = false
  }
}

// 用户从顶部下拉切换项目：级联重置环境 -> 类型 -> 实例选中态。
function onProjectChange() {
  activeEnvironmentId.value = environmentsInProject(activeProjectId.value)[0]?.id || ''
  onEnvironmentChange()
}

// 用户从顶部下拉切换环境：级联重置类型 -> 实例选中态。
function onEnvironmentChange() {
  activeTypeId.value = typesInEnvironment(activeEnvironmentId.value)[0]?.id || ''
  onTypeChange()
}

// 用户点击服务类型 Tab：关闭右侧详情面板（需要双击某一行实例才会重新打开）。
function onTypeChange() {
  activeInstanceId.value = ''
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
const instanceForm = reactive<ServiceInstanceForm>({ service_type_id: '', host_id: '', name: '', image: '', params: [], env_text: '', network: '', port_mapping: '', note: '' })
const instanceSubmitting = ref(false)

function openCreateInstance() {
  if (!activeType.value) return
  editingInstanceId.value = ''
  Object.assign(instanceForm, {
    service_type_id: activeType.value.id,
    host_id: hosts.value[0]?.id || '',
    name: '',
    image: '',
    params: [],
    env_text: activeProject.value?.env_vars || '',
    network: '',
    port_mapping: '',
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
    note: item.note,
  })
  instanceDialogVisible.value = true
}

async function saveInstance() {
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
    ElMessage.success(editingInstanceId.value ? '实例已更新' : '服务已部署')
    instanceDialogVisible.value = false
    await loadAll()
    activeInstanceId.value = saved.id
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

async function refreshAll() {
  await loadAll()
  if (visibleInstances.value.length) await Promise.all(visibleInstances.value.map((row) => refreshInstanceStatus(row)))
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
    // ignore transient status refresh errors; next poll will retry
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

function startStatusPolling() {
  stopStatusPolling()
  statusPollTimer = setInterval(async () => {
    if (!visibleInstances.value.length) return
    await Promise.all(visibleInstances.value.map((row) => refreshInstanceStatus(row)))
  }, 10000)
}

function stopStatusPolling() {
  if (statusPollTimer) {
    clearInterval(statusPollTimer)
    statusPollTimer = null
  }
}

// -------- 日志查看 --------
const logsDialogVisible = ref(false)
const logsLoading = ref(false)
const logsContent = ref('')
const logsTargetName = ref('')

async function viewLogs(row: ServiceInstance) {
  logsTargetName.value = row.name
  logsDialogVisible.value = true
  logsLoading.value = true
  logsContent.value = ''
  try {
    logsContent.value = await instanceApi.logs(row.id, 500)
  } catch (error: any) {
    logsContent.value = error?.response?.data?.error || '日志获取失败'
  } finally {
    logsLoading.value = false
  }
}

// -------- 右侧详情面板：选中实例 + 概览/配置/事件 --------
const activeInstanceId = ref('')
const activeInstance = computed(() => instances.value.find((i) => i.id === activeInstanceId.value) || null)
const detailTab = ref<'overview' | 'config' | 'events'>('overview')

const detailData = ref<ContainerDetail | null>(null)
const detailLoading = ref(false)

async function loadDetail(instanceId: string) {
  detailLoading.value = true
  detailData.value = null
  try {
    detailData.value = await instanceApi.detail(instanceId)
  } catch {
    detailData.value = null
  } finally {
    detailLoading.value = false
  }
}

const events = ref<InstanceEvent[]>([])
const eventsLoading = ref(false)

async function loadEvents(instanceId: string) {
  eventsLoading.value = true
  try {
    events.value = await eventApi.list(instanceId, 20)
  } catch {
    events.value = []
  } finally {
    eventsLoading.value = false
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
  if (!id) return
  const item = instances.value.find((i) => i.id === id)
  if (item && item.status !== 'not_deployed') loadDetail(id)
  loadEvents(id)
})

onMounted(() => {
  loadAll().then(() => {
    startStatusPolling()
    if (activeInstanceId.value) {
      const item = instances.value.find((i) => i.id === activeInstanceId.value)
      if (item && item.status !== 'not_deployed') loadDetail(activeInstanceId.value)
      loadEvents(activeInstanceId.value)
    }
  })
})
onUnmounted(stopStatusPolling)
</script>

<template>
  <AppLayout>
    <div class="services-page">
      <!-- 顶部面包屑 + 筛选 -->
      <div class="top-bar">
        <div class="breadcrumb">
          <template v-if="activeProject">
            <span>{{ activeProject.name }}</span>
          </template>
          <template v-if="activeEnvironment">
            <span class="breadcrumb-sep">/</span>
            <span>{{ activeEnvironment.name }}</span>
          </template>
          <template v-if="activeType">
            <span class="breadcrumb-sep">/</span>
            <span class="breadcrumb-current">{{ activeType.name }}</span>
          </template>
        </div>
        <div class="top-bar-actions">
          <div class="top-bar-field">
            <span>项目</span>
            <el-select v-model="activeProjectId" placeholder="选择项目" :suffix-icon="ArrowDown" @change="onProjectChange">
              <el-option v-for="project in projects" :key="project.id" :label="project.name" :value="project.id" />
            </el-select>
          </div>
          <div class="top-bar-field">
            <span>环境</span>
            <el-select v-model="activeEnvironmentId" placeholder="选择环境" :suffix-icon="ArrowDown" @change="onEnvironmentChange">
              <el-option v-for="env in environmentsInProject(activeProjectId)" :key="env.id" :label="env.name" :value="env.id" />
            </el-select>
          </div>
          <el-tooltip content="刷新" placement="top">
            <el-button :icon="Refresh" :loading="loading" circle @click="refreshAll" />
          </el-tooltip>
          <el-button v-if="canManage" type="primary" :icon="Plus" :disabled="!activeType" @click="openCreateInstance">部署实例</el-button>
        </div>
      </div>

      <el-empty v-if="!loading && !projects.length" description="暂无项目，请先前往「项目管理」新建项目" :image-size="54" />
      <el-empty v-else-if="!loading && activeProjectId && !environmentsInProject(activeProjectId).length" description="当前项目暂无环境，请先添加环境">
        <el-button v-if="canManage" type="primary" :icon="Plus" @click="openCreateEnvironment">添加环境</el-button>
      </el-empty>

      <template v-else-if="activeEnvironmentId">
        <!-- 服务类型 Tab 条 -->
        <div class="type-tabs">
          <button
            v-for="type in typesInEnvironment(activeEnvironmentId)"
            :key="type.id"
            type="button"
            class="type-tab"
            :class="{ active: activeTypeId === type.id }"
            @click="() => { activeTypeId = type.id; onTypeChange() }"
          >
            <span>{{ type.name }}</span>
            <em>{{ countInType(type.id) }}</em>
            <span v-if="canManage" class="type-tab-actions">
              <el-icon class="type-tab-action" @click.stop="openEditType(type)"><Edit /></el-icon>
              <el-icon class="type-tab-action danger" @click.stop="removeType(type)"><Delete /></el-icon>
            </span>
          </button>
          <button v-if="canManage" type="button" class="type-tab-add" @click="openCreateType">
            <el-icon><Plus /></el-icon><span>新建服务类型</span>
          </button>
          <span v-if="canManage" class="type-tab-env-actions">
            <el-tooltip content="编辑当前环境" placement="top">
              <el-icon class="env-manage-action" @click="activeEnvironment && openEditEnvironment(activeEnvironment)"><Edit /></el-icon>
            </el-tooltip>
            <el-tooltip content="删除当前环境" placement="top">
              <el-icon class="env-manage-action danger" @click="activeEnvironment && removeEnvironment(activeEnvironment)"><Delete /></el-icon>
            </el-tooltip>
          </span>
        </div>

        <el-empty v-if="!typesInEnvironment(activeEnvironmentId).length" description="当前环境暂无服务类型，请先新建服务类型" :image-size="54" />

        <!-- 主体：左侧实例表格 + 右侧详情面板 -->
        <div v-else-if="activeType" class="master-detail" v-loading="loading">
          <section class="instance-panel">
            <div class="instance-toolbar">
              <span class="instance-toolbar-title">{{ activeType.name }}</span>
              <span class="instance-toolbar-count">{{ filteredInstances.length }} 个实例 · {{ filteredInstances.filter(isHealthy).length }} 个健康</span>
              <el-input v-model="keyword" :prefix-icon="Search" clearable placeholder="搜索实例、镜像或端口" class="instance-search" />
            </div>

            <el-table
              :data="filteredInstances"
              class="instance-table"
              highlight-current-row
              :current-row-key="activeInstanceId"
              row-key="id"
              @row-dblclick="(row: ServiceInstance) => (activeInstanceId = row.id)"
            >
              <el-table-column label="实例" min-width="140">
                <template #default="{ row }">
                  <span class="instance-name">{{ row.name }}</span>
                </template>
              </el-table-column>
              <el-table-column label="状态" width="120">
                <template #default="{ row }">
                  <span class="status-dot-label"><span class="status-dot" :class="`status-${row.status}`"></span>{{ statusLabel(row) }}</span>
                </template>
              </el-table-column>
              <el-table-column label="镜像" min-width="200">
                <template #default="{ row }"><span class="mono-text">{{ row.image }}</span></template>
              </el-table-column>
              <el-table-column label="网络" width="120">
                <template #default="{ row }">{{ row.network || '默认网络' }}</template>
              </el-table-column>
              <el-table-column label="端口" width="130">
                <template #default="{ row }"><span class="mono-text">{{ row.port_mapping || '未映射' }}</span></template>
              </el-table-column>
              <el-table-column label="运行时长" width="100">
                <template #default="{ row }">{{ row.status === 'running' ? formatRelativeTime(row.started_at) : '-' }}</template>
              </el-table-column>
              <el-table-column label="操作" width="150" fixed="right">
                <template #default="{ row }">
                  <div class="row-actions" @click.stop>
                    <el-link v-if="row.status !== 'not_deployed'" type="primary" :underline="false" @click="viewLogs(row)">日志</el-link>
                    <el-link
                      v-if="canManage && (row.status === 'running' || row.status === 'restarting')"
                      type="primary"
                      :underline="false"
                      @click="restartInstance(row)"
                    >重启</el-link>
                    <el-dropdown v-if="canManage" trigger="click">
                      <el-icon class="row-more"><MoreFilled /></el-icon>
                      <template #dropdown>
                        <el-dropdown-menu>
                          <el-dropdown-item v-if="row.status === 'not_deployed' || row.status === 'error'" :icon="Upload" @click="deployInstance(row)">部署</el-dropdown-item>
                          <el-dropdown-item v-if="row.status === 'stopped'" :icon="VideoPlay" @click="startInstance(row)">启动</el-dropdown-item>
                          <el-dropdown-item v-if="row.status === 'running' || row.status === 'restarting'" :icon="VideoPause" @click="stopInstance(row)">停止</el-dropdown-item>
                          <el-dropdown-item :icon="Refresh" @click="openUpdateImage(row)">更新镜像</el-dropdown-item>
                          <el-dropdown-item :icon="Edit" divided @click="openEditInstance(row)">编辑配置</el-dropdown-item>
                          <el-dropdown-item :icon="Delete" class="danger-menu-item" @click="removeInstance(row)">删除实例</el-dropdown-item>
                        </el-dropdown-menu>
                      </template>
                    </el-dropdown>
                  </div>
                </template>
              </el-table-column>
            </el-table>
            <el-empty
              v-if="!loading && !filteredInstances.length"
              :description="visibleInstances.length ? '没有符合筛选条件的实例' : '该类型暂无实例，点击右上角部署实例'"
              :image-size="60"
            />
          </section>

          <!-- 右侧详情面板 -->
          <aside v-if="activeInstance" class="detail-panel">
            <div class="detail-header">
              <div class="detail-header-icon" :class="`status-${activeInstance.status}`"><el-icon :size="20"><Box /></el-icon></div>
              <div class="detail-header-info">
                <div class="detail-header-name">{{ activeInstance.name }}</div>
              </div>
              <el-icon class="detail-close" @click="activeInstanceId = ''"><Close /></el-icon>
              <div v-if="canManage" class="detail-header-actions">
                <el-button v-if="activeInstance.status === 'not_deployed' || activeInstance.status === 'error'" size="small" type="primary" :icon="Upload" :loading="actionLoadingId === activeInstance.id" @click="deployInstance(activeInstance)">部署</el-button>
                <el-button v-if="activeInstance.status === 'stopped'" size="small" type="success" plain :icon="VideoPlay" :loading="actionLoadingId === activeInstance.id" @click="startInstance(activeInstance)">启动</el-button>
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
                <div class="detail-scroll">
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
            </el-tabs>
          </aside>
        </div>
      </template>
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
        <el-form-item label="部署主机" required>
          <el-select v-model="instanceForm.host_id" placeholder="请选择 Docker 主机" style="width: 100%">
            <el-option v-for="host in hosts" :key="host.id" :label="host.name" :value="host.id">
              <span>{{ host.name }}</span>
              <span class="host-option-detail">{{ host.docker_host }}</span>
            </el-option>
          </el-select>
        </el-form-item>
        <el-form-item label="实例名称" required>
          <el-input v-model="instanceForm.name" placeholder="例如 game-1、game-2" />
        </el-form-item>
        <el-form-item label="镜像" required>
          <el-input v-model="instanceForm.image" placeholder="例如 gogs-game:v1001-3" />
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

    <!-- 日志查看 -->
    <el-dialog v-model="logsDialogVisible" :title="`容器日志 - ${logsTargetName}`" width="760px" destroy-on-close>
      <el-input
        :model-value="logsContent"
        type="textarea"
        :rows="20"
        readonly
        class="logs-viewer"
        :placeholder="logsLoading ? '加载中...' : '暂无日志'"
      />
      <template #footer>
        <el-button @click="logsDialogVisible = false">关闭</el-button>
      </template>
    </el-dialog>
  </AppLayout>
</template>

<style scoped>
.services-page {
  display: flex;
  flex-direction: column;
  height: calc(100vh - 88px);
  min-height: 560px;
}

/* -------- 顶部面包屑 + 筛选 -------- */
.top-bar {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 16px;
  margin-bottom: 14px;
}

.breadcrumb {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 6px;
  min-width: 0;
  font-size: 15px;
  color: var(--ops-text-secondary);
}

.breadcrumb-sep { color: #c5ccd5; }
.breadcrumb-current { color: var(--ops-text); font-weight: 600; }

.top-bar-actions {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-left: auto;
}

.top-bar-field {
  display: flex;
  align-items: center;
  gap: 6px;
}

.top-bar-field > span {
  color: var(--ops-text-secondary);
  font-size: 12px;
  white-space: nowrap;
}

.top-bar-field .el-select { width: 140px; }

/* -------- 服务类型 Tab 条 -------- */
.type-tabs {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 14px;
  padding-bottom: 10px;
  border-bottom: 1px solid var(--ops-border);
  overflow-x: auto;
  scrollbar-width: none;
}

.type-tabs::-webkit-scrollbar { display: none; }

.type-tab {
  display: flex;
  align-items: center;
  gap: 8px;
  flex: 0 0 auto;
  height: 34px;
  padding: 0 6px 0 14px;
  border: 0;
  border-radius: 8px;
  background: transparent;
  color: #667287;
  font: inherit;
  font-size: 13px;
  font-weight: 600;
  cursor: pointer;
  transition: background .15s, color .15s;
}

.type-tab:hover { background: var(--ops-bg); color: var(--ops-text); }

.type-tab.active {
  background: var(--ops-primary-light);
  color: var(--ops-primary);
}

.type-tab em {
  display: grid;
  min-width: 20px;
  height: 20px;
  padding: 0 5px;
  place-items: center;
  border-radius: 999px;
  background: #eef2f6;
  color: #778495;
  font-size: 11px;
  font-style: normal;
}

.type-tab.active em { background: var(--ops-primary); color: #fff; }

.type-tab-actions { display: none; align-items: center; gap: 3px; margin-left: 2px; padding: 0 6px; }
.type-tab:hover .type-tab-actions, .type-tab.active .type-tab-actions { display: flex; }
.type-tab-action { font-size: 12px; opacity: .6; }
.type-tab-action:hover { opacity: 1; }
.type-tab-action.danger:hover { color: var(--el-color-danger); }

.type-tab-add {
  display: flex;
  align-items: center;
  gap: 5px;
  flex: 0 0 auto;
  height: 34px;
  padding: 0 12px;
  border: 1px dashed var(--ops-border);
  border-radius: 8px;
  background: transparent;
  color: #9aa4b2;
  font: inherit;
  font-size: 13px;
  cursor: pointer;
}

.type-tab-add:hover { border-color: #9fc8ff; color: var(--ops-primary); }

.type-tab-env-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-left: auto;
  padding-left: 10px;
  color: #9aa4b2;
  font-size: 13px;
}

.env-manage-action { cursor: pointer; }
.env-manage-action:hover { color: var(--ops-primary); }
.env-manage-action.danger:hover { color: var(--el-color-danger); }

/* -------- 主体：左右分栏 -------- */
.master-detail {
  display: flex;
  align-items: stretch;
  flex: 1;
  min-height: 0;
  border: 1px solid var(--ops-border);
  border-radius: 12px;
  background: #fff;
  box-shadow: 0 8px 28px rgba(31, 45, 61, .05);
  overflow: hidden;
}

.instance-panel {
  display: flex;
  flex: 1;
  min-width: 0;
  flex-direction: column;
}

.instance-toolbar {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 14px 18px;
  border-bottom: 1px solid var(--ops-border);
}

.instance-toolbar-title { font-size: 15px; font-weight: 700; color: var(--ops-text); }
.instance-toolbar-count { color: var(--ops-text-secondary); font-size: 12px; }
.instance-search { width: 240px; margin-left: auto; }

.instance-table { flex: 1; }
.instance-table :deep(.el-table__row) { cursor: pointer; }
.instance-table :deep(.current-row td) { background: var(--ops-primary-light) !important; }
.instance-name { font-weight: 600; color: var(--ops-text); }
.mono-text { font-family: "SFMono-Regular", Consolas, Monaco, monospace; font-size: 12.5px; color: #52657a; }

.status-dot-label { display: inline-flex; align-items: center; gap: 6px; line-height: 1; }
.status-dot { display: inline-block; width: 8px; height: 8px; flex: 0 0 auto; border-radius: 50%; background: #c0c4cc; }
.status-dot.status-running { background: #52c41a; box-shadow: 0 0 0 3px rgba(82, 196, 26, .14); }
.status-dot.status-error { background: #f56c6c; }
.status-dot.status-stopped, .status-dot.status-restarting { background: #e6a23c; }

.row-actions { display: flex; align-items: center; gap: 10px; }
.row-more { cursor: pointer; color: var(--ops-text-secondary); font-size: 16px; }
.row-more:hover { color: var(--ops-primary); }

/* -------- 右侧详情面板 -------- */
.detail-panel {
  display: flex;
  flex-direction: column;
  width: 380px;
  flex: 0 0 380px;
  min-width: 0;
  border-left: 1px solid var(--ops-border);
  background: #fafcff;
}


.detail-header {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  padding: 18px 18px 14px;
  border-bottom: 1px solid var(--ops-border);
}

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
.detail-header-name { font-size: 16px; font-weight: 700; color: var(--ops-text); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

.detail-close {
  flex: 0 0 auto;
  padding: 4px;
  border-radius: 4px;
  color: var(--ops-text-secondary);
  font-size: 15px;
  cursor: pointer;
}
.detail-close:hover { background: var(--ops-bg); color: var(--ops-text); }

.detail-header-actions { display: flex; flex-wrap: wrap; justify-content: flex-end; gap: 6px; flex-basis: 100%; margin-top: 10px; }

.detail-tabs { flex: 1; min-height: 0; display: flex; flex-direction: column; }
.detail-tabs :deep(.el-tabs__header) { margin: 0; padding: 0 18px; }
.detail-tabs :deep(.el-tabs__content) { flex: 1; min-height: 0; overflow-y: auto; }
.detail-tabs :deep(.el-tab-pane) { height: 100%; }

.detail-scroll { padding: 16px 18px 20px; }

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

@media (max-width: 1100px) {
  .master-detail { flex-direction: column; overflow: visible; }
  .instance-panel:has(+ .detail-panel) { border-bottom: 1px solid var(--ops-border); }
  .detail-panel { width: 100%; flex: 1 1 auto; border-left: 0; border-top: 1px solid var(--ops-border); }
}

@media (max-width: 900px) {
  .services-page { height: auto; min-height: 0; }
  .top-bar { flex-direction: column; align-items: stretch; gap: 10px; }
  .top-bar-actions { flex-wrap: wrap; margin-left: 0; }
  .top-bar-field .el-select { flex: 1; width: auto; }
  .instance-toolbar { flex-wrap: wrap; }
  .instance-search { width: 100%; margin-left: 0; }
  :deep(.el-dialog) { width: calc(100vw - 24px) !important; margin-top: 3vh !important; }
}
</style>
