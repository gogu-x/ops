import { createRouter, createWebHistory } from 'vue-router'
import Login from '../views/Login.vue'
import Home from '../views/Home.vue'
import Hosts from '../views/Hosts.vue'
import Services from '../views/Services.vue'
import Projects from '../views/Projects.vue'
import Permissions from '../views/Permissions.vue'

function defaultPathForUser(user: { role?: string; permissions?: string[] }) {
  if (user.role === 'admin') return '/'
  const permissions = user.permissions || []
  if (permissions.includes('dashboard.view')) return '/'
  if (permissions.includes('services.view') || permissions.includes('services.manage')) return '/services'
  if (permissions.includes('projects.view')) return '/projects'
  if (permissions.includes('hosts.view') || permissions.includes('hosts.manage')) return '/hosts'
  return '/login'
}

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/login', component: Login, meta: { public: true } },
    { path: '/', component: Home },
    { path: '/hosts', component: Hosts },
    { path: '/services', component: Services },
    { path: '/projects', component: Projects },
    { path: '/permissions', component: Permissions },
  ],
})

router.beforeEach((to) => {
  if (!to.meta.public && !localStorage.getItem('ops_access_token')) return '/login'
  let user: { role?: string; permissions?: string[] }
  try { user = JSON.parse(localStorage.getItem('ops_user') || '{}') } catch { user = {} }
  if (to.path === '/login' && localStorage.getItem('ops_access_token')) return defaultPathForUser(user)
  if (to.path === '/' && user.role !== 'admin' && !user.permissions?.includes('dashboard.view')) return defaultPathForUser(user)
  if (to.path === '/permissions') {
    if (user.role !== 'admin') return defaultPathForUser(user)
  }
  if (to.path === '/services' || to.path === '/projects' || to.path === '/hosts') {
    const permissions = user.role === 'admin' ? ['services.view', 'projects.view', 'hosts.view'] : (user.permissions || [])
    const required = to.path === '/services' ? 'services.view' : to.path === '/projects' ? 'projects.view' : 'hosts.view'
    const allowed = permissions.includes(required) || (required === 'services.view' && permissions.includes('services.manage')) || (required === 'hosts.view' && permissions.includes('hosts.manage'))
    if (!allowed) return defaultPathForUser(user)
  }
})

export default router
