<script setup>
import { onMounted, onUnmounted, ref, computed } from 'vue'
import { get, post } from '../api/client'

const props = defineProps({ id: { type: [String, Number], required: true } })
const project = ref({})
const packages = ref([])
const timeline = ref({ documents: [], revisions: [], corrections: [], review_runs: [] })
const versionsOf = ref({})
const rawVersion = ref(null)
const exportData = ref(null)
const msg = ref('')

async function load() {
  project.value = await get(`/projects/${props.id}`)
  const [pk, tl] = await Promise.all([
    get(`/projects/${props.id}/packages`),
    get(`/projects/${props.id}/timeline`),
  ])
  packages.value = pk.packages
  timeline.value = tl
}

// 当前采用值按标包分组
const currentByPkg = computed(() => {
  const m = {}
  for (const r of timeline.value.revisions || []) {
    if (!r.is_current) continue
    const k = r.package_no || '（项目级）'
    if (!m[k]) m[k] = []
    m[k].push(r)
  }
  return m
})

async function showVersions(doc) {
  if (versionsOf.value[doc.id]) { delete versionsOf.value[doc.id]; return }
  const data = await get(`/documents/${doc.id}/versions`)
  versionsOf.value = { ...versionsOf.value, [doc.id]: data.versions }
}
async function showRaw(v) {
  const resp = await fetch(`/api/versions/${v.id}/raw`, {
    headers: { Authorization: `Bearer ${localStorage.getItem('pw_token')}` },
  })
  rawVersion.value = { id: v.id, html: await resp.text() }
}

async function doExport() {
  msg.value = ''
  try {
    exportData.value = await post(`/projects/${props.id}/export`)
  } catch (e) {
    msg.value = e.message
  }
}

function badgeFor(s) {
  return { pending: ['amber', '待处理'], contradictory: ['red', '矛盾待核实'], confirmed: ['green', '已确认'],
    returned: ['gray', '已退回'], superseded: ['purple', '已被取消'] }[s] || ['gray', s]
}
function kindLabel(k) {
  return { initial: '初始采用', correction: '更正确认', cancel_correction: '取消更正后采用' }[k] || k
}
function resultBadge(r) {
  return { unchanged: ['green', '正文未变'], changed: ['amber', '正文变化'],
    inaccessible: ['gray', '不可访问'], parse_failed: ['red', '解析失败'] }[r] || ['gray', r]
}

onMounted(load)
const timer = setInterval(load, 6000)
onUnmounted(() => clearInterval(timer))
</script>

<template>
  <div>
    <RouterLink to="/projects" class="small">← 返回项目列表</RouterLink>
    <div class="card mt">
      <div class="row spread">
        <div>
          <h2 style="margin-bottom:4px">{{ project.title }}</h2>
          <p class="muted mono">采购编号：{{ project.project_no || '（缺失，待人工确认）' }}</p>
        </div>
        <button @click="doExport">导出清单（导出前复核撤标/不可访问）</button>
      </div>
      <p class="small muted mt">
        最后核查：{{ project.last_checked_at ? new Date(project.last_checked_at).toLocaleString() : '尚未核查' }}
        ｜{{ project.check_note || '暂无核查备注' }}
      </p>
    </div>

    <div v-if="exportData" class="card">
      <h3>导出快照与核验结论</h3>
      <p :class="exportData.warnings?.length ? 'notice warn' : 'notice info'">
        {{ exportData.verify_note }}
      </p>
      <ul v-if="exportData.warnings?.length">
        <li v-for="(w, i) in exportData.warnings" :key="i" class="small" style="color:var(--warn)">{{ w }}</li>
      </ul>
      <table>
        <thead><tr><th>标包</th><th>字段</th><th>采用值</th><th>来源公告/版本/指纹</th><th>修订类型</th></tr></thead>
        <tbody>
          <tr v-for="(it, i) in exportData.items" :key="i">
            <td>{{ it.package_no || '项目级' }}</td><td>{{ it.field_name }}</td><td>{{ it.field_value }}</td>
            <td class="small mono">{{ it.source_ref }} #v{{ it.source_version_id }}<br />{{ it.source_fingerprint.slice(0, 16) }}…</td>
            <td>{{ kindLabel(it.change_kind) }}</td>
          </tr>
        </tbody>
      </table>
    </div>

    <div class="card">
      <h3>当前采用结论（原件与采用值分别展示；新增原件不等于可执行要求）</h3>
      <div v-for="(items, pkg) in currentByPkg" :key="pkg" class="mt">
        <strong class="small">标包：{{ pkg }}</strong>
        <table>
          <thead><tr><th>字段</th><th>当前采用值</th><th>类型</th><th>采用时间</th><th>来源</th></tr></thead>
          <tbody>
            <tr v-for="r in items" :key="r.id">
              <td>{{ r.field_name }}</td><td>{{ r.field_value }}</td>
              <td><span class="badge blue">{{ kindLabel(r.change_kind) }}</span></td>
              <td class="small">{{ new Date(r.adopted_at).toLocaleString() }}</td>
              <td class="small mono">文档#{{ r.source_document_id }} / 版本#{{ r.source_version_id }}</td>
            </tr>
          </tbody>
        </table>
      </div>
      <p v-if="!Object.keys(currentByPkg).length" class="muted small">暂无已采用字段（可能正文不完整或仍待人工确认）。</p>
    </div>

    <div class="card">
      <h3>项目时间线（文档版本、更正、复查）</h3>
      <div class="timeline">
        <div v-for="d in timeline.documents" :key="d.id" class="tl-item">
          <div class="row spread">
            <div>
              <span class="badge purple">{{ { notice: '公告', correction: '更正', cancellation: '撤标', cancel_correction: '取消更正' }[d.doc_type] || d.doc_type }}</span>
              <strong class="mono">{{ d.source_ref }}</strong>
              <span class="muted small">（{{ d.version_count }} 个正文版本）</span>
            </div>
            <button class="sm ghost" @click="showVersions(d)">{{ versionsOf[d.id] ? '收起版本' : '查看正文版本与差异' }}</button>
          </div>
          <div v-if="versionsOf[d.id]" class="mt">
            <table>
              <thead><tr><th>版本</th><th>指纹</th><th>解析</th><th>原件</th><th>抓取时间</th></tr></thead>
              <tbody>
                <tr v-for="v in versionsOf[d.id]" :key="v.id">
                  <td>#{{ v.id }}</td><td class="mono small">{{ v.fingerprint.slice(0, 18) }}…</td>
                  <td>
                    <span v-if="v.parsed_ok" class="badge green">解析成功</span>
                    <span v-else class="badge red">解析失败/半份</span>
                    <div v-if="v.parse_error" class="small muted">{{ v.parse_error }}</div>
                  </td>
                  <td><button class="sm ghost" @click="showRaw(v)">查看原件（{{ v.raw_len }} 字符）</button></td>
                  <td class="small">{{ new Date(v.fetched_at).toLocaleString() }}</td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>

        <div v-for="c in timeline.corrections" :key="'c' + c.id" class="tl-item">
          <span class="badge" :class="badgeFor(c.status)[0]">{{ badgeFor(c.status)[1] }}</span>
          <strong class="mono small">{{ c.correction_no }}</strong>
          <span class="muted small">
            作用范围：{{ c.scope_all_packages ? '全部标包' : (c.scope_package_nos || []).join('、') || '未声明' }}
          </span>
          <RouterLink class="small" :to="`/corrections/${c.id}`">处理更正 →</RouterLink>
          <div v-if="c.contradictory_note" class="small" style="color:var(--danger)">{{ c.contradictory_note }}</div>
        </div>

        <div v-for="r in timeline.review_runs" :key="'r' + r.id" class="tl-item">
          <span class="badge" :class="resultBadge(r.result)[0]">{{ resultBadge(r.result)[1] }}</span>
          <span class="small muted">{{ new Date(r.checked_at).toLocaleString() }} · 文档#{{ r.document_id }}</span>
          <div class="small muted">{{ r.note }}</div>
        </div>
      </div>
    </div>

    <div v-if="rawVersion" class="card">
      <div class="row spread"><h3>正文原件（版本 #{{ rawVersion.id }}，未经解析的原始下载内容）</h3>
        <button class="sm ghost" @click="rawVersion = null">关闭</button></div>
      <iframe class="raw-frame" :srcdoc="rawVersion.html" title="raw"></iframe>
    </div>
  </div>
</template>
