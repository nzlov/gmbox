<template>
  <q-page class="gmbox-page">
    <div class="gmbox-page-shell">
    <q-card bordered>
      <q-card-section class="row items-center justify-between gmbox-col-gap-md">
        <div class="col">
          <div class="text-h6 text-weight-bold">同步日志</div>
          <div class="text-body2 text-grey-7 gmbox-section-hint">按时间倒序查看邮箱同步历史，点击行可展开详细结果。</div>
        </div>
        <div class="col-12 col-md-4">
          <q-select v-model="selectedAccountID" outlined dense emit-value map-options :options="accountOptions" label="筛选邮箱" @update:model-value="loadLogs(1)" />
        </div>
      </q-card-section>

      <q-separator />

      <q-card-section>
        <q-banner v-if="error" rounded dense class="gmbox-banner-error gmbox-banner-gap">{{ error }}</q-banner>
        <q-list v-if="logs.length > 0" bordered separator>
          <q-expansion-item v-for="item in logs" :key="item.id" expand-separator icon="sync" :label="`${item.account_name} / ${item.account_email}`" :caption="`${formatDate(item.started_at)} · ${item.success ? '成功' : '失败'}`">
            <q-card flat>
              <q-card-section class="row gmbox-col-gap-md">
                <div class="col-12 col-md-6">
                  <div>触发方式：{{ item.trigger }}</div>
                  <div class="gmbox-top-gap-sm">协议：{{ item.protocol.toUpperCase() }}</div>
                  <div class="gmbox-top-gap-sm">耗时：{{ item.duration_ms }} ms</div>
                  <div class="gmbox-top-gap-sm">新邮件：{{ item.new_messages }}</div>
                </div>
                <div class="col-12 col-md-6">
                  <div>文件夹数：{{ item.mailbox_count }}</div>
                  <div class="gmbox-top-gap-sm">自动刷新 OAuth：{{ item.retried_oauth ? '是' : '否' }}</div>
                  <div class="gmbox-top-gap-sm">结束时间：{{ formatDate(item.finished_at) }}</div>
                  <div class="gmbox-top-gap-sm">结果：{{ item.success ? '成功' : '失败' }}</div>
                </div>
                <div class="col-12">
                  <q-banner rounded dense :class="item.success ? 'gmbox-banner-success' : 'gmbox-banner-error'">
                    {{ item.error_message || item.summary_message || '无详细信息' }}
                  </q-banner>
                </div>
              </q-card-section>
            </q-card>
          </q-expansion-item>
        </q-list>
        <div v-else class="gmbox-empty-state">
          <q-icon name="history_toggle_off" size="var(--gmbox-empty-icon-size)" color="grey-5" />
          <div class="text-subtitle1 gmbox-empty-title">暂无同步日志</div>
        </div>
      </q-card-section>

      <q-separator />

      <q-card-section class="row items-center justify-center gmbox-inline-gap-sm">
        <q-btn outline color="primary" no-caps :disable="page <= 1" label="上一页" @click="loadLogs(page - 1)" />
        <q-select v-model="pageSize" outlined dense emit-value map-options :options="pageSizeOptions" class="gmbox-page-size-select" @update:model-value="loadLogs(1)" />
        <div class="text-body2 text-grey-7">第 {{ page }} / {{ totalPages }} 页</div>
        <div class="text-body2 text-grey-7">共 {{ total }} 条</div>
        <q-btn outline color="primary" no-caps :disable="page >= totalPages" label="下一页" @click="loadLogs(page + 1)" />
      </q-card-section>
    </q-card>
    </div>
  </q-page>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { request, type MailAccount, type SyncLogItem, type SyncLogListResponse } from '@/api'

const accounts = ref<MailAccount[]>([])
const logs = ref<SyncLogItem[]>([])
const selectedAccountID = ref('')
const page = ref(1)
const pageSize = ref(20)
const total = ref(0)
const error = ref('')

const totalPages = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)))
const pageSizeOptions = [
  { label: '10 条/页', value: 10 },
  { label: '20 条/页', value: 20 },
  { label: '50 条/页', value: 50 },
]
const accountOptions = computed(() => [
  { label: '全部邮箱', value: '' },
  ...accounts.value.map((item) => ({ label: `${item.name} / ${item.email}`, value: String(item.id) })),
])

async function loadAccounts() {
  accounts.value = await request<MailAccount[]>('/api/accounts')
}

async function loadLogs(nextPage = page.value) {
  error.value = ''
  try {
    page.value = nextPage
    const params = new URLSearchParams({ page: String(page.value), page_size: String(pageSize.value) })
    if (selectedAccountID.value) {
      params.set('account_id', selectedAccountID.value)
    }
    const response = await request<SyncLogListResponse>(`/api/sync-logs?${params.toString()}`)
    logs.value = response.items
    total.value = response.total
    page.value = response.page
  } catch (err) {
    error.value = err instanceof Error ? err.message : '加载同步日志失败'
  }
}

function formatDate(value: string) {
  return value ? new Date(value).toLocaleString('zh-CN') : '刚刚'
}

onMounted(async () => {
  await loadAccounts()
  await loadLogs(1)
})
</script>
