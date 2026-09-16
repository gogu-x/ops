import axios, { type AxiosError, type InternalAxiosRequestConfig } from 'axios'

const api = axios.create({
  baseURL: '/api',
  timeout: 15000,
})

let refreshing: Promise<string> | null = null

api.interceptors.request.use((config: InternalAxiosRequestConfig) => {
  const token = localStorage.getItem('ops_access_token')
  if (token) config.headers.Authorization = `Bearer ${token}`
  return config
})

api.interceptors.response.use(
  (response) => response,
  async (error: AxiosError) => {
    const original = error.config as (InternalAxiosRequestConfig & { _retry?: boolean }) | undefined
    const refresh = localStorage.getItem('ops_refresh_token')
    if (error.response?.status !== 401 || !original || original._retry || !refresh || original.url?.includes('/auth/')) {
      return Promise.reject(error)
    }
    original._retry = true
    refreshing ||= axios.post('/api/auth/refresh', { refresh_token: refresh }).then((res) => {
      const data = res.data.data
      localStorage.setItem('ops_access_token', data.access_token)
      localStorage.setItem('ops_refresh_token', data.refresh_token)
      localStorage.setItem('ops_user', JSON.stringify(data.user))
      return data.access_token as string
    }).finally(() => { refreshing = null })
    const token = await refreshing
    original.headers.Authorization = `Bearer ${token}`
    return api(original)
  },
)

export default api
