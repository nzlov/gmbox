<template>
  <AppShell
    active="inbox"
    eyebrow="聚合视图"
    title="聚合信息"
    subtitle="按账户、文件夹与关键词交叉筛选邮件，保持多邮箱处理链路集中在一个页面。"
    @logout="logout"
  >
    <template #actions>
      <q-btn flat round dense icon="refresh" @click="refreshAll" />
    </template>

    <template #hero-actions>
      <q-fab color="primary" icon="refresh" direction="down" vertical-actions-align="right">
        <q-tooltip>刷新操作</q-tooltip>

        <q-fab-action color="primary" icon="refresh" label="刷新列表" label-position="left" @click="refreshAll">
          <q-tooltip>刷新列表</q-tooltip>
        </q-fab-action>
      </q-fab>
    </template>

    <div class="row q-col-gutter-md">
      <div class="col-12 col-lg-4 col-xl-3">
        <q-card bordered>
          <q-card-section>
            <div class="text-subtitle1 text-weight-bold">邮箱与文件夹</div>
            <div class="text-body2 text-grey-7 q-mt-xs">切换账户后会联动刷新文件夹与邮件列表。</div>
          </q-card-section>
          <q-card-section class="q-pt-none">
            <q-select
              v-model="selectedAccount"
              outlined
              dense
              use-input
              input-debounce="0"
              fill-input
              hide-selected
              emit-value
              map-options
              :options="accountOptions"
              label="筛选邮箱"
              @filter="filterAccounts"
              @update:model-value="handleAccountChange"
            />
          </q-card-section>
          <q-list bordered separator>
            <q-item clickable :active="selectedFolder === ''" active-class="bg-primary text-white" @click="selectFolder('')">
              <q-item-section>
                <q-item-label>全部文件夹</q-item-label>
                <q-item-label caption :class="selectedFolder === '' ? 'text-white' : 'text-grey-6'">显示当前筛选下的所有邮件</q-item-label>
              </q-item-section>
              <q-item-section side>
                <q-badge color="primary" text-color="white">{{ total }}</q-badge>
              </q-item-section>
            </q-item>
            <q-item
              v-for="mailbox in mailboxes"
              :key="mailbox.id"
              clickable
              :active="selectedFolder === mailbox.path"
              active-class="bg-primary text-white"
              @click="selectFolder(mailbox.path)"
            >
              <q-item-section>
                <q-item-label>{{ mailbox.name }}</q-item-label>
                <q-item-label caption :class="selectedFolder === mailbox.path ? 'text-white' : 'text-grey-6'">
                  {{ mailbox.role || mailbox.path }}
                </q-item-label>
              </q-item-section>
            </q-item>
          </q-list>
        </q-card>
      </div>

      <div class="col-12 col-lg-8 col-xl-9">
        <q-card bordered>
          <q-card-section class="row q-col-gutter-md items-center">
            <div class="col-12 col-md-6">
              <q-input v-model.trim="keywordInput" outlined dense label="搜索主题、发件人或摘要" @keyup.enter="applyKeywordNow">
                <template #append>
                  <q-btn v-if="keywordInput" flat round dense icon="close" @click="clearKeyword" />
                </template>
              </q-input>
            </div>
            <div class="col-12 col-md row q-gutter-sm justify-end">
              <q-select
                v-model="pageSize"
                outlined
                dense
                emit-value
                map-options
                :options="pageSizeOptions"
                style="min-width: 128px"
                @update:model-value="handlePageSizeChange"
              />
              <q-btn color="primary" unelevated no-caps icon="search" label="搜索" @click="applyKeywordNow" />
            </div>
          </q-card-section>

          <q-separator />

          <q-card-section>
            <q-banner v-if="error" rounded dense class="bg-red-1 text-negative q-mb-md">
              {{ error }}
            </q-banner>

            <q-list v-if="messages.length > 0" bordered separator>
              <q-item v-for="item in messages" :key="item.id" clickable @click="openDetail(item.id)">
                <q-item-section>
                  <q-item-label :class="item.is_read ? 'text-subtitle2' : 'text-subtitle2 text-weight-bold'">
                    {{ item.subject || '(无主题)' }}
                  </q-item-label>
                  <q-item-label caption class="q-mt-xs">{{ item.from_name || item.from_address }}</q-item-label>
                  <q-item-label caption class="q-mt-xs text-grey-7">{{ item.snippet || '暂无摘要' }}</q-item-label>
                  <div class="row q-gutter-sm q-mt-sm">
                    <q-badge color="grey-3" text-color="dark">{{ item.folder }}</q-badge>
                    <q-badge v-if="item.has_attachment" color="grey-3" text-color="dark">含附件</q-badge>
                    <q-badge v-if="!item.is_read" color="primary" text-color="white">未读</q-badge>
                  </div>
                </q-item-section>
                <q-item-section side top>
                  <div class="text-caption text-grey-6">{{ formatDate(item.sent_at) }}</div>
                </q-item-section>
              </q-item>
            </q-list>

            <div v-else class="column items-center justify-center text-center q-py-xl text-grey-7">
              <q-icon name="inbox" size="56px" color="grey-5" />
              <div class="text-subtitle1 q-mt-md">暂无匹配邮件</div>
              <div class="text-body2 q-mt-xs">可以调整账户、文件夹或搜索词后重试。</div>
            </div>
          </q-card-section>

          <q-separator />

          <q-card-section class="row items-center justify-between q-gutter-sm">
            <div class="text-body2 text-grey-7">共 {{ total }} 封，第 {{ page }} / {{ totalPages }} 页</div>
            <div class="row q-gutter-sm">
              <q-btn outline color="primary" no-caps :disable="page <= 1" label="上一页" @click="loadMessages(page - 1)" />
              <q-btn outline color="primary" no-caps :disable="page >= totalPages" label="下一页" @click="loadMessages(page + 1)" />
            </div>
          </q-card-section>
        </q-card>
      </div>
    </div>
  </AppShell>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { request, type MailAccount, type MailboxItem, type MessageItem, type MessageListResponse } from '@/api'
import AppShell from '@/components/AppShell.vue'

const router = useRouter()
const accounts = ref<MailAccount[]>([])
const mailboxes = ref<MailboxItem[]>([])
const messages = ref<MessageItem[]>([])
const error = ref('')
const selectedAccount = ref('')
const selectedFolder = ref('')
const accountFilter = ref('')
const keyword = ref('')
const keywordInput = ref('')
const page = ref(1)
const pageSize = ref(20)
const total = ref(0)
let keywordTimer: ReturnType<typeof setTimeout> | null = null

const totalPages = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)))
const accountOptions = computed(() => [
  { label: '全部邮箱', value: '' },
  ...accounts.value
    .filter((account) => {
      if (!accountFilter.value.trim()) {
        return true
      }
      const keyword = accountFilter.value.trim().toLowerCase()
      return `${account.name} ${account.email}`.toLowerCase().includes(keyword)
    })
    .map((account) => ({ label: `${account.name} / ${account.email}`, value: String(account.id) })),
])
const pageSizeOptions = [
  { label: '10 条/页', value: 10 },
  { label: '20 条/页', value: 20 },
  { label: '50 条/页', value: 50 },
  { label: '100 条/页', value: 100 },
]

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

// filterAccounts 允许在邮箱筛选下拉中按名称或地址输入搜索，减少账号较多时的滚动查找成本。
function filterAccounts(value: string, update: (callbackFn: () => void) => void) {
  update(() => {
    accountFilter.value = value
  })
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
