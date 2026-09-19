<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus, Edit, Delete, FolderOpened, Key } from '@element-plus/icons-vue'
import { useAuthStore } from '../stores/auth'
import { projectApi, type Project, type ProjectForm } from '../api/projects'
import { serviceApi } from '../api/services'
import AppLayout from '../components/AppLayout.vue'

const auth = useAuthStore()
const projects = ref<Project[]>([])
const serviceCountByProject = ref<Record<string, number>>({})
const loading = ref(false)
const submitting = ref(false)
const dialogVisible = ref(false)
const editingId = ref('')

const canManage = computed(() => auth.user?.role === 'admin')
const form = reactive<ProjectForm>({ name: '', note: '', env_vars: '' })

async function loadAll() {
  loading.value = true
  try {
    const [availableProjects, serviceTypes] = await Promise.all([projectApi.list(), serviceApi.list()])
    projects.value = availableProjects
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
  Object.assign(form, { name: '', note: '', env_vars: '' })
  dialogVisible.value = true
}

function openEdit(project: Project) {
  editingId.value = project.id
  Object.assign(form, { name: project.name, note: project.note, env_vars: project.env_vars || '' })
  dialogVisible.value = true
}

async function save() {
  if (!form.name.trim()) {
    ElMessage.warning('请输入项目名称')
    return
  }
  submitting.value = true
  try {
    const payload = { name: form.name.trim(), note: form.note.trim(), env_vars: form.env_vars }
    if (editingId.value) await projectApi.update(editingId.value, payload)
    else await projectApi.create(payload)
    ElMessage.success(editingId.value ? '项目已更新' : '项目已创建')
    dialogVisible.value = false
    await loadAll()
  } catch (error: any) {
    ElMessage.error(error?.response?.data?.error || '保存失败')
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
    <div class="page-heading page-heading-modern">
      <div><h2>项目管理</h2><p>按业务场景组织服务，并维护可复用的部署环境模板。</p></div>
      <el-button v-if="canManage" type="primary" :icon="Plus" @click="openCreate">新建项目</el-button>
    </div>

    <el-card class="plain-card" shadow="never" v-loading="loading">
      <div class="project-list">
        <div v-for="row in projects" :key="row.id" class="project-item">
          <div class="project-item-icon">
            <el-icon :size="16"><FolderOpened /></el-icon>
          </div>
          <div class="project-item-main">
            <div class="project-item-title-row">
              <span class="project-item-name">{{ row.name }}</span>
              <el-tag size="small" type="info" effect="plain" round>{{ serviceCountByProject[row.id] || 0 }} 个服务类型</el-tag>
              <el-tag v-if="row.env_vars" size="small" type="success" effect="plain" round>
                <el-icon style="vertical-align: -2px; margin-right: 2px"><Key /></el-icon>已配置环境变量
              </el-tag>
            </div>
            <div v-if="row.note" class="project-item-note">{{ row.note }}</div>
          </div>
          <div v-if="canManage" class="project-item-actions">
            <el-button plain :icon="Edit" @click="openEdit(row)">编辑</el-button>
            <el-button plain type="danger" :icon="Delete" @click="remove(row)">删除</el-button>
          </div>
        </div>
        <el-empty v-if="!loading && !projects.length" description="暂无项目，请先新建项目" :image-size="60" />
      </div>
    </el-card>

    <el-dialog v-model="dialogVisible" :title="editingId ? '编辑项目' : '新建项目'" width="760px" destroy-on-close>
      <el-form label-width="90px">
        <el-form-item label="项目名称" required>
          <el-input v-model="form.name" placeholder="例如 主线游戏服、活动服" />
        </el-form-item>
        <el-form-item label="备注">
          <el-input v-model="form.note" type="textarea" :rows="3" placeholder="可选，项目说明" />
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
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
  gap: 14px;
}

.project-item {
  display: flex;
  align-items: flex-start;
  gap: 14px;
  min-height: 150px;
  padding: 20px;
  border: 1px solid var(--ops-border);
  border-radius: 12px;
  background: #fff;
  box-shadow: var(--ops-shadow);
  transition: transform .18s, box-shadow .18s;
}
.project-item:hover { transform: translateY(-2px); box-shadow: 0 10px 28px rgba(31,45,61,.08); }

.project-item-icon {
  flex-shrink: 0;
  width: 42px;
  height: 42px;
  border-radius: 10px;
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
  font-size: 16px;
  font-weight: 700;
  color: var(--ops-text);
}

.project-item-note {
  margin-top: 10px;
  font-size: 13px;
  line-height: 1.6;
  color: var(--ops-text-secondary);
}

.project-item-actions {
  width: 100%;
  align-self: flex-end;
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}

.project-item { flex-wrap: wrap; }
.project-item-main { min-height: 70px; }
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

@media (max-width: 760px) {
  .page-heading-modern { flex-direction: column; }
  .page-heading-modern .el-button { width: 100%; }
  .project-list { grid-template-columns: 1fr; }
  .project-item { min-height: 0; padding: 16px; }
  :deep(.el-dialog) { width: calc(100vw - 24px) !important; margin-top: 3vh !important; }
}
</style>
