<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { Calendar, CircleCheck, Clock, Collection, Plus, Search, Warning } from '@element-plus/icons-vue'
import AppLayout from '../components/AppLayout.vue'
import { autoOpenApi, type AutoOpenPolicy, type AutoOpenSettings, type AutoOpenTemplate } from '../api/autoOpen'
import { environmentApi, type Environment } from '../api/environments'
import { projectApi, type Project } from '../api/projects'
import { useAuthStore } from '../stores/auth'

const auth = useAuthStore()
const canManage = computed(() => auth.hasPermission('services.manage'))
const policies = ref<AutoOpenPolicy[]>([])
const templates = ref<AutoOpenTemplate[]>([])
const projects = ref<Project[]>([])
const environments = ref<Environment[]>([])
const loading = ref(true)
const reading = ref(false)
const loadError = ref('')
const keyword = ref('')
const statusFilter = ref<PolicyFilter>('all')
const loadingTemplates = ref(false)
const templateError = ref('')
const editing = ref<AutoOpenPolicy>()
const editorVisible = ref(false)
const enableTarget = ref<AutoOpenPolicy>()
const enableVisible = ref(false)
const recordsId = ref('')
const saving = ref(false)
const changingId = ref('')
const firstOpenAt = ref<Date>()
const form = reactive<AutoOpenSettings>(defaultSettings())
const selectedTemplate = computed(() => templates.value.find(item => item.id === form.template_id))
const records = computed(() => policies.value.find(item => item.id === recordsId.value))

const filters = [
  { key: 'all', label: '全部策略', icon: Collection },
  { key: 'enabled', label: '已启用', icon: Clock },
  { key: 'completed', label: '已完成', icon: CircleCheck },
  { key: 'failed', label: '执行失败', icon: Warning },
] as const

type PolicyFilter = typeof filters[number]['key']

const policyRows = computed(() => policies.value.map(policy => ({ ...policy, state: status(policy) })))
const counts = computed(() => {
  const totals = { all: policyRows.value.length, enabled: 0, completed: 0, failed: 0 }
  for (const { state } of policyRows.value) {
    if (state.key !== 'disabled') totals[state.key]++
  }
  return totals
})
const visiblePolicies = computed(() => {
  const term = keyword.value.trim().toLowerCase()
  return policyRows.value.filter(policy => {
    if (statusFilter.value !== 'all' && policy.state.key !== statusFilter.value) return false
    if (!term) return true
    return [policy.name, policy.template_name, projectName(policy.project_id), environmentName(policy.environment_id), serverRange(policy)]
      .some(value => value.toLowerCase().includes(term))
  })
})

let polling: ReturnType<typeof setInterval> | undefined

function defaultSettings(): AutoOpenSettings {
  return {
    name: '', template_id: '', name_prefix: 'game-', first_server_id: 1,
    first_open_at: '', interval_days: 0, count: 1, first_host_port: 0,
  }
}

function errorMessage(error: unknown, fallback: string) {
  const detail = error as { response?: { data?: { error?: string } } }
  return detail.response?.data?.error || fallback
}

function projectName(id: string) {
  return projects.value.find(item => item.id === id)?.name || id
}

function environmentName(id: string) {
  return environments.value.find(item => item.id === id)?.name || id
}

function templateLabel(item: AutoOpenTemplate) {
  return [projectName(item.project_id), environmentName(item.environment_id), item.name].join(' / ')
}

function formatTime(value: string) {
  return new Date(value).toLocaleString('zh-CN', { hour12: false })
}

function status(policy: AutoOpenPolicy) {
  if (policy.running) return { label: '开服中', key: 'enabled' as const }
  if (policy.completed >= policy.count) return { label: '已完成', key: 'completed' as const }
  if (policy.enabled) return { label: '待执行', key: 'enabled' as const }
  if (policy.recent_runs[0]?.error) return { label: '执行失败', key: 'failed' as const }
  return { label: '未启用', key: 'disabled' as const }
}

function serverRange(policy: AutoOpenPolicy) {
  const start = policy.name_prefix + policy.first_server_id
  return policy.count > 1 ? `${start} 至 ${policy.name_prefix}${policy.first_server_id + policy.count - 1}` : start
}

async function loadPolicies() {
  if (reading.value) return
  reading.value = true
  try {
    policies.value = await autoOpenApi.list()
    loadError.value = ''
  } catch (error) {
    loadError.value = errorMessage(error, '读取自动开服策略失败')
  } finally {
    loading.value = false
    reading.value = false
  }
}

async function showEditor(policy?: AutoOpenPolicy) {
  editing.value = policy
  const settings = defaultSettings()
  if (policy) {
    for (const key of Object.keys(settings) as (keyof AutoOpenSettings)[]) {
      Object.assign(settings, { [key]: policy[key] })
    }
  }
  Object.assign(form, settings)
  firstOpenAt.value = policy ? new Date(policy.first_open_at) : undefined
  editorVisible.value = true
  loadingTemplates.value = true
  templateError.value = ''
  try {
    templates.value = await autoOpenApi.templates()
  } catch (error) {
    templateError.value = errorMessage(error, '读取 Game 模板失败')
  } finally {
    loadingTemplates.value = false
  }
}

async function save() {
  if (saving.value || !firstOpenAt.value) return
  saving.value = true
  try {
    await autoOpenApi.save({
      ...form,
      first_open_at: firstOpenAt.value.toISOString(),
      count: form.interval_days ? form.count : 1,
      first_host_port: form.first_host_port || 0,
    }, editing.value)
    editorVisible.value = false
    ElMessage.success('策略已保存')
    await loadPolicies()
  } catch (error) {
    ElMessage.error(errorMessage(error, '保存策略失败'))
  } finally {
    saving.value = false
  }
}

function changeEnabled(policy: AutoOpenPolicy) {
  if (policy.enabled) {
    void updateEnabled(policy, false)
    return
  }
  enableTarget.value = policy
  enableVisible.value = true
}

async function updateEnabled(policy: AutoOpenPolicy, enabled: boolean) {
  if (changingId.value) return
  changingId.value = policy.id
  try {
    const updated = await autoOpenApi.setEnabled(policy, enabled)
    policies.value = policies.value.map(item => item.id === updated.id ? updated : item)
    if (enabled) enableVisible.value = false
    ElMessage.success(enabled ? '策略已启用' : '策略已停用')
  } catch (error) {
    ElMessage.error(errorMessage(error, '更新策略状态失败'))
  } finally {
    changingId.value = ''
    await loadPolicies()
  }
}

onMounted(() => {
  void loadPolicies()
  void projectApi.list().then(items => { projects.value = items }).catch(() => undefined)
  void environmentApi.list().then(items => { environments.value = items }).catch(() => undefined)
  polling = setInterval(() => void loadPolicies(), 5000)
})
onUnmounted(() => { if (polling) clearInterval(polling) })
</script>

<template>
  <AppLayout>
    <div class="auto-open-page">
      <el-alert v-if="loadError" class="load-error" :title="loadError" type="error" :closable="false" show-icon />
      <div v-if="!loadError || policies.length" class="policy-overview" role="group" aria-label="按状态筛选策略">
        <button
          v-for="filter in filters"
          :key="filter.key"
          type="button"
          class="overview-card"
          :data-kind="filter.key"
          :aria-pressed="statusFilter === filter.key"
          @click="statusFilter = filter.key"
        >
          <span class="overview-icon" aria-hidden="true"><el-icon><component :is="filter.icon" /></el-icon></span>
          <span class="overview-label">{{ filter.label }}</span>
          <strong class="overview-value">{{ loading ? '—' : counts[filter.key] }}</strong>
        </button>
      </div>

      <section class="policy-list" aria-labelledby="policy-list-heading" v-loading="loading">
        <header class="policy-list-header ops-list-toolbar">
          <h2 id="policy-list-heading">策略列表</h2>
          <div class="ops-list-toolbar-actions">
            <el-input
              v-if="policies.length"
              v-model="keyword"
              class="policy-search"
              :prefix-icon="Search"
              placeholder="搜索策略、模板或环境"
              aria-label="搜索开服策略"
              clearable
            />
            <el-button v-if="canManage" type="primary" :icon="Plus" @click="showEditor()">新建策略</el-button>
          </div>
        </header>
        <div v-if="visiblePolicies.length" class="policy-table-scroll" tabindex="0" aria-label="开服策略表格" role="region">
          <table class="policy-table" aria-labelledby="policy-list-heading">
            <thead>
              <tr>
                <th scope="col">策略 / 环境</th>
                <th scope="col">Game 模板</th>
                <th scope="col">执行计划</th>
                <th scope="col">下次开服</th>
                <th scope="col">状态 / 进度</th>
                <th scope="col">操作</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="row in visiblePolicies" :key="row.id">
                <th scope="row" :title="`${row.name} · ${projectName(row.project_id)} / ${environmentName(row.environment_id)}`">
                  <div class="policy-line">
                    <strong class="policy-name">{{ row.name }}</strong>
                    <span class="policy-meta">{{ projectName(row.project_id) }} / {{ environmentName(row.environment_id) }}</span>
                  </div>
                </th>
                <td :title="row.template_name">
                  <code class="template-name">{{ row.template_name }}</code>
                </td>
                <td :title="serverRange(row)">
                  <div class="policy-line">
                    <span>{{ row.interval_days ? `每隔 ${row.interval_days} 天` : '单次开服' }}</span>
                    <span class="policy-meta">{{ serverRange(row) }}</span>
                  </div>
                </td>
                <td class="policy-next">
                  <time v-if="row.completed < row.count" :datetime="row.next_open_at">{{ formatTime(row.next_open_at) }}</time>
                  <span v-else class="muted-value">—</span>
                </td>
                <td class="policy-status" :data-kind="row.state.key">
                  <div class="policy-line">
                    <span class="state-label">{{ row.state.label }}</span>
                    <progress :value="row.completed" :max="row.count" :aria-label="`${row.name} 开服进度`" />
                    <span class="policy-progress-count">{{ row.completed }} / {{ row.count }}</span>
                  </div>
                </td>
                <td class="policy-actions">
                  <el-button v-if="canManage && !row.enabled && !row.running && !row.recent_runs.length" link @click="showEditor(row)">编辑</el-button>
                  <el-button v-if="canManage && row.completed < row.count" type="primary" link :disabled="row.running" :loading="changingId === row.id" @click="changeEnabled(row)">
                    {{ row.enabled ? '停用' : row.recent_runs[0]?.error ? '重试' : '启用' }}
                  </el-button>
                  <el-button link @click="recordsId = row.id">记录</el-button>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
        <div v-else class="policy-empty">
          <template v-if="!loading">
            <span class="empty-icon" aria-hidden="true"><el-icon><Search v-if="policies.length" /><Calendar v-else /></el-icon></span>
            <strong>{{ policies.length ? '没有匹配的策略' : loadError ? '策略列表暂不可用' : '暂无开服策略' }}</strong>
            <el-button v-if="policies.length" type="primary" link @click="keyword = ''; statusFilter = 'all'">重置筛选</el-button>
            <p v-else-if="!loadError && canManage">新建策略后，即可安排首服时间和开服间隔。</p>
          </template>
        </div>
      </section>

      <el-drawer
        v-model="editorVisible"
        :title="editing ? '编辑开服策略' : '新建开服策略'"
        direction="rtl"
        size="min(560px, 100vw)"
        class="policy-drawer"
        :close-on-click-modal="false"
        :close-on-press-escape="!saving"
        :show-close="!saving"
        destroy-on-close
      >
        <el-form label-position="top" class="policy-form" :disabled="saving">
          <section class="policy-section" aria-labelledby="policy-template-heading">
            <h3 id="policy-template-heading">基本设置</h3>
            <el-form-item label="策略名称" required>
              <el-input v-model="form.name" maxlength="60" placeholder="例如 周末新服" />
            </el-form-item>
            <el-form-item label="Game 模板" required>
              <el-select v-model="form.template_id" filterable :loading="loadingTemplates" placeholder="选择现有 Game 实例">
                <el-option v-for="item in templates" :key="item.id" :label="templateLabel(item)" :value="item.id" />
              </el-select>
              <small v-if="templateError" class="field-error">{{ templateError }}</small>
              <small v-else-if="selectedTemplate">开服时读取模板当前的镜像、主机、网络和配置。</small>
            </el-form-item>
          </section>
          <section class="policy-section" aria-labelledby="policy-schedule-heading">
            <h3 id="policy-schedule-heading">时间计划</h3>
            <el-form-item label="首服开服时间" required>
              <el-date-picker v-model="firstOpenAt" type="datetime" placeholder="选择日期和时间" />
              <small>使用当前浏览器时区。</small>
            </el-form-item>
            <div class="policy-fields">
              <el-form-item label="开服间隔（天）">
                <el-input-number v-model="form.interval_days" :min="0" :max="365" :precision="0" controls-position="right" />
                <small>填 0 时只开一次。</small>
              </el-form-item>
              <el-form-item v-if="form.interval_days > 0" label="开服数量" required>
                <el-input-number v-model="form.count" :min="1" :max="1000" :precision="0" controls-position="right" />
              </el-form-item>
            </div>
          </section>
          <section class="policy-section" aria-labelledby="policy-instance-heading">
            <h3 id="policy-instance-heading">区服配置</h3>
            <div class="policy-fields">
              <el-form-item label="实例名称前缀" required>
                <el-input v-model="form.name_prefix" maxlength="53" placeholder="game-" />
                <small v-if="form.name_prefix && form.first_server_id">首个实例为 {{ form.name_prefix }}{{ form.first_server_id }}</small>
              </el-form-item>
              <el-form-item label="起始区服编号" required>
                <el-input-number v-model="form.first_server_id" :min="1" :max="65535" :precision="0" controls-position="right" />
                <small>每成功开服一次，编号加一。</small>
              </el-form-item>
            </div>
            <el-form-item label="主机起始端口">
              <el-input-number v-model="form.first_host_port" :min="0" :max="65535" :precision="0" controls-position="right" />
              <small>填 0 时不发布主机端口。填写后，每新开一服端口加一。</small>
            </el-form-item>
          </section>
          <details v-if="selectedTemplate" class="startup-parameters" open>
            <summary>Game 启动参数</summary>
            <p>区服参数按策略生成，连接配置继承自所选模板。开服时使用模板中的原始值。</p>
            <dl>
              <div><dt><code>--server-id</code></dt><dd>{{ form.first_server_id || '待填写' }} <small>首服编号</small></dd></div>
              <div><dt><code>--grpc-host</code></dt><dd>{{ form.name_prefix && form.first_server_id ? form.name_prefix + form.first_server_id : '待填写' }}</dd></div>
              <div><dt><code>--server-open-ts</code></dt><dd>{{ firstOpenAt ? Math.floor(firstOpenAt.getTime() / 1000) : '待选择开服时间' }}</dd></div>
              <div v-for="param in selectedTemplate.parameters" :key="param.flag">
                <dt><code>{{ param.flag }}</code></dt>
                <dd :class="{ 'field-muted': !param.display_value }">{{ param.display_value || (param.configured ? '已配置，值已隐藏' : '模板未配置') }}</dd>
              </div>
            </dl>
          </details>
        </el-form>
        <template #footer>
          <div class="policy-footer">
            <small v-if="!editing">保存后可在列表中启用。</small>
            <div class="policy-footer-actions">
              <el-button :disabled="saving" @click="editorVisible = false">取消</el-button>
              <el-button type="primary" :loading="saving" :disabled="!form.name.trim() || !form.template_id || !firstOpenAt" @click="save">保存策略</el-button>
            </div>
          </div>
        </template>
      </el-drawer>

      <el-drawer
        v-model="enableVisible"
        title="启用自动开服"
        direction="rtl"
        size="min(460px, 100vw)"
        class="policy-drawer"
        :close-on-click-modal="false"
        :close-on-press-escape="!changingId"
        :show-close="!changingId"
        @closed="enableTarget = undefined"
      >
        <template v-if="enableTarget">
          <section class="next-opening" aria-labelledby="next-opening-heading">
            <h3 id="next-opening-heading">下一次开服</h3>
            <strong>{{ enableTarget.name_prefix }}{{ enableTarget.first_server_id + enableTarget.completed }}</strong>
            <time :datetime="enableTarget.next_open_at">{{ formatTime(enableTarget.next_open_at) }}</time>
          </section>
          <el-alert
            v-if="Date.parse(enableTarget.next_open_at) <= Date.now()"
            class="enable-warning"
            title="计划时间已过，启用后会立即执行。"
            type="warning"
            :closable="false"
            show-icon
          />
          <dl class="enable-details">
            <div><dt>策略</dt><dd>{{ enableTarget.name }}</dd></div>
            <div><dt>部署范围</dt><dd>{{ projectName(enableTarget.project_id) }} / {{ environmentName(enableTarget.environment_id) }}</dd></div>
            <div><dt>Game 模板</dt><dd>{{ enableTarget.template_name }}</dd></div>
            <div><dt>开服间隔</dt><dd>{{ enableTarget.interval_days ? `每隔 ${enableTarget.interval_days} 天` : '单次' }}</dd></div>
            <div><dt>剩余区服</dt><dd>{{ enableTarget.count - enableTarget.completed }} 服</dd></div>
          </dl>
        </template>
        <template #footer>
          <div class="policy-footer-actions">
            <el-button :disabled="!!changingId" @click="enableVisible = false">取消</el-button>
            <el-button type="primary" :loading="!!changingId" :disabled="!enableTarget" @click="enableTarget && updateEnabled(enableTarget, true)">启用策略</el-button>
          </div>
        </template>
      </el-drawer>

      <el-dialog :model-value="!!recordsId" :title="records ? `${records.name} · 开服记录` : '开服记录'" width="min(750px, 94vw)" @update:model-value="recordsId = ''">
        <el-table :data="records?.recent_runs || []" empty-text="暂无开服记录" max-height="440">
          <el-table-column label="区服" prop="instance_name" min-width="100" />
          <el-table-column label="计划时间" min-width="150">
            <template #default="{ row }">{{ formatTime(row.open_at) }}</template>
          </el-table-column>
          <el-table-column label="完成时间" min-width="150">
            <template #default="{ row }">{{ formatTime(row.finished_at) }}</template>
          </el-table-column>
          <el-table-column label="结果" min-width="180">
            <template #default="{ row }">
              <el-tag size="small" :type="row.error ? 'danger' : 'success'">{{ row.error ? '失败' : '已部署' }}</el-tag>
              <small v-if="row.error" class="field-error">{{ row.error }}</small>
            </template>
          </el-table-column>
        </el-table>
      </el-dialog>
    </div>
  </AppLayout>
</template>

<style scoped>
.auto-open-page { width: 100%; min-width: 0; }
.overview-icon, .empty-icon { display: grid; flex: none; place-items: center; }
.policy-overview { display: grid; grid-template-columns: repeat(4, minmax(168px, 1fr)); gap: 12px; margin-bottom: 16px; overflow-x: auto; }
.overview-card { display: flex; align-items: center; gap: 10px; min-height: 66px; padding: 11px 14px; border: 1px solid var(--ops-border); border-radius: 12px; background: #fff; font: inherit; text-align: left; cursor: pointer; white-space: nowrap; }
.overview-card:hover { border-color: #b8cdfa; }
.overview-card[aria-pressed="true"] { border-color: #9cb9fa; background: #f7faff; }
.overview-card:focus-visible, .policy-table-scroll:focus-visible { outline: 2px solid var(--ops-primary); outline-offset: -2px; }
.overview-icon { width: 32px; height: 32px; border-radius: 9px; background: var(--ops-primary-light); color: var(--ops-primary); font-size: 18px; }
.overview-card[data-kind="completed"] .overview-icon { background: #e7f6ef; color: #20785e; }
.overview-card[data-kind="failed"] .overview-icon { background: #fceff0; color: #b73547; }
.overview-label { color: #566780; font-size: 13px; }
.overview-value { margin-left: auto; color: var(--ops-text); font-size: 22px; font-variant-numeric: tabular-nums; line-height: 1; }
.policy-list { position: relative; overflow: hidden; border: 1px solid var(--ops-border); border-radius: 14px; background: #fff; box-shadow: var(--ops-shadow); }
.policy-list-header { border-bottom: 1px solid var(--ops-border); }
.policy-search { width: 245px; min-width: 0; }
.policy-table-scroll { overflow-x: auto; }
.policy-table { width: 100%; min-width: 976px; border-collapse: collapse; table-layout: fixed; text-align: left; white-space: nowrap; }
.policy-table th, .policy-table td { overflow: hidden; padding: 13px 16px; text-overflow: ellipsis; }
.policy-table thead th { padding-block: 10px; border-bottom: 1px solid var(--ops-border); background: #f7f9fd; color: #586982; font-size: 12px; font-weight: 550; }
.policy-table thead th:first-child { width: 23%; }
.policy-table thead th:nth-child(2) { width: 11%; }
.policy-table thead th:nth-child(3) { width: 20%; }
.policy-table thead th:nth-child(4) { width: 16%; }
.policy-table thead th:nth-child(5) { width: 18%; }
.policy-table thead th:last-child { width: 130px; }
.policy-table tbody th, .policy-table tbody td { height: 57px; color: var(--ops-text); font-size: 13px; font-weight: 400; vertical-align: middle; }
.policy-table tbody tr + tr { border-top: 1px solid var(--ops-border); }
.policy-table tbody tr:hover { background: #f9fbff; }
.policy-line { display: flex; align-items: center; gap: 8px; min-width: 0; }
.policy-name, .policy-meta { overflow: hidden; text-overflow: ellipsis; }
.policy-name { font-weight: 650; }
.policy-meta { flex: 0 1 auto; color: #5e6f86; font-size: 12px; }
.policy-meta::before { margin-right: 7px; color: #8b9cb3; content: '·'; }
.template-name { display: inline-block; overflow: hidden; max-width: 100%; padding: 3px 8px; border-radius: 5px; background: #f0f3f9; color: #43536b; text-overflow: ellipsis; vertical-align: middle; }
.policy-next time, .policy-progress-count { font-variant-numeric: tabular-nums; }
.muted-value, .policy-progress-count { color: #5e6f86; }
.policy-status { --state-color: #586982; --state-bg: #eff2f6; }
.policy-status[data-kind="enabled"] { --state-color: #244fb8; --state-bg: #edf2ff; }
.policy-status[data-kind="completed"] { --state-color: #20785e; --state-bg: #e7f6ef; }
.policy-status[data-kind="failed"] { --state-color: #b73547; --state-bg: #fceff0; }
.state-label { display: inline-flex; flex: none; align-items: center; gap: 5px; padding: 4px 7px; border-radius: 20px; background: var(--state-bg); color: var(--state-color); font-size: 12px; line-height: 1.2; }
.state-label::before { width: 5px; height: 5px; border-radius: 50%; background: currentColor; content: ''; }
.policy-status progress { appearance: none; flex: 0 1 45px; min-width: 15px; width: 45px; height: 4px; overflow: hidden; border: 0; border-radius: 4px; background: #e8edf5; color: var(--state-color); }
.policy-status progress::-webkit-progress-bar { background: #e8edf5; }
.policy-status progress::-webkit-progress-value { border-radius: 4px; background: var(--state-color); }
.policy-status progress::-moz-progress-bar { background: var(--state-color); }
.policy-progress-count { font-size: 12px; }
.policy-actions :deep(.el-button + .el-button) { margin-left: 7px; }
.policy-empty { display: grid; justify-items: center; align-content: center; gap: 8px; min-height: 205px; padding: 24px 16px; text-align: center; }
.empty-icon { width: 44px; height: 44px; margin-bottom: 4px; border-radius: 13px; background: var(--ops-primary-light); color: var(--ops-primary); font-size: 21px; }
.policy-empty strong { font-size: 14px; font-weight: 600; }
.policy-empty p { margin: 0; color: var(--ops-text-secondary); font-size: 12px; }
.load-error { margin-bottom: 12px; }
.policy-overview::-webkit-scrollbar, .policy-table-scroll::-webkit-scrollbar { display: block; height: 5px; }
.policy-overview::-webkit-scrollbar-thumb, .policy-table-scroll::-webkit-scrollbar-thumb { border-radius: 5px; background: #cbd7e9; }
small { display: block; margin-top: 4px; color: var(--ops-text-secondary); font-size: 11px; line-height: 1.5; }
.field-error { color: var(--el-color-danger); white-space: normal; overflow-wrap: anywhere; }
.auto-open-page :deep(.policy-drawer .el-drawer__header) { margin-bottom: 0; padding: 21px 24px 18px; border-bottom: 1px solid var(--ops-border); }
.auto-open-page :deep(.policy-drawer .el-drawer__title) { color: var(--ops-text); font-size: 18px; font-weight: 650; }
.auto-open-page :deep(.policy-drawer .el-drawer__body) { padding: 20px 24px; background: #f7f9fd; }
.auto-open-page :deep(.policy-drawer .el-drawer__footer) { padding: 14px 24px; padding-bottom: max(14px, env(safe-area-inset-bottom)); border-top: 1px solid var(--ops-border); }
.policy-form { display: flex; flex-direction: column; gap: 12px; }
.policy-section, .startup-parameters { padding: 15px 16px; border: 1px solid var(--ops-border); border-radius: 11px; background: #fff; }
.policy-section { display: flex; flex-direction: column; gap: 14px; }
.policy-section h3 { margin: 0; color: var(--ops-text); font-size: 14px; font-weight: 650; }
.policy-section :deep(.el-form-item) { min-width: 0; margin-bottom: 0; }
.policy-fields { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); align-items: start; gap: 14px 16px; }
.policy-form :deep(.el-input-number), .policy-form :deep(.el-date-editor), .policy-form :deep(.el-select) { width: 100%; }
.startup-parameters summary { color: var(--ops-text); cursor: pointer; font-size: 14px; font-weight: 650; }
.startup-parameters p { margin: 8px 0 10px; color: var(--ops-text-secondary); font-size: 12px; line-height: 1.55; }
.startup-parameters dl { margin: 0; border: 1px solid var(--ops-border); border-radius: 8px; }
.startup-parameters dl > div { display: grid; grid-template-columns: minmax(0, 42%) minmax(0, 1fr); gap: 8px; align-items: baseline; padding: 7px 10px; }
.startup-parameters dl > div + div { border-top: 1px solid var(--ops-border); }
.startup-parameters dt, .startup-parameters dd { min-width: 0; overflow-wrap: anywhere; }
.startup-parameters dd { margin: 0; color: var(--ops-text); font-size: 12px; }
.startup-parameters dd.field-muted { color: var(--ops-text-secondary); }
.startup-parameters dd small { display: inline; margin-left: 3px; }
.policy-footer { display: flex; flex-wrap: wrap; align-items: center; gap: 8px 12px; }
.policy-footer > small { margin: 0; }
.policy-footer-actions { display: flex; justify-content: flex-end; margin-left: auto; }
.next-opening { display: grid; gap: 7px; border: 1px solid #dce7ff; border-radius: 11px; padding: 15px 16px; background: linear-gradient(120deg, #edf3ff, #fff); }
.next-opening h3 { margin: 0; color: var(--ops-text-secondary); font-size: 12px; font-weight: 500; }
.next-opening strong { color: var(--ops-text); font-size: 19px; overflow-wrap: anywhere; }
.next-opening time { color: var(--ops-text-secondary); font-size: 13px; font-variant-numeric: tabular-nums; }
.enable-warning { margin-top: 12px; }
.enable-details { margin: 16px 0 0; padding: 3px 15px; border: 1px solid var(--ops-border); border-radius: 11px; background: #fff; }
.enable-details > div { display: grid; grid-template-columns: 85px minmax(0, 1fr); gap: 10px; padding: 10px 0; font-size: 13px; }
.enable-details > div + div { border-top: 1px solid var(--ops-border); }
.enable-details dt { color: var(--ops-text-secondary); }
.enable-details dd { margin: 0; color: var(--ops-text); overflow-wrap: anywhere; }
@media (max-width: 400px) {
  .auto-open-page :deep(.policy-drawer .el-drawer__body) { padding-inline: 12px; }
  .policy-fields { grid-template-columns: minmax(0, 1fr); }
}
</style>
