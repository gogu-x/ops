<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus, Edit, Delete, Box, VideoPlay, VideoPause, RefreshRight, Refresh, Upload, Document, InfoFilled, Search, MoreFilled } from '@element-plus/icons-vue'
import { useAuthStore } from '../stores/auth'
import { environmentApi, type Environment, type EnvironmentForm } from '../api/environments'
import { serviceApi, type ServiceType, type ServiceTypeForm } from '../api/services'
import { hostApi, type Host } from '../api/hosts'
import { projectApi, type Project } from '../api/projects'
import { instanceApi, type ServiceInstance, type ServiceInstanceForm, type ContainerDetail } from '../api/instances'
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

// -------- 顶部项目 Tab + 环境 Tab + 左侧服务类型列表 --------
const activeProjectId = ref<string>('')
const activeEnvironmentId = ref<string>('')
const activeTypeId = ref<string>('')
const keyword = ref('')
const statusFilter = ref('all')

function environmentsInProject(projectId: string): Environment[] {
  return environments.value.filter((e) => e.project_id === projectId)
}

function typesInEnvironment(environmentId: string): ServiceType[] {
  return types.value.filter((t) => t.environment_id === environmentId)
}

function countInProject(projectId: string): number {
  const envIds = new Set(environmentsInProject(projectId).map((e) => e.id))
  const typeIds = new Set(types.value.filter((t) => envIds.has(t.environment_id)).map((t) => t.id))
  return instances.value.filter((i) => typeIds.has(i.service_type_id)).length
}

function countInEnvironment(environmentId: string): number {
  const typeIds = new Set(typesInEnvironment(environmentId).map((t) => t.id))
  return instances.value.filter((i) => typeIds.has(i.service_type_id)).length
}

function countInType(typeId: string): number {
  return instances.value.filter((i) => i.service_type_id === typeId).length
}

const activeType = computed(() => types.value.find((t) => t.id === activeTypeId.value) || null)
const activeEnvironment = computed(() => environments.value.find((e) => e.id === activeEnvironmentId.value) || null)
const activeProject = computed(() => (activeEnvironment.value ? projects.value.find((p) => p.id === activeEnvironment.value!.project_id) : null))
const currentProject = computed(() => projects.value.find((p) => p.id === activeProjectId.value) || null)

const visibleInstances = computed(() => {
  if (!activeTypeId.value) return []
  return instances.value.filter((i) => i.service_type_id === activeTypeId.value)
})

const filteredInstances = computed(() => {
  const normalizedKeyword = keyword.value.trim().toLowerCase()
  return visibleInstances.value.filter((item) => {
    const matchesStatus = statusFilter.value === 'all' || item.status === statusFilter.value
    const matchesKeyword = !normalizedKeyword || [item.name, item.image, item.note]
      .some((value) => value?.toLowerCase().includes(normalizedKeyword))
    return matchesStatus && matchesKeyword
  })
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
    if (activeTypeId.value && !types.value.some((t) => t.id === activeTypeId.value)) {
      activeTypeId.value = ''
    }
    selectFirstAvailableType()
  } catch {
    ElMessage.error('数据加载失败')
  } finally {
    loading.value = false
  }
}

function selectFirstAvailableType() {
  if (!activeEnvironmentId.value || activeTypeId.value) return
  activeTypeId.value = typesInEnvironment(activeEnvironmentId.value)[0]?.id || ''
}

// -------- 环境表单 --------
const environmentDialogVisible = ref(false)
const editingEnvironmentId = ref('')
const environmentForm = reactive<EnvironmentForm>({ project_id: '', name: '' })
const environmentSubmitting = ref(false)

function openCreateEnvironment() {
  editingEnvironmentId.value = ''
  Object.assign(environmentForm, {
    project_id: activeProjectId.value || projects.value[0]?.id || '',
    name: '',
  })
  environmentDialogVisible.value = true
}

function openEditEnvironment(env: Environment) {
  editingEnvironmentId.value = env.id
  Object.assign(environmentForm, {
    project_id: env.project_id,
    name: env.name,
  })
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
  const relatedCount = countInEnvironment(env.id)
  try {
    await ElMessageBox.confirm(
      relatedCount > 0
        ? `环境"${env.name}"下还有 ${relatedCount} 个服务实例，删除环境不会自动删除服务类型或实例，但它们将失去所属环境。是否继续？`
        : `确定删除环境"${env.name}"吗？`,
      '删除确认',
      { type: 'warning' },
    )
    await environmentApi.remove(env.id)
    ElMessage.success('环境已删除')
    if (activeEnvironmentId.value === env.id) activeEnvironmentId.value = ''
    await loadAll()
  } catch (error: any) {
    if (error !== 'cancel' && error !== 'close') ElMessage.error(error?.response?.data?.error || '删除失败')
  }
}

// -------- 类型（分类）表单 --------
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
  Object.assign(typeForm, {
    project_id: type.project_id,
    environment_id: type.environment_id,
    name: type.name,
  })
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
    env_text: activeProject.value?.env_vars || currentProject.value?.env_vars || '',
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
    if (editingInstanceId.value) await instanceApi.update(editingInstanceId.value, payload)
    else await instanceApi.create(payload)
    ElMessage.success(editingInstanceId.value ? '实例已更新' : '服务已部署')
    instanceDialogVisible.value = false
    await loadAll()
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
    await loadAll()
  } catch (error: any) {
    if (error !== 'cancel' && error !== 'close') ElMessage.error(error?.response?.data?.error || '删除失败')
  }
}

watch(activeProjectId, () => {
  activeTypeId.value = ''
  activeEnvironmentId.value = ''
  keyword.value = ''
  statusFilter.value = 'all'
  selectFirstAvailableType()
})

watch(activeEnvironmentId, () => {
  activeTypeId.value = ''
  keyword.value = ''
  statusFilter.value = 'all'
  selectFirstAvailableType()
})

watch(activeTypeId, () => {
  keyword.value = ''
  statusFilter.value = 'all'
})

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

function formatRelativeTime(value?: string) {
  if (!value) return '尚未启动'
  const timestamp = new Date(value).getTime()
  if (Number.isNaN(timestamp)) return '启动时间未知'
  const minutes = Math.max(0, Math.floor((Date.now() - timestamp) / 60000))
  if (minutes < 1) return '刚刚启动'
  if (minutes < 60) return `${minutes} 分钟前启动`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours} 小时前启动`
  return `${Math.floor(hours / 24)} 天前启动`
}

async function refreshVisibleInstances() {
  if (!visibleInstances.value.length) return
  await Promise.all(visibleInstances.value.map((row) => refreshInstanceStatus(row)))
  ElMessage.success('实例状态已刷新')
}

const actionLoadingId = ref('')

async function deployInstance(row: ServiceInstance) {
  actionLoadingId.value = row.id
  try {
    await instanceApi.deploy(row.id)
    ElMessage.success(`实例"${row.name}"已部署并启动`)
    await refreshInstanceStatus(row)
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
  } catch (error: any) {
    ElMessage.error(error?.response?.data?.error || '重启失败')
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

// -------- 容器详情查看 --------
const detailDialogVisible = ref(false)
const detailLoading = ref(false)
const detailError = ref('')
const detailData = ref<ContainerDetail | null>(null)
const detailTargetName = ref('')

const detailStatusMeta = statusMeta

function formatDateTime(value?: string): string {
  if (!value) return '-'
  const time = new Date(value)
  if (Number.isNaN(time.getTime()) || value.startsWith('0001-01-01')) return '-'
  return time.toLocaleString()
}

async function viewDetail(row: ServiceInstance) {
  detailTargetName.value = row.name
  detailDialogVisible.value = true
  detailLoading.value = true
  detailError.value = ''
  detailData.value = null
  try {
    detailData.value = await instanceApi.detail(row.id)
  } catch (error: any) {
    detailError.value = error?.response?.data?.error || '获取容器详情失败'
  } finally {
    detailLoading.value = false
  }
}

onMounted(() => {
  loadAll().then(startStatusPolling)
})
onUnmounted(stopStatusPolling)
</script>

<template>
  <AppLayout>
    <div class="mobile-project-filter">
      <span class="mobile-project-title">项目</span>
      <div class="mobile-project-list">
        <button
          v-for="project in projects"
          :key="project.id"
          type="button"
          :class="{ active: activeProjectId === project.id }"
          @click="activeProjectId = project.id"
        >
          <span>{{ project.name }}</span><em>{{ countInProject(project.id) }}</em>
        </button>
      </div>
    </div>

    <el-empty v-if="!loading && !projects.length" description="暂无项目，请先前往「项目管理」新建项目" :image-size="54" />

    <el-card v-if="activeProjectId" class="plain-card master-detail" shadow="never" v-loading="loading">
      <div class="browser-tabs">
        <div class="browser-tabs-scroll">
          <button
            v-for="env in environmentsInProject(activeProjectId)"
            :key="env.id"
            type="button"
            class="browser-tab"
            :class="{ active: activeEnvironmentId === env.id }"
            @click="activeEnvironmentId = env.id"
          >
            <span class="browser-tab-name">{{ env.name }}</span>
            <em>{{ countInEnvironment(env.id) }}</em>
            <span v-if="canManage" class="env-tab-actions">
              <el-icon class="env-tab-action" @click.stop="openEditEnvironment(env)"><Edit /></el-icon>
              <el-icon class="env-tab-action danger" @click.stop="removeEnvironment(env)"><Delete /></el-icon>
            </span>
          </button>
          <button v-if="canManage" type="button" class="browser-tab-add" @click="openCreateEnvironment">
            <el-icon><Plus /></el-icon><span>添加环境</span>
          </button>
        </div>
      </div>

      <el-empty v-if="!environmentsInProject(activeProjectId).length" description="当前项目暂无环境，请先添加环境" :image-size="54" class="browser-tabs-empty" />

      <div v-else class="master-detail-body">
        <aside class="filter-sidebar">
        <div class="filter-heading">
          <div><span>服务类型</span></div>
          <el-button v-if="canManage && activeEnvironmentId" text :icon="Plus" @click="openCreateType">添加</el-button>
        </div>

        <div class="service-tree">
          <button
            v-for="type in typesInEnvironment(activeEnvironmentId)"
            :key="type.id"
            type="button"
            class="tree-type flat"
            :class="{ active: activeTypeId === type.id }"
            @click="activeTypeId = type.id"
          >
            <el-icon><Box /></el-icon>
            <span class="tree-name">{{ type.name }}</span>
            <em>{{ countInType(type.id) }}</em>
            <span v-if="canManage" class="tree-type-actions">
              <el-tooltip content="编辑类型" placement="top">
                <span class="tree-action" @click.stop="openEditType(type)"><el-icon><Edit /></el-icon></span>
              </el-tooltip>
              <el-tooltip content="删除类型" placement="top">
                <span class="tree-action danger" @click.stop="removeType(type)"><el-icon><Delete /></el-icon></span>
              </el-tooltip>
            </span>
          </button>
          <div v-if="!typesInEnvironment(activeEnvironmentId).length" class="filter-empty">当前环境暂无服务类型</div>
        </div>

      </aside>

      <section class="service-panel">
        <template v-if="activeType">
          <div class="service-toolbar">
            <el-input v-model="keyword" :prefix-icon="Search" clearable placeholder="搜索实例名称、镜像或备注" class="service-search" />
            <el-select v-model="statusFilter" class="status-filter" aria-label="状态筛选">
              <el-option label="全部状态" value="all" />
              <el-option label="运行中" value="running" />
              <el-option label="已停止" value="stopped" />
              <el-option label="未部署" value="not_deployed" />
              <el-option label="部署失败" value="error" />
            </el-select>
            <span class="result-count">{{ filteredInstances.length }} 个结果</span>
            <el-button :icon="Refresh" @click="refreshVisibleInstances">刷新</el-button>
            <el-button v-if="canManage" type="primary" :icon="Plus" @click="openCreateInstance">部署服务</el-button>
          </div>

          <div class="service-list">
            <div v-for="row in filteredInstances" :key="row.id" class="service-item" :class="`status-${row.status}`">
              <div class="service-item-icon" :class="`status-${row.status}`">
                <el-icon :size="18"><Box /></el-icon>
              </div>
              <div class="service-item-main">
                <div class="service-item-title-row">
                  <span class="service-item-name">{{ row.name }}</span>
                  <el-tag size="small" :type="statusType(row)" effect="light" round>{{ statusLabel(row) }}</el-tag>
                </div>
                <div class="service-meta-grid">
                  <div class="meta-block image-meta">
                    <small>镜像</small>
                    <span class="service-item-image" :title="row.image">{{ row.image }}</span>
                  </div>
                  <div class="meta-block">
                    <small>网络</small>
                    <span>{{ row.network || '默认网络' }}</span>
                  </div>
                  <div class="meta-block">
                    <small>端口</small>
                    <span>{{ row.port_mapping || '未映射' }}</span>
                  </div>
                  <div class="meta-block time-meta">
                    <small>运行时间</small>
                    <span>{{ formatRelativeTime(row.started_at) }}</span>
                  </div>
                </div>
                <div v-if="row.status === 'error' && row.deploy_error" class="service-item-error">部署失败：{{ row.deploy_error }}</div>
                <div v-if="row.note" class="service-item-note">{{ row.note }}</div>
              </div>
              <div class="service-item-actions">
                <template v-if="canManage">
                  <el-tooltip v-if="row.status === 'not_deployed' || row.status === 'error'" content="部署" placement="top">
                    <el-button type="primary" circle :icon="Upload" :loading="actionLoadingId === row.id" @click="deployInstance(row)" />
                  </el-tooltip>
                  <el-tooltip v-if="row.status === 'stopped'" content="启动" placement="top">
                    <el-button type="success" plain circle :icon="VideoPlay" :loading="actionLoadingId === row.id" @click="startInstance(row)" />
                  </el-tooltip>
                  <el-tooltip v-if="row.status === 'running' || row.status === 'restarting'" content="停止" placement="top">
                    <el-button plain circle :icon="VideoPause" :loading="actionLoadingId === row.id" @click="stopInstance(row)" />
                  </el-tooltip>
                  <el-tooltip v-if="row.status === 'running'" content="重启" placement="top">
                    <el-button plain circle :icon="RefreshRight" :loading="actionLoadingId === row.id" @click="restartInstance(row)" />
                  </el-tooltip>
                  <el-tooltip v-if="row.status !== 'not_deployed'" content="更新镜像" placement="top">
                    <el-button plain type="primary" circle :icon="Refresh" @click="openUpdateImage(row)" />
                  </el-tooltip>
                </template>
                <el-dropdown trigger="click">
                  <el-button plain circle :icon="MoreFilled" />
                  <template #dropdown>
                    <el-dropdown-menu>
                      <el-dropdown-item v-if="row.status !== 'not_deployed'" :icon="InfoFilled" @click="viewDetail(row)">容器详情</el-dropdown-item>
                      <el-dropdown-item v-if="row.status !== 'not_deployed'" :icon="Document" @click="viewLogs(row)">查看日志</el-dropdown-item>
                      <el-dropdown-item v-if="canManage" :icon="Edit" divided @click="openEditInstance(row)">编辑配置</el-dropdown-item>
                      <el-dropdown-item v-if="canManage" :icon="Delete" class="danger-menu-item" @click="removeInstance(row)">删除实例</el-dropdown-item>
                    </el-dropdown-menu>
                  </template>
                </el-dropdown>
              </div>
            </div>
            <el-empty v-if="!loading && !filteredInstances.length" :description="visibleInstances.length ? '没有符合筛选条件的实例' : '该类型暂无实例，点击右上角部署服务'" :image-size="60" />
          </div>
        </template>

        <el-empty v-else description="请从左侧选择一个服务类型查看实例" :image-size="80" class="service-panel-placeholder" />
      </section>
      </div>
    </el-card>

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
    <el-dialog v-model="instanceDialogVisible" :title="editingInstanceId ? '编辑实例' : '部署服务'" width="800px" destroy-on-close>
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

    <!-- 容器详情 -->
    <el-dialog v-model="detailDialogVisible" :title="`容器详情 - ${detailTargetName}`" width="900px" top="5vh" destroy-on-close>
      <div v-loading="detailLoading" class="detail-body">
        <el-alert v-if="detailError" :title="detailError" type="error" :closable="false" show-icon />
        <template v-else-if="detailData">
          <el-descriptions :column="2" border size="small">
            <el-descriptions-item label="容器 ID">{{ detailData.container_id?.slice(0, 12) || '-' }}</el-descriptions-item>
            <el-descriptions-item label="容器名称">{{ detailData.name }}</el-descriptions-item>
            <el-descriptions-item label="状态">
              <el-tag size="small" :type="detailStatusMeta[detailData.status]?.type || 'info'" effect="light">
                {{ detailStatusMeta[detailData.status]?.text || detailData.state }}
              </el-tag>
            </el-descriptions-item>
            <el-descriptions-item label="重启次数">{{ detailData.restart_count }}</el-descriptions-item>
            <el-descriptions-item label="镜像">{{ detailData.image }}</el-descriptions-item>
            <el-descriptions-item label="网络模式">{{ detailData.network_mode || '-' }}</el-descriptions-item>
            <el-descriptions-item label="重启策略">{{ detailData.restart_policy || '-' }}</el-descriptions-item>
            <el-descriptions-item label="IP 地址">{{ detailData.ip_address || '-' }}</el-descriptions-item>
            <el-descriptions-item label="创建时间">{{ formatDateTime(detailData.created) }}</el-descriptions-item>
            <el-descriptions-item label="启动时间">{{ formatDateTime(detailData.started_at) }}</el-descriptions-item>
            <el-descriptions-item v-if="!detailData.running" label="结束时间">{{ formatDateTime(detailData.finished_at) }}</el-descriptions-item>
            <el-descriptions-item v-if="!detailData.running" label="退出码">{{ detailData.exit_code }}</el-descriptions-item>
          </el-descriptions>

          <el-alert v-if="detailData.error" :title="`容器错误：${detailData.error}`" type="error" :closable="false" show-icon class="detail-section" />
          <el-alert
            v-else-if="!detailData.running && detailData.exit_code !== 0"
            :title="`容器已退出，退出码 ${detailData.exit_code}（非 0 通常表示程序启动失败或缺少必需参数），请查看日志排查`"
            type="warning"
            :closable="false"
            show-icon
            class="detail-section"
          />

          <div class="detail-section">
            <div class="detail-section-title">启动命令</div>
            <div v-if="detailData.cmd?.length" class="detail-code">{{ detailData.cmd.join(' ') }}</div>
          </div>

          <div class="detail-section">
            <div class="detail-section-title">端口映射</div>
            <div v-if="detailData.ports?.length" class="detail-list">
              <div v-for="port in detailData.ports" :key="port" class="detail-list-row">{{ port }}</div>
            </div>
            <span v-else class="detail-empty">未映射端口</span>
          </div>

          <div class="detail-section">
            <div class="detail-section-title">挂载卷</div>
            <div v-if="detailData.mounts?.length" class="detail-list">
              <div v-for="mount in detailData.mounts" :key="mount" class="detail-list-row">{{ mount }}</div>
            </div>
            <span v-else class="detail-empty">未挂载数据卷</span>
          </div>

          <div class="detail-section">
            <div class="detail-section-title">环境变量</div>
            <div v-if="detailData.env?.length" class="detail-list">
              <div v-for="item in detailData.env" :key="item" class="detail-list-row">{{ item }}</div>
            </div>
            <span v-else class="detail-empty">未配置环境变量</span>
          </div>

          <div v-if="detailData.labels && Object.keys(detailData.labels).length" class="detail-section">
            <div class="detail-section-title">标签</div>
            <div class="detail-list">
              <div v-for="(value, key) in detailData.labels" :key="key" class="detail-list-row">{{ key }} = {{ value }}</div>
            </div>
          </div>
        </template>
      </div>
      <template #footer>
        <el-button @click="detailDialogVisible = false">关闭</el-button>
      </template>
    </el-dialog>
  </AppLayout>
</template>

<style scoped>
.plain-card {
  border-radius: 12px;
  border: 1px solid var(--ops-border);
  box-shadow: 0 8px 28px rgba(31, 45, 61, .05);
  overflow: hidden;
}

.plain-card :deep(.el-card__body) {
  padding: 0;
}

.master-detail :deep(.el-card__body) {
  display: flex;
  flex-direction: column;
  min-height: calc(100vh - 220px);
}

.master-detail {
  width: 100%;
}

.master-detail-body {
  display: flex;
  align-items: stretch;
  flex: 1;
  min-height: 0;
}

.browser-tabs {
  display: flex;
  align-items: flex-end;
  flex: 0 0 auto;
  padding: 10px 14px 0;
  border-bottom: 1px solid var(--ops-border);
  background: linear-gradient(180deg, #f3f5f8 0%, #eef1f5 100%);
}

.browser-tabs-scroll {
  display: flex;
  align-items: flex-end;
  gap: 4px;
  min-width: 0;
  overflow-x: auto;
  scrollbar-width: none;
}

.browser-tabs-scroll::-webkit-scrollbar { display: none; }

.browser-tab {
  display: flex;
  align-items: center;
  gap: 7px;
  flex: 0 0 auto;
  height: 34px;
  padding: 0 14px;
  border: 1px solid var(--ops-border);
  border-bottom: 0;
  border-radius: 8px 8px 0 0;
  background: #e4e8ed;
  color: #667287;
  font: inherit;
  font-size: 12px;
  cursor: pointer;
  transition: background .15s, color .15s;
}

.browser-tab:hover { background: #edf1f5; color: var(--ops-text); }

.browser-tab.active {
  position: relative;
  height: 36px;
  background: #fff;
  color: var(--ops-text);
  font-weight: 600;
}

.browser-tab.active::after {
  content: '';
  position: absolute;
  left: 0;
  right: 0;
  bottom: -1px;
  height: 2px;
  background: #fff;
}

.browser-tab-name { max-width: 140px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.browser-tab em { min-width: 18px; padding: 1px 5px; border-radius: 999px; background: rgba(0, 0, 0, .06); color: #778495; font-size: 10px; font-style: normal; text-align: center; }
.browser-tab.active em { background: var(--ops-primary-light); color: var(--ops-primary); }

.env-tab-actions { display: flex; align-items: center; gap: 3px; margin-left: 1px; }
.env-tab-action { font-size: 12px; color: inherit; opacity: .55; }
.env-tab-action:hover { opacity: 1; }
.env-tab-action.danger:hover { color: var(--el-color-danger); }

.browser-tab-add {
  display: flex;
  align-items: center;
  gap: 5px;
  flex: 0 0 auto;
  height: 34px;
  margin-left: 4px;
  padding: 0 12px;
  border: 1px dashed transparent;
  border-radius: 8px 8px 0 0;
  background: transparent;
  color: #9aa4b2;
  font: inherit;
  font-size: 12px;
  cursor: pointer;
}

.browser-tab-add:hover { border-color: #c3cddb; color: var(--ops-primary); }

.browser-tabs-empty { margin: auto; }

.mobile-project-filter {
  display: block;
  margin-bottom: 14px;
  padding: 10px 12px;
  border: 1px solid var(--ops-border);
  border-radius: 9px;
  background: #fff;
  box-shadow: var(--ops-shadow);
}

.mobile-project-title { display: block; margin-bottom: 8px; color: #697586; font-size: 11px; font-weight: 600; }
.mobile-project-list { display: flex; gap: 7px; margin: 0 -12px; padding: 0 12px 2px; overflow-x: auto; scrollbar-width: none; }
.mobile-project-list::-webkit-scrollbar { display: none; }
.mobile-project-list button { display: flex; align-items: center; gap: 7px; flex: 0 0 auto; padding: 7px 11px; border: 1px solid var(--ops-border); border-radius: 7px; background: #fff; color: #586779; font: inherit; font-size: 12px; cursor: pointer; }
.mobile-project-list button:hover { border-color: #9fc8ff; color: var(--ops-primary); }
.mobile-project-list button.active { border-color: var(--ops-primary); background: var(--ops-primary); color: #fff; }
.mobile-project-list em { min-width: 18px; padding: 1px 5px; border-radius: 999px; background: #eef2f6; color: #778495; font-size: 10px; font-style: normal; text-align: center; }
.mobile-project-list button.active em { background: rgba(255,255,255,.22); color: #fff; }

.filter-sidebar {
  width: 232px;
  flex: 0 0 232px;
  padding: 18px 14px;
  border-right: 1px solid var(--ops-border);
  background: linear-gradient(180deg, #fbfcfe 0%, #f7f9fc 100%);
}

.filter-heading {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 18px;
  padding: 0 2px 12px;
  border-bottom: 1px solid #e9edf2;
}

.filter-heading > div { display: flex; flex-direction: column; gap: 3px; }
.filter-heading span { color: var(--ops-text); font-size: 14px; font-weight: 700; }
.filter-heading small { color: var(--ops-text-secondary); font-size: 11px; font-weight: 400; }
.filter-heading .el-button { padding: 5px; }

.filter-label {
  display: block;
  margin: 0 2px 6px;
  color: #697586;
  font-size: 11px;
  font-weight: 600;
}

.filter-select { width: 100%; margin-bottom: 16px; }
.filter-select :deep(.el-select__wrapper) { min-height: 36px; border-radius: 7px; box-shadow: 0 0 0 1px #dde3eb inset; }
.filter-select :deep(.el-select__wrapper:hover) { box-shadow: 0 0 0 1px #9fc8ff inset; }

.service-tree { display: flex; flex-direction: column; gap: 3px; }
.tree-type { display: flex; position: relative; align-items: center; width: 100%; gap: 7px; padding: 9px 8px; border: 0; border-radius: 7px; background: transparent; color: #526071; font: inherit; font-size: 12px; cursor: pointer; }
.tree-name { min-width: 0; flex: 1; overflow: hidden; text-align: left; text-overflow: ellipsis; white-space: nowrap; }
.tree-type em { min-width: 20px; padding: 1px 5px; border-radius: 999px; background: #fff; color: #8491a3; font-size: 10px; font-style: normal; text-align: center; }
.tree-type:hover { background: #edf3fa; color: var(--ops-primary); }
.tree-type.active { background: #e4f1ff; color: var(--ops-primary); font-weight: 600; }
.tree-type.active em { background: var(--ops-primary); color: #fff; }
.tree-type-actions { display: flex; align-items: center; gap: 2px; margin-left: 1px; }
.tree-action { display: grid; width: 22px; height: 22px; place-items: center; border-radius: 5px; color: #718096; font-size: 12px; transition: background .15s, color .15s; }
.tree-action:hover { background: #d8eaff; color: var(--ops-primary); }
.tree-action.danger:hover { background: #fee9e9; color: var(--el-color-danger); }
.tree-type.active .tree-action { color: #3977b7; }

.type-list-heading { display: flex; align-items: center; justify-content: space-between; margin-top: 2px; }
.type-list-heading > span { min-width: 20px; padding: 1px 6px; border-radius: 999px; background: #e9edf3; color: #718096; font-size: 10px; text-align: center; }
.filter-type-list { display: flex; flex-direction: column; gap: 4px; margin-top: 2px; }

.filter-type-item {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  padding: 9px 8px;
  border: 0;
  border-radius: 7px;
  background: transparent;
  color: #526071;
  font: inherit;
  font-size: 12px;
  text-align: left;
  cursor: pointer;
  transition: background .16s, color .16s;
}

.filter-type-item:hover { background: #edf3fa; color: var(--ops-primary); }
.filter-type-item.active { background: #e7f2ff; color: var(--ops-primary); font-weight: 600; }
.filter-type-icon { display: grid; width: 25px; height: 25px; flex: 0 0 auto; place-items: center; border-radius: 6px; background: #e9edf3; }
.filter-type-item.active .filter-type-icon { background: var(--ops-primary); color: #fff; }
.filter-type-name { min-width: 0; flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.filter-type-item em { min-width: 20px; padding: 1px 5px; border-radius: 999px; background: #fff; color: #8491a3; font-size: 10px; font-style: normal; text-align: center; }
.filter-empty { padding: 18px 4px; color: var(--ops-text-secondary); font-size: 12px; text-align: center; }
.filter-actions { display: flex; justify-content: space-between; margin-top: 14px; padding-top: 10px; border-top: 1px solid #e9edf2; }
.filter-actions .el-button + .el-button { margin-left: 0; }

.scope-selector {
  display: flex;
  align-items: flex-end;
  gap: 10px;
  margin-bottom: 16px;
  padding: 12px 14px;
  border: 1px solid var(--ops-border);
  border-radius: 10px;
  background: #fff;
  box-shadow: var(--ops-shadow);
}

.scope-title {
  align-self: center;
  margin-right: 4px;
  color: var(--ops-text);
  font-size: 13px;
  font-weight: 700;
}

.scope-field {
  width: 210px;
  min-width: 0;
}

.scope-field.type-field { width: 230px; }
.scope-field label { display: block; margin: 0 0 5px 2px; color: var(--ops-text-secondary); font-size: 11px; }
.scope-select { width: 100%; }
.scope-separator { align-self: center; margin-top: 17px; color: #c5ccd5; font-size: 16px; }
.scope-actions { display: flex; align-items: center; gap: 2px; margin-left: auto; padding-bottom: 1px; }
.scope-result { align-self: center; color: var(--ops-text-secondary); font-size: 12px; white-space: nowrap; }
.scope-actions .el-button + .el-button { margin-left: 0; }
.option-count { float: right; margin-left: 24px; color: var(--ops-text-secondary); font-size: 11px; }

.deployment-explorer {
  padding: 20px 24px 18px;
  border-bottom: 1px solid var(--ops-border);
  background: #fff;
}

.explorer-heading {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 14px;
}

.explorer-heading h3 { margin: 1px 0 0; font-size: 16px; }
.explorer-heading > span { color: var(--ops-text-secondary); font-size: 12px; }
.explorer-eyebrow { color: var(--ops-primary); font-size: 9px; font-weight: 800; letter-spacing: .12em; }

.host-selector {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(210px, 1fr));
  gap: 10px;
}

.host-option {
  position: relative;
  display: flex;
  align-items: center;
  gap: 11px;
  min-width: 0;
  padding: 12px;
  border: 1px solid var(--ops-border);
  border-radius: 9px;
  background: #fff;
  color: var(--ops-text);
  font: inherit;
  text-align: left;
  cursor: pointer;
  transition: .18s ease;
}

.host-option:hover { border-color: #9fc8ff; background: #f8fbff; }
.host-option.active { border-color: var(--ops-primary); background: #f2f8ff; box-shadow: inset 0 0 0 1px var(--ops-primary); }
.host-option-icon { display: grid; width: 34px; height: 34px; flex: 0 0 auto; place-items: center; border-radius: 8px; background: #edf2f7; color: #61758a; }
.host-option.active .host-option-icon { background: var(--ops-primary); color: #fff; }
.host-option-content { min-width: 0; flex: 1; }
.host-option-content strong, .host-option-content small { display: block; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.host-option-content strong { font-size: 13px; }
.host-option-content small { margin-top: 3px; color: var(--ops-text-secondary); font-size: 11px; }
.host-option-state { width: 7px; height: 7px; flex: 0 0 auto; border-radius: 50%; background: #67c23a; box-shadow: 0 0 0 3px rgba(103, 194, 58, .12); }

.type-selector {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 12px;
  padding-top: 12px;
  border-top: 1px dashed #e3e8ef;
  overflow-x: auto;
}

.type-selector-label { flex: 0 0 auto; margin-right: 2px; color: var(--ops-text-secondary); font-size: 11px; }
.type-option { display: flex; align-items: center; gap: 6px; flex: 0 0 auto; padding: 7px 10px; border: 1px solid var(--ops-border); border-radius: 999px; background: #fff; color: #586779; font: inherit; font-size: 12px; cursor: pointer; }
.type-option:hover { border-color: #9fc8ff; color: var(--ops-primary); }
.type-option.active { border-color: var(--ops-primary); background: var(--ops-primary); color: #fff; }
.type-option em { min-width: 18px; padding: 1px 5px; border-radius: 999px; background: #eef2f6; color: #778495; font-size: 10px; font-style: normal; text-align: center; }
.type-option.active em { background: rgba(255,255,255,.2); color: #fff; }
.type-manage { display: flex; gap: 5px; margin-left: 2px; padding-left: 7px; border-left: 1px solid currentColor; opacity: .7; }
.type-manage .el-icon:hover { opacity: 1; }
.type-empty { color: var(--ops-text-secondary); font-size: 12px; }

.project-tabs {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-bottom: 16px;
}

.project-tab {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 8px 14px;
  border-radius: 6px;
  background: #fff;
  border: 1px solid var(--ops-border);
  cursor: pointer;
  font-size: 14px;
  color: var(--ops-text-secondary);
  transition: all 0.15s;
}

.project-tab:hover {
  border-color: var(--ops-primary);
  color: var(--ops-primary);
}

.project-tab.active {
  background: var(--ops-primary);
  border-color: var(--ops-primary);
  color: #fff;
}

.project-tab-name {
  max-width: 160px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.project-tab-count {
  font-size: 12px;
  background: var(--ops-bg);
  color: var(--ops-text-secondary);
  border-radius: 10px;
  padding: 1px 7px;
}

.project-tab.active .project-tab-count {
  background: rgba(255, 255, 255, 0.25);
  color: #fff;
}

.project-tabs-empty {
  width: 100%;
  padding: 8px 0;
}

.summary-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 12px;
  margin-bottom: 16px;
}

.summary-card {
  display: flex;
  align-items: center;
  gap: 12px;
  min-height: 76px;
  padding: 14px 16px;
  background: #fff;
  border: 1px solid var(--ops-border);
  border-radius: 8px;
}

.summary-icon {
  width: 38px;
  height: 38px;
  display: grid;
  place-items: center;
  border-radius: 8px;
  font-size: 19px;
  background: var(--ops-primary-light);
  color: var(--ops-primary);
}

.summary-card.success .summary-icon { background: #f0f9eb; color: #45a52e; }
.summary-card.warning .summary-icon { background: #fdf6ec; color: #e6a23c; }
.summary-card.danger .summary-icon { background: #fef0f0; color: #e64c4c; }

.summary-card span {
  display: block;
  margin-bottom: 3px;
  color: var(--ops-text-secondary);
  font-size: 12px;
}

.summary-card strong {
  color: var(--ops-text);
  font-size: 24px;
  line-height: 1;
}

.project-nav {
  width: 248px;
  flex-shrink: 0;
  border-right: 1px solid var(--ops-border);
  padding: 18px 10px;
  display: flex;
  flex-direction: column;
  overflow-y: auto;
}

.nav-heading {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 8px 14px;
  color: var(--ops-text);
  font-size: 12px;
  font-weight: 600;
}

.nav-heading small {
  color: var(--ops-text-secondary);
  font-weight: 400;
}

.nav-item {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 10px 10px;
  border-radius: 7px;
  font-size: 14px;
  color: var(--ops-text-secondary);
  cursor: pointer;
  transition: background 0.15s, color 0.15s;
}

.nav-item:hover {
  background: var(--ops-bg);
  color: var(--ops-text);
}

.nav-item.level-host {
  font-size: 13px;
}

.nav-item.level-type {
  margin: 2px 0 2px 18px;
  padding-left: 12px;
  font-size: 13px;
}

.nav-item.level-type.active {
  background: linear-gradient(90deg, #e9f4ff, #f4f9ff);
  color: var(--ops-primary);
  font-weight: 600;
}

.nav-caret {
  font-size: 12px;
  color: var(--ops-text-secondary);
  flex-shrink: 0;
}

.nav-name {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.nav-count {
  font-size: 12px;
  color: var(--ops-text-secondary);
  background: var(--ops-bg);
  border-radius: 10px;
  padding: 1px 7px;
}

.nav-item.level-type.active .nav-count {
  background: rgba(22, 119, 255, 0.14);
  color: var(--ops-primary);
}

.nav-manage {
  display: none;
  align-items: center;
  gap: 6px;
}

.nav-item:hover .nav-manage {
  display: flex;
}

.nav-manage .el-icon {
  font-size: 13px;
  color: var(--ops-text-secondary);
}

.nav-manage .el-icon:hover {
  color: var(--ops-primary);
}

.nav-children {
  display: flex;
  flex-direction: column;
}

.nav-subgroup {
  display: flex;
  flex-direction: column;
}

.nav-empty {
  padding: 6px 0 6px 46px;
  text-align: left;
}

.nav-empty :deep(.el-empty__description) {
  margin-top: 0;
  font-size: 12px;
}

.service-panel {
  flex: 1;
  min-width: 0;
  padding: 6px 20px 12px;
  background: #fff;
  display: flex;
  flex-direction: column;
}

.service-panel-header {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 10px;
  min-height: 104px;
  padding: 22px 24px;
  border-bottom: 1px solid var(--ops-border);
  background:
    radial-gradient(circle at 78% -50%, rgba(22, 119, 255, .16), transparent 48%),
    linear-gradient(135deg, #fff 0%, #f6faff 100%);
}

.service-identity { min-width: 0; }

.service-kicker {
  margin-bottom: 5px;
  color: var(--ops-primary);
  font-size: 11px;
  font-weight: 700;
  letter-spacing: .08em;
}

.service-title-line {
  display: flex;
  align-items: center;
  gap: 8px;
}

.health-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: #c0c4cc;
}

.health-dot.healthy {
  background: #55b938;
  box-shadow: 0 0 0 3px rgba(85, 185, 56, 0.13);
}

.service-context {
  margin-top: 5px;
  color: var(--ops-text-secondary);
  font-size: 12px;
}

.service-panel-title {
  font-size: 22px;
  font-weight: 700;
  color: var(--ops-text);
}

.service-panel-actions {
  margin-left: auto;
  display: flex;
  gap: 6px;
}

.service-toolbar {
  display: flex;
  align-items: center;
  gap: 10px;
  margin: 0;
  padding: 12px 0;
  border-bottom: 1px solid var(--ops-border);
  background: #fff;
}

.service-search { width: min(360px, 48%); }
.status-filter { width: 136px; }
.result-count { margin-left: auto; color: var(--ops-text-secondary); font-size: 12px; }

.service-panel-placeholder {
  margin: auto;
}

.service-list {
  display: flex;
  flex-direction: column;
  gap: 0;
  padding: 0;
}

.service-list > :deep(.el-empty) { grid-column: 1 / -1; }

.service-item {
  display: flex;
  align-items: center;
  flex-wrap: nowrap;
  gap: 16px;
  min-width: 0;
  min-height: 0;
  padding: 18px 4px;
  margin-bottom: 0;
  border: 0;
  border-bottom: 1px solid var(--ops-border);
  border-radius: 0;
  background: #fff;
  transition: background .18s;
}

.service-item:hover {
  border-color: var(--ops-border);
  box-shadow: none;
  transform: none;
  background: #fafcff;
}

.service-item:last-child { border-bottom: 0; }

.service-item-icon {
  flex-shrink: 0;
  width: 42px;
  height: 42px;
  border-radius: 10px;
  background: var(--ops-primary-light);
  color: var(--ops-primary);
  display: flex;
  align-items: center;
  justify-content: center;
  margin-top: 0;
}

.service-item-icon.status-running { background: #f0f9eb; color: #55a532; }
.service-item-icon.status-error { background: #fef0f0; color: #f56c6c; }
.service-item-icon.status-stopped { background: #fdf6ec; color: #e6a23c; }

.service-item-main {
  flex: 1;
  min-width: 0;
  display: flex;
  align-items: center;
  gap: 20px;
  flex-wrap: wrap;
}

.service-item-title-row {
  display: flex;
  align-items: center;
  flex: 0 0 175px;
  min-width: 0;
  gap: 10px;
  margin-bottom: 0;
}

.service-item-name {
  font-size: 16px;
  font-weight: 700;
  color: var(--ops-text);
}

.service-meta-grid {
  display: grid;
  flex: 1;
  min-width: 420px;
  grid-template-columns: minmax(170px, 1.6fr) minmax(80px, .7fr) minmax(90px, .8fr) minmax(105px, .9fr);
  gap: 8px 16px;
}

.meta-block {
  min-width: 0;
}

.meta-block small {
  display: block;
  margin-bottom: 3px;
  color: #a0a8b3;
  font-size: 10px;
  line-height: 1;
}

.meta-block > span {
  display: block;
  overflow: hidden;
  color: #526071;
  font-size: 12px;
  line-height: 18px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.service-item-image {
  font-family: "SFMono-Regular", Consolas, monospace;
  color: #52657a;
}

.service-item-note {
  flex-basis: 100%;
  margin: -8px 0 0 195px;
  font-size: 12px;
  color: var(--ops-text-secondary);
}

.service-item-error {
  flex-basis: 100%;
  margin: -8px 0 0 195px;
  font-size: 12px;
  color: var(--el-color-danger);
}

.service-item-actions {
  width: auto;
  flex-shrink: 0;
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  margin-top: 0;
  padding-top: 2px;
  border-top: 0;
  align-items: center;
}

.service-item-actions :deep(.el-button + .el-button) { margin-left: 0; }

:global(.danger-menu-item) { color: var(--el-color-danger); }

@media (max-width: 1100px) {
  .summary-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .service-item { flex-wrap: wrap; }
  .service-item-actions { width: 100%; padding: 10px 0 0 58px; }
  .service-meta-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
}

@media (max-width: 900px) {
  .mobile-project-filter { margin-bottom: 12px; }
  .master-detail :deep(.el-card__body) { min-height: 0; }
  .master-detail-body { display: block; }
  .browser-tabs { padding: 8px 10px 0; }
  .browser-tab-name { max-width: 90px; }
  .filter-sidebar { width: 100%; max-height: none; padding: 14px; border-right: 0; border-bottom: 1px solid var(--ops-border); }
  .filter-heading { margin-bottom: 10px; padding-bottom: 10px; }
  .service-tree { max-height: 230px; overflow-y: auto; }
.tree-type-actions { margin-left: 5px; }
  .scope-selector { align-items: stretch; flex-direction: column; gap: 9px; padding: 12px; }
  .scope-title { align-self: flex-start; }
  .scope-field, .scope-field.type-field { width: 100%; }
  .scope-field label { margin-bottom: 4px; }
  .scope-separator { display: none; }
  .scope-actions { width: 100%; margin-left: 0; padding-top: 4px; border-top: 1px solid #eef1f5; }
  .scope-actions .el-button { flex: 1; }
  .scope-result { align-self: flex-start; }
  .project-tabs { flex-wrap: nowrap; margin: 0 -12px 14px; padding: 0 12px 4px; overflow-x: auto; scrollbar-width: none; }
  .project-tabs::-webkit-scrollbar { display: none; }
  .project-tab { flex: 0 0 auto; }
  .summary-grid { grid-template-columns: 1fr 1fr; gap: 8px; }
  .summary-card { min-height: 68px; padding: 10px; }
  .summary-icon { width: 34px; height: 34px; }
  .summary-card strong { font-size: 20px; }
  .deployment-explorer { padding: 16px 14px; }
  .explorer-heading { align-items: flex-start; }
  .explorer-heading > span { max-width: 130px; text-align: right; }
  .host-selector { display: flex; margin: 0 -14px; padding: 0 14px 3px; overflow-x: auto; scrollbar-width: none; }
  .host-selector::-webkit-scrollbar, .type-selector::-webkit-scrollbar { display: none; }
  .host-option { min-width: 220px; }
  .type-selector { margin-right: -14px; padding-right: 14px; }
  .service-panel-header { align-items: flex-start; min-height: 0; padding: 18px 16px; }
  .service-panel-title { font-size: 19px; }
  .service-panel-actions { width: 100%; margin-left: 0; }
  .service-panel-actions .el-button { flex: 1; margin-left: 0; }
  .service-panel { padding: 4px 14px 10px; }
  .service-toolbar { flex-wrap: wrap; margin: 0; padding: 10px 0; }
  .service-search { width: 100%; }
  .status-filter { flex: 1; width: auto; }
  .result-count { margin-left: 0; }
  .service-list { display: flex; padding: 0; }
  .service-item { display: grid; grid-template-columns: 40px minmax(0, 1fr); min-height: 0; gap: 12px; padding: 14px; }
  .service-item-icon { width: 40px; height: 40px; }
  .service-item-main { display: block; }
  .service-item-title-row { flex-wrap: wrap; gap: 6px; }
  .service-meta-grid { width: 100%; min-width: 0; margin-top: 12px; grid-template-columns: 1fr 1fr; gap: 10px 12px; }
  .service-item-error, .service-item-note { margin: 10px 0 0; }
  .service-item-actions { grid-column: 1 / -1; width: 100%; justify-content: center; flex-wrap: wrap; gap: 10px; padding: 12px 0 0; border-top: 1px solid #eef1f5; }
  .service-item-actions .el-button.is-circle { flex: 0 0 auto; }
  :deep(.el-dialog) { width: calc(100vw - 24px) !important; margin-top: 3vh !important; }
  .detail-body { max-height: 68vh; }
}

@media (max-width: 480px) {
  .summary-grid { grid-template-columns: 1fr; }
  .summary-card { min-height: 58px; }
  .summary-card > div:last-child { display: flex; flex: 1; align-items: center; justify-content: space-between; }
  .summary-card span { margin: 0; }
  .service-meta-grid { grid-template-columns: 1fr; }
  .service-item-actions { justify-content: center; }
  .service-item-actions .el-button.is-circle { flex: 0 0 auto; }
}

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

.detail-body {
  min-height: 120px;
  max-height: 75vh;
  overflow-y: auto;
  padding-right: 4px;
}

.detail-section {
  margin-top: 16px;
}

.detail-section-title {
  font-size: 13px;
  font-weight: 600;
  color: var(--ops-text);
  margin-bottom: 6px;
}

.detail-code {
  font-family: "SFMono-Regular", Consolas, Monaco, monospace;
  font-size: 12px;
  background: #f6f8fa;
  border-radius: 4px;
  padding: 8px 10px;
  word-break: break-all;
}

.detail-list {
  display: flex;
  flex-direction: column;
  gap: 4px;
  background: #f6f8fa;
  border-radius: 4px;
  padding: 8px 10px;
}

.detail-list-row {
  font-family: "SFMono-Regular", Consolas, Monaco, monospace;
  font-size: 12px;
  color: var(--ops-text);
  word-break: break-all;
}

.detail-empty {
  font-size: 12px;
  color: var(--ops-text-secondary);
}
</style>
