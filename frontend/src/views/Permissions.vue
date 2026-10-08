<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { Close, FolderOpened, Key, Plus, Refresh, Search, User } from '@element-plus/icons-vue'
import AppLayout from '../components/AppLayout.vue'
import { projectApi, type Project } from '../api/projects'
import { userApi, type ManagedUser, type UserAccessForm, type UserPermission } from '../api/users'

const users = ref<ManagedUser[]>([])
const projects = ref<Project[]>([])
const loading = ref(false)
const saving = ref(false)
const searchKeyword = ref('')
const dialogVisible = ref(false)
const passwordDialogVisible = ref(false)
const editingUserId = ref('')
const passwordTarget = ref<ManagedUser | null>(null)
const detailVisible = ref(false)
const permissionEditing = ref(false)
const activePermissionUser = ref<ManagedUser | null>(null)
const passwordForm = reactive({ password: '' })
const form = reactive<UserAccessForm & { password: string }>({ username: '', role: 'user', project_ids: [], permissions: [], disabled: false, password: '' })
const permissionTreeRef = ref<any>()
const permissionTreeProps = { label: 'label', children: 'children' }

const projectNameById = computed(() => new Map(projects.value.map((project) => [project.id, project.name])))
const permissionOptions: { value: UserPermission; label: string; description: string }[] = [
  { value: 'projects.view', label: '查看项目', description: '查看授权项目及环境概况' },
  { value: 'services.view', label: '查看服务', description: '查看实例、镜像、运行状态、日志和事件' },
  { value: 'services.manage', label: '管理服务', description: '部署、编辑、更新镜像及控制容器' },
  { value: 'hosts.view', label: '查看主机', description: '查看授权项目关联的主机信息' },
  { value: 'hosts.manage', label: '管理主机', description: '配置或删除主机并测试连接' },
]
const permissionTree = [
  {
    id: 'projects', label: '项目管理', description: '项目范围与项目信息', children: [
      { id: 'projects.view', label: '查看项目', description: '查看授权项目及环境概况' },
    ],
  },
  {
    id: 'services', label: '服务管理', description: '实例、镜像及运行操作', children: [
      { id: 'services.view', label: '查看服务', description: '查看实例、镜像、运行状态、日志和事件' },
      { id: 'services.manage', label: '管理服务', description: '部署、更新镜像及控制容器' },
    ],
  },
  {
    id: 'hosts', label: '主机管理', description: '主机信息与连接配置', children: [
      { id: 'hosts.view', label: '查看主机', description: '查看授权项目关联的主机信息' },
      { id: 'hosts.manage', label: '管理主机', description: '配置、删除主机并测试连接' },
    ],
  },
]

function projectLabels(user: ManagedUser) {
  if (user.role === 'admin') return '全部项目'
  return user.project_ids.map((id) => projectNameById.value.get(id) || '已删除项目').join('、') || '未分配项目'
}
function permissionLabels(user: ManagedUser) {
  if (user.role === 'admin') return ['全部权限']
  return user.permissions.map((permission) => permissionOptions.find((option) => option.value === permission)?.label || permission)
}
const filteredUsers = computed(() => {
  const query = searchKeyword.value.trim().toLocaleLowerCase()
  if (!query) return users.value
  return users.value.filter((user) => {
    const searchableText = [
      user.username,
      user.role === 'admin' ? '平台管理员 管理员' : '普通用户',
      projectLabels(user),
      ...permissionLabels(user),
      user.disabled ? '已停用' : '正常 启用',
    ].join(' ').toLocaleLowerCase()
    return searchableText.includes(query)
  })
})
function permissionTreeLabels(user: ManagedUser): UserPermission[] {
  if (user.role === 'admin') return permissionOptions.map((option) => option.value)
  return user.permissions || []
}
function openPermissionDetails(user: ManagedUser) {
  activePermissionUser.value = user
  permissionEditing.value = false
  editingUserId.value = ''
  detailVisible.value = true
}
function onPermissionCheck() {
  form.permissions = (permissionTreeRef.value?.getCheckedKeys(true) || []) as UserPermission[]
}
function resetForm() {
  Object.assign(form, { username: '', role: 'user', project_ids: [], permissions: [], disabled: false, password: '' })
  editingUserId.value = ''
}
function openCreate() { resetForm(); permissionEditing.value = false; dialogVisible.value = true }
function openEdit(user: ManagedUser) {
  resetForm()
  editingUserId.value = user.id
  Object.assign(form, { username: user.username, role: user.role, project_ids: [...(user.project_ids || [])], permissions: [...(user.permissions || [])], disabled: user.disabled })
  activePermissionUser.value = user
  permissionEditing.value = true
  detailVisible.value = true
}
function openPassword(user: ManagedUser) {
  passwordTarget.value = user
  passwordForm.password = ''
  passwordDialogVisible.value = true
}
function apiError(error: any, fallback: string) { return error?.response?.data?.error || fallback }
async function loadAll() {
  loading.value = true
  try { [users.value, projects.value] = await Promise.all([userApi.list(), projectApi.list()]) }
  catch (error) { ElMessage.error(apiError(error, '用户权限加载失败')) }
  finally { loading.value = false }
}
async function saveUser() {
  if (!form.username.trim()) { ElMessage.warning('请输入用户名'); return }
  if (!editingUserId.value && form.password.length < 8) { ElMessage.warning('初始密码至少需要 8 位'); return }
  if (form.role === 'user' && !form.project_ids.length) { ElMessage.warning('请至少分配一个项目'); return }
  if (form.role === 'user' && !form.permissions.length) { ElMessage.warning('请至少授予一项功能权限'); return }
  saving.value = true
  try {
    const payload = { ...form, username: form.username.trim(), project_ids: form.role === 'admin' ? [] : form.project_ids, permissions: form.role === 'admin' ? [] : form.permissions }
    if (editingUserId.value) await userApi.update(editingUserId.value, payload)
    else await userApi.create(payload)
    const editingInDrawer = permissionEditing.value
    ElMessage.success(editingUserId.value ? '用户权限已更新' : '用户已创建')
    if (!editingInDrawer) dialogVisible.value = false
    await loadAll()
    if (editingInDrawer && editingUserId.value) {
      activePermissionUser.value = users.value.find((user) => user.id === editingUserId.value) || activePermissionUser.value
      permissionEditing.value = false
      editingUserId.value = ''
    }
  } catch (error) { ElMessage.error(apiError(error, '保存用户失败')) }
  finally { saving.value = false }
}
async function savePassword() {
  if (!passwordTarget.value || passwordForm.password.length < 8) { ElMessage.warning('新密码至少需要 8 位'); return }
  saving.value = true
  try {
    await userApi.resetPassword(passwordTarget.value.id, passwordForm.password)
    ElMessage.success('密码已重置')
    passwordDialogVisible.value = false
  } catch (error) { ElMessage.error(apiError(error, '重置密码失败')) }
  finally { saving.value = false }
}
onMounted(loadAll)
</script>

<template>
  <AppLayout>
    <div class="access-page page-content-loading" v-loading="loading">
      <el-card class="users-card" shadow="never">
        <div class="table-heading"><div class="table-heading-copy"><h2>账号列表</h2></div><div class="table-tools"><el-input v-model="searchKeyword" class="user-search" clearable :prefix-icon="Search" placeholder="搜索用户、项目或权限" /><el-button class="refresh-button" text :icon="Refresh" :loading="loading" @click="loadAll">刷新</el-button><el-button class="add-user-button" type="primary" :icon="Plus" @click="openCreate">添加用户</el-button></div></div>
        <el-table class="access-table" :data="filteredUsers" row-key="id" :empty-text="searchKeyword ? '没有匹配的用户' : '暂无用户'">
          <el-table-column label="用户" min-width="150"><template #default="{ row }"><div class="user-cell"><span class="user-avatar">{{ row.username.slice(0, 1).toUpperCase() }}</span><span><b>{{ row.username }}</b><small>{{ row.role === 'admin' ? '平台管理员' : '普通用户' }}</small></span></div></template></el-table-column>
          <el-table-column label="项目范围" min-width="190"><template #default="{ row }"><span class="scope-text">{{ projectLabels(row) }}</span></template></el-table-column>
          <el-table-column label="功能权限" min-width="190"><template #default="{ row }"><button class="permission-summary" @click="openPermissionDetails(row)"><span class="permission-summary-icon"><el-icon><Key /></el-icon></span><span><b>{{ row.role === 'admin' ? '全部权限' : `${row.permissions.length} 项权限` }}</b><small>{{ row.role === 'admin' ? '项目 · 服务 · 主机' : permissionLabels(row).slice(0, 2).join('、') || '未配置' }}</small></span></button></template></el-table-column>
          <el-table-column label="状态" width="95"><template #default="{ row }"><el-tag :type="row.disabled ? 'info' : 'success'" size="small" effect="light">{{ row.disabled ? '已停用' : '正常' }}</el-tag></template></el-table-column>
          <el-table-column label="操作" width="100" fixed="right"><template #default="{ row }"><el-button link @click="openPassword(row)">重置密码</el-button></template></el-table-column>
        </el-table>
      </el-card>
    </div>

    <el-dialog v-model="dialogVisible" width="860px" destroy-on-close class="access-dialog">
      <template #header>
        <div class="access-dialog-heading">
          <span class="dialog-heading-icon"><el-icon><User /></el-icon></span>
          <span class="dialog-heading-copy"><b>{{ editingUserId ? '编辑用户权限' : '添加用户' }}</b><small>{{ editingUserId ? '调整账号状态、项目范围和功能授权' : '创建账号并配置可访问的项目与功能' }}</small></span>
          <el-tag size="small" effect="plain">{{ editingUserId ? '用户设置' : '新用户' }}</el-tag>
        </div>
      </template>
      <el-form label-position="top" class="access-form">
        <div class="form-row">
          <el-form-item label="用户名" required><el-input v-model="form.username" :disabled="Boolean(editingUserId)" placeholder="3-64 位字母、数字、点、下划线或短横线" /></el-form-item>
          <el-form-item v-if="!editingUserId" label="初始密码" required><el-input v-model="form.password" type="password" show-password placeholder="至少 8 位" /></el-form-item>
        </div>
        <div class="form-row role-row">
          <el-form-item label="账号角色"><el-select v-model="form.role" style="width: 100%"><el-option label="普通用户" value="user" /><el-option label="平台管理员" value="admin" /></el-select></el-form-item>
          <el-form-item label="账号状态"><div class="switch-control"><el-switch v-model="form.disabled" active-text="停用账号" inactive-text="正常使用" /><span class="switch-hint">{{ form.disabled ? '该用户无法登录控制台' : '账号可正常登录并使用授权功能' }}</span></div></el-form-item>
        </div>
        <template v-if="form.role === 'user'">
          <div class="access-section-heading"><span class="section-index">01</span><span><b>项目访问范围</b><small>先选择该用户可以访问的数据项目</small></span></div>
          <el-form-item label="可访问项目" required class="project-scope-item"><div class="scope-field"><el-select v-model="form.project_ids" multiple filterable collapse-tags collapse-tags-tooltip placeholder="选择该用户可访问的项目" style="width: 100%"><el-option v-for="project in projects" :key="project.id" :label="project.name" :value="project.id" /></el-select><div class="form-hint"><el-icon><Key /></el-icon> 项目范围会同步限制项目、服务和主机数据的可见范围。</div></div></el-form-item>
          <div class="access-section-heading permission-section-heading"><span class="section-index">02</span><span><b>功能权限</b><small>按模块勾选可查看或管理的功能</small></span></div>
          <el-form-item label="功能权限" class="permission-tree-item"><div class="permission-tree-panel"><div class="tree-panel-heading"><div><b>授权权限树</b><span>新增用户默认未授权，展开模块后选择具体权限。</span></div><span class="selected-count">已选 {{ form.permissions.length }} 项</span></div><el-tree ref="permissionTreeRef" :key="`${editingUserId || 'new'}-${dialogVisible}`" :data="permissionTree" node-key="id" :props="permissionTreeProps" :default-checked-keys="form.permissions" :check-strictly="false" :default-expand-all="false" :expand-on-click-node="false" show-checkbox class="permission-tree" @check="onPermissionCheck"><template #default="{ data }"><span class="permission-tree-label" :class="{ 'is-module': data.children }"><span class="tree-node-icon"><el-icon><FolderOpened v-if="data.children" /><Key v-else /></el-icon></span><span class="tree-node-copy"><b>{{ data.label }}</b></span><el-tag v-if="data.id.endsWith('.manage')" size="small" type="warning" effect="light">管理</el-tag><el-tag v-else-if="!data.children" size="small" type="info" effect="plain">查看</el-tag></span></template></el-tree></div></el-form-item>
        </template>
        <div v-else class="admin-notice"><span class="admin-notice-icon"><el-icon><Key /></el-icon></span><span><b>平台管理员</b><small>拥有全部项目范围及项目、服务、主机的全部权限。</small></span><el-tag type="success" effect="light">全部授权</el-tag></div>
      </el-form>
      <template #footer><el-button @click="dialogVisible = false">取消</el-button><el-button type="primary" :loading="saving" @click="saveUser">保存</el-button></template>
    </el-dialog>

    <el-dialog v-model="passwordDialogVisible" title="重置登录密码" width="440px">
      <el-form label-position="top"><el-form-item :label="`为 ${passwordTarget?.username || ''} 设置新密码`"><el-input v-model="passwordForm.password" type="password" show-password placeholder="至少 8 位" /></el-form-item></el-form>
      <template #footer><el-button @click="passwordDialogVisible = false">取消</el-button><el-button type="primary" :loading="saving" @click="savePassword">重置密码</el-button></template>
    </el-dialog>

    <el-drawer v-model="detailVisible" direction="rtl" size="520px" :with-header="false">
      <div v-if="activePermissionUser" class="permission-detail-panel">
        <div class="permission-detail-header">
          <div class="detail-header-main"><span class="permission-detail-icon"><el-icon><User /></el-icon></span><div class="detail-header-info"><div class="detail-title">用户权限</div><div class="detail-header-name">{{ activePermissionUser.username }}</div></div><el-button class="detail-close-button" circle :icon="Close" aria-label="关闭用户权限详情" @click="detailVisible = false" /></div>
          <div class="permission-detail-badges"><el-tag :type="activePermissionUser.disabled ? 'info' : 'success'" effect="light">{{ activePermissionUser.disabled ? '已停用' : '正常' }}</el-tag><el-tag effect="plain">{{ activePermissionUser.role === 'admin' ? '平台管理员' : '普通用户' }}</el-tag></div>
        </div>
        <el-form v-if="permissionEditing" class="permission-detail-scroll permission-inline-form" label-position="top">
          <div class="detail-section-title">账号设置</div>
          <el-form-item label="用户名"><el-input v-model="form.username" disabled /></el-form-item>
          <el-form-item label="账号角色"><el-select v-model="form.role" style="width: 100%"><el-option label="普通用户" value="user" /><el-option label="平台管理员" value="admin" /></el-select></el-form-item>
          <el-form-item label="账号状态"><div class="switch-control"><el-switch v-model="form.disabled" active-text="停用账号" inactive-text="正常使用" /></div></el-form-item>
          <template v-if="form.role === 'user'">
            <div class="detail-section-title permission-detail-section">项目范围</div>
            <el-form-item label="可访问项目"><el-select v-model="form.project_ids" multiple filterable collapse-tags collapse-tags-tooltip placeholder="选择该用户可访问的项目" style="width: 100%"><el-option v-for="project in projects" :key="project.id" :label="project.name" :value="project.id" /></el-select></el-form-item>
            <div class="detail-section-title permission-detail-section">功能权限</div>
            <div class="permission-tree-panel drawer-permission-tree"><div class="tree-panel-heading"><div><b>授权权限树</b><span>按需勾选模块权限</span></div><span class="selected-count">已选 {{ form.permissions.length }} 项</span></div><el-tree ref="permissionTreeRef" :key="`${editingUserId}-${permissionEditing}`" :data="permissionTree" node-key="id" :props="permissionTreeProps" :default-checked-keys="form.permissions" :check-strictly="false" :default-expand-all="false" :expand-on-click-node="false" show-checkbox class="permission-tree" @check="onPermissionCheck"><template #default="{ data }"><span class="permission-tree-label" :class="{ 'is-module': data.children }"><span class="tree-node-icon"><el-icon><FolderOpened v-if="data.children" /><Key v-else /></el-icon></span><span class="tree-node-copy"><b>{{ data.label }}</b></span><el-tag v-if="data.id.endsWith('.manage')" size="small" type="warning" effect="light">管理</el-tag><el-tag v-else-if="!data.children" size="small" type="info" effect="plain">查看</el-tag></span></template></el-tree></div>
          </template>
          <div v-else class="admin-notice drawer-admin-notice"><span class="admin-notice-icon"><el-icon><Key /></el-icon></span><span><b>平台管理员</b><small>拥有全部项目范围及功能权限。</small></span><el-tag type="success" effect="light">全部授权</el-tag></div>
        </el-form>
        <div v-else class="permission-detail-scroll">
          <div class="detail-section-title">项目范围</div>
          <div class="permission-detail-card"><div v-if="activePermissionUser.role === 'admin'" class="permission-detail-project"><span>全部项目</span><el-tag type="success" size="small" effect="light">不限范围</el-tag></div><div v-else-if="activePermissionUser.project_ids.length" class="permission-detail-project" v-for="id in activePermissionUser.project_ids" :key="id"><span>{{ projectNameById.get(id) || '已删除项目' }}</span><el-icon><FolderOpened /></el-icon></div><div v-else class="permission-detail-empty">未分配项目</div></div>
          <div class="detail-section-title permission-detail-section">功能权限</div>
          <div class="permission-detail-card permission-detail-tree-card"><el-tree :data="permissionTree" node-key="id" :props="permissionTreeProps" :default-checked-keys="permissionTreeLabels(activePermissionUser)" :check-strictly="false" :default-expand-all="true" :expand-on-click-node="false" show-checkbox class="readonly-permission-tree"><template #default="{ data }"><span class="readonly-tree-label" :class="{ 'tree-group-label': data.children }"><span>{{ data.label }}</span></span></template></el-tree></div>
          <div class="detail-section-title permission-detail-section">账号信息</div>
          <div class="permission-detail-kv"><div><span>创建时间</span><b>{{ activePermissionUser.created_at || '—' }}</b></div><div><span>授权项目</span><b>{{ activePermissionUser.role === 'admin' ? '全部项目' : `${activePermissionUser.project_ids.length} 个` }}</b></div><div><span>功能权限</span><b>{{ activePermissionUser.role === 'admin' ? '全部权限' : `${activePermissionUser.permissions.length} 项` }}</b></div></div>
        </div>
        <div class="permission-detail-footer"><template v-if="permissionEditing"><el-button @click="permissionEditing = false; editingUserId = ''">取消</el-button><el-button type="primary" :loading="saving" @click="saveUser">保存</el-button></template><el-button v-else type="primary" :icon="Key" @click="openEdit(activePermissionUser!)">编辑权限</el-button></div>
      </div>
    </el-drawer>
  </AppLayout>
</template>

<style scoped>
.access-page { width: 100%; max-width: none; margin: 0; }
.access-dialog-heading { display: flex; align-items: center; gap: 12px; padding: 4px 2px 14px; border-bottom: 1px solid #edf0f5; }
.dialog-heading-icon { display: grid; flex: 0 0 42px; width: 42px; height: 42px; place-items: center; border: 1px solid #dce6ff; border-radius: 12px; background: linear-gradient(145deg, #f1f5ff, #e7edff); color: var(--ops-primary); font-size: 19px; }
.dialog-heading-copy { display: flex; min-width: 0; flex: 1; flex-direction: column; gap: 5px; }
.dialog-heading-copy b { color: #293a53; font-size: 16px; font-weight: 660; }
.dialog-heading-copy small { color: #8290a4; font-size: 11px; }
.access-dialog-heading :deep(.el-tag) { border-radius: 20px; }
.access-dialog :deep(.el-dialog__header) { margin-right: 0; padding: 20px 24px 0; }
.access-dialog :deep(.el-dialog__headerbtn) { top: 20px; right: 22px; }
.access-dialog :deep(.el-dialog__body) { padding: 20px 24px 8px; }
.access-dialog :deep(.el-dialog__footer) { padding: 14px 24px 20px; border-top: 1px solid #edf0f5; background: #fcfdff; }
.access-dialog :deep(.el-dialog__footer .el-button) { min-width: 86px; height: 36px; border-radius: 7px; }
.access-heading { display: flex; justify-content: space-between; align-items: center; gap: 18px; margin: 0 0 24px; }
.heading-copy { min-width: 0; }
.access-heading h1 { margin: 0 0 7px; font-size: 25px; font-weight: 680; letter-spacing: -.35px; }
.access-heading p { margin: 0; color: var(--ops-text-secondary); font-size: 13px; }
.eyebrow { display: flex; align-items: center; gap: 7px; margin-bottom: 7px; color: #8390a3; font-size: 10px; font-weight: 650; letter-spacing: 1.7px; }
.eyebrow-mark { width: 6px; height: 6px; border-radius: 50%; background: var(--ops-primary); box-shadow: 0 0 0 3px var(--ops-primary-light); }
.add-user-button { height: 38px; padding: 0 16px; border-radius: 7px; box-shadow: 0 5px 12px rgba(56, 107, 232, .16); }
.access-stats { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 14px; margin-bottom: 18px; }
.access-stats > div { display: flex; justify-content: space-between; align-items: center; min-height: 86px; padding: 16px 19px; border: 1px solid var(--ops-border); border-radius: 10px; background: linear-gradient(135deg, #fff 55%, #f9fbff); box-shadow: 0 2px 7px rgba(33, 52, 84, .025); }
.access-stats > div > span { display: flex; align-items: center; gap: 10px; color: var(--ops-text-secondary); font-size: 13px; }
.access-stats .stat-icon { display: grid; width: 36px; height: 36px; place-items: center; border-radius: 9px; font-style: normal; font-size: 16px; }
.stat-total .stat-icon { color: #386be8; background: #edf2ff; }
.stat-enabled .stat-icon { color: #239477; background: #e8f7f1; }
.stat-admin .stat-icon { color: #b17c22; background: #fff5df; }
.access-stats strong { font-size: 23px; font-weight: 670; font-variant-numeric: tabular-nums; }
.users-card { border-color: var(--ops-border); border-radius: 10px; box-shadow: 0 2px 8px rgba(28, 47, 78, .025); }
.users-card :deep(.el-card__body) { padding: 0 20px 16px; }
.table-heading { display: flex; justify-content: space-between; align-items: center; gap: 12px; padding: 18px 0 15px; }
.table-heading-copy { display: flex; flex-direction: column; gap: 4px; }
.table-tools { display: flex; min-width: 0; align-items: center; gap: 8px; }
.user-search { width: 260px; }
.user-search :deep(.el-input__wrapper) { min-height: 34px; border-radius: 7px; box-shadow: 0 0 0 1px #e2e8f0 inset; }
.section-kicker { color: #9aa5b5 !important; font-size: 9px !important; font-weight: 650; letter-spacing: 1.35px; }
.table-heading h2 { margin: 0; color: var(--ops-text); font-size: 15px; font-weight: 650; }
.table-heading-copy > span:last-child { color: var(--ops-text-secondary); font-size: 12px; }
.refresh-button { color: #62738d; }
.access-table { width: 100%; }
.access-table :deep(th.el-table__cell) { height: 43px; background: #f8fafd; color: #77859a; font-size: 11px; font-weight: 600; }
.access-table :deep(td.el-table__cell) { height: 65px; }
.access-table :deep(.el-table__inner-wrapper::before) { height: 0; }
.user-cell { display: flex; align-items: center; gap: 10px; }
.user-avatar { display: grid; width: 34px; height: 34px; place-items: center; border: 1px solid #dce6ff; border-radius: 10px; background: linear-gradient(145deg, #f2f5ff, #e9efff); color: var(--ops-primary); font-size: 12px; font-weight: 650; }
.user-cell b, .user-cell small { display: block; }
.user-cell b { font-size: 13px; font-weight: 600; }
.user-cell small { margin-top: 3px; color: var(--ops-text-secondary); font-size: 11px; }
.scope-text { display: block; max-width: 300px; overflow: hidden; color: #53647c; font-size: 12px; text-overflow: ellipsis; white-space: nowrap; }
.permission-summary { display: flex; align-items: center; width: 100%; max-width: 245px; gap: 9px; padding: 5px 7px; border: 1px solid transparent; border-radius: 8px; background: transparent; color: inherit; text-align: left; cursor: pointer; transition: background .15s, border-color .15s; }
.permission-summary:hover { border-color: #e5ebf5; background: #f8faff; }
.permission-summary-icon { display: grid; flex: 0 0 29px; width: 29px; height: 29px; place-items: center; border-radius: 8px; background: #f1f4fb; color: #687c9a; }
.permission-summary > span:nth-child(2) { min-width: 0; flex: 1; }
.permission-summary b, .permission-summary small { display: block; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.permission-summary b { font-size: 12px; font-weight: 600; }
.permission-summary small { margin-top: 3px; color: var(--ops-text-secondary); font-size: 10px; }
.summary-arrow { color: #a4afbf; font-size: 12px; }
.scope-text { line-height: 1.5; }
.access-table :deep(.el-button.is-link) { font-size: 12px; }
.form-row { display: grid; grid-template-columns: 1fr 1fr; gap: 16px; }
.access-form :deep(.el-form-item) { margin-bottom: 19px; }
.access-form :deep(.el-form-item__label) { padding-bottom: 7px; color: #36465e; font-size: 12px; font-weight: 600; }
.access-form :deep(.el-input__wrapper), .access-form :deep(.el-select__wrapper) { min-height: 38px; border-radius: 7px; box-shadow: 0 0 0 1px #e2e8f0 inset; }
.access-form :deep(.el-input__wrapper.is-focus), .access-form :deep(.el-select__wrapper.is-focused) { box-shadow: 0 0 0 1px var(--ops-primary) inset, 0 0 0 3px rgba(56, 107, 232, .08); }
.switch-control { display: flex; min-height: 32px; align-items: center; gap: 12px; }
.switch-hint { color: var(--ops-text-secondary); font-size: 11px; }
.access-section-heading { display: flex; align-items: center; gap: 10px; margin: 4px 0 10px; }
.access-section-heading .section-index { display: grid; width: 25px; height: 25px; place-items: center; border-radius: 7px; background: #edf2ff; color: var(--ops-primary); font-size: 10px; font-weight: 700; }
.access-section-heading > span:last-child { display: flex; flex-direction: column; gap: 3px; }
.access-section-heading b { color: #35465f; font-size: 12px; font-weight: 650; }
.access-section-heading small { color: #8995a6; font-size: 10px; }
.permission-section-heading { margin-top: 5px; }
.scope-field { width: 100%; }
.form-hint { display: flex; align-items: center; gap: 4px; margin-top: 7px; color: #8290a4; font-size: 11px; line-height: 1.5; }
.form-hint .el-icon { color: var(--ops-primary); }
.permission-tree-panel { width: 100%; overflow: hidden; border: 1px solid #e4eaf3; border-radius: 9px; background: #fbfcfe; }
.tree-panel-heading { display: flex; justify-content: space-between; align-items: center; gap: 12px; padding: 13px 15px; border-bottom: 1px solid #e9edf4; background: #fff; }
.tree-panel-heading > div { display: flex; flex-direction: column; gap: 4px; }
.tree-panel-heading b { color: #2b3b53; font-size: 12px; font-weight: 650; }
.tree-panel-heading span { color: #8693a5; font-size: 10px; }
.tree-panel-heading .selected-count { flex-shrink: 0; padding: 4px 8px; border-radius: 20px; background: #edf2ff; color: var(--ops-primary); font-size: 10px; }
.permission-tree { padding: 8px 12px 11px; background: transparent; }
.permission-tree :deep(.el-tree-node__content) { min-height: 42px; height: auto; border-radius: 6px; }
.permission-tree :deep(.el-tree-node__content:hover) { background: #f2f6ff; }
.permission-tree :deep(.el-tree-node__expand-icon) { color: #96a2b3; }
.permission-tree :deep(.el-checkbox) { margin-right: 8px; }
.permission-tree-label { display: flex; width: 100%; align-items: center; gap: 9px; padding: 4px 8px 4px 0; }
.tree-node-icon { display: grid; flex: 0 0 26px; width: 26px; height: 26px; place-items: center; border-radius: 7px; background: #eef2fa; color: #687b98; }
.permission-tree-label.is-module .tree-node-icon { background: #eaf0ff; color: var(--ops-primary); }
.tree-node-copy { display: flex; min-width: 0; flex: 1; flex-direction: column; gap: 3px; }
.tree-node-copy b { color: #35455d; font-size: 11px; font-weight: 600; }
.tree-node-copy small { overflow: hidden; color: #8a96a7; font-size: 10px; text-overflow: ellipsis; white-space: nowrap; }
.permission-tree-label :deep(.el-tag) { margin-right: 5px; }
.admin-notice { display: flex; align-items: center; gap: 12px; padding: 15px; border: 1px solid #dcefe6; border-radius: 9px; background: #f5fbf7; }
.admin-notice-icon { display: grid; flex: 0 0 34px; width: 34px; height: 34px; place-items: center; border-radius: 9px; background: #e3f4eb; color: #279071; }
.admin-notice > span:nth-child(2) { display: flex; min-width: 0; flex: 1; flex-direction: column; gap: 4px; }
.admin-notice b { color: #325546; font-size: 12px; }
.admin-notice small { color: #718b7f; font-size: 11px; }
.permission-popover { padding: 1px 2px 3px; }
.popover-title { display: flex; justify-content: space-between; align-items: center; margin: 1px 1px 9px; color: #33445d; font-size: 12px; font-weight: 650; }
.readonly-permission-tree { background: transparent; pointer-events: none; }
.readonly-permission-tree :deep(.el-tree-node__content) { height: 30px; border-radius: 5px; }
.readonly-permission-tree :deep(.el-checkbox.is-disabled.is-checked .el-checkbox__inner) { border-color: var(--ops-primary); background-color: var(--ops-primary); }
.readonly-permission-tree :deep(.el-checkbox.is-disabled.is-checked .el-checkbox__inner::after) { border-color: #fff; }
.readonly-permission-tree :deep(.el-checkbox.is-disabled .el-checkbox__label) { color: #485a72; }
.readonly-tree-label { display: flex; align-items: center; gap: 8px; font-size: 11px; }
.readonly-tree-label small { color: #8b97a8; font-size: 10px; }
.readonly-tree-label.tree-group-label { font-weight: 600; }
@media (max-width: 760px) {
  .access-heading { align-items: flex-start; }
  .users-card :deep(.el-card__body) { padding: 0 12px 12px; }
  .access-table :deep(.el-table__body-wrapper) { overflow-x: auto; }
  .scope-text { max-width: 190px; }
  .user-search { width: 220px; }
  :deep(.el-dialog) { width: calc(100vw - 24px) !important; margin-top: 3vh !important; }
}
@media (max-width: 540px) {
  .access-heading { gap: 12px; margin-bottom: 17px; }
  .access-heading h1 { font-size: 21px; }
  .access-heading p { max-width: 250px; line-height: 1.5; }
  .add-user-button { height: 34px; padding: 0 10px; font-size: 12px; }
  .table-heading { padding: 15px 0 12px; }
  .table-tools { flex: 1; }
  .user-search { width: auto; min-width: 0; flex: 1; }
  .form-row { grid-template-columns: 1fr; gap: 0; }
  .role-row { gap: 0; }
  .access-dialog :deep(.el-dialog__header) { padding: 16px 16px 0; }
  .access-dialog :deep(.el-dialog__headerbtn) { top: 14px; right: 12px; }
  .access-dialog :deep(.el-dialog__body) { padding: 16px 16px 5px; }
  .access-dialog :deep(.el-dialog__footer) { padding: 12px 16px 16px; }
  .access-dialog-heading { gap: 9px; padding-bottom: 12px; }
  .dialog-heading-icon { flex-basis: 36px; width: 36px; height: 36px; border-radius: 10px; font-size: 17px; }
  .dialog-heading-copy b { font-size: 14px; }
  .dialog-heading-copy small { max-width: 220px; line-height: 1.4; }
  .tree-panel-heading { align-items: flex-start; padding: 11px; }
  .tree-panel-heading > div > span { max-width: 205px; line-height: 1.5; }
  .tree-node-copy small { max-width: 200px; }
  .permission-tree { padding: 6px 7px 8px; }
}
.permission-detail-panel { display: flex; width: 100%; height: 100%; min-height: 0; flex-direction: column; background: #fafcff; }
:deep(.el-drawer__body) { padding: 0; overflow: hidden; }
.permission-detail-header { display: flex; flex-direction: column; gap: 13px; padding: 18px; border-bottom: 1px solid var(--ops-border); background: #fff; }
.permission-detail-header .detail-header-main { display: flex; min-width: 0; align-items: center; gap: 12px; }
.permission-detail-header .detail-header-info { flex: 1; min-width: 0; }
.permission-detail-header .detail-title { margin-bottom: 4px; color: var(--ops-text-secondary); font-size: 12px; }
.permission-detail-header .detail-header-name { overflow: hidden; color: var(--ops-text); font-size: 16px; font-weight: 700; text-overflow: ellipsis; white-space: nowrap; }
.permission-detail-header .detail-close-button { width: 36px; height: 36px; flex: 0 0 36px; margin-left: auto; border-color: var(--ops-border); color: var(--ops-text-secondary); }
.permission-detail-header .detail-close-button:hover { border-color: var(--ops-primary); background: var(--ops-primary-light); color: var(--ops-primary); }
.permission-detail-icon { display: grid; flex: 0 0 40px; width: 40px; height: 40px; place-items: center; border-radius: 10px; background: var(--ops-primary-light); color: var(--ops-primary); font-size: 19px; }
.permission-detail-badges { display: flex; gap: 7px; padding-left: 52px; }
.permission-detail-scroll { flex: 1; min-height: 0; overflow-y: auto; padding: 18px; }
.permission-detail-section { margin-top: 22px; }
.permission-detail-card { overflow: hidden; border: 1px solid var(--ops-border); border-radius: 8px; background: #fff; }
.permission-detail-project { display: flex; min-height: 42px; justify-content: space-between; align-items: center; gap: 12px; padding: 8px 12px; border-bottom: 1px solid #f0f2f6; color: #45566e; font-size: 12px; }
.permission-detail-project:last-child { border-bottom: 0; }
.permission-detail-project .el-icon { color: #7a8aa1; }
.permission-detail-empty { padding: 14px 12px; color: #8491a3; font-size: 12px; }
.permission-detail-tree-card { padding: 8px 12px; }
.permission-detail-tree-card .readonly-permission-tree { pointer-events: none; }
.permission-inline-form { padding-bottom: 18px; }
.permission-inline-form :deep(.el-form-item) { margin-bottom: 16px; }
.permission-inline-form :deep(.el-form-item__label) { padding-bottom: 6px; color: #52627a; font-size: 12px; font-weight: 600; }
.drawer-permission-tree { margin-bottom: 12px; }
.drawer-admin-notice { margin-top: 8px; }
.permission-detail-footer { display: flex; justify-content: flex-end; gap: 8px; padding: 13px 18px; border-top: 1px solid var(--ops-border); background: #fff; }
.permission-detail-kv { display: flex; flex-direction: column; gap: 12px; padding: 13px; border: 1px solid var(--ops-border); border-radius: 8px; background: #fff; }
.permission-detail-kv > div { display: flex; justify-content: space-between; gap: 12px; font-size: 12px; }
.permission-detail-kv span { color: var(--ops-text-secondary); }
.permission-detail-kv b { color: #35465d; font-weight: 550; }
@media (max-width: 760px) { :deep(.el-drawer) { width: min(520px, calc(100vw - 18px)) !important; } }
</style>
