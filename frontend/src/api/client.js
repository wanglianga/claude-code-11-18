// 统一 API 客户端：自动携带 Bearer 令牌，401 时清理登录态并跳转。
const TOKEN_KEY = 'pw_token'

export function getToken() {
  return localStorage.getItem(TOKEN_KEY) || ''
}
export function setToken(t) {
  localStorage.setItem(TOKEN_KEY, t)
}
export function clearToken() {
  localStorage.removeItem(TOKEN_KEY)
}

export async function api(path, options = {}) {
  const headers = { 'Content-Type': 'application/json', ...(options.headers || {}) }
  const token = getToken()
  if (token) headers.Authorization = `Bearer ${token}`
  const resp = await fetch(`/api${path}`, { ...options, headers })
  if (resp.status === 401) {
    clearToken()
    if (location.hash !== '#/login') location.hash = '#/login'
    throw new Error('登录已过期，请重新登录')
  }
  const text = await resp.text()
  const data = text ? JSON.parse(text) : {}
  if (!resp.ok) throw new Error(data.error || `请求失败(${resp.status})`)
  return data
}

export const get = (p) => api(p)
export const post = (p, body) => api(p, { method: 'POST', body: JSON.stringify(body || {}) })
