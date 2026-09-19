<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import axios from 'axios'
import { Monitor, Grid, FolderOpened, DataLine, CircleCheck, Warning, CircleClose } from '@element-plus/icons-vue'
import AppLayout from '../components/AppLayout.vue'
import { hostApi } from '../api/hosts'
import { serviceApi } from '../api/services'
import { projectApi } from '../api/projects'

const router = useRouter()
const backendStatus = ref<'检查中' | '正常' | '异常' | '不可用'>('检查中')
const hostCount = ref<number | null>(null)
const serviceCount = ref<number | null>(null)
const projectCount = ref<number | null>(null)

const statusTagType = () => {
  if (backendStatus.value === '正常') return 'success'
  if (backendStatus.value === '检查中') return 'info'
  return 'danger'
}

onMounted(async () => {
  try {
    const response = await axios.get('/health')
    backendStatus.value = response.data.status === 'ok' ? '正常' : '异常'
  } catch {
    backendStatus.value = '不可用'
  }

  try {
    const projects = await projectApi.list()
    projectCount.value = projects.length
  } catch {
    projectCount.value = null
  }

  try {
    const services = await serviceApi.list()
    serviceCount.value = services.length
  } catch {
    serviceCount.value = null
  }

  try {
    const hosts = await hostApi.list()
    hostCount.value = hosts.length
  } catch {
    hostCount.value = null
  }
})
</script>

<template>
  <AppLayout>
    <div class="page-heading">
      <div>
        <h2>控制台</h2>
        <p>欢迎回来，这里是 Ops Platform 运维平台总览，服务管理是核心操作入口，主机仅作为部署载体。</p>
      </div>
      <el-tag :type="statusTagType()" effect="light" round size="large">
        <el-icon style="vertical-align: -2px; margin-right: 4px">
          <component :is="backendStatus === '正常' ? CircleCheck : backendStatus === '检查中' ? Warning : CircleClose" />
        </el-icon>
        后端服务 {{ backendStatus }}
      </el-tag>
    </div>

    <el-row :gutter="20" class="dashboard-stats">
      <el-col :span="8">
        <el-card class="stat-card is-clickable" shadow="never" @click="router.push('/services')">
          <div class="stat-card-head">
            <span>项目数量</span>
            <span class="stat-icon blue"><el-icon><FolderOpened /></el-icon></span>
          </div>
          <div class="metric">{{ projectCount ?? '—' }}</div>
          <div class="stat-foot">
            <span>按业务项目分类管理服务</span>
            <el-icon><DataLine /></el-icon>
          </div>
        </el-card>
      </el-col>
      <el-col :span="8">
        <el-card class="stat-card is-clickable" shadow="never" @click="router.push('/services')">
          <div class="stat-card-head">
            <span>服务类型</span>
            <span class="stat-icon green"><el-icon><Grid /></el-icon></span>
          </div>
          <div class="metric">{{ serviceCount ?? '—' }}</div>
          <div class="stat-foot">
            <span>已配置镜像与参数模板</span>
            <el-icon><DataLine /></el-icon>
          </div>
        </el-card>
      </el-col>
      <el-col :span="8">
        <el-card class="stat-card is-clickable" shadow="never" @click="router.push('/hosts')">
          <div class="stat-card-head">
            <span>主机数量</span>
            <span class="stat-icon orange"><el-icon><Monitor /></el-icon></span>
          </div>
          <div class="metric">{{ hostCount ?? '—' }}</div>
          <div class="stat-foot">
            <span>基础设施 · 部署载体</span>
            <el-icon><DataLine /></el-icon>
          </div>
        </el-card>
      </el-col>
    </el-row>

    <el-card class="ops-card quick-card" shadow="never">
      <template #header><div class="card-heading"><span>快速入口</span><small>常用运维功能</small></div></template>
      <el-row :gutter="16" class="quick-grid">
        <el-col :span="12">
          <div class="quick-entry" @click="router.push('/services')">
            <el-icon :size="20" color="#52c41a"><Grid /></el-icon>
            <div>
              <div class="quick-entry-title">服务管理</div>
              <div class="quick-entry-desc">按项目分类管理服务、配置镜像与启动参数</div>
            </div>
          </div>
        </el-col>
        <el-col :span="12">
          <div class="quick-entry" @click="router.push('/hosts')">
            <el-icon :size="20" color="#8c8c8c"><Monitor /></el-icon>
            <div>
              <div class="quick-entry-title">基础设施</div>
              <div class="quick-entry-desc">添加 / 测试 Docker 主机连接</div>
            </div>
          </div>
        </el-col>
      </el-row>
    </el-card>
  </AppLayout>
</template>

<style scoped>
.page-heading { padding: 22px 24px; border: 1px solid var(--ops-border); border-radius: 12px; background: linear-gradient(135deg, #fff, #f5f9ff); box-shadow: var(--ops-shadow); }
.dashboard-stats { margin-top: 20px; }
.quick-card { margin-top: 20px; }
.card-heading { display: flex; align-items: center; justify-content: space-between; }
.card-heading small { color: var(--ops-text-secondary); font-size: 11px; font-weight: 400; }

.quick-entry {
  display: flex;
  align-items: center;
  gap: 12px;
  min-height: 82px;
  padding: 18px;
  border-radius: 10px;
  border: 1px solid var(--ops-border);
  cursor: pointer;
  transition: border-color 0.2s, background 0.2s, transform .2s;
}

.quick-entry:hover {
  border-color: var(--ops-primary);
  background: var(--ops-primary-light);
  transform: translateY(-2px);
}

.quick-entry-title {
  font-weight: 600;
  font-size: 14px;
  color: var(--ops-text);
}

.quick-entry-desc {
  font-size: 12px;
  color: var(--ops-text-secondary);
  margin-top: 2px;
}

@media (max-width: 900px) {
  .dashboard-stats :deep(.el-col) { flex: 0 0 50%; max-width: 50%; margin-bottom: 16px; }
}

@media (max-width: 640px) {
  .page-heading { flex-direction: column; padding: 18px; }
  .page-heading .el-tag { align-self: flex-start; }
  .dashboard-stats { margin-top: 14px; }
  .dashboard-stats :deep(.el-col), .quick-grid :deep(.el-col) { flex: 0 0 100%; max-width: 100%; margin-bottom: 12px; }
  .quick-card { margin-top: 4px; }
  .stat-card :deep(.el-card__body) { padding: 16px; }
}
</style>
