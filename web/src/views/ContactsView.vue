<template>
  <q-page class="gmbox-page">
    <div class="gmbox-page-shell">
    <div class="row gmbox-col-gap-md contacts-layout">
      <q-card bordered class="col-12 col-lg-auto contacts-sidebar-card">
        <q-card-section class="row gmbox-col-gap-sm items-center">
          <div class="col">
            <q-input v-model.trim="contactKeyword" outlined dense label="搜索发件人" />
          </div>
          <div class="col-auto">
            <q-btn round color="primary" icon="edit_square" @click="openComposeForSelected"><q-tooltip>写信</q-tooltip></q-btn>
          </div>
        </q-card-section>

        <q-list bordered separator>
          <q-item v-for="item in contacts" :key="item.address" clickable :active="selectedAddress === item.address" active-class="bg-primary text-white" @click="selectContact(item.address)">
            <q-item-section>
              <q-item-label>{{ item.name || item.address }}</q-item-label>
              <q-item-label caption :class="selectedAddress === item.address ? 'text-white' : 'text-grey-6'">{{ item.address }}</q-item-label>
            </q-item-section>
          </q-item>
        </q-list>
      </q-card>

      <q-card bordered class="col-12 col-lg">
        <q-card-section class="row items-center justify-between gmbox-col-gap-md">
          <div class="col">
            <div class="text-h6 text-weight-bold">{{ selectedAddress ? selectedContactName : '联系人邮件' }}</div>
            <div class="text-body2 text-grey-7">{{ selectedAddress || '请选择左侧联系人后查看邮件列表' }}</div>
          </div>
        </q-card-section>

        <q-separator />

        <q-card-section>
          <q-banner v-if="error" rounded dense class="gmbox-banner-error gmbox-banner-gap">{{ error }}</q-banner>
          <div v-if="selectedAddress && messages.length > 0">
            <MessageThreadCard v-for="item in messages" :key="item.id" :message="item" :initial-expanded="true" :collapsible="false" hide-sender show-reply :show-folder="!selectedAddress" @changed="loadMessages(page)" @deleted="removeMessage(item.id)" @reply="openReplyDialog" />
          </div>
          <div v-else-if="!selectedAddress" class="gmbox-empty-state">
            <q-icon name="groups" size="var(--gmbox-empty-icon-size)" color="grey-5" />
            <div class="text-subtitle1 gmbox-empty-title">请选择联系人</div>
            <div class="text-body2 gmbox-empty-text">仅在选择联系人后加载并显示邮件列表</div>
          </div>
          <div v-else class="gmbox-empty-state">
            <q-icon name="group_off" size="var(--gmbox-empty-icon-size)" color="grey-5" />
            <div class="text-subtitle1 gmbox-empty-title">暂无联系人邮件</div>
          </div>
        </q-card-section>

        <q-separator />

        <q-card-section class="row items-center justify-center gmbox-inline-gap-sm">
          <q-btn outline color="primary" no-caps :disable="!selectedAddress || page <= 1" label="上一页" @click="loadMessages(page - 1)" />
          <q-select v-model="pageSize" outlined dense emit-value map-options :options="pageSizeOptions" class="gmbox-page-size-select" :disable="!selectedAddress" @update:model-value="loadMessages(1)" />
          <div class="text-body2 text-grey-7">第 {{ page }} / {{ totalPages }} 页</div>
          <div class="text-body2 text-grey-7">共 {{ total }} 封</div>
          <q-btn outline color="primary" no-caps :disable="!selectedAddress || page >= totalPages" label="下一页" @click="loadMessages(page + 1)" />
        </q-card-section>
      </q-card>
    </div>

    <ComposeDialog v-model="showComposeDialog" :preset="composePreset" @sent="loadMessages(page)" />

    <q-page-sticky position="bottom-right" :offset="stickyOffset">
      <HoverActionFab primary-icon="edit_square" primary-label="写信" secondary-icon="refresh" secondary-label="刷新列表" @primary="openComposeForSelected" @secondary="refreshAll" />
    </q-page-sticky>
    </div>
  </q-page>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { request, type ContactItem, type ContactListResponse, type MessageItem, type MessageListResponse } from '@/api'
import ComposeDialog from '@/components/ComposeDialog.vue'
import HoverActionFab from '@/components/HoverActionFab.vue'
import MessageThreadCard from '@/components/MessageThreadCard.vue'
import { useResponsiveStickyOffset } from '@/uiMetrics'

const contacts = ref<ContactItem[]>([])
const messages = ref<MessageItem[]>([])
const selectedAddress = ref('')
const contactKeyword = ref('')
const error = ref('')
const page = ref(1)
const pageSize = ref(20)
const total = ref(0)
const showComposeDialog = ref(false)
const composePreset = ref<{ title?: string; account_id?: number; to?: string; subject?: string; body?: string } | null>(null)
const stickyOffset = useResponsiveStickyOffset()
let keywordTimer: ReturnType<typeof setTimeout> | null = null

const totalPages = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)))
const selectedContactName = computed(() => contacts.value.find((item) => item.address === selectedAddress.value)?.name || selectedAddress.value)
const pageSizeOptions = [
  { label: '10 条/页', value: 10 },
  { label: '20 条/页', value: 20 },
  { label: '50 条/页', value: 50 },
]

async function loadContacts() {
  const params = new URLSearchParams({ page: '1', page_size: '100' })
  if (contactKeyword.value.trim()) {
    params.set('keyword', contactKeyword.value.trim())
  }
  const response = await request<ContactListResponse>(`/api/contacts?${params.toString()}`)
  contacts.value = response.items
  if (selectedAddress.value && !contacts.value.some((item) => item.address === selectedAddress.value)) {
    selectedAddress.value = ''
  }
}

async function loadMessages(nextPage = page.value) {
  if (!selectedAddress.value) {
    error.value = ''
    messages.value = []
    total.value = 0
    page.value = 1
    return
  }
  error.value = ''
  try {
    page.value = nextPage
    const params = new URLSearchParams({ page: String(page.value), page_size: String(pageSize.value) })
    const endpoint = `/api/contact-messages?address=${encodeURIComponent(selectedAddress.value)}&${params.toString()}`
    const response = await request<MessageListResponse>(endpoint)
    messages.value = response.items
    total.value = response.total
    page.value = response.page
  } catch (err) {
    error.value = err instanceof Error ? err.message : '加载联系人邮件失败'
  }
}

async function refreshAll() {
  try {
    await loadContacts()
    await loadMessages(1)
  } catch (err) {
    error.value = err instanceof Error ? err.message : '刷新失败'
  }
}

function selectContact(address: string) {
  selectedAddress.value = address
  void loadMessages(1)
}

function openComposeForSelected() {
  const recentAccountID = messages.value[0]?.account_id
  composePreset.value = {
    title: selectedAddress.value ? '给联系人写信' : '写信',
    account_id: recentAccountID,
    to: selectedAddress.value,
  }
  showComposeDialog.value = true
}

function openReplyDialog(payload: { account_id: number; to: string; subject: string; body: string }) {
  composePreset.value = { title: '回复邮件', ...payload }
  showComposeDialog.value = true
}

function removeMessage(messageID: number) {
  messages.value = messages.value.filter((item) => item.id !== messageID)
  total.value = Math.max(0, total.value - 1)
}

watch(contactKeyword, () => {
  if (keywordTimer) {
    clearTimeout(keywordTimer)
  }
  keywordTimer = setTimeout(async () => {
    await loadContacts()
    await loadMessages(1)
  }, 300)
})

onMounted(refreshAll)

onBeforeUnmount(() => {
  if (keywordTimer) {
    clearTimeout(keywordTimer)
  }
})
</script>

<style scoped>
.contacts-sidebar-card {
  width: var(--gmbox-sidebar-width);
  max-width: 100%;
}

@media (max-width: 63.9375rem) {
  .contacts-layout {
    flex-direction: column;
  }

  .contacts-sidebar-card {
    width: 100%;
  }
}
</style>
