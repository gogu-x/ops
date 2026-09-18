<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus, Edit, Delete, Box, FolderOpened, Monitor, CaretRight, CaretBottom, Key, VideoPlay, VideoPause, RefreshRight, Refresh, Upload, Document, InfoFilled } from '@element-plus/icons-vue'
import { useAuthStore } from '../stores/auth'
import { serviceApi, type ServiceType, type ServiceTypeForm } from '../api/services'
import { hostApi, type Host } from '../api/hosts'
import { projectApi, type Project } from '../api/projects'
import { instanceApi, type ServiceInstance, type ServiceInstanceForm, type ContainerDetail } from '../api/instances'
import AppLayout from '../components/AppLayout.vue'

const auth = useAuthStore()
const types = ref<ServiceType[]>([])
const instances = ref<ServiceInstance[]>([])
const hosts = ref<Host[]>([])
const projects = ref<Project[]>([])
const loading = ref(false)
const submitting = ref(false)
const canManage = computed(() => auth.user?.role === 'admin')

// -------- 顶部项目 Tab + 左侧导航选择状态：主机 -> 类型 --------
const activeProjectId = ref<string>('')
const expandedHostId = ref<string>('')
const activeTypeId = ref<string>('')

function hostsInProject(projectId: string): Host[] {
  const hostIds = new Set(types.value.filter((t) => t.project_id === projectId).map((t) => t.host_id))
  return hosts.value.filter((h) => hostIds.has(h.id))
}

function typesInHost(projectId: string, hostId: string): ServiceType[] {
  return types.value.filter((t) => t.project_id === projectId && t.host_id === hostId)
}

function countInProject(projectId: string): number {
  const typeIds = new Set(types.value.filter((t) => t.project_id === projectId).map((t) => t.id))
  return instances.value.filter((i) => typeIds.has(i.service_type_id)).length
}

function countInType(typeId: string): number {
  return instances.value.filter((i) => i.service_type_id === typeId).length
}

function toggleHost(hostId: string) {
  expandedHostId.value = expandedHostId.value === hostId ? '' : hostId
}

function selectType(type: ServiceType) {
  activeTypeId.value = type.id
}

const activeType = computed(() => types.value.find((t) => t.id === activeTypeId.value) || null)
const activeProject = computed(() => (activeType.value ? projects.value.find((p) => p.id === activeType.value!.project_id) : null))
const activeHost = computed(() => (activeType.value ? hosts.value.find((h) => h.id === activeType.value!.host_id) : null))
const currentProject = computed(() => projects.value.find((p) => p.id === activeProjectId.value) || null)

const visibleInstances = computed(() => {
  if (!activeTypeId.value) return []
  return instances.value.filter((i) => i.service_type_id === activeTypeId.value)
})

function hostName(id: string): string {
  return hosts.value.find((h) => h.id === id)?.name || id || '未绑定'
}

async function loadAll() {
  loading.value = true
  try {
    const [serviceTypes, serviceInstances, availableHosts, availableProjects] = await Promise.all([
      serviceApi.list(),
      instanceApi.list(),
      hostApi.list(),
      projectApi.list(),
    ])
    types.value = serviceTypes
    instances.value = serviceInstances
    hosts.value = availableHosts
    projects.value = availableProjects
    if (!activeProjectId.value || !projects.value.some((p) => p.id === activeProjectId.value)) {
      activeProjectId.value = projects.value[0]?.id || ''
    }
    if (activeTypeId.value && !types.value.some((t) => t.id === activeTypeId.value)) {
      activeTypeId.value = ''
    }
  } catch {
    ElMessage.error('数据加载失败')
  } finally {
    loading.value = false
  }
}

// -------- 类型（分类）表单 --------
const typeDialogVisible = ref(false)
const editingTypeId = ref('')
const typeForm = reactive<ServiceTypeForm>({ project_id: '', host_id: '', name: '' })

function openCreateType() {
  editingTypeId.value = ''
  Object.assign(typeForm, {
    project_id: activeProjectId.value || projects.value[0]?.id || '',
    host_id: expandedHostId.value || hosts.value[0]?.id || '',
    name: '',
  })
  typeDialogVisible.value = true
}

function openEditType(type: ServiceType) {
  editingTypeId.value = type.id
  Object.assign(typeForm, {
    project_id: type.project_id,
    host_id: type.host_id,
    name: type.name,
  })
  typeDialogVisible.value = true
}

async function saveType() {
  if (!typeForm.project_id) {
    ElMessage.warning('请先选择所属项目')
    return
  }
  if (!typeForm.host_id) {
    ElMessage.warning('请先选择部署主机')
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
const instanceForm = reactive<ServiceInstanceForm>({ service_type_id: '', name: '', image: '', params: [], env_text: '', network: '', port_mapping: '', note: '' })
const instanceSubmitting = ref(false)

function openCreateInstance() {
  if (!activeType.value) return
  editingInstanceId.value = ''
  Object.assign(instanceForm, {
    service_type_id: activeType.value.id,
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
  expandedHostId.value = ''
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
    <div class="page-heading-actions-only">
      <el-button v-if="canManage" type="primary" :icon="Plus" @click="openCreateType">添加服务类型</el-button>
    </div>

    <div class="project-tabs">
      <div
        v-for="project in projects"
        :key="project.id"
        class="project-tab"
        :class="{ active: activeProjectId === project.id }"
        @click="activeProjectId = project.id"
      >
        <el-icon><FolderOpened /></el-icon>
        <span class="project-tab-name">{{ project.name }}</span>
        <span class="project-tab-count">{{ countInProject(project.id) }}</span>
      </div>
      <el-empty v-if="!projects.length" description="暂无项目，请先前往「项目管理」新建项目" :image-size="40" class="project-tabs-empty" />
    </div>

    <el-card class="plain-card master-detail" shadow="never" v-loading="loading">
      <aside class="project-nav">
        <div v-for="host in hostsInProject(activeProjectId)" :key="host.id" class="nav-subgroup">
          <div
            class="nav-item level-host"
            :class="{ expanded: expandedHostId === host.id }"
            @click="toggleHost(host.id)"
          >
            <el-icon class="nav-caret"><component :is="expandedHostId === host.id ? CaretBottom : CaretRight" /></el-icon>
            <el-icon><Monitor /></el-icon>
            <span class="nav-name">{{ host.name }}</span>
            <span class="nav-count">{{ typesInHost(activeProjectId, host.id).reduce((sum, t) => sum + countInType(t.id), 0) }}</span>
          </div>

          <div v-if="expandedHostId === host.id" class="nav-children">
            <div
              v-for="type in typesInHost(activeProjectId, host.id)"
              :key="type.id"
              class="nav-item level-type"
              :class="{ active: activeTypeId === type.id }"
              @click="selectType(type)"
            >
              <el-icon :size="13"><Box /></el-icon>
              <span class="nav-name">{{ type.name }}</span>
              <span class="nav-count">{{ countInType(type.id) }}</span>
              <span v-if="canManage" class="nav-manage">
                <el-icon @click.stop="openEditType(type)"><Edit /></el-icon>
                <el-icon @click.stop="removeType(type)"><Delete /></el-icon>
              </span>
            </div>
            <el-empty
              v-if="!typesInHost(activeProjectId, host.id).length"
              description="暂无类型"
              :image-size="36"
              class="nav-empty"
            />
          </div>
        </div>
        <el-empty v-if="activeProjectId && !hostsInProject(activeProjectId).length" description="暂无主机" :image-size="36" class="nav-empty" />
        <el-empty v-if="!activeProjectId" description="请先选择项目" :image-size="36" class="nav-empty" />
      </aside>

      <section class="service-panel">
        <template v-if="activeType">
          <div class="service-panel-header">
            <span class="service-panel-title">{{ activeType.name }}</span>
            <el-tag size="small" type="info" effect="plain" round>{{ activeProject?.name }}</el-tag>
            <el-tag size="small" type="info" effect="plain" round>{{ activeHost?.name }}</el-tag>
            <div class="service-panel-actions">
              <el-button v-if="canManage" size="small" type="primary" :icon="Plus" @click="openCreateInstance">部署服务</el-button>
            </div>
          </div>

          <div class="service-list">
            <div v-for="row in visibleInstances" :key="row.id" class="service-item">
              <div class="service-item-icon">
                <el-icon :size="18"><Box /></el-icon>
              </div>
              <div class="service-item-main">
                <div class="service-item-title-row">
                  <span class="service-item-name">{{ row.name }}</span>
                  <el-tag size="small" :type="statusType(row)" effect="light" round>{{ statusLabel(row) }}</el-tag>
                  <el-tag v-if="row.env_text" size="small" type="success" effect="plain" round>
                    <el-icon style="vertical-align: -2px; margin-right: 2px"><Key /></el-icon>已配置环境变量
                  </el-tag>
                </div>
                <div class="service-item-image">{{ row.image }}</div>
                <div v-if="row.status === 'error' && row.deploy_error" class="service-item-error">部署失败：{{ row.deploy_error }}</div>
                <div v-if="row.note" class="service-item-note">{{ row.note }}</div>
              </div>
              <div class="service-item-actions">
                <template v-if="canManage">
                  <el-button v-if="row.status === 'not_deployed' || row.status === 'error'" link type="primary" :icon="Upload" :loading="actionLoadingId === row.id" @click="deployInstance(row)">部署</el-button>
                  <el-button v-if="row.status === 'stopped'" link type="success" :icon="VideoPlay" :loading="actionLoadingId === row.id" @click="startInstance(row)">启动</el-button>
                  <el-button v-if="row.status === 'running' || row.status === 'restarting'" link type="warning" :icon="VideoPause" :loading="actionLoadingId === row.id" @click="stopInstance(row)">停止</el-button>
                  <el-button v-if="row.status === 'running'" link type="primary" :icon="RefreshRight" :loading="actionLoadingId === row.id" @click="restartInstance(row)">重启</el-button>
                  <el-button v-if="row.status !== 'not_deployed'" link type="primary" :icon="Refresh" @click="openUpdateImage(row)">更新镜像</el-button>
                </template>
                <el-button v-if="row.status !== 'not_deployed'" link type="info" :icon="InfoFilled" @click="viewDetail(row)">详情</el-button>
                <el-button v-if="row.status !== 'not_deployed'" link type="info" :icon="Document" @click="viewLogs(row)">日志</el-button>
                <template v-if="canManage">
                  <el-button link type="primary" :icon="Edit" @click="openEditInstance(row)">编辑</el-button>
                  <el-button link type="danger" :icon="Delete" @click="removeInstance(row)">删除</el-button>
                </template>
              </div>
            </div>
            <el-empty v-if="!loading && !visibleInstances.length" description="该类型暂无实例，点击右上角部署服务" :image-size="60" />
          </div>
        </template>

        <el-empty v-else description="请从左侧选择一个服务类型查看实例" :image-size="80" class="service-panel-placeholder" />
      </section>
    </el-card>

    <!-- 服务类型表单 -->
    <el-dialog v-model="typeDialogVisible" :title="editingTypeId ? '编辑服务类型' : '添加服务类型'" width="560px" destroy-on-close>
      <el-form label-width="100px">
        <el-form-item label="所属项目" required>
          <el-select v-model="typeForm.project_id" placeholder="请选择项目" style="width: 100%">
            <el-option v-for="project in projects" :key="project.id" :label="project.name" :value="project.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="部署主机" required>
          <el-select v-model="typeForm.host_id" placeholder="请选择 Docker 主机" style="width: 100%">
            <el-option v-for="host in hosts" :key="host.id" :label="host.name" :value="host.id">
              <span>{{ host.name }}</span>
              <span class="host-option-detail">{{ host.docker_host }}</span>
            </el-option>
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
.page-heading-actions-only {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
  margin-bottom: 12px;
}

.plain-card {
  border-radius: 6px;
  border: 1px solid var(--ops-border);
  box-shadow: none;
}

.plain-card :deep(.el-card__body) {
  padding: 0;
}

.master-detail :deep(.el-card__body) {
  display: flex;
  align-items: stretch;
  min-height: calc(100vh - 220px);
}

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

.project-nav {
  width: 260px;
  flex-shrink: 0;
  border-right: 1px solid var(--ops-border);
  padding: 12px 0 12px 4px;
  display: flex;
  flex-direction: column;
  overflow-y: auto;
}

.nav-item {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 9px 12px;
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
  padding-left: 30px;
  font-size: 13px;
}

.nav-item.level-type.active {
  background: var(--ops-primary-light);
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
  padding: 16px 20px;
  display: flex;
  flex-direction: column;
}

.service-panel-header {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 10px;
  padding-bottom: 14px;
  margin-bottom: 8px;
  border-bottom: 1px solid var(--ops-border);
}

.service-panel-title {
  font-size: 16px;
  font-weight: 600;
  color: var(--ops-text);
}

.service-panel-actions {
  margin-left: auto;
  display: flex;
  gap: 6px;
}

.service-panel-placeholder {
  margin: auto;
}

.service-list {
  display: flex;
  flex-direction: column;
}

.service-item {
  display: flex;
  align-items: flex-start;
  gap: 14px;
  padding: 16px 4px;
  border-bottom: 1px solid var(--ops-border);
}

.service-item:last-child {
  border-bottom: 0;
}

.service-item-icon {
  flex-shrink: 0;
  width: 36px;
  height: 36px;
  border-radius: 8px;
  background: var(--ops-primary-light);
  color: var(--ops-primary);
  display: flex;
  align-items: center;
  justify-content: center;
  margin-top: 2px;
}

.service-item-main {
  flex: 1;
  min-width: 0;
}

.service-item-title-row {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 6px;
}

.service-item-name {
  font-size: 15px;
  font-weight: 600;
  color: var(--ops-text);
}

.service-item-image {
  font-family: "SFMono-Regular", Consolas, monospace;
  font-size: 12px;
  color: var(--ops-text-secondary);
  margin-bottom: 6px;
}

.service-item-note {
  margin-top: 8px;
  font-size: 12px;
  color: var(--ops-text-secondary);
}

.service-item-error {
  margin-top: 8px;
  font-size: 12px;
  color: var(--el-color-danger);
}

.service-item-actions {
  flex-shrink: 0;
  display: flex;
  gap: 4px;
  padding-top: 2px;
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
