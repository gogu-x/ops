<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Delete, Plus, Refresh } from '@element-plus/icons-vue'
import AppLayout from '../components/AppLayout.vue'
import { useAuthStore } from '../stores/auth'
import { serviceApi, type ServiceParameterField, type ServiceType, type ServiceTypeForm } from '../api/services'

const auth = useAuthStore()
const canManage = computed(() => auth.hasPermission('service_types.manage'))
const types = ref<ServiceType[]>([])
const keyword = ref('')
const loading = ref(false)
const submitting = ref(false)
const loadError = ref('')

const filteredTypes = computed(() => {
  const query = keyword.value.trim().toLowerCase()
  return types.value.filter(item => item.name.toLowerCase().includes(query))
})

async function loadAll() {
  loading.value = true
  loadError.value = ''
  try {
    types.value = await serviceApi.list()
  } catch (error: any) {
    loadError.value = error?.response?.data?.error || '读取服务类型失败'
  } finally {
    loading.value = false
  }
}

const typeDialogVisible = ref(false)
const editingTypeId = ref('')
const typeForm = reactive<ServiceTypeForm>({ name: '', readiness_probe: { type: '', port: 0, path: '/health' } })

interface ParameterFieldDraft extends Omit<ServiceParameterField, 'options'> {
  key: number
  optionsText: string
}

const parameterFields = ref<ParameterFieldDraft[]>([])
const expandedFieldKey = ref<number | null>(null)
let nextFieldKey = 0

function addParameterField() {
  const key = nextFieldKey++
  parameterFields.value.push({
    key,
    label: '',
    flag: '',
    kind: 'text',
    required: false,
    default_value: '',
    optionsText: '',
  })
  expandedFieldKey.value = key
}

function removeParameterField(index: number) {
  parameterFields.value.splice(index, 1)
  expandedFieldKey.value = null
}

function openCreateType() {
  editingTypeId.value = ''
  parameterFields.value = []
  expandedFieldKey.value = null
  Object.assign(typeForm, {
    name: '',
    readiness_probe: { type: '', port: 0, path: '/health' },
  })
  typeDialogVisible.value = true
}

function openType(type: ServiceType) {
  editingTypeId.value = type.id
  expandedFieldKey.value = null
  parameterFields.value = (type.parameter_fields || []).map(({ options, ...field }) => ({
    ...field,
    key: nextFieldKey++,
    optionsText: (options || []).join('\n'),
  }))
  Object.assign(typeForm, { name: type.name, readiness_probe: { ...(type.readiness_probe || { type: '', port: 0, path: '/health' }) } })
  typeDialogVisible.value = true
}

async function saveType() {
  if (!canManage.value) return
  if (!typeForm.name.trim()) {
    ElMessage.warning('请填写类型名称')
    return
  }
  submitting.value = true
  try {
    const payload: ServiceTypeForm = {
      ...typeForm,
      name: typeForm.name.trim(),
      parameter_fields: parameterFields.value.map(field => ({
        label: field.label.trim(),
        flag: field.flag.trim(),
        kind: field.kind,
        required: field.required,
        default_value: field.kind === 'text' ? field.default_value : field.default_value.trim(),
        ...(field.kind === 'select' ? { options: field.optionsText.split('\n').map(option => option.trim()).filter(Boolean) } : {}),
      })),
    }
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
  try {
    await ElMessageBox.confirm(
      `确定删除「${type.name}」吗？仍有实例使用的类型不能删除。`,
      '删除确认',
      { type: 'warning' },
    )
    await serviceApi.remove(type.id)
    ElMessage.success('服务类型已删除')
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
      <header class="types-page-header">
        <div><h1>服务类型</h1><p>管理所有项目共用的服务类型、启动参数和健康检查</p></div>
        <div class="types-actions">
          <el-button :icon="Refresh" :loading="loading" @click="loadAll">刷新</el-button>
          <el-button v-if="canManage" :icon="Plus" type="primary" @click="openCreateType">新建服务类型</el-button>
        </div>
      </header>
      <el-alert v-if="loadError" :title="loadError" type="error" show-icon :closable="false" />
      <section class="types-list">
        <div class="types-filters">
          <el-input v-model="keyword" clearable placeholder="搜索类型名称" aria-label="搜索类型名称" />
        </div>
        <el-table :data="filteredTypes" row-key="id" empty-text="没有符合条件的服务类型">
          <el-table-column prop="name" label="类型名称" min-width="150" />
          <el-table-column label="启动参数表单" min-width="135"><template #default="{ row }">{{ row.parameter_fields?.length ? `${row.parameter_fields.length} 个字段` : '未配置' }}</template></el-table-column>
          <el-table-column label="操作" width="128" fixed="right">
            <template #default="{ row }">
              <el-button link type="primary" @click="openType(row)">{{ canManage ? '编辑' : '查看' }}</el-button>
              <el-button v-if="canManage" link type="danger" @click="removeType(row)">删除</el-button>
            </template>
          </el-table-column>
        </el-table>
      </section>
    </div>
    <!-- 服务类型表单 -->
    <el-dialog v-model="typeDialogVisible" :title="canManage ? (editingTypeId ? '编辑服务类型' : '添加服务类型') : '服务类型详情'" width="min(680px, 94vw)" destroy-on-close>
      <el-form label-width="100px" :disabled="!canManage">
        <el-form-item label="类型名称" required>
          <el-input v-model="typeForm.name" placeholder="例如 game、battle、gate" />
        </el-form-item>
        <el-form-item label="健康检查">
          <el-select v-model="typeForm.readiness_probe.type">
            <el-option label="未配置" value="" />
            <el-option label="Docker HEALTHCHECK" value="docker" />
            <el-option label="TCP" value="tcp" />
            <el-option label="HTTP" value="http" />
          </el-select>
        </el-form-item>
        <el-form-item v-if="typeForm.readiness_probe.type === 'tcp' || typeForm.readiness_probe.type === 'http'" label="检查端口">
          <el-input-number v-model="typeForm.readiness_probe.port" :min="1" :max="65535" />
        </el-form-item>
        <el-form-item v-if="typeForm.readiness_probe.type === 'http'" label="检查路径">
          <el-input v-model="typeForm.readiness_probe.path" placeholder="/health" />
        </el-form-item>
      </el-form>
      <section class="parameter-fields">
        <header class="parameter-fields-header">
          <div>
            <h3>启动参数表单</h3>
            <p>配置此服务类型需要填写的启动参数。无需额外参数时可留空。</p>
          </div>
          <el-button v-if="canManage" size="small" :icon="Plus" @click="addParameterField">添加字段</el-button>
        </header>

        <section v-for="(field, index) in parameterFields" :key="field.key" class="parameter-field">
          <header class="parameter-field-summary">
            <div class="parameter-field-identity">
              <strong>{{ field.label || '新字段' }}</strong>
              <code v-if="field.flag">{{ field.flag }}</code>
            </div>
            <el-button
              link
              type="primary"
              :aria-expanded="expandedFieldKey === field.key"
              @click="expandedFieldKey = expandedFieldKey === field.key ? null : field.key"
            >{{ expandedFieldKey === field.key ? '收起' : canManage ? '修改' : '查看' }}</el-button>
          </header>
          <el-form v-if="expandedFieldKey === field.key" label-position="top" class="parameter-field-editor" :disabled="!canManage">
            <div class="parameter-field-grid">
              <el-form-item label="显示名称" required>
                <el-input v-model="field.label" placeholder="例如 最大玩家数" />
              </el-form-item>
              <el-form-item label="启动参数" required>
                <el-input v-model="field.flag" placeholder="例如 --max-players" />
              </el-form-item>
              <el-form-item label="输入类型">
                <el-select v-model="field.kind">
                  <el-option label="文本" value="text" />
                  <el-option label="整数" value="integer" />
                  <el-option label="下拉选择" value="select" />
                </el-select>
              </el-form-item>
              <el-form-item label="默认值">
                <el-input v-model="field.default_value" placeholder="可留空" />
              </el-form-item>
            </div>
            <el-form-item v-if="field.kind === 'select'" label="可选值" required>
              <el-input v-model="field.optionsText" type="textarea" :rows="3" placeholder="每行填写一个可选值" />
            </el-form-item>
            <div class="parameter-field-actions">
              <el-checkbox v-model="field.required">必填</el-checkbox>
              <el-button v-if="canManage" link type="danger" :icon="Delete" @click="removeParameterField(index)">移除</el-button>
            </div>
          </el-form>
        </section>
      </section>
      <template #footer>
        <el-button @click="typeDialogVisible = false">{{ canManage ? '取消' : '关闭' }}</el-button>
        <el-button v-if="canManage" type="primary" :loading="submitting" @click="saveType">保存</el-button>
      </template>
    </el-dialog>
  </AppLayout>
</template>

<style scoped>
.types-page-header { display: flex; flex-wrap: wrap; justify-content: space-between; align-items: center; gap: 16px; margin-bottom: 22px; }
.types-page-header h1 { margin: 0; font-size: 25px; color: var(--ops-text); }
.types-page-header p { margin: 5px 0 0; font-size: 13px; color: var(--ops-text-secondary); }
.types-actions { display: flex; flex-wrap: wrap; }
.types-list { margin-top: 12px; overflow: hidden; border: 1px solid var(--ops-border); border-radius: 12px; background: #fff; }
.types-list :deep(.el-table) { --el-table-header-bg-color: #f7f9fc; }
.types-list :deep(.el-table__cell) { padding: 12px 0; }
.types-filters { display: grid; grid-template-columns: minmax(0, 300px); gap: 10px; padding: 16px; }
@media (max-width: 640px) {
  .types-filters { grid-template-columns: 1fr; }
}
.parameter-fields { margin-top: 20px; padding-top: 16px; border-top: 1px solid var(--ops-border); }
.parameter-fields-header { display: flex; align-items: flex-start; justify-content: space-between; gap: 12px; }
.parameter-fields-header h3 { margin: 0; font-size: 15px; color: var(--ops-text); }
.parameter-fields-header p { margin: 5px 0 0; color: var(--ops-text-secondary); font-size: 12px; }
.parameter-fields-header .el-button { flex-shrink: 0; }
.parameter-field { margin-top: 10px; border: 1px solid var(--ops-border); border-radius: 8px; overflow: hidden; }
.parameter-field-summary, .parameter-field-actions { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.parameter-field-summary { padding: 10px 14px; }
.parameter-field-summary .el-button { flex-shrink: 0; }
.parameter-field-identity { display: flex; flex-wrap: wrap; align-items: center; gap: 6px 10px; min-width: 0; overflow-wrap: anywhere; }
.parameter-field-identity strong { color: var(--ops-text); font-size: 13px; }
.parameter-field-identity code { padding: 2px 6px; border-radius: 4px; background: #eef3fb; color: var(--ops-text-secondary); font-size: 12px; }
.parameter-field-editor { padding: 12px 14px; border-top: 1px solid var(--ops-border); background: #f8faff; }
.parameter-field-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 0 12px; }
.parameter-field-grid .el-select { width: 100%; }
@media (max-width: 520px) {
  .parameter-field-grid { grid-template-columns: 1fr; }
}
</style>
