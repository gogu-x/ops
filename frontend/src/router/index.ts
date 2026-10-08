import { createRouter, createWebHistory } from 'vue-router'
import Login from '../views/Login.vue'
import Home from '../views/Home.vue'
import Hosts from '../views/Hosts.vue'
import Services from '../views/Services.vue'
import Projects from '../views/Projects.vue'
import Permissions from '../views/Permissions.vue'

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
  if (to.path === '/login' && localStorage.getItem('ops_access_token')) return '/'
  if (to.path === '/permissions') {
    try { if (JSON.parse(localStorage.getItem('ops_user') || '{}').role !== 'admin') return '/' } catch { return '/login' }
  }
  if (to.path === '/services' || to.path === '/projects' || to.path === '/hosts') {
    try {
      const user = JSON.parse(localStorage.getItem('ops_user') || '{}')
      const permissions = user.role === 'admin' ? ['services.view', 'projects.view', 'hosts.view'] : (user.permissions || [])
      const required = to.path === '/services' ? 'services.view' : to.path === '/projects' ? 'projects.view' : 'hosts.view'
      const allowed = permissions.includes(required) || (required === 'services.view' && permissions.includes('services.manage')) || (required === 'hosts.view' && permissions.includes('hosts.manage'))
      if (!allowed) return '/'
    } catch { return '/login' }
  }
})

export default router
