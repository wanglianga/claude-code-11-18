<script setup>
import { onMounted, onUnmounted, ref } from 'vue'
import { RouterLink } from 'vue-router'
import { get, post } from '../api/client'

const tab = ref('corrections')
const corrections = ref([])
const candidates = ref([])
const projects = ref([])
const msg = ref('')

// 候选确认表单
const confirmForm = ref({})
function formFor(id) {
  if (!confirmForm.value[id]) confirmForm.value[id] = {}
  return confirmForm.value[id]
}

async function load() {
  const [c, lc, p] = await Promise.all([
    get('/corrections?status=pending'),
    get('/candidates?status=pending'),
    get('/projects'),
  ])
  corrections.value = c.corrections
  candidates.value = lc.candidates
  projects.value = p.projects
}

async function confirmCandidate(lc) {
  const f = confirmForm.value[lc.id] || {}
  if (f.new_project) {
    await post(`/candidates/${lc.id}/confirm`, { new_project: true, project_no: f.project_no, title: f.title, note: f.note })
  } else {
    if (!f.project_id) { msg.value = '请选择归属项目，或勾选新建项目'; return }
    await post(`/candidates/${lc.id}/confirm`, { project_id: Number(f.project_id), note: f.note })
  }
  msg.value = '关联已确认，文档已按所选项目重新处理'
  await load()
}
async function rejectCandidate(lc) {
  await post(`/candidates/${lc.id}/reject`, { note: '采购专员判定不属于现有项目' })
  await load()
}
async function recheckCandidate(lc) {
  await post(`/candidates/${lc.id}/recheck`, { note: '依据不足，退回补查' })
  await load()
}

function corrBadge(s) {
  return { pending: ['amber', '待处理'], contradictory: ['red', '矛盾待核实'], confirmed: ['green', '已确认'],
    returned: ['gray', '已退回'], superseded: ['purple', '已被取消'] }[s] || ['gray', s]
}

onMounted(load)
const timer = setInterval(load, 6000)
onUnmounted(() => clearInterval(timer))
</script>

<template>
  <div>
    <h2>待核实工作台</h2>
    <div v-if="msg" class="notice info">{{ msg }}</div>
    <div class="tabs">
      <button :class="tab === 'corrections' ? 'active' : ''" @click="tab = 'corrections'">
        更正通知<span v-if="corrections.length" class="pill-count">{{ corrections.length }}</span>
      </button>
      <button :class="tab === 'candidates' ? 'active' : ''" @click="tab = 'candidates'">
        关联候选（缺依据）<span v-if="candidates.length" class="pill-count">{{ candidates.length }}</span>
      </button>
    </div>

    <div v-if="tab === 'corrections'" class="card">
      <p class="muted small">发布时间晚不代表必然取代早公告；只有明确引用对象与作用范围、且无矛盾的更正才能逐字段确认。未确认内容不会替换当前采用值。</p>
      <table>
        <thead><tr><th>更正编号</th><th>来源公告</th><th>标题</th><th>作用范围</th><th>状态/原因</th><th></th></tr></thead>
        <tbody>
          <tr v-for="c in corrections" :key="c.id">
            <td class="mono small">{{ c.correction_no }}</td>
            <td class="mono small">{{ c.source_ref }}</td><td>{{ c.title }}</td>
            <td class="small">{{ c.scope_all_packages ? '全部标包' : (c.scope_package_nos || []).join('、') || '未声明' }}</td>
            <td><span class="badge" :class="corrBadge(c.status)[0]">{{ corrBadge(c.status)[1] }}</span>
              <div v-if="c.contradictory_note" class="small muted">{{ c.contradictory_note }}</div></td>
            <td><RouterLink class="small" :to="`/corrections/${c.id}`">进入处理 →</RouterLink></td>
          </tr>
        </tbody>
      </table>
      <p v-if="!corrections.length" class="muted small">暂无待处理更正。</p>
    </div>

    <div v-if="tab === 'candidates'" class="card">
      <p class="muted small">以下公告缺少自动关联依据（如未标注采购编号）。标题相近不等于同一项目，须由采购专员确认归属。</p>
      <div v-for="lc in candidates" :key="lc.id" class="card" style="box-shadow:none">
        <div class="row spread">
          <div>
            <strong class="mono">{{ lc.source_ref }}</strong> <span class="muted small">{{ lc.title }}</span>
            <div class="small" style="color:var(--warn)">{{ lc.reason }}</div>
          </div>
          <div class="row">
            <button class="sm" @click="confirmCandidate(lc)">确认关联</button>
            <button class="sm warn" @click="recheckCandidate(lc)">退回补查</button>
            <button class="sm danger" @click="rejectCandidate(lc)">拒绝</button>
          </div>
        </div>
        <div class="grid cols-2 mt">
          <div class="field">
            <label>归属到已有项目</label>
            <select v-model="formFor(lc.id).project_id">
              <option value="">请选择项目…</option>
              <option v-for="p in projects" :key="p.id" :value="p.id">{{ p.project_no }} · {{ p.title }}</option>
            </select>
          </div>
          <div class="field">
            <label>或新建项目（勾选后填写编号与标题）</label>
            <div class="row">
              <input placeholder="采购编号" v-model="formFor(lc.id).project_no" />
              <input placeholder="项目标题" v-model="formFor(lc.id).title" />
              <label class="row" style="margin:0;white-space:nowrap">
                <input type="checkbox" style="width:auto" v-model="formFor(lc.id).new_project" /> 新建
              </label>
            </div>
          </div>
        </div>
        <div class="field"><label>处理备注</label><input v-model="formFor(lc.id).note" placeholder="确认/退回依据" /></div>
      </div>
      <p v-if="!candidates.length" class="muted small">暂无待确认关联候选。</p>
    </div>
  </div>
</template>
