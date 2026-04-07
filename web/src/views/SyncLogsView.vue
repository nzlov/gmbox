<template>
  <q-page class="gmbox-page">
    <div class="gmbox-page-shell">
      <q-card bordered>
        <q-card-section>
          <div class="text-h6 text-weight-bold">同步日志</div>
          <div class="text-body2 text-grey-7 gmbox-section-hint">按时间倒序查看每轮同步汇总，展开后可查看各邮箱同步结果。</div>
        </q-card-section>

        <q-separator />

        <q-card-section>
          <q-banner v-if="error" rounded dense class="gmbox-banner-error gmbox-banner-gap">{{ error }}</q-banner>
          <q-list v-if="logs.length > 0" bordered separator>
            <q-expansion-item
              v-for="item in logs"
              :key="item.id"
              expand-separator
              icon="sync"
              :label="`${formatDate(item.started_at)} · ${item.summary_message || '同步完成'}`"
              :caption="`耗时 ${item.duration_ms} ms · 成功 ${item.success_count}/${item.account_count} · 成功率 ${formatSuccessRate(item.success_rate)}`"
            >
              <q-card flat>
                <q-card-section class="row gmbox-col-gap-md">
                  <div class="col-12 col-md-6">
                    <div>触发方式：{{ item.trigger }}</div>
                    <div class="gmbox-top-gap-sm">开始时间：{{ formatDate(item.started_at) }}</div>
                    <div class="gmbox-top-gap-sm">结束时间：{{ formatDate(item.finished_at) }}</div>
                  </div>
                  <div class="col-12 col-md-6">
                    <div>总耗时：{{ item.duration_ms }} ms</div>
                    <div class="gmbox-top-gap-sm">成功数：{{ item.success_count }} / {{ item.account_count }}</div>
                    <div class="gmbox-top-gap-sm">成功率：{{ formatSuccessRate(item.success_rate) }}</div>
                  </div>
                  <div class="col-12">
                    <q-banner rounded dense class="gmbox-banner-success">
                      {{ item.summary_message || '无详细信息' }}
                    </q-banner>
                  </div>
                </q-card-section>

                <q-separator inset />

                <q-list separator>
                  <q-item v-for="detail in item.details" :key="`${item.id}-${detail.account_id}`">
                    <q-item-section>
                      <q-item-label>{{ detail.account_name || '未命名邮箱' }} / {{ detail.account_email }}</q-item-label>
                      <q-item-label caption>
                        {{ detail.success ? '同步成功' : '同步失败' }}
                        · 新邮件 {{ detail.new_messages }}
                        · 耗时 {{ detail.duration_ms }} ms
                      </q-item-label>
                      <q-item-label v-if="detail.error_message" caption class="text-negative">
                        {{ detail.error_message }}
                      </q-item-label>
                    </q-item-section>
                  </q-item>
                </q-list>
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
import { request, type SyncLogItem, type SyncLogListResponse } from '@/api'

const logs = ref<SyncLogItem[]>([])
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

async function loadLogs(nextPage = page.value) {
  error.value = ''
  try {
    page.value = nextPage
    const params = new URLSearchParams({ page: String(page.value), page_size: String(pageSize.value) })
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

function formatSuccessRate(value: number) {
  return `${Number.isFinite(value) ? value.toFixed(0) : '0'}%`
}

onMounted(async () => {
  await loadLogs(1)
})
</script>
