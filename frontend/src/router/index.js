import { createRouter, createWebHashHistory } from 'vue-router'
import { useAuthStore } from '../stores/auth'

import Login from '../views/Login.vue'
import Sources from '../views/Sources.vue'
import Projects from '../views/Projects.vue'
import ProjectDetail from '../views/ProjectDetail.vue'
import Workbench from '../views/Workbench.vue'
import CorrectionDetail from '../views/CorrectionDetail.vue'

const router = createRouter({
  history: createWebHashHistory(),
  routes: [
    { path: '/login', component: Login },
    { path: '/', redirect: '/projects' },
    { path: '/sources', component: Sources, meta: { admin: true } },
    { path: '/projects', component: Projects },
    { path: '/projects/:id', component: ProjectDetail, props: true },
    { path: '/workbench', component: Workbench },
    { path: '/corrections/:id', component: CorrectionDetail, props: true },
  ],
})

router.beforeEach(async (to) => {
  const auth = useAuthStore()
  if (to.path === '/login') return true
  if (!auth.loggedIn) {
    const ok = await auth.fetchMe()
    if (!ok) return '/login'
  }
  if (to.meta.admin && !auth.isAdmin) return '/projects'
  return true
})

export default router
