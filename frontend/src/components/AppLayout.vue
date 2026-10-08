<script setup lang="ts">
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Box, FolderOpened, Grid, Monitor, SwitchButton, User } from '@element-plus/icons-vue'
import { useAuthStore } from '../stores/auth'

withDefaults(defineProps<{ fixedViewport?: boolean }>(), { fixedViewport: false })
const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const navigation = computed(() => [
  { path: '/', label: '控制台', icon: Grid, visible: true },
  { path: '/services', label: '服务', icon: Box, visible: auth.hasPermission('services.view') },
  { path: '/projects', label: '项目', icon: FolderOpened, visible: auth.hasPermission('projects.view') },
  { path: '/hosts', label: '主机', icon: Monitor, visible: auth.hasPermission('hosts.view') },
  { path: '/permissions', label: '权限管理', icon: User, visible: auth.user?.role === 'admin' },
].filter((item) => item.visible))
async function logout() {
  try { await ElMessageBox.confirm('确定要退出登录吗？', '退出登录', { type: 'warning' }) } catch { return }
  try { await auth.logout() } catch { ElMessage.warning('会话已在本机清除') }
  await router.push('/login')
}
</script>

<template>
  <div class="app-shell" :class="{ 'is-workspace': fixedViewport }">
    <header class="app-header">
      <RouterLink to="/" class="app-brand" aria-label="OPS 控制台"><span class="brand-symbol"><el-icon><Grid /></el-icon></span><span>OPS</span></RouterLink>
      <nav class="app-navigation" aria-label="主导航">
        <RouterLink v-for="item in navigation" :key="item.path" :to="item.path" :class="{ active: route.path === item.path }" :aria-current="route.path === item.path ? 'page' : undefined" :title="item.label"><el-icon><component :is="item.icon" /></el-icon><span>{{ item.label }}</span></RouterLink>
      </nav>
      <el-dropdown trigger="click" class="account-menu">
        <button class="account-button" aria-label="账户菜单"><span class="account-avatar">{{ (auth.user?.username || '?').slice(0, 1).toUpperCase() }}</span><span class="account-name">{{ auth.user?.username }}</span></button>
        <template #dropdown><el-dropdown-menu>
          <el-dropdown-item :icon="User" disabled>{{ auth.user?.role === 'admin' ? '平台管理员' : '普通用户' }}</el-dropdown-item>
          <el-dropdown-item :icon="SwitchButton" divided @click="logout">退出登录</el-dropdown-item>
        </el-dropdown-menu></template>
      </el-dropdown>
    </header>
    <main class="app-main"><slot name="header" /><slot /></main>
  </div>
</template>

<style scoped>
.app-shell { display: flex; min-height: 100vh; background: var(--ops-bg); }
.app-header { width: 220px; height: 100vh; height: 100dvh; min-height: 100vh; flex: 0 0 220px; align-self: flex-start; display: flex; flex-direction: column; align-items: stretch; gap: 30px; padding: 22px 12px 16px; background: var(--ops-sider-bg); border-right: 1px solid rgba(255, 255, 255, .08); position: sticky; top: 0; z-index: 20; overflow-y: auto; scrollbar-width: thin; scrollbar-color: rgba(255, 255, 255, .24) transparent; box-sizing: border-box; }
.app-brand { display: inline-flex; align-items: center; gap: 10px; color: #fff; font-size: 21px; font-weight: 650; letter-spacing: 1px; text-decoration: none; }
.brand-symbol { display: grid; place-items: center; width: 30px; height: 30px; border-radius: 8px; background: var(--ops-primary); color: #fff; font-size: 18px; }
.app-navigation { display: flex; flex-direction: column; gap: 5px; }
.app-navigation a { display: flex; align-items: center; gap: 12px; min-height: 42px; padding: 0 12px; border-radius: 7px; text-decoration: none; color: rgba(255, 255, 255, .68); font-size: 14px; }
.app-navigation a:hover { background: rgba(255, 255, 255, .08); color: #fff; }
.app-navigation a.active { background: var(--ops-primary); color: #fff; font-weight: 600; }
.app-navigation .el-icon { font-size: 17px; }
.account-menu { margin-top: auto; }
.account-button { display: flex; gap: 9px; align-items: center; padding: 5px 0 5px 10px; border: 0; background: transparent; color: #fff; font: inherit; cursor: pointer; }
.account-avatar { display: grid; place-items: center; width: 30px; height: 30px; border-radius: 50%; background: rgba(255, 255, 255, .14); color: #fff; font-size: 12px; }
.account-name { font-size: 12px; }
.app-main { width: 100%; max-width: 1600px; min-width: 0; flex: 1; margin: 0 auto; padding: 28px 32px; box-sizing: border-box; }
.is-workspace { height: 100dvh; min-height: 0; display: flex; flex-direction: row; overflow: hidden; background: #fff; }
.is-workspace .app-header { min-height: 0; height: 100%; flex: 0 0 220px; }
.is-workspace .app-main { display: flex; width: auto; max-width: none; min-height: 0; flex: 1; padding: 0; }
@media (max-width: 1000px) and (min-width: 761px) {
  .app-header { width: 64px; flex-basis: 64px; align-items: center; padding: 18px 8px 12px; gap: 26px; }
  .app-brand > span:last-child, .app-navigation a > span, .account-name { display: none; }
  .app-brand { justify-content: center; }
  .app-navigation { width: 100%; align-items: center; }
  .app-navigation a { justify-content: center; width: 44px; padding: 0; }
  .account-button { padding: 0; }
  .is-workspace .app-header { flex-basis: 64px; }
  .app-main { padding: 24px 20px; }
}
@media (max-width: 760px) {
  .app-shell { flex-direction: column; }
  .app-header { position: relative; width: 100%; min-height: 58px; height: 58px; flex: 0 0 58px; flex-direction: row; align-items: center; gap: 20px; padding: 0 16px; background: #fff; border-right: 0; border-bottom: 1px solid var(--ops-border); overflow: visible; }
  .app-brand { color: var(--ops-text); }
  .account-button { color: var(--ops-text); }
  .account-avatar { background: var(--ops-primary-light); color: var(--ops-primary); }
  .app-navigation { position: fixed; right: 0; bottom: 0; left: 0; z-index: 100; display: flex; height: calc(64px + env(safe-area-inset-bottom)); flex-direction: row; align-items: stretch; justify-content: space-around; gap: 0; padding: 4px 8px env(safe-area-inset-bottom); box-sizing: border-box; background: var(--ops-sider-bg); border-top: 1px solid rgba(255, 255, 255, .12); }
  .app-navigation a { flex: 1; flex-direction: column; justify-content: center; gap: 3px; min-height: 0; padding: 4px 2px; border: 0; border-radius: 6px; color: rgba(255, 255, 255, .68); font-size: 10px; }
  .app-navigation a:hover { background: rgba(255, 255, 255, .08); color: #fff; }
  .app-navigation a.active { background: var(--ops-primary); color: #fff; }
  .app-navigation .el-icon { display: block; font-size: 19px; }
  .account-menu { margin: 0 0 0 auto; }
  .app-main { padding: 20px 16px calc(84px + env(safe-area-inset-bottom)); }
  .is-workspace { flex-direction: column; }
  .is-workspace .app-header { height: 58px; min-height: 58px; flex: 0 0 58px; }
  .is-workspace .app-main { width: 100%; flex: 1; padding: 0 0 calc(64px + env(safe-area-inset-bottom)); box-sizing: border-box; }
}
@media (max-width: 480px) {
  .app-header { gap: 10px; padding: 0 10px; }
  .app-brand { gap: 6px; font-size: 17px; }
  .brand-symbol { width: 25px; height: 25px; font-size: 15px; }
  .app-navigation { justify-content: space-around; gap: 0; overflow: visible; }
  .app-navigation a { flex: 1; padding: 4px 2px; font-size: 10px; }
  .account-avatar { width: 25px; height: 25px; }
  .account-button { padding-left: 0; }
  .app-main { padding: 16px 12px calc(84px + env(safe-area-inset-bottom)); }
  .is-workspace .app-main { padding: 0 0 calc(64px + env(safe-area-inset-bottom)); }
}
</style>
