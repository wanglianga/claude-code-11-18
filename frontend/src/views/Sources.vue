<script setup>
import { onMounted, onUnmounted, ref } from 'vue'
import { get, post } from '../api/client'

const sources = ref([])
const selected = ref(null)
const discoveries = ref([])
const jobs = ref([])
const msg = ref('')
const manualRef = ref('')

async function load() {
  const data = await get('/sources')
  sources.value = data.sources
  if (selected.value) await select(selected.value)
}

async function select(src) {
  selected.value = src
  const [d, j] = await Promise.all([
    get(`/sources/${src.id}/discoveries`),
    get(`/sources/${src.id}/jobs`),
  ])
  discoveries.value = d.discoveries
  jobs.value = j.list_jobs
}

async function pause(src) {
  await post(`/sources/${src.id}/pause`)
  msg.value = '来源已暂停：仅停止新领取，执行中的任务仍按原凭证完成'
  await load()
}
async function resume(src) {
  await post(`/sources/${src.id}/resume`)
  msg.value = '来源已恢复领取'
  await load()
}
async function trigger(src) {
  await post(`/sources/${src.id}/list-jobs`)
  msg.value = '列表作业已入队（仍受站点间隔与并发限制）'
  setTimeout(load, 800)
}
async function manual(src) {
  if (!manualRef.value.trim()) return
  await post(`/sources/${src.id}/manual-detail`, { ref: manualRef.value.trim() })
  msg.value = '手动补抓已排队：与自动采集共用限流器，绕不开站点限额'
  manualRef.value = ''
  setTimeout(load, 800)
}

function statusBadge(s) {
  return {
    done: ['green', '已完成'], running: ['blue', '采集中'], idle: ['gray', '空闲'], error: ['red', '错误'],
  }[s] || ['gray', s]
}
function detailBadge(s) {
  return {
    done: ['green', '详情已取'], pending: ['amber', '详情待取'],
    failed: ['red', '下载失败'], inaccessible: ['gray', '不可访问'],
  }[s] || ['gray', s]
}

onMounted(load)
const timer = setInterval(load, 5000)
onUnmounted(() => clearInterval(timer))
</script>

<template>
  <div>
    <h2>采集来源与队列</h2>
    <div v-if="msg" class="notice info">{{ msg }}</div>

    <div class="grid cols-3">
      <div v-for="src in sources" :key="src.id" class="card" :style="selected?.id === src.id ? 'border-color: var(--brand)' : ''">
        <div class="row spread">
          <strong>{{ src.name }}</strong>
          <span class="badge" :class="src.enabled ? 'green' : 'gray'">{{ src.enabled ? '启用中' : '已暂停' }}</span>
        </div>
        <p class="muted small mono mt">{{ src.code }} · {{ src.list_mode === 'page' ? '页码分页' : '游标分页' }}</p>
        <div class="grid cols-3 small mt">
          <div><div class="stat"><div class="num">{{ src.counters.detail_done }}</div><div class="lbl">详情已取</div></div></div>
          <div><div class="stat"><div class="num">{{ src.counters.detail_pending }}</div><div class="lbl">详情待取/失败</div></div></div>
          <div><div class="stat"><div class="num">{{ src.counters.review_pending }}</div><div class="lbl">待复查</div></div></div>
        </div>
        <p class="small muted mt">
          列表状态：<span class="badge" :class="statusBadge(src.list_status)[0]">{{ statusBadge(src.list_status)[1] }}</span>
          ｜间隔 {{ src.interval_ms }}ms｜并发 {{ src.max_concurrency }}｜重试 {{ src.max_retries }}
        </p>
        <p class="small muted">断点：<code>{{ JSON.stringify(src.cursor_state) }}</code></p>
        <p v-if="src.last_error" class="small" style="color: var(--danger)">错误：{{ src.last_error }}</p>
        <div class="row mt">
          <button class="sm" @click="select(src)">查看明细</button>
          <button v-if="src.enabled" class="sm warn" @click="pause(src)">暂停</button>
          <button v-else class="sm ghost" @click="resume(src)">恢复</button>
          <button class="sm ghost" @click="trigger(src)">触发列表采集</button>
        </div>
      </div>
    </div>

    <div v-if="selected" class="card">
      <div class="row spread">
        <h3>{{ selected.name }} —— 发现记录与作业</h3>
        <div class="row">
          <input v-model="manualRef" placeholder="站内编号，如 A-005" style="width: 180px" />
          <button class="sm" @click="manual(selected)">手动补抓（受限额约束）</button>
        </div>
      </div>

      <h3 class="mt">列表作业（中断后租约过期可被重新领取，epoch 递增）</h3>
      <table>
        <thead><tr><th>ID</th><th>类型</th><th>状态</th><th>epoch</th><th>执行者</th><th>断点</th><th>错误</th></tr></thead>
        <tbody>
          <tr v-for="j in jobs" :key="j.id">
            <td>{{ j.id }}</td><td>{{ j.kind }}</td>
            <td><span class="badge" :class="j.status === 'succeeded' ? 'green' : j.status === 'failed' ? 'red' : 'blue'">{{ j.status }}</span></td>
            <td>{{ j.epoch }}</td><td class="mono small">{{ j.worker_id || '-' }}</td>
            <td class="mono small">{{ j.last_offset }}</td><td class="small">{{ j.error }}</td>
          </tr>
        </tbody>
      </table>

      <h3 class="mt">发现记录（发现即落库，详情失败不丢发现）</h3>
      <table>
        <thead><tr><th>站内编号</th><th>标题</th><th>详情状态</th><th>URL</th></tr></thead>
        <tbody>
          <tr v-for="d in discoveries" :key="d.id">
            <td class="mono">{{ d.source_ref }}</td><td>{{ d.title }}</td>
            <td><span class="badge" :class="detailBadge(d.detail_status)[0]">{{ detailBadge(d.detail_status)[1] }}</span></td>
            <td class="small muted mono">{{ d.url }}</td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>
