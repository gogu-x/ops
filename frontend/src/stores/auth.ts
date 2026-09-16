import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import api from '../api/client'

export interface User {
  id: string
  username: string
  role: string
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

  async function logout() {
    try {
      if (refreshToken.value) await api.post('/auth/logout', { refresh_token: refreshToken.value })
    } finally {
      clear()
    }
  }

  return { accessToken, refreshToken, user, isAuthenticated, login, logout, save, clear }
})
