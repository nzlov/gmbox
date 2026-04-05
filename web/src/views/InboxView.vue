<template>
  <q-page class="gmbox-page">
    <div class="gmbox-page-shell">
    <div class="row gmbox-col-gap-md">
      <q-card bordered class="col-12 col-lg-auto inbox-sidebar-card">
        <q-card-section>
          <div class="text-subtitle1 text-weight-bold">邮箱与文件夹</div>
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
            </q-item-section>
            <q-item-section side>
              <q-badge color="primary" text-color="white">{{ total }}</q-badge>
            </q-item-section>
          </q-item>
          <q-item
            v-for="mailbox in sortedMailboxes"
            :key="mailbox.id"
            clickable
            :active="selectedFolder === mailbox.path"
            active-class="bg-primary text-white"
            @click="selectFolder(mailbox.path)"
          >
            <q-item-section>
              <q-item-label>{{ mailbox.label }}</q-item-label>
              <q-item-label caption :class="selectedFolder === mailbox.path ? 'text-white' : 'text-grey-6'">{{ mailbox.path }}</q-item-label>
            </q-item-section>
          </q-item>
        </q-list>
      </q-card>

      <q-card bordered class="col-12 col-lg">
        <q-card-section class="row gmbox-col-gap-md items-center">
          <div class="col-12 col-md-8">
            <q-input v-model.trim="keywordInput" outlined dense label="搜索主题、发件人或摘要">
              <template #append>
                <q-btn v-if="keywordInput" flat round dense icon="close" @click="clearKeyword" />
              </template>
            </q-input>
          </div>
        </q-card-section>

        <q-separator />

        <q-card-section>
          <q-banner v-if="error" rounded dense class="gmbox-banner-error gmbox-banner-gap">{{ error }}</q-banner>
          <div v-if="messages.length > 0">
            <MessageThreadCard v-for="item in messages" :key="item.id" :message="item" show-folder @changed="loadMessages(page)" @deleted="handleMessageDeleted(item.id)" @reply="openReplyDialog" />
          </div>
          <div v-else class="gmbox-empty-state">
            <q-icon name="mail_off" size="var(--gmbox-empty-icon-size)" color="grey-5" />
            <div class="text-subtitle1 gmbox-empty-title">暂无匹配邮件</div>
          </div>
        </q-card-section>

        <q-separator />

        <q-card-section class="row items-center justify-center gmbox-inline-gap-sm">
          <q-btn outline color="primary" no-caps :disable="page <= 1" label="上一页" @click="loadMessages(page - 1)" />
          <q-select v-model="pageSize" outlined dense emit-value map-options :options="pageSizeOptions" class="gmbox-page-size-select" @update:model-value="handlePageSizeChange" />
          <div class="text-body2 text-grey-7">第 {{ page }} / {{ totalPages }} 页</div>
          <div class="text-body2 text-grey-7">共 {{ total }} 封</div>
          <q-btn outline color="primary" no-caps :disable="page >= totalPages" label="下一页" @click="loadMessages(page + 1)" />
        </q-card-section>
      </q-card>
    </div>

    <ComposeDialog v-model="showComposeDialog" :preset="composePreset" @sent="loadMessages(page)" />

    <q-page-sticky position="bottom-right" :offset="stickyOffset">
      <HoverActionFab primary-icon="edit_square" primary-label="写信" secondary-icon="refresh" secondary-label="刷新列表" @primary="openComposeDialog" @secondary="refreshAll" />
    </q-page-sticky>
    </div>
  </q-page>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { request, type MailAccount, type MailboxItem, type MessageItem, type MessageListResponse } from '@/api'
import ComposeDialog from '@/components/ComposeDialog.vue'
import HoverActionFab from '@/components/HoverActionFab.vue'
import MessageThreadCard from '@/components/MessageThreadCard.vue'
import { useResponsiveStickyOffset } from '@/uiMetrics'

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
const showComposeDialog = ref(false)
const composePreset = ref<{ title?: string; account_id?: number; to?: string; subject?: string; body?: string } | null>(null)
const stickyOffset = useResponsiveStickyOffset()
let keywordTimer: ReturnType<typeof setTimeout> | null = null

const totalPages = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)))
const accountOptions = computed(() => [
  { label: '全部邮箱', value: '' },
  ...accounts.value
    .filter((account) => {
      if (!accountFilter.value.trim()) {
        return true
      }
      const nextKeyword = accountFilter.value.trim().toLowerCase()
      return `${account.name} ${account.email}`.toLowerCase().includes(nextKeyword)
    })
    .map((account) => ({ label: `${account.name} / ${account.email}`, value: String(account.id) })),
])
const pageSizeOptions = [
  { label: '10 条/页', value: 10 },
  { label: '20 条/页', value: 20 },
  { label: '50 条/页', value: 50 },
  { label: '100 条/页', value: 100 },
]
const sortedMailboxes = computed(() =>
  mailboxes.value
    .map((mailbox) => ({ ...mailbox, label: mailboxLabel(mailbox) }))
    .sort((left, right) => mailboxOrder(left) - mailboxOrder(right) || left.label.localeCompare(right.label, 'zh-CN')),
)

async function refreshAll() {
  error.value = ''
  try {
    await Promise.all([loadAccounts(), loadMailboxes(), loadMessages(page.value)])
  } catch (err) {
    error.value = err instanceof Error ? err.message : '刷新失败'
  }
}

async function loadAccounts() {
  accounts.value = await request<MailAccount[]>('/api/accounts')
}

async function loadMailboxes() {
  const query = selectedAccount.value ? `?account_id=${selectedAccount.value}` : ''
  mailboxes.value = await request<MailboxItem[]>(`/api/mailboxes${query}`)
  if (selectedFolder.value && !mailboxes.value.some((item) => item.path === selectedFolder.value)) {
    selectedFolder.value = ''
  }
}

// loadMessages 根据邮箱、文件夹和搜索词刷新传统邮件列表。
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
    const response = await request<MessageListResponse>(`/api/messages?${params.toString()}`)
    messages.value = response.items
    total.value = response.total
    page.value = response.page
  } catch (err) {
    error.value = err instanceof Error ? err.message : '加载失败'
  }
}

async function handleAccountChange() {
  selectedFolder.value = ''
  await Promise.all([loadMailboxes(), loadMessages(1)])
}

function selectFolder(folder: string) {
  selectedFolder.value = folder
  void loadMessages(1)
}

function handlePageSizeChange() {
  void loadMessages(1)
}

function filterAccounts(value: string, update: (callbackFn: () => void) => void) {
  update(() => {
    accountFilter.value = value
  })
}

function clearKeyword() {
  if (keywordTimer) {
    clearTimeout(keywordTimer)
    keywordTimer = null
  }
  keywordInput.value = ''
  keyword.value = ''
  void loadMessages(1)
}

function openComposeDialog() {
  composePreset.value = selectedAccount.value ? { account_id: Number(selectedAccount.value), title: '写信' } : { title: '写信' }
  showComposeDialog.value = true
}

function openReplyDialog(payload: { account_id: number; to: string; subject: string; body: string }) {
  composePreset.value = { title: '回复邮件', ...payload }
  showComposeDialog.value = true
}

function handleMessageDeleted(messageID: number) {
  messages.value = messages.value.filter((item) => item.id !== messageID)
  total.value = Math.max(0, total.value - 1)
}

function mailboxLabel(mailbox: MailboxItem) {
  const value = `${mailbox.role || ''} ${mailbox.path}`.toLowerCase()
  if (value.includes('inbox')) return '收件箱'
  if (value.includes('sent')) return '已发送'
  if (value.includes('draft')) return '草稿箱'
  if (value.includes('trash') || value.includes('deleted')) return '已删除'
  if (value.includes('junk') || value.includes('spam')) return '垃圾邮件'
  if (value.includes('archive')) return '归档'
  return mailbox.name
}

function mailboxOrder(mailbox: MailboxItem & { label: string }) {
  const ordered = ['收件箱', '已发送', '草稿箱', '归档', '垃圾邮件', '已删除']
  const index = ordered.indexOf(mailbox.label)
  return index >= 0 ? index : ordered.length + 1
}

watch(keywordInput, (value) => {
  if (keywordTimer) {
    clearTimeout(keywordTimer)
  }
  keywordTimer = setTimeout(() => {
    keyword.value = value.trim()
    void loadMessages(1)
  }, 300)
})

onBeforeUnmount(() => {
  if (keywordTimer) {
    clearTimeout(keywordTimer)
  }
})

onMounted(refreshAll)
</script>

<style scoped>
.inbox-sidebar-card {
  width: var(--gmbox-sidebar-width-compact);
  max-width: 100%;
}

@media (max-width: 63.9375rem) {
  .inbox-sidebar-card {
    max-width: none;
  }
}
</style>
