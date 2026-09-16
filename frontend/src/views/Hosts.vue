<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useAuthStore } from '../stores/auth'
import { hostApi, type Host, type HostForm, type HostTestResult } from '../api/hosts'

const router = useRouter()
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
    await ElMessageBox.confirm(`确定删除主机“${host.name}”吗？`, '删除确认', { type: 'warning' })
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

async function logout() {
  await auth.logout()
  await router.push('/login')
}

onMounted(loadHosts)
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
        <el-menu default-active="hosts">
          <el-menu-item index="dashboard" @click="router.push('/')">控制台</el-menu-item>
          <el-menu-item index="hosts">主机管理</el-menu-item>
          <el-menu-item index="services">服务类型</el-menu-item>
          <el-menu-item index="audit">审计日志</el-menu-item>
        </el-menu>
      </el-aside>
      <el-main class="content">
        <div class="page-heading">
          <div>
            <h2>主机管理</h2>
            <p>所有 Docker 主机统一使用 TCP 2376 + TLS。CA、Client Cert、Client Key 可直接粘贴 PEM 内容；全部为空时不校验证书。</p>
          </div>
          <el-button v-if="canManage()" type="primary" @click="openCreate">添加主机</el-button>
        </div>

        <el-card shadow="never">
          <el-table v-loading="loading" :data="hosts" stripe>
            <el-table-column prop="name" label="名称" min-width="150" />
            <el-table-column label="Docker TLS 地址" min-width="230">
              <template #default="{ row }">
                <el-tag type="success">TCP TLS</el-tag>
                <span class="docker-host-text">{{ row.docker_host }}</span>
              </template>
            </el-table-column>
            <el-table-column label="TLS 证书" min-width="170">
              <template #default="{ row }">{{ row.tls_ca }} / {{ row.tls_cert }} / {{ row.tls_key }}</template>
            </el-table-column>
            <el-table-column prop="note" label="备注" min-width="180" show-overflow-tooltip />
            <el-table-column label="操作" width="220" fixed="right">
              <template #default="{ row }">
                <el-button link type="primary" :loading="testingId === row.id" @click="testHost(row)">测试连接</el-button>
                <el-button v-if="canManage()" link type="danger" @click="removeHost(row)">删除</el-button>
              </template>
            </el-table-column>
            <template #empty><el-empty description="暂无主机，请先添加主机" /></template>
          </el-table>
        </el-card>

        <el-card v-if="testResult" class="test-result" shadow="never">
          <template #header>最近一次连接结果</template>
          <el-descriptions :column="4" border>
            <el-descriptions-item label="主机">{{ testResult.host.name }}</el-descriptions-item>
            <el-descriptions-item label="Docker 版本">{{ testResult.docker.version || '—' }}</el-descriptions-item>
            <el-descriptions-item label="API 版本">{{ testResult.docker.api_version || '—' }}</el-descriptions-item>
            <el-descriptions-item label="系统">{{ testResult.docker.os }} / {{ testResult.docker.arch }}</el-descriptions-item>
          </el-descriptions>
        </el-card>
      </el-main>
    </el-container>

    <el-dialog v-model="dialogVisible" title="添加主机" width="520px" destroy-on-close>
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
  </el-container>
</template>
