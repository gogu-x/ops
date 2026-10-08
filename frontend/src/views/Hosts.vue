<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus, Connection, Delete, Monitor, View, Close } from '@element-plus/icons-vue'
import { useAuthStore } from '../stores/auth'
import { hostApi, type Host, type HostForm } from '../api/hosts'
import { projectApi, type Project } from '../api/projects'
import AppLayout from '../components/AppLayout.vue'

const auth = useAuthStore()
const hosts = ref<Host[]>([])
const projects = ref<Project[]>([])
const loading = ref(false)
const submitting = ref(false)
const dialogVisible = ref(false)
const testStates = reactive<Record<string, 'testing' | 'success' | 'error'>>({})
const detailVisible = ref(false)
const detailEditing = ref(false)
const detailSaving = ref(false)
const detailConfigLoading = ref(false)
const detailTestLoading = ref(false)
const detailTestState = ref<'idle' | 'testing' | 'success' | 'error'>('idle')
const testedConnectionSignature = ref('')
const activeHost = ref<Host | null>(null)

const form = reactive<HostForm>({
  project_ids: [],
  name: '',
  internal_ip: '',
  external_ip: '',
  docker_host: 'tcp://',
  tls_ca: '',
  tls_cert: '',
  tls_key: '',
  note: '',
})

const connectionSignature = computed(() => JSON.stringify([form.docker_host, form.tls_ca, form.tls_cert, form.tls_key]))
const connectionTestPassed = computed(() => detailTestState.value === 'success' && testedConnectionSignature.value === connectionSignature.value)

watch(connectionSignature, () => {
  if (!detailEditing.value) return
  testedConnectionSignature.value = ''
  if (detailTestState.value !== 'testing') detailTestState.value = 'idle'
})

const canManage = () => auth.hasPermission('hosts.manage')

async function loadHosts() {
  loading.value = true
  try {
    const [availableHosts, availableProjects] = await Promise.all([hostApi.list(), projectApi.list()])
    hosts.value = availableHosts
    projects.value = availableProjects
  } catch {
    ElMessage.error('主机列表加载失败')
  } finally {
    loading.value = false
  }
}

function projectName(projectId: string): string {
  return projects.value.find((project) => project.id === projectId)?.name || ''
}

function hostProjectNames(host: Host): string {
  const ids = [...new Set([...(host.project_ids || []), host.project_id].filter(Boolean))]
  const names = ids.map((id) => projectName(id)).filter(Boolean)
  return names.length ? names.join('、') : ids.length ? '所属项目' : '未绑定项目'
}

function testStatusText(hostId: string): string {
  const status = testStates[hostId]
  if (status === 'testing') return '检测中'
  if (status === 'success') return '连接正常'
  if (status === 'error') return '连接失败'
  return '未检测'
}

function testStatusType(hostId: string): 'info' | 'success' | 'danger' {
  const status = testStates[hostId]
  if (status === 'success') return 'success'
  if (status === 'error') return 'danger'
  return 'info'
}

function openCreate() {
  Object.assign(form, { project_ids: [], name: '', internal_ip: '', external_ip: '', docker_host: 'tcp://', tls_ca: '', tls_cert: '', tls_key: '', note: '' })
  dialogVisible.value = true
}

function openDetails(host: Host) {
  activeHost.value = host
  detailEditing.value = false
  detailVisible.value = true
}

async function beginHostEdit(host: Host) {
  Object.assign(form, { project_ids: [], name: host.name, internal_ip: host.internal_ip, external_ip: host.external_ip, docker_host: host.docker_host, tls_ca: '', tls_cert: '', tls_key: '', note: host.note })
  testedConnectionSignature.value = ''
  detailTestState.value = 'idle'
  detailEditing.value = true
  detailConfigLoading.value = true
  try {
    const configuration = await hostApi.configuration(host.id)
    if (activeHost.value?.id !== host.id || !detailEditing.value) return
    Object.assign(form, { tls_ca: configuration.tls_ca || '', tls_cert: configuration.tls_cert || '', tls_key: configuration.tls_key || '' })
  } catch (error: any) {
    detailEditing.value = false
    ElMessage.error(error?.response?.data?.error || '主机证书配置加载失败')
  } finally {
    detailConfigLoading.value = false
  }
}

function cancelHostEdit() {
  detailEditing.value = false
  testedConnectionSignature.value = ''
  detailTestState.value = 'idle'
}

async function testDraftHostConnection() {
  if (!activeHost.value) return
  const signature = connectionSignature.value
  detailTestLoading.value = true
  detailTestState.value = 'testing'
  try {
    await hostApi.testConfiguration(activeHost.value.id, { ...form, project_ids: undefined })
    if (signature === connectionSignature.value) {
      testedConnectionSignature.value = signature
      detailTestState.value = 'success'
      ElMessage.success('新连接配置验证通过，可以保存')
    } else {
      detailTestState.value = 'idle'
      ElMessage.warning('测试期间连接配置发生变化，请重新测试')
    }
  } catch (error: any) {
    testedConnectionSignature.value = ''
    detailTestState.value = 'error'
    ElMessage.error(error?.response?.data?.error || '连接验证失败，请检查地址和 TLS 证书')
  } finally {
    detailTestLoading.value = false
  }
}

async function saveHostEdit() {
  if (!activeHost.value || !form.name.trim()) { ElMessage.warning('请输入主机名称'); return }
  if (!connectionTestPassed.value) { ElMessage.warning('请先测试当前 Docker 地址和 TLS 证书，验证成功后再保存'); return }
  detailSaving.value = true
  try {
    await hostApi.update(activeHost.value.id, { ...form, project_ids: undefined, name: form.name.trim(), internal_ip: form.internal_ip.trim(), external_ip: form.external_ip.trim(), docker_host: form.docker_host.trim(), tls_ca: form.tls_ca.trim(), tls_cert: form.tls_cert.trim(), tls_key: form.tls_key.trim(), note: form.note.trim() })
    ElMessage.success('主机信息已更新')
    detailEditing.value = false
    await loadHosts()
    activeHost.value = hosts.value.find((host) => host.id === activeHost.value?.id) || activeHost.value
  } catch (error: any) {
    ElMessage.error(error?.response?.data?.error || '主机更新失败')
  } finally {
    detailSaving.value = false
  }
}

async function createHost() {
  if (!form.name.trim()) {
    ElMessage.warning('请输入主机名称')
    return
  }
  if (auth.user?.role !== 'admin' && !form.project_ids?.length) {
    ElMessage.warning('普通用户添加主机时必须选择授权项目')
    return
  }
  submitting.value = true
  try {
    await hostApi.create({
      project_ids: form.project_ids,
      name: form.name.trim(),
      internal_ip: form.internal_ip.trim(),
      external_ip: form.external_ip.trim(),
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
    delete testStates[host.id]
    ElMessage.success('主机已删除')
    await loadHosts()
  } catch (error: any) {
    if (error !== 'cancel' && error !== 'close') {
      ElMessage.error(error?.response?.data?.error || '主机删除失败')
    }
  }
}

async function testHost(host: Host) {
  testStates[host.id] = 'testing'
  try {
    await hostApi.test(host.id)
    testStates[host.id] = 'success'
  } catch {
    testStates[host.id] = 'error'
  }
}

onMounted(loadHosts)
</script>

<template>
  <AppLayout>
    <div class="page-content-loading" v-loading="loading">
    <div class="page-heading page-heading-modern page-heading-actions-only">
      <el-button v-if="canManage()" type="primary" :icon="Plus" @click="openCreate">添加主机</el-button>
    </div>

    <el-card class="plain-card" shadow="never">
      <div class="host-list">
        <div v-for="row in hosts" :key="row.id" class="host-item">
          <div class="host-item-icon">
            <el-icon :size="16"><Monitor /></el-icon>
          </div>
          <div class="host-item-main">
            <div class="host-item-title-row">
              <span class="host-item-name">{{ row.name }}</span>
              <el-tag size="small" :type="row.project_id || row.project_ids?.length ? 'success' : 'info'" effect="plain" round>
                {{ hostProjectNames(row) }}
              </el-tag>
              <el-tag size="small" :type="testStatusType(row.id)" effect="plain" round>
                {{ testStatusText(row.id) }}
              </el-tag>
              <span class="docker-host-text">{{ row.docker_host }}</span>
            </div>
            <div v-if="row.internal_ip || row.external_ip" class="host-item-ips">
              <span v-if="row.internal_ip">内网 IP：{{ row.internal_ip }}</span>
              <span v-if="row.external_ip">外网 IP：{{ row.external_ip }}</span>
            </div>
            <div v-if="row.note" class="host-item-note">{{ row.note }}</div>
          </div>
          <div class="host-item-actions">
            <el-button v-if="canManage()" link type="primary" :icon="Connection" :loading="testStates[row.id] === 'testing'" @click="testHost(row)">
              测试连接
            </el-button>
            <el-button link :icon="View" @click="openDetails(row)">详情</el-button>
            <el-button v-if="canManage()" link type="danger" :icon="Delete" @click="removeHost(row)">删除</el-button>
          </div>
        </div>
        <el-empty v-if="!loading && !hosts.length" description="暂无主机，请先添加主机" :image-size="60" />
      </div>
    </el-card>
    </div>

    <el-dialog v-model="dialogVisible" title="添加主机" width="680px" destroy-on-close>
      <el-form label-width="110px">
        <el-form-item label="名称" required>
          <el-input v-model="form.name" placeholder="例如 workstation 或 game-1" />
        </el-form-item>
        <el-form-item label="内网 IP">
          <el-input v-model="form.internal_ip" placeholder="例如 10.0.0.12" />
        </el-form-item>
        <el-form-item label="外网 IP">
          <el-input v-model="form.external_ip" placeholder="例如 203.0.113.10" />
        </el-form-item>
        <el-form-item label="Docker Host" required>
          <el-input v-model="form.docker_host" placeholder="tcp://192.168.1.100:2376" />
        </el-form-item>
        <el-form-item label="所属项目" :required="auth.user?.role !== 'admin'">
          <el-select v-model="form.project_ids" multiple filterable collapse-tags collapse-tags-tooltip placeholder="选择可访问的项目" style="width: 100%">
            <el-option v-for="project in projects" :key="project.id" :label="project.name" :value="project.id" />
          </el-select>
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

    <el-drawer v-model="detailVisible" direction="rtl" size="520px" :with-header="false">
      <div v-if="activeHost" class="host-detail-panel">
        <div class="host-detail-header">
          <div class="host-detail-header-main">
            <span class="host-detail-icon"><el-icon><Monitor /></el-icon></span>
            <div class="host-detail-header-info"><div class="host-detail-title">主机详情</div><div class="host-detail-subtitle">{{ activeHost.name }}</div></div>
            <el-button class="host-detail-close" circle :icon="Close" aria-label="关闭主机详情" @click="detailVisible = false" />
          </div>
          <div class="host-detail-actions">
            <el-tag :type="detailEditing ? detailTestState === 'success' && connectionTestPassed ? 'success' : detailTestState === 'error' ? 'danger' : 'info' : testStatusType(activeHost.id)" effect="light">{{ detailEditing ? detailTestState === 'testing' ? '验证中' : detailTestState === 'success' && connectionTestPassed ? '新配置验证通过' : detailTestState === 'error' ? '验证失败' : '待验证' : testStatusText(activeHost.id) }}</el-tag>
            <el-button v-if="canManage()" size="small" :type="detailEditing ? 'success' : 'primary'" plain :icon="Connection" :disabled="detailEditing && detailConfigLoading" :loading="detailEditing ? detailTestLoading : testStates[activeHost.id] === 'testing'" @click="detailEditing ? testDraftHostConnection() : testHost(activeHost)">{{ detailEditing ? '测试新配置' : '测试连接' }}</el-button>
          </div>
        </div>
        <div v-if="!detailEditing" class="host-detail-scroll">
          <div class="host-detail-section-title">连接信息</div>
          <div class="host-detail-kv">
            <div><span>Docker Host</span><b class="mono-text">{{ activeHost.docker_host || '—' }}</b></div>
            <div><span>内网 IP</span><b>{{ activeHost.internal_ip || '—' }}</b></div>
            <div><span>外网 IP</span><b>{{ activeHost.external_ip || '—' }}</b></div>
            <div><span>所属项目</span><b>{{ hostProjectNames(activeHost) }}</b></div>
          </div>
          <div class="host-detail-section-title host-detail-section-gap">TLS 证书</div>
          <div class="host-detail-cert-list">
            <div><span>TLS CA</span><el-tag size="small" :type="activeHost.tls_ca ? 'success' : 'info'" effect="light">{{ activeHost.tls_ca ? '已配置' : '未配置' }}</el-tag></div>
            <div><span>Client Cert</span><el-tag size="small" :type="activeHost.tls_cert ? 'success' : 'info'" effect="light">{{ activeHost.tls_cert ? '已配置' : '未配置' }}</el-tag></div>
            <div><span>Client Key</span><el-tag size="small" :type="activeHost.tls_key ? 'success' : 'info'" effect="light">{{ activeHost.tls_key ? '已配置' : '未配置' }}</el-tag></div>
          </div>
          <template v-if="activeHost.note">
            <div class="host-detail-section-title host-detail-section-gap">备注</div>
            <div class="host-detail-note">{{ activeHost.note }}</div>
          </template>
          <div class="host-detail-section-title host-detail-section-gap">记录信息</div>
          <div class="host-detail-kv"><div><span>创建时间</span><b>{{ activeHost.created_at || '—' }}</b></div><div><span>更新时间</span><b>{{ activeHost.updated_at || '—' }}</b></div></div>
        </div>
        <el-form v-else v-loading="detailConfigLoading" class="host-detail-scroll host-inline-form" label-position="top">
          <div class="host-detail-section-title">主机信息</div>
          <el-form-item label="主机名称" required><el-input v-model="form.name" /></el-form-item>
          <el-form-item label="Docker Host" required><el-input v-model="form.docker_host" placeholder="tcp://host:2376" /></el-form-item>
          <el-form-item label="内网 IP"><el-input v-model="form.internal_ip" placeholder="可选" /></el-form-item>
          <el-form-item label="外网 IP"><el-input v-model="form.external_ip" placeholder="可选" /></el-form-item>
          <div class="host-detail-section-title host-detail-section-gap">TLS 证书</div>
          <p class="host-cert-hint">替换证书后，请先测试当前连接配置；只有测试通过后才能保存。</p>
          <el-form-item label="TLS CA"><el-input v-model="form.tls_ca" type="textarea" :rows="4" /></el-form-item>
          <el-form-item label="Client Cert"><el-input v-model="form.tls_cert" type="textarea" :rows="4" /></el-form-item>
          <el-form-item label="Client Key"><el-input v-model="form.tls_key" type="textarea" :rows="4" /></el-form-item>
          <div class="host-detail-section-title host-detail-section-gap">备注</div>
          <el-form-item label="主机备注"><el-input v-model="form.note" type="textarea" :rows="3" /></el-form-item>
        </el-form>
        <div class="host-detail-footer"><template v-if="detailEditing"><span class="host-test-hint" :class="{ 'is-passed': connectionTestPassed }">{{ connectionTestPassed ? '当前连接和证书验证通过' : '请先测试当前地址及证书' }}</span><el-button @click="cancelHostEdit">取消</el-button><el-button type="primary" :disabled="!connectionTestPassed || detailConfigLoading" :loading="detailSaving" @click="saveHostEdit">保存</el-button></template><el-button v-else-if="canManage()" type="primary" :icon="View" @click="beginHostEdit(activeHost)">编辑主机</el-button></div>
      </div>
    </el-drawer>
  </AppLayout>
</template>

<style scoped>
.plain-card {
  border-radius: 10px;
  border: 1px solid var(--ops-border);
  box-shadow: var(--ops-shadow);
}

.plain-card :deep(.el-card__body) {
  padding: 8px 20px;
}

.host-list {
  display: flex;
  flex-direction: column;
}

.host-item {
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 18px 4px;
  border-bottom: 1px solid var(--ops-border);
}

.host-item:last-child {
  border-bottom: 0;
}

.host-item-icon {
  flex-shrink: 0;
  width: 44px;
  height: 44px;
  border-radius: 10px;
  background: var(--ops-primary-light);
  color: var(--ops-primary);
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
  flex-wrap: wrap;
}

.host-item-name {
  font-size: 14px;
  font-weight: 600;
  color: var(--ops-text);
}

.host-item-note {
  margin-top: 7px;
  font-size: 13px;
  color: var(--ops-text-secondary);
}

.host-item-actions {
  flex-shrink: 0;
  display: flex;
  gap: 8px;
}
.host-item-actions .el-button { margin-left: 0; }

.host-item-ips {
  display: flex;
  flex-wrap: wrap;
  gap: 14px;
  margin-top: 7px;
  font-size: 12px;
  color: var(--ops-text-secondary);
  overflow-wrap: anywhere;
}

.host-detail-panel { display: flex; width: 100%; height: 100%; min-height: 0; flex-direction: column; background: #fafcff; }
:deep(.el-drawer__body) { padding: 0; overflow: hidden; }
.host-detail-header { display: flex; flex-direction: column; gap: 14px; padding: 18px; border-bottom: 1px solid var(--ops-border); background: #fff; }
.host-detail-header-main { display: flex; align-items: center; gap: 12px; min-width: 0; }
.host-detail-icon { display: grid; flex: 0 0 40px; width: 40px; height: 40px; place-items: center; border-radius: 10px; background: var(--ops-primary-light); color: var(--ops-primary); font-size: 19px; }
.host-detail-header-info { flex: 1; min-width: 0; }
.host-detail-title { margin-bottom: 4px; color: var(--ops-text-secondary); font-size: 12px; }
.host-detail-subtitle { overflow: hidden; color: var(--ops-text); font-size: 16px; font-weight: 700; text-overflow: ellipsis; white-space: nowrap; }
.host-detail-close { width: 36px; height: 36px; flex: 0 0 36px; margin-left: auto; border-color: var(--ops-border); color: var(--ops-text-secondary); }
.host-detail-close:hover { border-color: var(--ops-primary); background: var(--ops-primary-light); color: var(--ops-primary); }
.host-detail-actions { display: flex; justify-content: flex-end; align-items: center; gap: 8px; }
.host-detail-scroll { flex: 1; min-height: 0; overflow-y: auto; padding: 18px; }
.host-detail-section-title { margin-bottom: 11px; color: var(--ops-text); font-size: 13px; font-weight: 700; }
.host-detail-section-gap { margin-top: 22px; }
.host-detail-kv { display: flex; flex-direction: column; gap: 12px; padding: 14px; border: 1px solid var(--ops-border); border-radius: 8px; background: #fff; }
.host-detail-kv > div, .host-detail-cert-list > div { display: flex; align-items: flex-start; justify-content: space-between; gap: 14px; font-size: 12px; }
.host-detail-kv span, .host-detail-cert-list > div > span { flex: 0 0 auto; color: var(--ops-text-secondary); }
.host-detail-kv b { min-width: 0; color: var(--ops-text); font-weight: 550; text-align: right; overflow-wrap: anywhere; }
.host-detail-kv b.mono-text { font-family: "SFMono-Regular", Consolas, Monaco, monospace; }
.host-detail-cert-list { display: flex; flex-direction: column; gap: 0; padding: 0 14px; border: 1px solid var(--ops-border); border-radius: 8px; background: #fff; }
.host-detail-cert-list > div { min-height: 42px; align-items: center; border-bottom: 1px solid #f0f2f6; }
.host-detail-cert-list > div:last-child { border-bottom: 0; }
.host-detail-note { padding: 13px; border: 1px solid var(--ops-border); border-radius: 8px; background: #fff; color: #52627a; font-size: 12px; line-height: 1.65; white-space: pre-wrap; overflow-wrap: anywhere; }
.host-inline-form :deep(.el-form-item) { margin-bottom: 14px; }
.host-inline-form :deep(.el-form-item__label) { padding-bottom: 5px; color: #52627a; font-size: 12px; font-weight: 600; }
.host-inline-form :deep(.el-input__wrapper) { min-height: 36px; border-radius: 7px; }
.host-inline-form :deep(.el-textarea__inner) { font-family: "SFMono-Regular", Consolas, Monaco, monospace; font-size: 11px; line-height: 1.5; }
.host-cert-hint { margin: -4px 0 11px; color: #8491a3; font-size: 11px; line-height: 1.5; }
.host-detail-footer { display: flex; justify-content: flex-end; gap: 8px; padding: 13px 18px; border-top: 1px solid var(--ops-border); background: #fff; }
.host-test-hint { flex: 1; align-self: center; color: #8a96a7; font-size: 11px; }
.host-test-hint.is-passed { color: #218263; }

@media (max-width: 760px) {
  .page-heading-modern { flex-direction: column; }
  .plain-card :deep(.el-card__body) { padding: 4px 14px; }
  .host-item { display: grid; grid-template-columns: 44px minmax(0, 1fr); gap: 12px; }
  .host-item-actions { grid-column: 1 / -1; width: 100%; padding-top: 10px; border-top: 1px solid #eef1f5; }
  .host-item-actions .el-button { flex: 1; }
  .docker-host-text { width: 100%; margin-left: 0; overflow-wrap: anywhere; }
  :deep(.el-dialog) { width: calc(100vw - 24px) !important; margin-top: 3vh !important; }
  :deep(.el-drawer) { width: calc(100vw - 24px) !important; }
}
</style>
