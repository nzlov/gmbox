<template>
  <div class="page-shell">
    <aside class="sidebar">
      <div>
        <div class="brand-pill">G</div>
        <h2>gmbox</h2>
      </div>
      <nav class="nav-links">
        <RouterLink to="/inbox">聚合收件箱</RouterLink>
        <RouterLink to="/accounts">邮箱管理</RouterLink>
      </nav>
      <button class="ghost-btn" @click="logout">退出登录</button>
    </aside>

    <main class="content-shell">
      <header class="topbar">
        <div>
          <p class="eyebrow">聚合视图</p>
          <h1>收件箱</h1>
        </div>
        <button class="primary-btn" @click="loadData">刷新</button>
      </header>

      <section class="status-grid">
        <article class="status-card">
          <span>邮件总数</span>
          <strong>{{ messages.length }}</strong>
        </article>
        <article class="status-card">
          <span>邮箱总数</span>
          <strong>{{ accounts.length }}</strong>
        </article>
      </section>

      <section class="panel">
        <div class="panel-head">
          <h3>聚合邮件列表</h3>
          <span class="muted">当前版本已打通聚合查询接口，后续可在同步器中接入实际抓取逻辑。</span>
        </div>
        <div v-if="error" class="error-text">{{ error }}</div>
        <div v-if="messages.length === 0" class="empty-state">暂无本地邮件，可先新增邮箱并触发同步。</div>
        <div v-for="item in messages" :key="item.id" class="mail-item">
          <div>
            <strong>{{ item.subject || '(无主题)' }}</strong>
            <p>{{ item.from_name || item.from_address }}</p>
          </div>
          <div class="mail-meta">
            <span>{{ item.snippet || '暂无摘要' }}</span>
            <time>{{ formatDate(item.sent_at) }}</time>
          </div>
        </div>
      </section>
    </main>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { request, type MailAccount, type MessageItem } from '@/api'

const router = useRouter()
const accounts = ref<MailAccount[]>([])
const messages = ref<MessageItem[]>([])
const error = ref('')

// loadData 并行刷新邮箱和聚合邮件列表，保持首页信息同步。
async function loadData() {
  error.value = ''
  try {
    const [accountList, messageList] = await Promise.all([
      request<MailAccount[]>('/api/accounts'),
      request<MessageItem[]>('/api/messages'),
    ])
    accounts.value = accountList
    messages.value = messageList
  } catch (err) {
    error.value = err instanceof Error ? err.message : '加载失败'
  }
}

// logout 通过后端清理 Cookie，避免前端误判登录状态。
async function logout() {
  await request('/api/auth/logout', { method: 'POST' })
  await router.push('/login')
}

// formatDate 统一处理时间显示，避免不同浏览器直接输出格式不一致。
function formatDate(value: string) {
  if (!value) {
    return '刚刚'
  }
  return new Date(value).toLocaleString('zh-CN')
}

onMounted(loadData)
</script>
