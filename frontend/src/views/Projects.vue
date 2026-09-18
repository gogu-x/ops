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
    <div class="page-heading-actions-only">
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
            <el-button link type="primary" :icon="Edit" @click="openEdit(row)">编辑</el-button>
            <el-button link type="danger" :icon="Delete" @click="remove(row)">删除</el-button>
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
.page-heading-actions-only {
  display: flex;
  justify-content: flex-end;
  margin-bottom: 12px;
}

.plain-card {
  border-radius: 6px;
  border: 1px solid var(--ops-border);
  box-shadow: none;
}

.plain-card :deep(.el-card__body) {
  padding: 4px 16px;
}

.project-list {
  display: flex;
  flex-direction: column;
}

.project-item {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px 4px;
  border-bottom: 1px solid var(--ops-border);
}

.project-item:last-child {
  border-bottom: 0;
}

.project-item-icon {
  flex-shrink: 0;
  width: 30px;
  height: 30px;
  border-radius: 6px;
  background: var(--ops-bg);
  color: var(--ops-text-secondary);
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
  gap: 10px;
}

.project-item-name {
  font-size: 14px;
  font-weight: 600;
  color: var(--ops-text);
}

.project-item-note {
  margin-top: 2px;
  font-size: 12px;
  color: var(--ops-text-secondary);
}

.project-item-actions {
  flex-shrink: 0;
  display: flex;
  gap: 4px;
}

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
</style>
