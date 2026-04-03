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
        <RouterLink to="/compose">写信</RouterLink>
      </nav>
      <button class="ghost-btn" @click="logout">退出登录</button>
    </aside>

    <main class="content-shell">
      <header class="topbar">
        <div>
          <p class="eyebrow">聚合视图</p>
          <h1>收件箱</h1>
        </div>
        <button class="primary-btn" @click="refreshAll">刷新</button>
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

      <section class="panel filter-bar">
        <select v-model="selectedAccount" @change="refreshAll">
          <option value="">全部邮箱</option>
          <option v-for="account in accounts" :key="account.id" :value="String(account.id)">
            {{ account.name }} / {{ account.email }}
          </option>
        </select>
        <select v-model="selectedFolder" @change="loadMessages">
          <option value="">全部文件夹</option>
          <option v-for="mailbox in mailboxes" :key="mailbox.id" :value="mailbox.path">
            {{ mailbox.name }}
          </option>
        </select>
      </section>

      <section class="panel">
        <div class="panel-head">
          <h3>聚合邮件列表</h3>
          <span class="muted">支持多文件夹同步、详情查看与附件下载。</span>
        </div>
        <div v-if="error" class="error-text">{{ error }}</div>
        <div v-if="messages.length === 0" class="empty-state">暂无本地邮件，可先新增邮箱并触发同步。</div>
        <button
          v-for="item in messages"
          :key="item.id"
          type="button"
          class="mail-item mail-button"
          @click="openDetail(item.id)"
        >
          <div>
            <strong :class="item.is_read ? 'mail-read' : 'mail-unread'">{{ item.subject || '(无主题)' }}</strong>
            <p>{{ item.from_name || item.from_address }}</p>
            <small>{{ item.folder }}</small>
          </div>
          <div class="mail-meta">
            <span>{{ item.snippet || '暂无摘要' }}</span>
            <small v-if="item.has_attachment">含附件</small>
            <time>{{ formatDate(item.sent_at) }}</time>
          </div>
        </button>
      </section>
    </main>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { request, type MailAccount, type MailboxItem, type MessageItem } from '@/api'

const router = useRouter()
const accounts = ref<MailAccount[]>([])
const mailboxes = ref<MailboxItem[]>([])
const messages = ref<MessageItem[]>([])
const error = ref('')
const selectedAccount = ref('')
const selectedFolder = ref('')

// refreshAll 统一刷新邮箱、文件夹和邮件列表，避免筛选项与数据源脱节。
async function refreshAll() {
  await Promise.all([loadAccounts(), loadMailboxes(), loadMessages()])
}

// loadAccounts 加载邮箱列表，供筛选器和首页统计共用。
async function loadAccounts() {
  accounts.value = await request<MailAccount[]>('/api/accounts')
}

// loadMailboxes 根据当前邮箱筛选刷新文件夹列表。
async function loadMailboxes() {
  const query = selectedAccount.value ? `?account_id=${selectedAccount.value}` : ''
  mailboxes.value = await request<MailboxItem[]>(`/api/mailboxes${query}`)
  if (selectedFolder.value && !mailboxes.value.some((item) => item.path === selectedFolder.value)) {
    selectedFolder.value = ''
  }
}

// loadMessages 根据邮箱和文件夹筛选加载邮件列表。
async function loadMessages() {
  error.value = ''
  try {
    const params = new URLSearchParams()
    if (selectedAccount.value) {
      params.set('account_id', selectedAccount.value)
    }
    if (selectedFolder.value) {
      params.set('folder', selectedFolder.value)
    }
    const query = params.toString() ? `?${params.toString()}` : ''
    messages.value = await request<MessageItem[]>(`/api/messages${query}`)
  } catch (err) {
    error.value = err instanceof Error ? err.message : '加载失败'
  }
}

// openDetail 进入详情页，以便继续查看正文和执行操作。
async function openDetail(messageID: number) {
  await router.push(`/messages/${messageID}`)
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

onMounted(refreshAll)
</script>
