import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import api from '../api/client'

export interface User {
  id: string
  username: string
  role: string
  project_ids?: string[]
  permissions?: string[]
  disabled: boolean
}

interface TokenData {
  access_token: string
  refresh_token: string
  expires_in: number
  user: User
}

export const useAuthStore = defineStore('auth', () => {
  const accessToken = ref(localStorage.getItem('ops_access_token') || '')
  const refreshToken = ref(localStorage.getItem('ops_refresh_token') || '')
  const user = ref<User | null>(JSON.parse(localStorage.getItem('ops_user') || 'null'))
  const isAuthenticated = computed(() => Boolean(accessToken.value))

  function hasPermission(permission: string) {
    if (user.value?.role === 'admin') return true
    const permissions = user.value?.permissions || []
    if (permissions.includes(permission)) return true
    if (permission === 'services.view') return permissions.includes('services.manage')
    if (permission === 'hosts.view') return permissions.includes('hosts.manage')
    return false
  }

  function save(data: TokenData) {
    accessToken.value = data.access_token
    refreshToken.value = data.refresh_token
    user.value = data.user
    localStorage.setItem('ops_access_token', data.access_token)
    localStorage.setItem('ops_refresh_token', data.refresh_token)
    localStorage.setItem('ops_user', JSON.stringify(data.user))
  }

  function clear() {
    accessToken.value = ''
    refreshToken.value = ''
    user.value = null
    localStorage.removeItem('ops_access_token')
    localStorage.removeItem('ops_refresh_token')
    localStorage.removeItem('ops_user')
  }

  async function login(username: string, password: string) {
    const { data } = await api.post('/auth/login', { username, password })
    save(data.data)
  }

  async function loadCurrentUser() {
    if (!accessToken.value) return
    try {
      const { data } = await api.get('/me')
      user.value = data.data
      localStorage.setItem('ops_user', JSON.stringify(data.data))
    } catch (error: any) {
      if (error?.response?.status === 401) clear()
    }
  }

  async function logout() {
    try {
      if (refreshToken.value) await api.post('/auth/logout', { refresh_token: refreshToken.value })
    } finally {
      clear()
    }
  }

  return { accessToken, refreshToken, user, isAuthenticated, hasPermission, login, loadCurrentUser, logout, save, clear }
})
