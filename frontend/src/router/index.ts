import { createRouter, createWebHistory } from 'vue-router'
import Login from '../views/Login.vue'
import Home from '../views/Home.vue'
import Hosts from '../views/Hosts.vue'
import Services from '../views/Services.vue'
import ServiceTypes from '../views/ServiceTypes.vue'
import Rollouts from '../views/Rollouts.vue'
import AutoOpen from '../views/AutoOpen.vue'
import Projects from '../views/Projects.vue'
import Permissions from '../views/Permissions.vue'
import { hasUserPermission } from '../stores/auth'

function defaultPathForUser(user: { role?: string; permissions?: string[] }) {
  if (hasUserPermission(user, 'dashboard.view')) return '/'
  if (hasUserPermission(user, 'services.view')) return '/services'
  if (hasUserPermission(user, 'service_types.view')) return '/services/types'
  if (hasUserPermission(user, 'projects.view')) return '/projects'
  if (hasUserPermission(user, 'hosts.view')) return '/hosts'
  return '/login'
}

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/login', component: Login, meta: { public: true } },
    { path: '/', component: Home },
    { path: '/hosts', component: Hosts },
    { path: '/services', component: Services },
    { path: '/services/types', component: ServiceTypes },
    { path: '/services/rollouts', component: Rollouts },
    { path: '/services/auto-open', component: AutoOpen },
    { path: '/projects', component: Projects },
    { path: '/permissions', component: Permissions },
  ],
})

router.beforeEach((to) => {
  if (!to.meta.public && !localStorage.getItem('ops_access_token')) return '/login'
  let user: { role?: string; permissions?: string[] }
  try { user = JSON.parse(localStorage.getItem('ops_user') || '{}') } catch { user = {} }
  if (to.path === '/login' && localStorage.getItem('ops_access_token')) return defaultPathForUser(user)
  if (to.path === '/' && !hasUserPermission(user, 'dashboard.view')) return defaultPathForUser(user)
  if (to.path === '/permissions') {
    if (user.role !== 'admin') return defaultPathForUser(user)
  }
  if (to.path === '/services/types' && hasUserPermission(user, 'service_types.view')) return
  if (to.path.startsWith('/services') && !hasUserPermission(user, 'services.view')) return defaultPathForUser(user)
  if (to.path === '/projects' && !hasUserPermission(user, 'projects.view')) return defaultPathForUser(user)
  if (to.path === '/hosts' && !hasUserPermission(user, 'hosts.view')) return defaultPathForUser(user)
})

export default router
