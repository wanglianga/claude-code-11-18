<script setup>
import { onMounted, onUnmounted, ref } from 'vue'
import { RouterLink } from 'vue-router'
import { get } from '../api/client'

const projects = ref([])

function statusBadge(s) {
  return {
    active: ['green', '有效（已关联）'], candidate: ['amber', '候选待确认'],
    withdrawn: ['red', '已撤标'], closed: ['gray', '已关闭'],
  }[s] || ['gray', s]
}

async function load() {
  const data = await get('/projects')
  projects.value = data.projects
}
onMounted(load)
const timer = setInterval(load, 5000)
onUnmounted(() => clearInterval(timer))
</script>

<template>
  <div>
    <h2>采购项目（项目与标包分别计数）</h2>
    <div class="notice info">
      提示：来源网页是否存在不等于业务状态——失效链接只记为“不可访问”；只有明确撤标公告才会把项目置为“已撤标”。
      长期未复查的项目仅展示最后核查时间，不能声称仍有效。
    </div>
    <div class="card">
      <table>
        <thead>
          <tr><th>采购编号</th><th>项目标题</th><th>状态</th><th>标包数</th><th>当前采用字段</th><th>最后核查</th><th></th></tr>
        </thead>
        <tbody>
          <tr v-for="p in projects" :key="p.id">
            <td class="mono">{{ p.project_no || '（缺编号）' }}</td>
            <td>{{ p.title }}</td>
            <td><span class="badge" :class="statusBadge(p.status)[0]">{{ statusBadge(p.status)[1] }}</span>
              <div v-if="p.status_reason" class="small muted">{{ p.status_reason }}</div>
            </td>
            <td>{{ p.package_count }}</td>
            <td>{{ p.adopted_count }}</td>
            <td class="small">
              <template v-if="p.last_checked_at">{{ new Date(p.last_checked_at).toLocaleString() }}
                <div class="muted">{{ p.check_note }}</div>
              </template>
              <span v-else class="muted">尚未核查</span>
            </td>
            <td><RouterLink class="small" :to="`/projects/${p.id}`">查看时间线 →</RouterLink></td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>
