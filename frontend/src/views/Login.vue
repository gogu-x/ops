<script setup lang="ts">
import { reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { User, Lock, Cloudy } from '@element-plus/icons-vue'
import { useAuthStore } from '../stores/auth'

const router = useRouter()
const auth = useAuthStore()
const loading = ref(false)
const form = reactive({ username: 'admin', password: '' })

async function submit() {
  if (!form.username || !form.password) {
    ElMessage.warning('请输入用户名和密码')
    return
  }
  loading.value = true
  try {
    await auth.login(form.username, form.password)
    await router.push('/')
  } catch {
    ElMessage.error('用户名或密码错误')
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <main class="auth-page">
    <div class="auth-card-wrap">
      <el-card class="login-card" shadow="never">
        <div class="brand">
          <el-icon class="brand-logo" :size="52" color="#1677ff"><Cloudy /></el-icon>
          <h1>Ops Platform</h1>
          <p>游戏服务运维平台</p>
        </div>
        <el-form @submit.prevent="submit">
          <el-form-item>
            <el-input v-model="form.username" placeholder="用户名" size="large" :prefix-icon="User" />
          </el-form-item>
          <el-form-item>
            <el-input
              v-model="form.password"
              placeholder="密码"
              type="password"
              show-password
              size="large"
              :prefix-icon="Lock"
              @keyup.enter="submit"
            />
          </el-form-item>
          <el-button class="full-width" type="primary" size="large" :loading="loading" @click="submit">
            登 录
          </el-button>
        </el-form>
      </el-card>
    </div>
    <p class="auth-footer">Ops Platform © {{ new Date().getFullYear() }} · Powered by Go &amp; Vue3</p>
  </main>
</template>
