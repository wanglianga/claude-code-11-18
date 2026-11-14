<script setup>
import { onMounted, ref } from 'vue'
import { RouterLink, useRouter } from 'vue-router'
import { get, post } from '../api/client'

const props = defineProps({ id: { type: [String, Number], required: true } })
const router = useRouter()
const head = ref({})
const changes = ref([])
const msg = ref('')
const busy = ref({})

async function load() {
  const data = await get(`/corrections/${props.id}`)
  head.value = data.correction
  changes.value = data.field_changes
}

async function confirmField(fc) {
  busy.value[fc.id] = true
  try {
    await post(`/corrections/${props.id}/fields/confirm`, { field: fc.field_name, package: fc.package_no })
    msg.value = `字段「${fc.field_name}」已确认，形成新的采用修订；其他字段不受影响`
    await load()
  } catch (e) {
    msg.value = e.message
  } finally {
    busy.value[fc.id] = false
  }
}
async function returnField(fc) {
  busy.value[fc.id] = true
  try {
    await post(`/corrections/${props.id}/fields/return`, { field: fc.field_name, package: fc.package_no })
    msg.value = `字段「${fc.field_name}」已退回补查，当前采用值不变`
    await load()
  } catch (e) {
    msg.value = e.message
  } finally {
    busy.value[fc.id] = false
  }
}
async function returnAll() {
  await post(`/corrections/${props.id}/return`, { note: '整份更正退回补查' })
  router.push('/workbench')
}

function decisionBadge(d) {
  return { pending: ['amber', '待确认'], confirmed: ['green', '已确认'], returned: ['gray', '已退回'] }[d] || ['gray', d]
}

onMounted(load)
</script>

<template>
  <div>
    <RouterLink to="/workbench" class="small">← 返回待核实工作台</RouterLink>
    <div class="card mt">
      <div class="row spread">
        <div>
          <h2 style="margin-bottom:4px">更正处理 <span class="mono muted small">{{ head.correction_no }}</span></h2>
          <p class="muted small">
            文档 #{{ head.document_id }} ｜
            作用范围：{{ head.scope_all_packages ? '全部标包（可共同更新）' : ((head.scope_package_nos || []).join('、') || '未声明，仅项目级字段受限处理') }}
          </p>
        </div>
        <span class="badge" :class="decisionBadge(head.status)[0]">
          {{ { pending: '待处理', contradictory: '矛盾待核实', confirmed: '已全部确认', returned: '已退回', superseded: '已被取消通知取代' }[head.status] || head.status }}
        </span>
      </div>
      <div v-if="head.contradictory_note" class="notice danger mt">{{ head.contradictory_note }}</div>
      <div v-if="head.status === 'superseded'" class="notice warn mt">
        该更正已被取消通知取代，其内容不得再确认；被取消的内容仍完整保留，请改以取消通知载明的取值为准（不会简单回退到最早值）。
      </div>
      <div v-if="msg" class="notice info mt">{{ msg }}</div>
    </div>

    <div class="card">
      <h3>逐字段核对（原采用值 / 新提取值 / 原文片段）</h3>
      <p class="muted small">只有明确适用的标包字段才会出现；不同标包的截止时间分别保存，未逐字段确认前不替换当前采用结论。</p>
      <div v-for="fc in changes" :key="fc.id" class="card" style="box-shadow:none;border:1px solid var(--line)">
        <div class="row spread">
          <div>
            <strong>{{ fc.field_name }}</strong>
            <span class="muted small">（{{ fc.package_no || '项目级' }}）</span>
            <span class="badge" :class="decisionBadge(fc.decision)[0]">{{ decisionBadge(fc.decision)[1] }}</span>
          </div>
          <div class="row" v-if="head.status !== 'superseded'">
            <button class="sm" :disabled="busy[fc.id] || fc.decision === 'confirmed'" @click="confirmField(fc)">确认此字段</button>
            <button class="sm warn" :disabled="busy[fc.id] || fc.decision === 'returned'" @click="returnField(fc)">退回补查</button>
          </div>
        </div>
        <div class="grid cols-2 mt">
          <div>
            <label>原采用值（当前生效）</label>
            <div class="diff-old">{{ fc.old_value || '（此前无采用值，确认后作为初始采用）' }}</div>
          </div>
          <div>
            <label>新提取值</label>
            <div class="diff-new">{{ fc.new_value }}</div>
          </div>
        </div>
        <div class="mt">
          <label>原文位置：{{ fc.source_locator }}</label>
          <div class="excerpt">{{ fc.source_excerpt }}</div>
        </div>
      </div>
      <div class="row mt" v-if="head.status !== 'superseded'">
        <button class="warn" @click="returnAll">整份更正退回补查</button>
        <span class="muted small">退回不改变任何当前采用值</span>
      </div>
    </div>
  </div>
</template>
