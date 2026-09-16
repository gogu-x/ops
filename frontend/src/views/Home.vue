<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import axios from 'axios'
import { useAuthStore } from '../stores/auth'

const router = useRouter()
const auth = useAuthStore()
const backendStatus = ref('检查中')

onMounted(async () => {
  try {
    const response = await axios.get('/health')
    backendStatus.value = response.data.status === 'ok' ? '正常' : '异常'
  } catch {
    backendStatus.value = '不可用'
  }
})

async function logout() {
  await auth.logout()
  await router.push('/login')
}

function comingSoon() {
  ElMessage.info('服务管理模块将在下一阶段接入 Docker API')
}
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
        <el-menu default-active="dashboard">
          <el-menu-item index="dashboard">控制台</el-menu-item>
          <el-menu-item index="hosts" @click="router.push('/hosts')">主机管理</el-menu-item>
          <el-menu-item index="services" @click="router.push('/services')">服务类型</el-menu-item>
          <el-menu-item index="audit" @click="comingSoon">审计日志</el-menu-item>
        </el-menu>
      </el-aside>
      <el-main class="content">
        <div class="page-heading">
          <div><h2>控制台</h2><p>后端基础骨架已运行，业务模块按简化架构逐步接入。</p></div>
          <el-tag type="success">后端 {{ backendStatus }}</el-tag>
        </div>
        <el-row :gutter="20">
          <el-col :span="8"><el-card><template #header>主机</template><div class="metric">—</div><span>等待 Docker 主机管理模块</span></el-card></el-col>
          <el-col :span="8"><el-card class="dashboard-card" shadow="hover" @click="router.push('/services')"><template #header>服务类型</template><div class="metric">配置</div><el-button link type="primary" @click.stop="router.push('/services')">打开服务类型管理 →</el-button></el-card></el-col>
          <el-col :span="8"><el-card><template #header>操作任务</template><div class="metric">—</div><span>等待部署操作模块</span></el-card></el-col>
        </el-row>
      </el-main>
    </el-container>
  </el-container>
</template>
