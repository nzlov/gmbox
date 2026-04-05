<template>
  <q-page class="gmbox-page">
    <div class="gmbox-page-shell">
    <q-card bordered>
      <q-card-section>
        <div class="text-h6 text-weight-bold">聚合消息</div>
        <div class="text-body2 text-grey-7 gmbox-section-hint">按时间倒序显示所有邮箱、所有文件夹中的邮件。</div>
      </q-card-section>

      <q-separator />

      <q-card-section>
        <q-banner v-if="error" rounded dense class="gmbox-banner-error gmbox-banner-gap">{{ error }}</q-banner>
        <div v-if="messages.length > 0">
          <MessageThreadCard v-for="item in messages" :key="item.id" :message="item" show-folder @changed="refreshAll" @deleted="removeMessage(item.id)" @reply="openReplyDialog" />
          <q-infinite-scroll ref="infiniteRef" :offset="180" @load="loadMore">
            <template #loading>
              <div class="row justify-center gmbox-block-gap-lg"><q-spinner color="primary" size="var(--gmbox-spinner-size)" /></div>
            </template>
          </q-infinite-scroll>
        </div>
        <div v-else class="gmbox-empty-state">
          <q-icon name="all_inbox" size="var(--gmbox-empty-icon-size)" color="grey-5" />
          <div class="text-subtitle1 gmbox-empty-title">暂无邮件</div>
        </div>
      </q-card-section>
    </q-card>

    <q-page-sticky position="bottom-right" :offset="stickyOffset">
      <q-btn round color="primary" icon="refresh" @click="refreshAll"><q-tooltip>刷新列表</q-tooltip></q-btn>
    </q-page-sticky>

    <ComposeDialog v-model="showComposeDialog" :preset="composePreset" @sent="refreshAll" />
    </div>
  </q-page>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import type { QInfiniteScroll } from 'quasar'
import { request, type MessageItem, type MessageListResponse } from '@/api'
import ComposeDialog from '@/components/ComposeDialog.vue'
import MessageThreadCard from '@/components/MessageThreadCard.vue'
import { useResponsiveStickyOffset } from '@/uiMetrics'

const messages = ref<MessageItem[]>([])
const error = ref('')
const page = ref(1)
const pageSize = 20
const total = ref(0)
const infiniteRef = ref<QInfiniteScroll | null>(null)
const showComposeDialog = ref(false)
const composePreset = ref<{ title?: string; account_id?: number; to?: string; subject?: string; body?: string } | null>(null)
const stickyOffset = useResponsiveStickyOffset()

async function fetchPage(targetPage: number) {
  const response = await request<MessageListResponse>(`/api/messages?page=${targetPage}&page_size=${pageSize}`)
  total.value = response.total
  page.value = response.page
  return response.items
}

async function refreshAll() {
  error.value = ''
  try {
    page.value = 1
    messages.value = await fetchPage(1)
    infiniteRef.value?.resume()
    if (messages.value.length >= total.value) {
      infiniteRef.value?.stop()
    }
  } catch (err) {
    error.value = err instanceof Error ? err.message : '刷新失败'
  }
}

async function loadMore(index: number, done: (stop?: boolean) => void) {
  try {
    const nextPage = index + 1
    const items = await fetchPage(nextPage)
    if (items.length === 0) {
      done(true)
      return
    }
    messages.value = [...messages.value, ...items]
    done(messages.value.length >= total.value)
  } catch (err) {
    error.value = err instanceof Error ? err.message : '加载更多失败'
    done(true)
  }
}

function removeMessage(messageID: number) {
  messages.value = messages.value.filter((item) => item.id !== messageID)
  total.value = Math.max(0, total.value - 1)
}

// openReplyDialog 让聚合列表展开后的详情也能直接进入回复流程。
function openReplyDialog(payload: { account_id: number; to: string; subject: string; body: string }) {
  composePreset.value = { title: '回复邮件', ...payload }
  showComposeDialog.value = true
}

onMounted(refreshAll)
</script>
