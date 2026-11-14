<script setup>
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { useAuthStore } from '../stores/auth'

const auth = useAuthStore()
const router = useRouter()
const username = ref('buyer')
const password = ref('buyer123')
const error = ref('')
const loading = ref(false)

async function submit() {
  error.value = ''
  loading.value = true
  try {
    await auth.login(username.value, password.value)
    router.push('/projects')
  } catch (e) {
    error.value = e.message
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="login-wrap">
    <div class="login-card">
      <h1>采购公告追溯平台</h1>
      <p class="sub">多来源采集 · 更正追溯 · 依据核查</p>
      <div class="field">
        <label>用户名</label>
        <input v-model="username" @keyup.enter="submit" placeholder="admin / buyer" />
      </div>
      <div class="field">
        <label>密码</label>
        <input v-model="password" type="password" @keyup.enter="submit" />
      </div>
      <div v-if="error" class="notice danger">{{ error }}</div>
      <button style="width: 100%" :disabled="loading" @click="submit">
        {{ loading ? '登录中…' : '登录' }}
      </button>
      <div class="login-hint">
        测试账号：<br />
        管理员 <code>admin / admin123</code>（来源管理、暂停、补抓）<br />
        采购专员 <code>buyer / buyer123</code>（待核实、逐字段确认、导出）
      </div>
    </div>
  </div>
</template>
