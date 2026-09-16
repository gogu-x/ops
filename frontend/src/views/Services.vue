<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useAuthStore } from '../stores/auth'
import { serviceApi, type ServiceParam, type ServiceType, type ServiceTypeForm } from '../api/services'
import { hostApi, type Host } from '../api/hosts'

const router = useRouter()
const auth = useAuthStore()
const items = ref<ServiceType[]>([])
const hosts = ref<Host[]>([])
const loading = ref(false)
const submitting = ref(false)
const dialogVisible = ref(false)
const editingId = ref('')

const form = reactive<ServiceTypeForm>({ host_id: '', name: '', image: '', params: [], note: '' })
const canManage = computed(() => auth.user?.role === 'admin')
const preview = computed(() => {
  const args = form.params
    .filter((param) => param.flag.trim() || param.value.trim())
    .map((param) => `${param.flag.trim()} ${param.value.trim()}`.trim())
  return [form.image.trim(), ...args].filter(Boolean).join(' ')
})

function blankParam(): ServiceParam {
  return { flag: '', value: '' }
}

function hostName(id: string): string {
  return hosts.value.find((host) => host.id === id)?.name || id || '未绑定'
}

async function loadItems() {
  loading.value = true
  try {
    const [serviceTypes, availableHosts] = await Promise.all([serviceApi.list(), hostApi.list()])
    items.value = serviceTypes
    hosts.value = availableHosts
  } catch {
    ElMessage.error('服务类型或主机列表加载失败')
  } finally {
    loading.value = false
  }
}

function openCreate() {
  editingId.value = ''
  Object.assign(form, { host_id: hosts.value[0]?.id || '', name: '', image: '', params: [blankParam()], note: '' })
  dialogVisible.value = true
}

function openEdit(item: ServiceType) {
  editingId.value = item.id
  Object.assign(form, {
    host_id: item.host_id,
    name: item.name,
    image: item.image,
    params: item.params.map((param) => ({ ...param })),
    note: item.note,
  })
  if (!form.params.length) form.params.push(blankParam())
  dialogVisible.value = true
}

function addParam() {
  form.params.push(blankParam())
}

function removeParam(index: number) {
  form.params.splice(index, 1)
}

async function save() {
  if (!form.host_id) {
    ElMessage.warning('请先选择部署主机')
    return
  }
  if (!form.name.trim() || !form.image.trim()) {
    ElMessage.warning('请填写服务名称和镜像')
    return
  }
  const params = form.params
    .map((param) => ({ flag: param.flag.trim(), value: param.value.trim() }))
    .filter((param) => param.flag || param.value)
  if (params.some((param) => !param.flag || !param.value)) {
    ElMessage.warning('参数行的 flag 和 value 不能为空')
    return
  }
  submitting.value = true
  try {
    const payload = { ...form, name: form.name.trim(), image: form.image.trim(), params }
    if (editingId.value) await serviceApi.update(editingId.value, payload)
    else await serviceApi.create(payload)
    ElMessage.success(editingId.value ? '服务类型已更新' : '服务类型已创建')
    dialogVisible.value = false
    await loadItems()
  } catch (error: any) {
    ElMessage.error(error?.response?.data?.error || '保存失败')
  } finally {
    submitting.value = false
  }
}

async function remove(item: ServiceType) {
  try {
    await ElMessageBox.confirm(`确定删除服务类型“${item.name}”吗？`, '删除确认', { type: 'warning' })
    await serviceApi.remove(item.id)
    ElMessage.success('服务类型已删除')
    await loadItems()
  } catch (error: any) {
    if (error !== 'cancel' && error !== 'close') ElMessage.error(error?.response?.data?.error || '删除失败')
  }
}

async function logout() {
  await auth.logout()
  await router.push('/login')
}

onMounted(loadItems)
</script>

<template>
  <el-container class="app-shell">
    <el-header class="topbar">
      <strong>Ops Platform</strong>
      <div class="topbar-right">
        <span>{{ auth.user?.username }} / {{ auth.user?.role }}</span>
        <el-button link type="primary" @click="logout">退出</el-button>
      </div>
    </el-header>
    <el-container>
      <el-aside width="220px" class="sidebar">
        <el-menu default-active="services">
          <el-menu-item index="dashboard" @click="router.push('/')">控制台</el-menu-item>
          <el-menu-item index="hosts" @click="router.push('/hosts')">主机管理</el-menu-item>
          <el-menu-item index="services">服务类型</el-menu-item>
          <el-menu-item index="audit">审计日志</el-menu-item>
        </el-menu>
      </el-aside>
      <el-main class="content">
        <div class="page-heading">
          <div>
            <h2>服务类型</h2>
            <p>配置 Docker 镜像和结构化启动参数，value 支持 {{ '{' }}{{ '{' }}id{{ '}' }}{{ '}' }} 等模板占位符。</p>
          </div>
          <el-button v-if="canManage" type="primary" @click="openCreate">添加服务类型</el-button>
        </div>

        <el-card shadow="never">
          <el-table v-loading="loading" :data="items" stripe>
            <el-table-column prop="name" label="名称" width="150" />
            <el-table-column label="部署主机" min-width="160">
              <template #default="{ row }">{{ hostName(row.host_id) }}</template>
            </el-table-column>
            <el-table-column prop="image" label="默认镜像" min-width="240" />
            <el-table-column label="参数" min-width="280">
              <template #default="{ row }">
                <el-tag v-for="param in row.params" :key="`${param.flag}-${param.value}`" size="small" class="param-tag">{{ param.flag }} {{ param.value }}</el-tag>
                <span v-if="!row.params.length">—</span>
              </template>
            </el-table-column>
            <el-table-column prop="note" label="备注" min-width="170" show-overflow-tooltip />
            <el-table-column v-if="canManage" label="操作" width="160" fixed="right">
              <template #default="{ row }">
                <el-button link type="primary" @click="openEdit(row)">编辑</el-button>
                <el-button link type="danger" @click="remove(row)">删除</el-button>
              </template>
            </el-table-column>
            <template #empty><el-empty description="暂无服务类型，请先添加" /></template>
          </el-table>
        </el-card>
      </el-main>
    </el-container>

    <el-dialog v-model="dialogVisible" :title="editingId ? '编辑服务类型' : '添加服务类型'" width="760px" destroy-on-close>
      <el-form label-width="100px">
        <el-form-item label="部署主机" required>
          <el-select v-model="form.host_id" placeholder="请选择 Docker 主机" style="width: 100%">
            <el-option v-for="host in hosts" :key="host.id" :label="host.name" :value="host.id">
              <span>{{ host.name }}</span>
              <span class="host-option-detail">{{ host.docker_host }}</span>
            </el-option>
          </el-select>
        </el-form-item>
        <el-form-item label="名称" required>
          <el-input v-model="form.name" placeholder="例如 game、gate、platform" />
        </el-form-item>
        <el-form-item label="默认镜像" required>
          <el-input v-model="form.image" placeholder="例如 gogs-game:v1001-3" />
        </el-form-item>
        <el-form-item label="启动参数">
          <div class="params-editor">
            <div v-for="(param, index) in form.params" :key="index" class="param-row">
              <el-input v-model="param.flag" placeholder="flag，例如 --server-id" />
              <el-input v-model="param.value" placeholder="value，例如 {{id}} 或 {{etcd}}" />
              <el-button type="danger" link @click="removeParam(index)">删除</el-button>
            </div>
            <el-button link type="primary" @click="addParam">+ 添加参数</el-button>
          </div>
        </el-form-item>
        <el-form-item label="参数预览">
          <el-input :model-value="preview" readonly />
        </el-form-item>
        <el-form-item label="备注">
          <el-input v-model="form.note" type="textarea" :rows="3" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="submitting" @click="save">保存</el-button>
      </template>
    </el-dialog>
  </el-container>
</template>

<style scoped>
.param-tag { margin: 2px 4px 2px 0; }
.params-editor { width: 100%; }
.param-row { display: grid; grid-template-columns: 1fr 1.5fr auto; gap: 8px; margin-bottom: 8px; align-items: center; }
.host-option-detail { float: right; color: #94a3b8; }
</style>
