<script setup>
import { onMounted } from 'vue'
import { RouterView, RouterLink, useRouter } from 'vue-router'
import { useAuthStore } from './stores/auth'

const auth = useAuthStore()
const router = useRouter()
onMounted(() => auth.fetchMe())

function logout() {
  auth.logout()
  router.push('/login')
}
</script>

<template>
  <RouterView v-if="$route.path === '/login'" />
  <div v-else class="layout">
    <header class="topbar">
      <span class="brand">采购公告采集与更正追溯平台</span>
      <nav>
        <RouterLink to="/projects">项目时间线</RouterLink>
        <RouterLink to="/workbench">待核实工作台</RouterLink>
        <RouterLink v-if="auth.isAdmin" to="/sources">来源与队列</RouterLink>
      </nav>
      <div class="user">
        <span>{{ auth.user?.display_name }}（{{ auth.user?.role === 'admin' ? '管理员' : '采购专员' }}）</span>
        <button class="sm ghost" @click="logout">退出</button>
      </div>
    </header>
    <main class="container">
      <RouterView />
    </main>
  </div>
</template>
