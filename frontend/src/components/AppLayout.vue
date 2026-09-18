<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  Odometer,
  Monitor,
  Grid,
  FolderOpened,
  DataAnalysis,
  Fold,
  Expand,
  User,
  SwitchButton,
  Setting,
  Cloudy,
} from '@element-plus/icons-vue'
import { useAuthStore } from '../stores/auth'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()

const collapsed = ref(false)

interface MenuItem {
  index: string
  path: string
  label: string
  icon: any
}

const menuItems: MenuItem[] = [
  { index: 'dashboard', path: '/', label: '控制台', icon: Odometer },
  { index: 'services', path: '/services', label: '服务管理', icon: Grid },
  { index: 'projects', path: '/projects', label: '项目管理', icon: FolderOpened },
  { index: 'hosts', path: '/hosts', label: '基础设施', icon: Monitor },
  { index: 'audit', path: '/audit', label: '审计日志', icon: DataAnalysis },
]

const activeIndex = computed(() => {
  const match = menuItems.find((item) => item.path === route.path)
  return match?.index || 'dashboard'
})

const activeLabel = computed(() => menuItems.find((item) => item.index === activeIndex.value)?.label || '控制台')

function go(item: MenuItem) {
  if (item.path === '/audit') {
    ElMessage.info('审计日志模块即将上线')
    return
  }
  if (route.path !== item.path) router.push(item.path)
}

async function logout() {
  try {
    await ElMessageBox.confirm('确定要退出登录吗？', '退出确认', { type: 'warning' })
  } catch {
    return
  }
  await auth.logout()
  await router.push('/login')
}
</script>

<template>
  <el-container class="pro-shell">
    <el-aside :width="collapsed ? '64px' : '220px'" class="pro-sider">
      <div class="pro-logo">
        <el-icon class="pro-logo-img" :size="24"><Cloudy /></el-icon>
        <span v-if="!collapsed" class="pro-logo-text">Ops Platform</span>
      </div>
      <el-menu
        class="pro-menu"
        background-color="transparent"
        text-color="rgba(255,255,255,0.65)"
        active-text-color="#ffffff"
        :default-active="activeIndex"
        :collapse="collapsed"
        :collapse-transition="false"
      >
        <el-menu-item v-for="item in menuItems" :key="item.index" :index="item.index" @click="go(item)">
          <el-icon><component :is="item.icon" /></el-icon>
          <template #title>{{ item.label }}</template>
        </el-menu-item>
      </el-menu>
    </el-aside>

    <el-container class="pro-body">
      <el-header class="pro-header">
        <div class="pro-header-left">
          <el-icon class="pro-collapse-btn" @click="collapsed = !collapsed">
            <component :is="collapsed ? Expand : Fold" />
          </el-icon>
          <el-breadcrumb separator="/" class="pro-breadcrumb">
            <el-breadcrumb-item :to="{ path: '/' }">首页</el-breadcrumb-item>
            <el-breadcrumb-item>{{ activeLabel }}</el-breadcrumb-item>
          </el-breadcrumb>
        </div>
        <div class="pro-header-right">
          <el-dropdown trigger="click">
            <span class="pro-user">
              <el-avatar :size="32" class="pro-avatar">{{ (auth.user?.username || '?').slice(0, 1).toUpperCase() }}</el-avatar>
              <span class="pro-user-name">{{ auth.user?.username }}</span>
              <el-tag size="small" :type="auth.user?.role === 'admin' ? 'danger' : 'info'" round class="pro-role-tag">
                {{ auth.user?.role }}
              </el-tag>
            </span>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item :icon="User" disabled>{{ auth.user?.username }}</el-dropdown-item>
                <el-dropdown-item :icon="Setting" disabled>个人设置</el-dropdown-item>
                <el-dropdown-item :icon="SwitchButton" divided @click="logout">退出登录</el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </div>
      </el-header>

      <el-main class="pro-main">
        <div class="pro-page">
          <slot name="header" />
          <slot />
        </div>
      </el-main>
    </el-container>
  </el-container>
</template>

<style scoped>
.pro-shell {
  min-height: 100vh;
  background: var(--ops-bg);
}

.pro-sider {
  background: var(--ops-sider-bg);
  transition: width 0.2s;
  overflow: hidden;
  display: flex;
  flex-direction: column;
}

.pro-logo {
  height: 56px;
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 0 20px;
  overflow: hidden;
  white-space: nowrap;
  border-bottom: 1px solid rgba(255, 255, 255, 0.08);
}

.pro-logo-img {
  flex-shrink: 0;
  color: #fff;
}

.pro-logo-text {
  color: #fff;
  font-size: 17px;
  font-weight: 600;
  letter-spacing: 0.5px;
}

.pro-menu {
  border-right: 0;
  flex: 1;
  padding-top: 8px;
}

.pro-menu :deep(.el-menu-item) {
  margin: 4px 8px;
  border-radius: 6px;
  height: 44px;
}

.pro-menu :deep(.el-menu-item.is-active) {
  background: var(--ops-primary) !important;
  color: #fff !important;
}

.pro-menu :deep(.el-menu-item:hover) {
  background: rgba(255, 255, 255, 0.08);
}

.pro-body {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.pro-header {
  height: 56px;
  background: #fff;
  border-bottom: 1px solid var(--ops-border);
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 20px;
  position: sticky;
  top: 0;
  z-index: 10;
}

.pro-header-left {
  display: flex;
  align-items: center;
  gap: 16px;
}

.pro-collapse-btn {
  font-size: 18px;
  cursor: pointer;
  color: var(--ops-text-secondary);
  transition: color 0.2s;
}

.pro-collapse-btn:hover {
  color: var(--ops-primary);
}

.pro-breadcrumb {
  font-size: 14px;
}

.pro-header-right {
  display: flex;
  align-items: center;
  gap: 12px;
}

.pro-user {
  display: flex;
  align-items: center;
  gap: 8px;
  cursor: pointer;
  padding: 6px 10px;
  border-radius: 6px;
  transition: background 0.2s;
}

.pro-user:hover {
  background: var(--ops-bg);
}

.pro-avatar {
  background: var(--ops-primary);
  font-size: 14px;
  font-weight: 600;
}

.pro-user-name {
  font-size: 14px;
  color: var(--ops-text);
}

.pro-role-tag {
  text-transform: uppercase;
  font-size: 11px;
}

.pro-main {
  padding: 20px;
  flex: 1;
}

.pro-page {
  width: 100%;
}
</style>
