<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { ElDialog, ElMessage, ElMessageBox } from 'element-plus'
import { Delete, Plus, Refresh, Search } from '@element-plus/icons-vue'
import ResizableDrawer from '../components/ResizableDrawer.vue'
import AppLayout from '../components/AppLayout.vue'
import { useAuthStore } from '../stores/auth'
import { serviceApi, type ServiceParameterField, type ServiceType, type ServiceTypeForm } from '../api/services'

const auth = useAuthStore()
const canManage = computed(() => auth.hasPermission(editingTypeId.value ? 'service_types.edit' : 'service_types.create'))
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
        required: field.kind !== 'fixed' && field.required,
        default_value: field.kind === 'text' || field.kind === 'fixed' ? field.default_value : field.default_value.trim(),
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
      <el-alert v-if="loadError" class="types-list-error" :title="loadError" type="error" show-icon :closable="false" />
      <section class="types-list ops-list-panel" aria-labelledby="types-list-heading">
        <header class="ops-list-toolbar types-list-toolbar">
          <h2 id="types-list-heading">服务类型 <small>{{ filteredTypes.length }}</small></h2>
          <div class="ops-list-toolbar-actions">
            <el-input v-model="keyword" class="types-search" :prefix-icon="Search" clearable placeholder="搜索类型名称" aria-label="搜索类型名称" />
            <el-button :icon="Refresh" :loading="loading" @click="loadAll">刷新</el-button>
            <el-button v-if="auth.hasPermission('service_types.create')" :icon="Plus" type="primary" @click="openCreateType">新建服务类型</el-button>
          </div>
        </header>
        <div class="type-row type-columns" aria-hidden="true"><span>类型名称</span><span>启动参数</span><span>操作</span></div>
        <ul class="type-list-rows">
          <li v-for="type in filteredTypes" :key="type.id" class="type-row">
            <button type="button" class="type-name" @click="openType(type)">{{ type.name }}</button>
            <div class="type-parameters">
              <template v-if="type.parameter_fields?.length">
                <span>{{ type.parameter_fields.length }} 个字段</span>
                <code v-for="(field, index) in type.parameter_fields.slice(0, 2)" :key="index">{{ field.flag || field.label }}</code>
              </template>
              <span v-else>未配置启动参数</span>
            </div>
            <div class="type-actions">
              <el-button link type="primary" @click="openType(type)">详情</el-button>
              <el-button v-if="auth.hasPermission('service_types.delete')" link type="danger" @click="removeType(type)">删除</el-button>
            </div>
          </li>
        </ul>
        <el-empty v-if="!loading && !loadError && !filteredTypes.length" :description="keyword.trim() ? '没有符合条件的服务类型' : '暂无服务类型'" :image-size="60" />

      </section>
    </div>
    <!-- 服务类型表单 -->
    <component :is="editingTypeId ? ResizableDrawer : ElDialog" v-model="typeDialogVisible"
      :title="editingTypeId ? '服务类型详情' : '添加服务类型'"
      v-bind="editingTypeId ? { size: 'min(600px, 100vw)', direction: 'rtl' } : { width: 'min(680px, 94vw)' }"
      destroy-on-close>
      <el-form label-width="100px" :disabled="!canManage">
        <el-form-item label="类型名称" required>
          <el-input v-model="typeForm.name" placeholder="例如 game、battle、gate" />
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
                  <el-option label="固定预设值" value="fixed" />
                  <el-option label="文本" value="text" />
                  <el-option label="整数" value="integer" />
                  <el-option label="下拉选择" value="select" />
                </el-select>
              </el-form-item>
              <el-form-item :label="field.kind === 'fixed' ? '固定预设值' : '默认值'" :required="field.kind === 'fixed'">
                <el-input v-model="field.default_value" :placeholder="field.kind === 'fixed' ? '所有实例部署时使用此值' : '可留空'" />
              </el-form-item>
            </div>
            <el-form-item v-if="field.kind === 'select'" label="可选值" required>
              <el-input v-model="field.optionsText" type="textarea" :rows="3" placeholder="每行填写一个可选值" />
            </el-form-item>
            <div class="parameter-field-actions">
              <el-checkbox v-if="field.kind !== 'fixed'" v-model="field.required">必填</el-checkbox>
              <el-button v-if="canManage" link type="danger" :icon="Delete" @click="removeParameterField(index)">移除</el-button>
            </div>
          </el-form>
        </section>
      </section>
      <template #footer>
        <el-button @click="typeDialogVisible = false">{{ canManage ? '取消' : '关闭' }}</el-button>
        <el-button v-if="canManage" type="primary" :loading="submitting" @click="saveType">保存</el-button>
      </template>
    </component>
  </AppLayout>
</template>

<style scoped>
.types-list-error { margin-bottom: 12px; }
.types-search { width: 215px; min-width: 0; }
.types-list-toolbar small { font-size: 13px; font-weight: 400; color: var(--ops-text-secondary); }
.type-list-rows { margin: 0; padding: 0; list-style: none; }
.type-row { display: grid; grid-template-columns: minmax(120px, 2fr) minmax(150px, 3fr) auto; align-items: center; gap: 12px; padding: 12px 18px; }
.type-row + .type-row { border-top: 1px solid var(--ops-border); }
.type-columns { color: var(--ops-text-secondary); background: #f7f9fc; font-size: 12px; }
.type-name { justify-self: start; max-width: 100%; padding: 0; border: 0; background: transparent; font: inherit; font-weight: 600; color: var(--ops-text); text-align: left; overflow-wrap: anywhere; cursor: pointer; }
.type-name:hover { color: var(--ops-primary); }
.type-parameters { display: flex; flex-wrap: wrap; gap: 6px; align-items: center; font-size: 12px; color: var(--ops-text-secondary); }
.type-parameters code { padding: 2px 5px; background: #f2f5fa; border-radius: 4px; overflow-wrap: anywhere; min-width: 0; }
.type-actions { white-space: nowrap; }
@media (max-width: 600px) {
  .type-columns { display: none; }
  .type-list-rows .type-row { grid-template-columns: minmax(0, 1fr) auto; gap: 5px 12px; padding: 12px; }
  .type-parameters { grid-column: 1; }
  .type-actions { grid-column: 2; grid-row: 1 / 3; }
}
@media (max-width: 420px) {
  .types-list-toolbar .el-button { padding-inline: 8px; }
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
