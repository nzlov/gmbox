<template>
  <q-page class="gmbox-page">
    <div class="gmbox-page-shell">
      <div class="row gmbox-col-gap-md contacts-layout">
        <q-card bordered class="col-12 col-lg-auto contacts-sidebar-card contacts-panel-card">
          <q-card-section class="row gmbox-col-gap-sm items-center">
            <div class="col">
              <q-input v-model.trim="contactKeyword" outlined dense label="搜索发件人" />
            </div>
            <div class="col-auto">
              <q-btn round color="primary" icon="hub" @click="openCreateAggregateDialog"><q-tooltip>聚合</q-tooltip></q-btn>
            </div>
          </q-card-section>

          <q-separator />

          <div class="contacts-sidebar-scroll">
            <q-list bordered separator>
              <q-item v-for="item in contacts" :key="item.address" clickable :active="selectedAddress === item.address" active-class="bg-primary text-white" @click="selectContact(item.address)">
                <q-item-section>
                  <q-item-label class="row items-center no-wrap gmbox-inline-gap-sm">
                    <span class="ellipsis">{{ item.name || item.address }}</span>
                    <q-icon v-if="item.member_count > 1" name="hub" size="18px">
                      <q-tooltip>已聚合 {{ item.member_count }} 个联系人</q-tooltip>
                    </q-icon>
                    <q-btn
                      v-if="item.member_count > 1"
                      flat
                      round
                      dense
                      size="sm"
                      color="secondary"
                      icon="edit"
                      @click.stop="openEditAggregateDialog(item)"
                    >
                      <q-tooltip>编辑聚合</q-tooltip>
                    </q-btn>
                  </q-item-label>
                  <q-item-label caption :class="selectedAddress === item.address ? 'text-white' : 'text-grey-6'">{{ item.address }}</q-item-label>
                </q-item-section>
              </q-item>
            </q-list>
          </div>
        </q-card>

        <q-card bordered class="col-12 col-lg contacts-panel-card">
          <q-card-section class="row items-start justify-between gmbox-col-gap-md">
            <div class="col">
              <div class="text-h6 text-weight-bold">{{ selectedAddress ? selectedContactName : '联系人邮件' }}</div>
              <div class="text-body2 text-grey-7">{{ selectedAddress || '请选择左侧联系人后查看邮件列表' }}</div>
              <div v-if="selectedContact && selectedContact.member_count > 1" class="row items-center gmbox-inline-gap-sm contacts-members">
                <q-chip
                  v-for="member in selectedContact.members"
                  :key="member.address"
                  dense
                  :color="member.address === selectedContact.address ? 'primary' : 'grey-3'"
                  :text-color="member.address === selectedContact.address ? 'white' : 'dark'"
                >
                  {{ member.name || member.address }}
                  <span class="contacts-member-address">{{ member.address }}</span>
                </q-chip>
              </div>
            </div>
          </q-card-section>

          <q-separator />

          <div class="contacts-message-scroll">
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
          </div>

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

      <q-dialog v-model="showAggregateDialog" @hide="closeAggregateDialog">
        <q-card class="full-width gmbox-dialog-wide">
          <q-card-section class="row items-start justify-between gmbox-col-gap-md">
            <div class="col">
              <div class="text-h6 text-weight-bold">{{ aggregateDialogTitle }}</div>
              <div class="text-body2 text-grey-7">通过搜索联系人列表维护聚合关系，支持新增邮箱、调整主联系人和解散聚合。</div>
            </div>
            <div class="col-auto">
              <q-btn flat round dense icon="close" @click="closeAggregateDialog" />
            </div>
          </q-card-section>

          <q-separator />

          <q-card-section class="column gmbox-col-gap-md">
            <q-input v-model.trim="aggregateKeyword" outlined dense label="搜索联系人" />
            <div class="row gmbox-col-gap-md aggregate-editor-layout">
              <div class="col-12 col-md-6">
                <div class="text-subtitle2 text-weight-medium gmbox-bottom-gap-sm">候选联系人</div>
                <q-list bordered separator class="aggregate-list">
                  <q-item v-for="item in aggregateCandidateItems" :key="item.address">
                    <q-item-section>
                      <q-item-label>{{ item.name || item.address }}</q-item-label>
                      <q-item-label caption>{{ item.address }}</q-item-label>
                    </q-item-section>
                    <q-item-section side>
                      <q-btn flat round dense color="secondary" icon="add_circle" @click="addAggregateMember(item.address)">
                        <q-tooltip>加入聚合</q-tooltip>
                      </q-btn>
                    </q-item-section>
                  </q-item>
                  <div v-if="aggregateCandidateItems.length === 0" class="aggregate-empty text-body2 text-grey-6">没有可加入的联系人</div>
                </q-list>
              </div>
              <div class="col-12 col-md-6">
                <div class="text-subtitle2 text-weight-medium gmbox-bottom-gap-sm">已选联系人</div>
                <q-list bordered separator class="aggregate-list">
                  <q-item v-for="item in aggregateMemberItems" :key="item.address">
                    <q-item-section>
                      <q-item-label>{{ item.name || item.address }}</q-item-label>
                      <q-item-label caption>{{ item.address }}</q-item-label>
                    </q-item-section>
                    <q-item-section side class="row items-center gmbox-inline-gap-sm no-wrap">
                      <q-btn
                        flat
                        dense
                        no-caps
                        :color="item.address === aggregatePrimary ? 'primary' : 'secondary'"
                        :label="item.address === aggregatePrimary ? '主联系人' : '设为主联系人'"
                        @click="aggregatePrimary = item.address"
                      />
                      <q-btn
                        v-if="aggregateSelectedAddresses.length > 1"
                        flat
                        round
                        dense
                        color="negative"
                        icon="remove_circle"
                        @click="removeAggregateMember(item.address)"
                      >
                        <q-tooltip>移出聚合</q-tooltip>
                      </q-btn>
                    </q-item-section>
                  </q-item>
                  <div v-if="aggregateMemberItems.length === 0" class="aggregate-empty text-body2 text-grey-6">请先从左侧加入联系人</div>
                </q-list>
              </div>
            </div>
            <div class="row gmbox-col-gap-sm items-end">
              <div class="col">
                <q-input v-model.trim="manualAggregateAddress" outlined dense label="新增邮箱地址" placeholder="name@example.com" @keyup.enter="addManualAggregateAddress" />
              </div>
              <div class="col-auto">
                <q-btn outline color="secondary" no-caps icon="add_link" label="加入" @click="addManualAggregateAddress" />
              </div>
            </div>
          </q-card-section>

          <q-separator />

          <q-card-actions align="between">
            <div>
              <q-btn v-if="isEditingAggregate" flat no-caps color="negative" label="解散聚合" @click="disbandAggregate" />
            </div>
            <div class="row gmbox-inline-gap-sm">
              <q-btn flat no-caps label="取消" @click="closeAggregateDialog" />
              <q-btn color="primary" no-caps :label="aggregateSubmitLabel" :disable="!canSubmitAggregateDialog" @click="submitAggregateDialog" />
            </div>
          </q-card-actions>
        </q-card>
      </q-dialog>

      <q-page-sticky position="bottom-right" :offset="stickyOffset">
        <HoverActionFab primary-icon="edit_square" primary-label="写信" secondary-icon="refresh" secondary-label="刷新列表" @primary="openComposeForSelected" @secondary="refreshAll" />
      </q-page-sticky>
    </div>
  </q-page>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useQuasar } from 'quasar'
import { request, type ContactItem, type ContactListResponse, type MessageItem, type MessageListResponse } from '@/api'
import ComposeDialog from '@/components/ComposeDialog.vue'
import HoverActionFab from '@/components/HoverActionFab.vue'
import MessageThreadCard from '@/components/MessageThreadCard.vue'
import { useResponsiveStickyOffset } from '@/uiMetrics'

type AggregateDialogMode = 'create' | 'edit'

const $q = useQuasar()
const contacts = ref<ContactItem[]>([])
const messages = ref<MessageItem[]>([])
const selectedAddress = ref('')
const contactKeyword = ref('')
const error = ref('')
const page = ref(1)
const pageSize = ref(20)
const total = ref(0)
const showComposeDialog = ref(false)
const showAggregateDialog = ref(false)
const aggregateDialogMode = ref<AggregateDialogMode>('create')
const editingAggregateAddress = ref('')
const aggregateSelectedAddresses = ref<string[]>([])
const aggregatePrimary = ref('')
const aggregateKeyword = ref('')
const manualAggregateAddress = ref('')
const composePreset = ref<{ title?: string; account_id?: number; to?: string; subject?: string; body?: string } | null>(null)
const stickyOffset = useResponsiveStickyOffset()
let keywordTimer: ReturnType<typeof setTimeout> | null = null

const totalPages = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)))
const selectedContact = computed(() => contacts.value.find((item) => item.address === selectedAddress.value) ?? null)
const selectedContactName = computed(() => selectedContact.value?.name || selectedAddress.value)
const isEditingAggregate = computed(() => aggregateDialogMode.value === 'edit')
const aggregateDialogTitle = computed(() => isEditingAggregate.value ? '编辑联系人聚合' : '聚合联系人')
const aggregateSubmitLabel = computed(() => isEditingAggregate.value ? '保存聚合' : '确认聚合')
const canSubmitAggregateDialog = computed(() => {
  if (!aggregatePrimary.value) {
    return false
  }
  if (isEditingAggregate.value) {
    return aggregateSelectedAddresses.value.length > 0
  }
  return aggregateSelectedAddresses.value.length >= 2
})
const pageSizeOptions = [
  { label: '10 条/页', value: 10 },
  { label: '20 条/页', value: 20 },
  { label: '50 条/页', value: 50 },
]

const aggregateMemberItems = computed(() => {
  const memberMap = new Map(contacts.value.flatMap((item) => item.members.map((member) => [member.address, member])))
  return aggregateSelectedAddresses.value.map((address) => {
    const contact = contacts.value.find((item) => item.address === address)
    if (contact) {
      return { address: contact.address, name: contact.name }
    }
    const member = memberMap.get(address)
    return { address, name: member?.name || '' }
  })
})

const aggregateCandidateItems = computed(() => {
  const selectedSet = new Set(aggregateSelectedAddresses.value)
  const keyword = aggregateKeyword.value.trim().toLowerCase()
  return contacts.value
    .filter((item) => !selectedSet.has(item.address))
    .filter((item) => {
      if (!keyword) {
        return true
      }
      const fields = [item.name, item.address, ...item.members.map((member) => `${member.name} ${member.address}`)]
      return fields.some((field) => field.toLowerCase().includes(keyword))
    })
})

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
  composePreset.value = {
    title: '回复邮件',
    ...payload,
    to: payload.to,
  }
  showComposeDialog.value = true
}

function removeMessage(messageID: number) {
  messages.value = messages.value.filter((item) => item.id !== messageID)
  total.value = Math.max(0, total.value - 1)
}

function openCreateAggregateDialog() {
  aggregateDialogMode.value = 'create'
  editingAggregateAddress.value = ''
  aggregateSelectedAddresses.value = []
  aggregatePrimary.value = ''
  aggregateKeyword.value = ''
  manualAggregateAddress.value = ''
  showAggregateDialog.value = true
}

function openEditAggregateDialog(contact: ContactItem) {
  aggregateDialogMode.value = 'edit'
  editingAggregateAddress.value = contact.address
  aggregateSelectedAddresses.value = contact.members.map((member) => member.address)
  aggregatePrimary.value = contact.address
  aggregateKeyword.value = ''
  manualAggregateAddress.value = ''
  showAggregateDialog.value = true
}

function closeAggregateDialog() {
  showAggregateDialog.value = false
  aggregateKeyword.value = ''
  manualAggregateAddress.value = ''
}

function addAggregateMember(address: string) {
  if (aggregateSelectedAddresses.value.includes(address)) {
    return
  }
  aggregateSelectedAddresses.value = [...aggregateSelectedAddresses.value, address]
  if (!aggregatePrimary.value) {
    aggregatePrimary.value = address
  }
}

function addManualAggregateAddress() {
  const address = manualAggregateAddress.value.trim().toLowerCase()
  if (!address) {
    return
  }
  addAggregateMember(address)
  manualAggregateAddress.value = ''
}

function removeAggregateMember(address: string) {
  aggregateSelectedAddresses.value = aggregateSelectedAddresses.value.filter((item) => item !== address)
  if (aggregatePrimary.value === address) {
    aggregatePrimary.value = aggregateSelectedAddresses.value[0] || ''
  }
}

async function submitAggregateDialog() {
  if (!aggregatePrimary.value || aggregateSelectedAddresses.value.length === 0) {
    return
  }
  try {
    if (isEditingAggregate.value && editingAggregateAddress.value) {
      await request<{ message: string }>('/api/contacts/aggregate', {
        method: 'PUT',
        body: JSON.stringify({
          current_address: editingAggregateAddress.value,
          primary_address: aggregatePrimary.value,
          addresses: aggregateSelectedAddresses.value,
        }),
      })
    } else if (aggregateSelectedAddresses.value.length >= 2) {
      await request<{ message: string }>('/api/contacts/aggregate', {
        method: 'POST',
        body: JSON.stringify({ primary_address: aggregatePrimary.value, addresses: aggregateSelectedAddresses.value }),
      })
    }
    selectedAddress.value = aggregatePrimary.value || aggregateSelectedAddresses.value[0] || ''
    closeAggregateDialog()
    await refreshAll()
    $q.notify({ type: 'positive', message: isEditingAggregate.value ? '聚合关系已更新' : '联系人聚合成功' })
  } catch (err) {
    $q.notify({ type: 'negative', message: err instanceof Error ? err.message : '保存聚合失败' })
  }
}

async function disbandAggregate() {
  if (!editingAggregateAddress.value) {
    return
  }
  try {
    await request<{ message: string }>('/api/contacts/separate', {
      method: 'POST',
      body: JSON.stringify({ addresses: [editingAggregateAddress.value] }),
    })
    if (selectedAddress.value === editingAggregateAddress.value) {
      selectedAddress.value = ''
    }
    closeAggregateDialog()
    await refreshAll()
    $q.notify({ type: 'positive', message: '聚合已解散' })
  } catch (err) {
    $q.notify({ type: 'negative', message: err instanceof Error ? err.message : '解散聚合失败' })
  }
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

watch(aggregateSelectedAddresses, (value) => {
  if (value.length === 0) {
    aggregatePrimary.value = ''
    return
  }
  if (!value.includes(aggregatePrimary.value)) {
    aggregatePrimary.value = value[0]
  }
})

onMounted(refreshAll)

onBeforeUnmount(() => {
  if (keywordTimer) {
    clearTimeout(keywordTimer)
  }
})
</script>

<style scoped>
.contacts-layout {
  align-items: stretch;
}

.contacts-panel-card {
  display: flex;
  flex-direction: column;
  height: calc(100vh - 10rem);
  overflow: hidden;
}

.contacts-sidebar-card {
  width: var(--gmbox-sidebar-width);
  max-width: 100%;
}

.contacts-sidebar-scroll,
.contacts-message-scroll {
  flex: 1;
  overflow: auto;
}

.contacts-members {
  margin-top: 0.75rem;
}

.aggregate-editor-layout {
  align-items: stretch;
}

.aggregate-list {
  max-height: 20rem;
  overflow: auto;
}

.aggregate-empty {
  padding: 1rem;
  text-align: center;
}

.contacts-member-address {
  margin-left: 0.375rem;
  opacity: 0.7;
}

@media (max-width: 63.9375rem) {
  .contacts-layout {
    flex-direction: column;
  }

  .contacts-panel-card {
    height: auto;
  }

  .contacts-sidebar-card {
    width: 100%;
  }

  .contacts-sidebar-scroll,
  .contacts-message-scroll {
    overflow: visible;
  }
}
</style>
