<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus, Connection, Delete, Monitor } from '@element-plus/icons-vue'
import { useAuthStore } from '../stores/auth'
import { hostApi, type Host, type HostForm, type HostTestResult } from '../api/hosts'
import AppLayout from '../components/AppLayout.vue'

const auth = useAuthStore()
const hosts = ref<Host[]>([])
const loading = ref(false)
const submitting = ref(false)
const dialogVisible = ref(false)
const testingId = ref('')
const testResult = ref<HostTestResult | null>(null)

const form = reactive<HostForm>({
  name: '',
  docker_host: 'tcp://',
  tls_ca: '',
  tls_cert: '',
  tls_key: '',
  note: '',
})

const canManage = () => auth.user?.role === 'admin'

async function loadHosts() {
  loading.value = true
  try {
    hosts.value = await hostApi.list()
  } catch {
    ElMessage.error('主机列表加载失败')
  } finally {
    loading.value = false
  }
}

function openCreate() {
  Object.assign(form, { name: '', docker_host: 'tcp://', tls_ca: '', tls_cert: '', tls_key: '', note: '' })
  dialogVisible.value = true
}

async function createHost() {
  if (!form.name.trim()) {
    ElMessage.warning('请输入主机名称')
    return
  }
  submitting.value = true
  try {
    await hostApi.create({
      name: form.name.trim(),
      docker_host: form.docker_host.trim(),
      tls_ca: form.tls_ca.trim(),
      tls_cert: form.tls_cert.trim(),
      tls_key: form.tls_key.trim(),
      note: form.note.trim(),
    })
    ElMessage.success('主机已添加')
    dialogVisible.value = false
    await loadHosts()
  } catch (error: any) {
    ElMessage.error(error?.response?.data?.error || '主机添加失败')
  } finally {
    submitting.value = false
  }
}

async function removeHost(host: Host) {
  try {
    await ElMessageBox.confirm(`确定删除主机"${host.name}"吗？删除前请确认该主机上没有正在使用的服务。`, '删除确认', { type: 'warning' })
    await hostApi.remove(host.id)
    ElMessage.success('主机已删除')
    await loadHosts()
  } catch (error: any) {
    if (error !== 'cancel' && error !== 'close') {
      ElMessage.error(error?.response?.data?.error || '主机删除失败')
    }
  }
}

async function testHost(host: Host) {
  testingId.value = host.id
  testResult.value = null
  try {
    testResult.value = await hostApi.test(host.id)
    ElMessage.success(`${host.name} Docker 连接成功`)
  } catch (error: any) {
    ElMessage.error(error?.response?.data?.error || `${host.name} Docker 连接失败`)
  } finally {
    testingId.value = ''
  }
}

onMounted(loadHosts)
</script>

<template>
  <AppLayout>
    <div class="page-heading-actions-only">
      <el-button v-if="canManage()" size="default" :icon="Plus" @click="openCreate">添加主机</el-button>
    </div>

    <el-card class="plain-card" shadow="never" v-loading="loading">
      <div class="host-list">
        <div v-for="row in hosts" :key="row.id" class="host-item">
          <div class="host-item-icon">
            <el-icon :size="16"><Monitor /></el-icon>
          </div>
          <div class="host-item-main">
            <div class="host-item-title-row">
              <span class="host-item-name">{{ row.name }}</span>
              <span class="docker-host-text">{{ row.docker_host }}</span>
            </div>
            <div v-if="row.note" class="host-item-note">{{ row.note }}</div>
          </div>
          <div class="host-item-actions">
            <el-button link type="primary" size="small" :icon="Connection" :loading="testingId === row.id" @click="testHost(row)">
              测试连接
            </el-button>
            <el-button v-if="canManage()" link type="danger" size="small" :icon="Delete" @click="removeHost(row)">删除</el-button>
          </div>
        </div>
        <el-empty v-if="!loading && !hosts.length" description="暂无主机，请先添加主机" :image-size="60" />
      </div>
    </el-card>

    <el-card v-if="testResult" class="plain-card test-result" shadow="never">
      <template #header><span class="test-result-title">最近一次连接结果</span></template>
      <el-descriptions :column="4" size="small" border>
        <el-descriptions-item label="主机">{{ testResult.host.name }}</el-descriptions-item>
        <el-descriptions-item label="Docker 版本">{{ testResult.docker.version || '—' }}</el-descriptions-item>
        <el-descriptions-item label="API 版本">{{ testResult.docker.api_version || '—' }}</el-descriptions-item>
        <el-descriptions-item label="系统">{{ testResult.docker.os }} / {{ testResult.docker.arch }}</el-descriptions-item>
      </el-descriptions>
    </el-card>

    <el-dialog v-model="dialogVisible" title="添加主机" width="680px" destroy-on-close>
      <el-form label-width="110px">
        <el-form-item label="名称" required>
          <el-input v-model="form.name" placeholder="例如 workstation 或 game-1" />
        </el-form-item>
        <el-form-item label="Docker Host" required>
          <el-input v-model="form.docker_host" placeholder="tcp://192.168.1.100:2376" />
        </el-form-item>
        <el-form-item label="TLS CA">
          <el-input v-model="form.tls_ca" type="textarea" :rows="5" placeholder="可选，粘贴 CA PEM 内容；为空时跳过服务端证书校验" />
        </el-form-item>
        <el-form-item label="TLS Client Cert">
          <el-input v-model="form.tls_cert" type="textarea" :rows="5" placeholder="可选，粘贴客户端证书 PEM 内容" />
        </el-form-item>
        <el-form-item label="TLS Client Key">
          <el-input v-model="form.tls_key" type="textarea" :rows="5" placeholder="可选，粘贴客户端私钥 PEM 内容" />
        </el-form-item>
        <el-form-item label="备注">
          <el-input v-model="form.note" type="textarea" :rows="3" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="submitting" @click="createHost">保存</el-button>
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

.host-list {
  display: flex;
  flex-direction: column;
}

.host-item {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px 4px;
  border-bottom: 1px solid var(--ops-border);
}

.host-item:last-child {
  border-bottom: 0;
}

.host-item-icon {
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

.host-item-main {
  flex: 1;
  min-width: 0;
}

.host-item-title-row {
  display: flex;
  align-items: center;
  gap: 10px;
}

.host-item-name {
  font-size: 14px;
  font-weight: 600;
  color: var(--ops-text);
}

.host-item-note {
  margin-top: 2px;
  font-size: 12px;
  color: var(--ops-text-secondary);
}

.host-item-actions {
  flex-shrink: 0;
  display: flex;
  gap: 4px;
}

.test-result {
  margin-top: 16px;
}

.test-result-title {
  font-size: 13px;
  color: var(--ops-text-secondary);
  font-weight: 500;
}
</style>
