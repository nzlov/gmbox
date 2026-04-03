<template>
  <div class="page-shell">
    <aside class="sidebar">
      <div>
        <div class="brand-pill">G</div>
        <h2>gmbox</h2>
      </div>
      <nav class="nav-links">
        <RouterLink to="/inbox">聚合信息</RouterLink>
        <RouterLink to="/compose">写信</RouterLink>
        <RouterLink to="/accounts">邮箱管理</RouterLink>
      </nav>
      <button class="ghost-btn sidebar-logout" @click="logout">退出登录</button>
    </aside>

    <main class="content-shell">
      <header class="topbar">
        <div>
          <p class="eyebrow">聚合视图</p>
          <h1>聚合信息</h1>
        </div>
        <button class="primary-btn" @click="refreshAll">刷新</button>
      </header>

      <section class="inbox-layout">
        <aside class="panel inbox-sidebar-panel">
          <div class="panel-head panel-head-stack">
            <div>
              <h3>邮箱与文件夹</h3>
              <span class="muted">邮箱切换后联动刷新文件夹与邮件列表。</span>
            </div>
          </div>

          <select v-model="selectedAccount" @change="handleAccountChange">
            <option value="">全部邮箱</option>
            <option v-for="account in accounts" :key="account.id" :value="String(account.id)">
              {{ account.name }} / {{ account.email }}
            </option>
          </select>

          <div class="folder-list">
            <button
              type="button"
              class="folder-item"
              :class="{ active: selectedFolder === '' }"
              @click="selectFolder('')"
            >
              <span>全部文件夹</span>
              <small>{{ total }}</small>
            </button>
            <button
              v-for="mailbox in mailboxes"
              :key="mailbox.id"
              type="button"
              class="folder-item"
              :class="{ active: selectedFolder === mailbox.path }"
              @click="selectFolder(mailbox.path)"
            >
              <span>{{ mailbox.name }}</span>
              <small>{{ mailbox.role || mailbox.path }}</small>
            </button>
          </div>
        </aside>

        <section class="panel inbox-main-panel">
          <div class="panel-head panel-head-stack panel-tools">
            <div>
              <h3>邮件列表</h3>
              <span class="muted">按时间倒序展示，搜索结果受左侧邮箱与文件夹选择约束。</span>
            </div>
            <div class="toolbar-grid">
              <div class="search-box">
                <input v-model.trim="keywordInput" placeholder="搜索主题、发件人或摘要" @keyup.enter="applyKeywordNow" />
                <button v-if="keywordInput" type="button" class="search-clear-btn" @click="clearKeyword">清空</button>
              </div>
              <div class="toolbar-actions">
                <select v-model.number="pageSize" @change="handlePageSizeChange">
                  <option :value="10">10 条/页</option>
                  <option :value="20">20 条/页</option>
                  <option :value="50">50 条/页</option>
                  <option :value="100">100 条/页</option>
                </select>
                <button class="ghost-btn" @click="applyKeywordNow">搜索</button>
              </div>
            </div>
          </div>

          <div v-if="error" class="error-text">{{ error }}</div>
          <div v-else-if="messages.length === 0" class="empty-state">暂无匹配邮件，可调整筛选条件后重试。</div>
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

          <div class="pagination-bar">
            <span class="muted">共 {{ total }} 封，第 {{ page }} / {{ totalPages }} 页</span>
            <div class="toolbar-actions">
              <button class="ghost-btn" :disabled="page <= 1" @click="loadMessages(page - 1)">上一页</button>
              <button class="ghost-btn" :disabled="page >= totalPages" @click="loadMessages(page + 1)">下一页</button>
            </div>
          </div>
        </section>
      </section>
    </main>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { request, type MailAccount, type MailboxItem, type MessageItem, type MessageListResponse } from '@/api'

const router = useRouter()
const accounts = ref<MailAccount[]>([])
const mailboxes = ref<MailboxItem[]>([])
const messages = ref<MessageItem[]>([])
const error = ref('')
const selectedAccount = ref('')
const selectedFolder = ref('')
const keyword = ref('')
const keywordInput = ref('')
const page = ref(1)
const pageSize = ref(20)
const total = ref(0)
let keywordTimer: ReturnType<typeof setTimeout> | null = null

const totalPages = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)))

// refreshAll 统一刷新邮箱、文件夹和邮件列表，避免筛选项与数据源脱节。
async function refreshAll() {
  error.value = ''
  try {
    await Promise.all([loadAccounts(), loadMailboxes(), loadMessages(page.value)])
  } catch (err) {
    error.value = err instanceof Error ? err.message : '刷新失败'
  }
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

// loadMessages 根据左侧条件和顶部搜索词加载分页邮件列表。
async function loadMessages(nextPage = page.value) {
  try {
    error.value = ''
    page.value = nextPage
    const params = new URLSearchParams()
    if (selectedAccount.value) {
      params.set('account_id', selectedAccount.value)
    }
    if (selectedFolder.value) {
      params.set('folder', selectedFolder.value)
    }
    if (keyword.value) {
      params.set('keyword', keyword.value)
    }
    params.set('page', String(page.value))
    params.set('page_size', String(pageSize.value))
    const query = params.toString() ? `?${params.toString()}` : ''
    const response = await request<MessageListResponse>(`/api/messages${query}`)
    messages.value = response.items
    total.value = response.total
    page.value = response.page
  } catch (err) {
    error.value = err instanceof Error ? err.message : '加载失败'
  }
}

// handleAccountChange 切换邮箱后清空文件夹并回到第一页，避免旧筛选残留。
async function handleAccountChange() {
  selectedFolder.value = ''
  page.value = 1
  await Promise.all([loadMailboxes(), loadMessages(1)])
}

// selectFolder 通过左侧文件夹按钮联动右侧邮件列表。
function selectFolder(folder: string) {
  selectedFolder.value = folder
  void loadMessages(1)
}

// handlePageSizeChange 切换分页大小后强制回到第一页，避免页码越界。
function handlePageSizeChange() {
  void loadMessages(1)
}

// applyKeywordNow 在用户主动确认时立即应用搜索词，避免回车仍等待防抖延迟。
function applyKeywordNow() {
  if (keywordTimer) {
    clearTimeout(keywordTimer)
    keywordTimer = null
  }
  keyword.value = keywordInput.value.trim()
  void loadMessages(1)
}

// clearKeyword 统一清空输入和已生效搜索词，并立即恢复默认结果。
function clearKeyword() {
  if (keywordTimer) {
    clearTimeout(keywordTimer)
    keywordTimer = null
  }
  keywordInput.value = ''
  keyword.value = ''
  void loadMessages(1)
}

// watch(keywordInput) 为搜索输入增加防抖，减少连续输入时的重复请求。
watch(keywordInput, (value) => {
  if (keywordTimer) {
    clearTimeout(keywordTimer)
  }
  keywordTimer = setTimeout(() => {
    keyword.value = value.trim()
    void loadMessages(1)
  }, 300)
})

// openDetail 进入详情页，以便继续查看正文和执行操作。
async function openDetail(messageID: number) {
  if (!Number.isFinite(messageID) || messageID <= 0) {
    error.value = '邮件 ID 无效，无法打开详情'
    return
  }
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

// onBeforeUnmount 清理残留定时器，避免页面切换后还触发旧搜索请求。
onBeforeUnmount(() => {
  if (keywordTimer) {
    clearTimeout(keywordTimer)
  }
})

onMounted(refreshAll)
</script>
