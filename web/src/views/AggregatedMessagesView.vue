<template>
  <q-page class="q-pa-md">
    <q-card bordered>
      <q-card-section>
        <div class="text-h6 text-weight-bold">聚合消息</div>
        <div class="text-body2 text-grey-7 q-mt-xs">按时间倒序显示所有邮箱、所有文件夹中的邮件。</div>
      </q-card-section>

      <q-separator />

      <q-card-section>
        <q-banner v-if="error" rounded dense class="bg-red-1 text-negative q-mb-md">{{ error }}</q-banner>
        <div v-if="messages.length > 0">
          <MessageThreadCard v-for="item in messages" :key="item.id" :message="item" show-folder @changed="refreshAll" @deleted="removeMessage(item.id)" />
          <q-infinite-scroll ref="infiniteRef" :offset="180" @load="loadMore">
            <template #loading>
              <div class="row justify-center q-my-md"><q-spinner color="primary" size="32px" /></div>
            </template>
          </q-infinite-scroll>
        </div>
        <div v-else class="column items-center justify-center text-center q-py-xl text-grey-7">
          <q-icon name="all_inbox" size="56px" color="grey-5" />
          <div class="text-subtitle1 q-mt-md">暂无邮件</div>
        </div>
      </q-card-section>
    </q-card>

    <q-page-sticky position="bottom-right" :offset="[24, 24]">
      <q-btn round color="primary" icon="refresh" @click="refreshAll"><q-tooltip>刷新列表</q-tooltip></q-btn>
    </q-page-sticky>
  </q-page>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import type { QInfiniteScroll } from 'quasar'
import { request, type MessageItem, type MessageListResponse } from '@/api'
import MessageThreadCard from '@/components/MessageThreadCard.vue'

const messages = ref<MessageItem[]>([])
const error = ref('')
const page = ref(1)
const pageSize = 20
const total = ref(0)
const infiniteRef = ref<QInfiniteScroll | null>(null)

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

onMounted(refreshAll)
</script>
