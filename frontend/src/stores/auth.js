import { defineStore } from 'pinia'
import { get, post, setToken, clearToken, getToken } from '../api/client'

export const useAuthStore = defineStore('auth', {
  state: () => ({ user: null, token: getToken() }),
  getters: {
    isAdmin: (s) => s.user?.role === 'admin',
    loggedIn: (s) => !!s.user,
  },
  actions: {
    async login(username, password) {
      const data = await post('/auth/login', { username, password })
      setToken(data.token)
      this.token = data.token
      this.user = data.user
    },
    async fetchMe() {
      if (!this.token) return false
      try {
        this.user = await get('/auth/me')
        return true
      } catch {
        this.logout()
        return false
      }
    },
    logout() {
      clearToken()
      this.token = ''
      this.user = null
    },
  },
})
