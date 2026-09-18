import { createRouter, createWebHistory } from 'vue-router'
import Login from '../views/Login.vue'
import Home from '../views/Home.vue'
import Hosts from '../views/Hosts.vue'
import Services from '../views/Services.vue'
import Projects from '../views/Projects.vue'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/login', component: Login, meta: { public: true } },
    { path: '/', component: Home },
    { path: '/hosts', component: Hosts },
    { path: '/services', component: Services },
    { path: '/projects', component: Projects },
  ],
})

router.beforeEach((to) => {
  if (!to.meta.public && !localStorage.getItem('ops_access_token')) return '/login'
  if (to.path === '/login' && localStorage.getItem('ops_access_token')) return '/'
})

export default router
