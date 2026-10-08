<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus, Edit, Delete, FolderOpened, Key, Close, View, Monitor } from '@element-plus/icons-vue'
import { useAuthStore } from '../stores/auth'
import { projectApi, type Project, type ProjectForm } from '../api/projects'
import { serviceApi } from '../api/services'
import { hostApi, type Host } from '../api/hosts'
import AppLayout from '../components/AppLayout.vue'

const auth = useAuthStore()
const router = useRouter()
const projects = ref<Project[]>([])
const hosts = ref<Host[]>([])
const serviceCountByProject = ref<Record<string, number>>({})
const loading = ref(false)
const submitting = ref(false)
const dialogVisible = ref(false)
const detailVisible = ref(false)
const detailEditing = ref(false)
const activeProject = ref<Project | null>(null)
const editingId = ref('')
const selectedHostIDs = ref<string[]>([])

const canManage = computed(() => auth.user?.role === 'admin')
const form = reactive<ProjectForm>({ name: '', note: '', env_vars: '' })

async function loadAll() {
  loading.value = true
  try {
    const [availableProjects, serviceTypes, availableHosts] = await Promise.all([projectApi.list(), serviceApi.list(), hostApi.list()])
    projects.value = availableProjects
    hosts.value = availableHosts
    const counts: Record<string, number> = {}
    for (const type of serviceTypes) {
      counts[type.project_id] = (counts[type.project_id] || 0) + 1
    }
    serviceCountByProject.value = counts
  } catch {
    ElMessage.error('项目列表加载失败')
  } finally {
    loading.value = false
  }
}

function openCreate() {
  editingId.value = ''
  selectedHostIDs.value = []
  Object.assign(form, { name: '', note: '', env_vars: '' })
  dialogVisible.value = true
}

function beginProjectEdit(project: Project) {
  editingId.value = project.id
  selectedHostIDs.value = projectHosts(project.id).map((host) => host.id)
  Object.assign(form, { name: project.name, note: project.note, env_vars: project.env_vars || '' })
  detailEditing.value = true
}

function openProjectDetails(project: Project) {
  activeProject.value = project
  detailEditing.value = false
  detailVisible.value = true
}

const assignableHosts = computed(() => hosts.value)

function hostHasProject(host: Host, projectId: string): boolean {
  return host.project_id === projectId || host.project_ids?.includes(projectId) === true
}

function hostProjectNames(host: Host): string {
  const ids = [...new Set([...(host.project_ids || []), host.project_id].filter(Boolean))]
  return ids.map((id) => projects.value.find((project) => project.id === id)?.name || '其他项目').join('、')
}

function projectHosts(projectId: string): Host[] {
  return hosts.value.filter((host) => hostHasProject(host, projectId))
}

async function save() {
  if (!form.name.trim()) {
    ElMessage.warning('请输入项目名称')
    return
  }
  submitting.value = true
  let projectSaved = false
  try {
    const payload = { name: form.name.trim(), note: form.note.trim(), env_vars: form.env_vars }
    if (editingId.value) {
      await projectApi.update(editingId.value, payload)
      projectSaved = true
      await projectApi.updateHosts(editingId.value, selectedHostIDs.value)
    } else {
      await projectApi.create(payload)
    }
    ElMessage.success(editingId.value ? '项目已更新' : '项目已创建')
    dialogVisible.value = false
    await loadAll()
  } catch (error: any) {
    const reason = error?.response?.data?.error || '保存失败'
    ElMessage.error(projectSaved ? `项目信息已保存，但主机绑定失败：${reason}` : reason)
    if (projectSaved) {
      await loadAll()
      selectedHostIDs.value = projectHosts(editingId.value).map((host) => host.id)
    }
  } finally {
    submitting.value = false
  }
}

async function saveProjectDetail() {
  if (!activeProject.value || !form.name.trim()) {
    ElMessage.warning('请输入项目名称')
    return
  }
  submitting.value = true
  let projectSaved = false
  try {
    await projectApi.update(activeProject.value.id, { name: form.name.trim(), note: form.note.trim(), env_vars: form.env_vars })
    projectSaved = true
    await projectApi.updateHosts(activeProject.value.id, selectedHostIDs.value)
    ElMessage.success('项目已更新')
    detailEditing.value = false
    editingId.value = ''
    await loadAll()
    activeProject.value = projects.value.find((project) => project.id === activeProject.value?.id) || activeProject.value
  } catch (error: any) {
    const reason = error?.response?.data?.error || '保存失败'
    ElMessage.error(projectSaved ? `项目信息已保存，但主机绑定失败：${reason}` : reason)
    if (projectSaved) {
      await loadAll()
      activeProject.value = projects.value.find((project) => project.id === activeProject.value?.id) || activeProject.value
      Object.assign(form, { name: activeProject.value.name, note: activeProject.value.note, env_vars: activeProject.value.env_vars || '' })
      selectedHostIDs.value = projectHosts(activeProject.value.id).map((host) => host.id)
    }
  } finally {
    submitting.value = false
  }
}

async function remove(project: Project) {
  const relatedCount = serviceCountByProject.value[project.id] || 0
  try {
    await ElMessageBox.confirm(
      relatedCount > 0
        ? `项目"${project.name}"下还有 ${relatedCount} 个服务类型，删除项目不会自动删除它们，但会失去项目分类。是否继续？`
        : `确定删除项目"${project.name}"吗？`,
      '删除确认',
      { type: 'warning' },
    )
    await projectApi.remove(project.id)
    ElMessage.success('项目已删除')
    await loadAll()
  } catch (error: any) {
    if (error !== 'cancel' && error !== 'close') ElMessage.error(error?.response?.data?.error || '删除失败')
  }
}

onMounted(loadAll)
</script>

<template>
  <AppLayout>
    <div class="page-content-loading" v-loading="loading">
    <div class="page-heading page-heading-modern page-heading-actions-only">
      <el-button v-if="canManage" type="primary" :icon="Plus" @click="openCreate">新建项目</el-button>
    </div>

    <el-card class="plain-card" shadow="never">
      <div class="project-list">
        <div v-for="row in projects" :key="row.id" class="project-item">
          <div class="project-item-icon">
            <el-icon :size="16"><FolderOpened /></el-icon>
          </div>
          <div class="project-item-main">
            <div class="project-item-title-row">
              <button class="project-item-name project-entry" @click="openProjectDetails(row)">{{ row.name }}</button>
              <el-tag v-if="row.env_vars" size="small" type="success" effect="plain" round>
                <el-icon style="vertical-align: -2px; margin-right: 2px"><Key /></el-icon>已配置环境变量
              </el-tag>
              <el-tag size="small" :type="projectHosts(row.id).length ? 'success' : 'warning'" effect="plain" round>
                {{ projectHosts(row.id).length ? `${projectHosts(row.id).length} 台部署主机` : '未绑定部署主机' }}
              </el-tag>
            </div>
          </div>
          <div class="project-item-actions">
            <el-button link :icon="View" @click="openProjectDetails(row)">详情</el-button>
            <el-button link type="primary" @click="router.push({ path: '/services', query: { project: row.id } })">查看服务</el-button>
            <el-button v-if="canManage" link type="danger" :icon="Delete" @click="remove(row)">删除</el-button>
          </div>
        </div>
        <el-empty v-if="!loading && !projects.length" description="暂无项目，请先新建项目" :image-size="60" />
      </div>
    </el-card>
    </div>

    <el-dialog v-model="dialogVisible" title="新建项目" width="760px" destroy-on-close>
      <el-form label-width="90px">
        <el-form-item label="项目名称" required>
          <el-input v-model="form.name" placeholder="例如 主线游戏服、活动服" />
        </el-form-item>
        <p v-if="!editingId" class="env-vars-hint">项目可以先独立创建，之后编辑时再绑定部署主机；未绑定主机的项目无法创建或部署服务实例。</p>
        <el-form-item label="备注">
          <el-input v-model="form.note" type="textarea" :rows="3" placeholder="可选，项目说明" />
        </el-form-item>
        <el-form-item v-if="editingId" label="部署主机">
          <el-select v-model="selectedHostIDs" multiple collapse-tags collapse-tags-tooltip placeholder="可选，选择此项目可部署实例的主机" style="width: 100%">
            <el-option v-for="host in assignableHosts" :key="host.id" :label="host.name" :value="host.id">
              <span>{{ host.name }}</span>
              <span class="host-option-detail">{{ host.docker_host }}{{ hostProjectNames(host) ? ` · 已绑定：${hostProjectNames(host)}` : '' }}</span>
            </el-option>
          </el-select>
          <p class="env-vars-hint">同一主机可以绑定到多个项目。移除某项目的主机绑定前，该项目下仍部署在此主机上的服务实例需要先迁移或删除。</p>
        </el-form-item>
        <el-form-item label="环境变量">
          <el-input
            v-model="form.env_vars"
            type="textarea"
            :rows="14"
            class="env-vars-editor"
            placeholder="直接粘贴部署参数/环境变量文本，例如：
--etcd etcd:2379 \
--nats-url nats://nats:4222 \
--mongo-url mongodb://172.17.0.1:27018 \
--mongo-username admin \
--mongo-password ****** \
--platform-grpc-addr 127.0.0.1:7000"
          />
          <p class="env-vars-hint">
            这里粘贴的文本会作为该项目的默认环境变量模板，创建服务实例时自动预填，可在实例上自由修改，不影响项目模板本身。
          </p>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="submitting" @click="save">保存</el-button>
      </template>
    </el-dialog>

    <el-drawer v-model="detailVisible" direction="rtl" size="520px" :with-header="false">
      <div v-if="activeProject" class="project-detail-panel">
        <div class="project-detail-header">
          <div class="project-detail-header-main"><span class="project-detail-icon"><el-icon><FolderOpened /></el-icon></span><div class="project-detail-header-info"><div class="project-detail-title">项目详情</div><div class="project-detail-name">{{ activeProject.name }}</div></div><el-button class="project-detail-close" circle :icon="Close" aria-label="关闭项目详情" @click="detailVisible = false" /></div>
          <div class="project-detail-badges"><el-tag type="info" effect="plain" round>{{ serviceCountByProject[activeProject.id] || 0 }} 个服务类型</el-tag><el-tag :type="projectHosts(activeProject.id).length ? 'success' : 'warning'" effect="plain" round>{{ projectHosts(activeProject.id).length }} 台部署主机</el-tag></div>
        </div>
        <div v-if="!detailEditing" class="project-detail-scroll">
          <div class="project-detail-section-title">项目信息</div>
          <div class="project-detail-kv"><div><span>项目名称</span><b>{{ activeProject.name }}</b></div><div><span>服务类型</span><b>{{ serviceCountByProject[activeProject.id] || 0 }} 个</b></div><div><span>创建时间</span><b>{{ activeProject.created_at || '—' }}</b></div><div><span>更新时间</span><b>{{ activeProject.updated_at || '—' }}</b></div></div>
          <template v-if="activeProject.note"><div class="project-detail-section-title project-detail-section-gap">项目备注</div><div class="project-detail-note">{{ activeProject.note }}</div></template>
          <div class="project-detail-section-title project-detail-section-gap">部署主机</div>
          <div class="project-detail-host-list"><div v-for="host in projectHosts(activeProject.id)" :key="host.id" class="project-detail-host"><span class="project-host-icon"><el-icon><Monitor /></el-icon></span><span><b>{{ host.name }}</b><small>{{ host.docker_host }}</small></span><el-tag size="small" type="success" effect="light">已绑定</el-tag></div><div v-if="!projectHosts(activeProject.id).length" class="project-detail-empty">尚未绑定部署主机</div></div>
          <template v-if="activeProject.env_vars"><div class="project-detail-section-title project-detail-section-gap">环境变量模板</div><pre class="project-detail-code">{{ activeProject.env_vars }}</pre></template>
        </div>
        <el-form v-else class="project-detail-scroll project-inline-form" label-position="top">
          <div class="project-detail-section-title">项目信息</div>
          <el-form-item label="项目名称" required><el-input v-model="form.name" placeholder="输入项目名称" /></el-form-item>
          <el-form-item label="项目备注"><el-input v-model="form.note" type="textarea" :rows="3" placeholder="可选，项目说明" /></el-form-item>
          <div class="project-detail-section-title project-detail-section-gap">部署主机</div>
          <el-form-item label="绑定的主机"><el-select v-model="selectedHostIDs" multiple filterable collapse-tags collapse-tags-tooltip placeholder="选择此项目可部署实例的主机" style="width: 100%"><el-option v-for="host in assignableHosts" :key="host.id" :label="host.name" :value="host.id"><span>{{ host.name }}</span><span class="host-option-detail">{{ host.docker_host }}</span></el-option></el-select><p class="env-vars-hint">移除主机绑定前，请先迁移或删除仍部署在该主机上的服务实例。</p></el-form-item>
          <div class="project-detail-section-title project-detail-section-gap">环境变量模板</div>
          <el-form-item label="默认环境变量"><el-input v-model="form.env_vars" type="textarea" :rows="12" class="env-vars-editor" placeholder="粘贴项目默认环境变量模板" /><p class="env-vars-hint">创建服务实例时会自动预填，实例中可单独修改，不影响此模板。</p></el-form-item>
        </el-form>
        <div v-if="canManage" class="project-detail-footer"><template v-if="detailEditing"><el-button @click="detailEditing = false; editingId = ''">取消</el-button><el-button type="primary" :loading="submitting" @click="saveProjectDetail">保存</el-button></template><template v-else><el-button :icon="Edit" @click="beginProjectEdit(activeProject)">编辑</el-button><el-button type="primary" @click="detailVisible = false; router.push({ path: '/services', query: { project: activeProject!.id } })">查看服务</el-button></template></div>
      </div>
    </el-drawer>
  </AppLayout>
</template>

<style scoped>
.plain-card {
  border: 0;
  background: transparent;
}

.plain-card :deep(.el-card__body) {
  padding: 0;
}

.project-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.project-item {
  display: flex;
  align-items: center;
  gap: 14px;
  min-height: 88px;
  padding: 18px 20px;
  border: 1px solid var(--ops-border);
  border-radius: 12px;
  background: #fff;
  box-shadow: 0 2px 8px rgba(28, 47, 78, .035);
  transition: border-color .16s ease, box-shadow .16s ease, transform .16s ease;
}
.project-item:hover { border-color: #d8e2f3; background: #fff; box-shadow: 0 5px 16px rgba(28, 47, 78, .07); transform: translateY(-1px); }
.project-entry { padding: 0; font: inherit; border: 0; background: transparent; cursor: pointer; }
.project-entry:hover { color: var(--ops-primary); }

.project-item-icon {
  flex-shrink: 0;
  width: 38px;
  height: 38px;
  border-radius: 9px;
  background: var(--ops-primary-light);
  color: var(--ops-primary);
  display: flex;
  align-items: center;
  justify-content: center;
}

.project-item-main {
  flex: 1;
  min-width: 0;
}

.project-item-title-row {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
}

.project-item-name {
  font-size: 15px;
  font-weight: 600;
  color: var(--ops-text);
}

.project-item-note {
  margin-top: 10px;
  font-size: 13px;
  line-height: 1.6;
  color: var(--ops-text-secondary);
}

.project-host-names {
  margin-top: 8px;
  font-size: 12px;
  color: var(--ops-text-secondary);
  overflow-wrap: anywhere;
}

.project-item-actions {
  flex-shrink: 0;
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}

.project-item-main { min-height: 0; }
.project-item-actions .el-button { margin-left: 0; }

.env-vars-hint {
  margin: 6px 0 0;
  font-size: 12px;
  color: var(--ops-text-secondary);
  line-height: 1.6;
}

.env-vars-editor :deep(textarea) {
  font-family: "SFMono-Regular", Consolas, Monaco, monospace;
  font-size: 13px;
  line-height: 1.6;
  background: #f6f8fa;
}

.project-detail-panel { display: flex; width: 100%; height: 100%; min-height: 0; flex-direction: column; background: #fafcff; }
:deep(.el-drawer__body) { padding: 0; overflow: hidden; }
.project-detail-header { display: flex; flex-direction: column; gap: 13px; padding: 18px; border-bottom: 1px solid var(--ops-border); background: #fff; }
.project-detail-header-main { display: flex; align-items: center; gap: 12px; min-width: 0; }
.project-detail-icon { display: grid; flex: 0 0 40px; width: 40px; height: 40px; place-items: center; border-radius: 10px; background: var(--ops-primary-light); color: var(--ops-primary); font-size: 19px; }
.project-detail-header-info { flex: 1; min-width: 0; }
.project-detail-title { margin-bottom: 4px; color: var(--ops-text-secondary); font-size: 12px; }
.project-detail-name { overflow: hidden; color: var(--ops-text); font-size: 16px; font-weight: 700; text-overflow: ellipsis; white-space: nowrap; }
.project-detail-close { width: 36px; height: 36px; flex: 0 0 36px; margin-left: auto; border-color: var(--ops-border); color: var(--ops-text-secondary); }
.project-detail-close:hover { border-color: var(--ops-primary); background: var(--ops-primary-light); color: var(--ops-primary); }
.project-detail-badges { display: flex; flex-wrap: wrap; gap: 7px; padding-left: 52px; }
.project-detail-scroll { flex: 1; min-height: 0; overflow-y: auto; padding: 18px; }
.project-inline-form :deep(.el-form-item) { margin-bottom: 16px; }
.project-inline-form :deep(.el-form-item__label) { padding-bottom: 6px; color: #52627a; font-size: 12px; font-weight: 600; }
.project-detail-section-title { margin-bottom: 11px; color: var(--ops-text); font-size: 13px; font-weight: 700; }
.project-detail-section-gap { margin-top: 22px; }
.project-detail-kv { display: flex; flex-direction: column; gap: 12px; padding: 14px; border: 1px solid var(--ops-border); border-radius: 8px; background: #fff; }
.project-detail-kv > div { display: flex; justify-content: space-between; gap: 12px; font-size: 12px; }
.project-detail-kv span { color: var(--ops-text-secondary); }
.project-detail-kv b { color: var(--ops-text); font-weight: 550; }
.project-detail-note { padding: 13px; border: 1px solid var(--ops-border); border-radius: 8px; background: #fff; color: #52627a; font-size: 12px; line-height: 1.65; white-space: pre-wrap; overflow-wrap: anywhere; }
.project-detail-host-list { overflow: hidden; border: 1px solid var(--ops-border); border-radius: 8px; background: #fff; }
.project-detail-host { display: flex; align-items: center; gap: 10px; min-height: 58px; padding: 9px 12px; border-bottom: 1px solid #f0f2f6; }
.project-detail-host:last-child { border-bottom: 0; }
.project-host-icon { display: grid; flex: 0 0 30px; width: 30px; height: 30px; place-items: center; border-radius: 8px; background: #eef3ff; color: var(--ops-primary); }
.project-detail-host > span:nth-child(2) { display: flex; min-width: 0; flex: 1; flex-direction: column; gap: 4px; }
.project-detail-host b { color: #34455e; font-size: 12px; }
.project-detail-host small { overflow: hidden; color: #8491a3; font-size: 10px; text-overflow: ellipsis; white-space: nowrap; }
.project-detail-empty { padding: 14px; color: var(--ops-text-secondary); font-size: 12px; }
.project-detail-code { margin: 0; padding: 12px; overflow: auto; border: 1px solid var(--ops-border); border-radius: 8px; background: #fff; color: #3c4b62; font: 11px/1.6 "SFMono-Regular", Consolas, Monaco, monospace; white-space: pre-wrap; overflow-wrap: anywhere; }
.project-detail-footer { display: flex; justify-content: flex-end; gap: 8px; padding: 13px 18px; border-top: 1px solid var(--ops-border); background: #fff; }

@media (max-width: 760px) {
  .page-heading-modern { flex-direction: column; }
  .project-list { gap: 10px; }
  .project-item { min-height: 0; padding: 14px; flex-wrap: wrap; }
  .project-item-actions { width: 100%; padding-top: 10px; }
  :deep(.el-dialog) { width: calc(100vw - 24px) !important; margin-top: 3vh !important; }
  :deep(.el-drawer) { width: min(520px, calc(100vw - 18px)) !important; }
}
</style>
