<template>
  <q-page class="q-pa-md">
    <q-card bordered>
      <q-card-section v-if="(message || error) && !showModal && !showImportModal" class="q-pt-none">
        <q-banner v-if="message" rounded class="bg-green-1 text-positive q-mb-sm">{{ message }}</q-banner>
        <q-banner v-if="error" rounded class="bg-red-1 text-negative">{{ error }}</q-banner>
      </q-card-section>

      <q-table
        v-model:selected="selectedRows"
        flat
        row-key="id"
        selection="multiple"
        :rows="accounts"
        :columns="columns"
        :filter="tableFilter"
        :pagination="tablePagination"
        :rows-per-page-options="[10, 20, 50, 100]"
        no-data-label="暂无邮箱，请先添加。"
      >
        <template #top>
          <div class="full-width row q-col-gutter-md items-center">
            <div class="col-12 col-xl-5">
              <q-input v-model.trim="tableFilter" outlined dense clearable label="搜索名称、邮箱或服务商" />
            </div>
            <div class="col-12 col-xl row q-gutter-sm wrap justify-end">
              <q-btn outline color="primary" no-caps :disable="!hasSelection" label="启用" @click="batchUpdateEnabled(true)" />
              <q-btn outline color="primary" no-caps :disable="!hasSelection" label="禁用" @click="batchUpdateEnabled(false)" />
              <q-btn outline color="secondary" no-caps :disable="!hasSelection" label="同步" @click="batchSync" />
              <q-btn outline color="secondary" no-caps :disable="!hasSelection" label="测试" @click="batchTest" />
              <q-btn outline color="negative" no-caps :disable="!hasSelection" label="删除" @click="batchDelete" />
            </div>
          </div>
        </template>

        <template #body-cell-auth_type="props">
          <q-td :props="props">
            <q-badge :color="props.row.auth_type === 'oauth' ? 'primary' : 'grey-4'" :text-color="props.row.auth_type === 'oauth' ? 'white' : 'dark'">
              {{ props.row.auth_type === 'oauth' ? 'OAuth' : '密码' }}
            </q-badge>
          </q-td>
        </template>

        <template #body-cell-enabled="props">
          <q-td :props="props">
            <q-badge :color="props.row.enabled ? 'positive' : 'grey-4'" :text-color="props.row.enabled ? 'white' : 'dark'">
              {{ props.row.enabled ? '已启用' : '已禁用' }}
            </q-badge>
          </q-td>
        </template>

        <template #body-cell-actions="props">
          <q-td :props="props">
            <div class="row q-gutter-xs no-wrap">
              <q-btn flat dense no-caps color="primary" label="编辑" @click="openEditModal(props.row)" />
              <q-btn flat dense no-caps color="secondary" label="测试" @click="test(props.row.id)" />
              <q-btn flat dense no-caps color="secondary" label="同步" @click="sync(props.row.id)" />
            </div>
          </q-td>
        </template>
      </q-table>
    </q-card>

    <q-dialog v-model="showModal" persistent @hide="closeModal">
        <q-card class="full-width" style="max-width: 920px">
        <q-card-section class="row items-start justify-between q-col-gutter-md">
          <div class="col">
            <div class="text-h6 text-weight-bold">{{ editingID ? '修改邮箱' : '添加邮箱' }}</div>
            <div class="text-body2 text-grey-7 q-mt-xs">选择常见服务商后自动填充 IMAP、POP3、SMTP，仍可继续修改。</div>
          </div>
          <div class="col-auto">
            <q-btn flat round dense icon="close" @click="closeModal" />
          </div>
        </q-card-section>

        <q-separator />

        <q-card-section>
          <q-banner v-if="showModal && error" rounded class="bg-red-1 text-negative q-mb-md">
            {{ error }}
          </q-banner>
          <q-banner v-if="showModal && message" rounded class="bg-green-1 text-positive q-mb-md">
            {{ message }}
          </q-banner>

          <q-form class="row q-col-gutter-md" @submit.prevent="submit">
            <div class="col-12">
              <q-select
                v-model="form.provider"
                outlined
                dense
                emit-value
                map-options
                :options="providerOptions"
                label="服务商"
                @update:model-value="handleProviderChange"
              />
            </div>

            <div v-if="form.provider === 'custom'" class="col-12">
              <q-input v-model="form.provider_name" outlined dense label="自定义服务商名称" placeholder="例如：公司邮箱" />
            </div>

            <div class="col-12 col-md-6">
              <q-input v-model="form.name" outlined dense label="展示名称" />
            </div>
            <div class="col-12 col-md-6">
              <q-input v-model="form.email" outlined dense label="邮箱地址" />
            </div>
            <div class="col-12 col-md-6">
              <q-input v-model="form.username" outlined dense label="登录用户名" hint="默认建议填邮箱地址" />
            </div>
            <div class="col-12 col-md-6">
              <q-select
                v-model="form.auth_type"
                outlined
                dense
                emit-value
                map-options
                :options="authOptions"
                label="认证方式"
                @update:model-value="handleAuthTypeChange"
              />
            </div>

            <div v-if="form.auth_type === 'password'" class="col-12">
              <q-input
                v-model="form.password"
                outlined
                dense
                type="password"
                label="密码或授权码"
                :placeholder="editingID ? '留空则保持原密码' : '密码或授权码'"
              />
            </div>

            <div v-else class="col-12">
              <q-banner rounded class="bg-blue-1 text-primary">
                <div class="text-subtitle2 text-weight-medium">微软 OAuth 授权</div>
                <div class="text-body2 q-mt-xs">完成授权后会自动接入 Outlook / Microsoft 365 邮箱，并可继续修改 IMAP、SMTP 配置。</div>
                <template #action>
                  <q-btn
                    flat
                    no-caps
                    color="primary"
                    :disable="!microsoftOAuthEnabled"
                    :label="microsoftOAuthEnabled ? '连接微软邮箱' : '未配置微软 OAuth'"
                    @click="startMicrosoftOAuth"
                  />
                </template>
              </q-banner>
            </div>

            <div class="col-12 col-md-6">
              <q-select
                v-model="form.incoming_protocol"
                outlined
                dense
                emit-value
                map-options
                :options="protocolOptions"
                label="收信协议"
                :disable="form.auth_type === 'oauth'"
              />
            </div>
            <div class="col-12 col-md-6 row items-center">
              <q-toggle v-model="form.use_tls" color="primary" label="启用 TLS" :disable="form.auth_type === 'oauth'" />
            </div>

            <template v-if="form.incoming_protocol === 'imap'">
              <div class="col-12 col-md-6">
                <q-input v-model="form.imap_host" outlined dense label="IMAP Host" />
              </div>
              <div class="col-12 col-md-6">
                <q-input v-model.number="form.imap_port" outlined dense type="number" label="IMAP Port" />
              </div>
            </template>
            <template v-else>
              <div class="col-12 col-md-6">
                <q-input v-model="form.pop3_host" outlined dense label="POP3 Host" />
              </div>
              <div class="col-12 col-md-6">
                <q-input v-model.number="form.pop3_port" outlined dense type="number" label="POP3 Port" />
              </div>
            </template>

            <div class="col-12 col-md-6">
              <q-input v-model="form.smtp_host" outlined dense label="SMTP Host" />
            </div>
            <div class="col-12 col-md-6">
              <q-input v-model.number="form.smtp_port" outlined dense type="number" label="SMTP Port" />
            </div>
            <div class="col-12">
              <q-toggle v-model="form.enabled" color="primary" label="启用账户" />
            </div>
            <div class="col-12 row justify-end q-gutter-sm">
              <q-btn flat no-caps label="取消" @click="closeModal" />
              <q-btn color="primary" unelevated no-caps type="submit" :label="editingID ? '保存修改' : '保存邮箱'" />
            </div>
          </q-form>
        </q-card-section>
      </q-card>
    </q-dialog>

    <q-dialog v-model="showImportModal" persistent @hide="closeImportModal">
        <q-card class="full-width" style="max-width: 920px">
        <q-card-section class="row items-start justify-between q-col-gutter-md">
          <div class="col">
            <div class="text-h6 text-weight-bold">批量导入非 OAuth 邮箱</div>
            <div class="text-body2 text-grey-7 q-mt-xs">每行一个邮箱，支持英文逗号或制表符分隔，已知服务商可自动补齐服务器配置。</div>
          </div>
          <div class="col-auto">
            <q-btn flat round dense icon="close" @click="closeImportModal" />
          </div>
        </q-card-section>

        <q-separator />

        <q-card-section class="column q-gutter-md">
          <q-banner v-if="showImportModal && error" rounded class="bg-red-1 text-negative">
            {{ error }}
          </q-banner>
          <q-banner v-if="showImportModal && message" rounded class="bg-green-1 text-positive">
            {{ message }}
          </q-banner>

          <q-input
            v-model="importText"
            outlined
            autogrow
            type="textarea"
            label="导入内容"
            placeholder="name,email,username,password,provider,protocol,incoming_host,incoming_port,smtp_host,smtp_port,use_tls"
          />

          <q-banner rounded class="bg-grey-1 text-grey-8">
            <div>列顺序：名称、邮箱、用户名、密码、服务商、协议、收信主机、收信端口、SMTP 主机、SMTP 端口、是否 TLS。</div>
            <div class="q-mt-sm">服务商可填：gmail、qq、163、126、aliyun、outlook、yahoo、custom。已知服务商留空主机和端口时会自动补齐。</div>
            <div class="q-mt-sm">示例：张三,zs@example.com,zs@example.com,authcode123,gmail,imap,,,,,true</div>
          </q-banner>

          <div class="row justify-end q-gutter-sm">
            <q-btn flat no-caps label="取消" @click="closeImportModal" />
            <q-btn color="primary" unelevated no-caps label="开始导入" @click="submitImport" />
          </div>
        </q-card-section>
      </q-card>
    </q-dialog>

    <q-page-sticky position="bottom-right" :offset="[24, 24]">
      <q-fab color="primary" icon="add" direction="up" vertical-actions-align="right">
        <q-tooltip>添加操作</q-tooltip>

        <q-fab-action color="primary" icon="person_add" label="添加邮箱" label-position="left" @click="openCreateModal">
          <q-tooltip>添加邮箱</q-tooltip>
        </q-fab-action>

        <q-fab-action color="secondary" icon="upload_file" label="批量导入" label-position="left" @click="openImportModal">
          <q-tooltip>批量导入</q-tooltip>
        </q-fab-action>
      </q-fab>
    </q-page-sticky>
  </q-page>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  request,
  type AccountProvidersResponse,
  type MailAccount,
  type MicrosoftOAuthConfigResponse,
  type ProviderPreset,
} from '@/api'
import { createCodeChallenge, createMicrosoftOAuthSession, saveMicrosoftOAuthSession } from '@/utils/oauth'

type MicrosoftOAuthMessage = {
  type: 'microsoft-oauth'
  success: boolean
  message: string
}

type AccountForm = {
  provider: string
  provider_name: string
  auth_type: 'password' | 'oauth'
  name: string
  email: string
  username: string
  password: string
  incoming_protocol: 'imap' | 'pop3'
  imap_host: string
  imap_port: number
  pop3_host: string
  pop3_port: number
  smtp_host: string
  smtp_port: number
  use_tls: boolean
  enabled: boolean
}

const router = useRouter()
const route = useRoute()
const accounts = ref<MailAccount[]>([])
const providers = ref<ProviderPreset[]>([])
const microsoftOAuthEnabled = ref(false)
const error = ref('')
const message = ref('')
const showModal = ref(false)
const showImportModal = ref(false)
const editingID = ref<number | null>(null)
const selectedRows = ref<MailAccount[]>([])
const importText = ref('')
const tableFilter = ref('')
const tablePagination = { rowsPerPage: 10 }
const form = reactive<AccountForm>(createDefaultForm())
let oauthPopup: Window | null = null

const hasSelection = computed(() => selectedRows.value.length > 0)
const currentProviderPreset = computed(() => providers.value.find((item) => item.key === form.provider) ?? null)
const oauthAvailable = computed(() => form.provider === 'outlook' && microsoftOAuthEnabled.value)
const providerOptions = computed(() => providers.value.map((item) => ({ label: item.name, value: item.key })))
const authOptions = computed(() => [
  { label: '密码 / 授权码', value: 'password' },
  { label: '微软 OAuth', value: 'oauth', disable: !oauthAvailable.value },
])
const protocolOptions = computed(() => [
  { label: 'IMAP', value: 'imap' },
  { label: 'POP3', value: 'pop3', disable: form.auth_type === 'oauth' },
])

// columns 统一定义 QTable 列，避免模板层重复拼装字段和格式化逻辑。
const columns = [
  { name: 'name', label: '名称', field: 'name', align: 'left' as const, sortable: true },
  { name: 'email', label: '邮箱', field: 'email', align: 'left' as const, sortable: true },
  { name: 'provider_name', label: '服务商', field: 'provider_name', align: 'left' as const, sortable: true },
  { name: 'auth_type', label: '认证', field: 'auth_type', align: 'left' as const, sortable: true },
  {
    name: 'incoming_protocol',
    label: '协议',
    field: (row: MailAccount) => row.incoming_protocol.toUpperCase(),
    align: 'left' as const,
    sortable: true,
  },
  {
    name: 'incoming_config',
    label: '收信配置',
    field: (row: MailAccount) => `${incomingHost(row)}:${incomingPort(row)}`,
    align: 'left' as const,
  },
  {
    name: 'smtp_config',
    label: 'SMTP',
    field: (row: MailAccount) => `${row.smtp_host}:${row.smtp_port}`,
    align: 'left' as const,
  },
  { name: 'enabled', label: '状态', field: 'enabled', align: 'left' as const, sortable: true },
  { name: 'actions', label: '操作', field: 'id', align: 'left' as const },
]

// createDefaultForm 统一生成表单初始值，避免重置时遗漏新增字段。
function createDefaultForm(): AccountForm {
  return {
    provider: 'custom',
    provider_name: '自定义',
    auth_type: 'password',
    name: '',
    email: '',
    username: '',
    password: '',
    incoming_protocol: 'imap',
    imap_host: '',
    imap_port: 993,
    pop3_host: '',
    pop3_port: 995,
    smtp_host: '',
    smtp_port: 465,
    use_tls: true,
    enabled: true,
  }
}

// resetForm 让新增和编辑共用一套清理逻辑，减少状态串扰。
function resetForm() {
  Object.assign(form, createDefaultForm())
}

// loadProviders 拉取服务商默认配置和微软 OAuth 可用状态。
async function loadProviders() {
  const response = await request<AccountProvidersResponse>('/api/account-providers')
  providers.value = response.items
  microsoftOAuthEnabled.value = response.microsoft_oauth_enabled
}

// loadAccounts 刷新当前邮箱列表，保持页面与数据库状态一致。
async function loadAccounts() {
  error.value = ''
  accounts.value = await request<MailAccount[]>('/api/accounts')
  selectedRows.value = selectedRows.value.filter((selected) => accounts.value.some((item) => item.id === selected.id))
}

// applyProviderPreset 用所选服务商默认值覆盖连接配置，但保留用户后续继续修改的能力。
function applyProviderPreset(providerKey: string) {
  const preset = providers.value.find((item) => item.key === providerKey)
  if (!preset) {
    return
  }
  form.provider = preset.key
  form.provider_name = preset.key === 'custom' ? form.provider_name || '自定义' : preset.name
  form.incoming_protocol = preset.incoming_protocol
  form.imap_host = preset.imap_host
  form.imap_port = preset.imap_port
  form.pop3_host = preset.pop3_host
  form.pop3_port = preset.pop3_port
  form.smtp_host = preset.smtp_host
  form.smtp_port = preset.smtp_port
  form.use_tls = preset.use_tls
  if (form.auth_type === 'oauth') {
    form.incoming_protocol = 'imap'
  }
}

// handleProviderChange 在切换服务商时更新默认配置，并收敛不兼容的 OAuth 选项。
function handleProviderChange() {
  applyProviderPreset(form.provider)
  if (form.provider === 'outlook' && microsoftOAuthEnabled.value) {
    form.auth_type = 'oauth'
    handleAuthTypeChange()
    return
  }
  if (form.provider !== 'custom' && currentProviderPreset.value) {
    form.provider_name = currentProviderPreset.value.name
  }
  if (!oauthAvailable.value && form.auth_type === 'oauth') {
    form.auth_type = 'password'
  }
}

// handleAuthTypeChange 保证 OAuth 仅走 IMAP，并让密码字段与协议选择保持一致。
function handleAuthTypeChange() {
  if (form.auth_type === 'oauth') {
    form.provider = 'outlook'
    applyProviderPreset('outlook')
    form.incoming_protocol = 'imap'
    form.use_tls = true
    form.password = ''
    return
  }
  if (form.provider === 'custom' && !form.provider_name.trim()) {
    form.provider_name = '自定义'
  }
}

// submit 统一承接新增和编辑提交，确保服务商、认证方式和服务器配置一起落库。
async function submit() {
  error.value = ''
  message.value = ''
  try {
    if (form.auth_type === 'oauth' && !editingID.value) {
      throw new Error('请先完成微软 OAuth 授权，授权成功后系统会自动创建邮箱')
    }
    const payload = {
      ...form,
      provider_name: form.provider === 'custom' ? form.provider_name : currentProviderPreset.value?.name || form.provider_name,
      password: form.auth_type === 'oauth' ? '' : form.password,
      use_tls: form.auth_type === 'oauth' ? true : form.use_tls,
      incoming_protocol: form.auth_type === 'oauth' ? 'imap' : form.incoming_protocol,
      imap_host: form.incoming_protocol === 'imap' || form.auth_type === 'oauth' ? form.imap_host : '',
      imap_port: form.incoming_protocol === 'imap' || form.auth_type === 'oauth' ? form.imap_port : 0,
      pop3_host: form.auth_type === 'oauth' ? '' : form.incoming_protocol === 'pop3' ? form.pop3_host : '',
      pop3_port: form.auth_type === 'oauth' ? 0 : form.incoming_protocol === 'pop3' ? form.pop3_port : 0,
    }
    if (editingID.value) {
      await request<MailAccount>(`/api/accounts/${editingID.value}`, {
        method: 'PUT',
        body: JSON.stringify(payload),
      })
      message.value = '邮箱修改成功'
    } else {
      await request<MailAccount>('/api/accounts', {
        method: 'POST',
        body: JSON.stringify(payload),
      })
      message.value = '邮箱添加成功'
    }
    closeModal()
    await loadAccounts()
  } catch (err) {
    error.value = err instanceof Error ? err.message : '保存邮箱失败'
  }
}

// test 让用户在保存后即可验证远端服务是否可连通。
async function test(id: number) {
  if (!Number.isFinite(id) || id <= 0) {
    error.value = '邮箱 ID 无效，无法测试连接'
    return
  }
  try {
    await request(`/api/accounts/${id}/test`, { method: 'POST' })
    message.value = '连接测试成功'
    error.value = ''
  } catch (err) {
    error.value = err instanceof Error ? err.message : '连接测试失败'
  }
}

// sync 允许用户手动触发一轮单邮箱同步，便于验证调度链路。
async function sync(id: number) {
  if (!Number.isFinite(id) || id <= 0) {
    error.value = '邮箱 ID 无效，无法执行同步'
    return
  }
  try {
    await request(`/api/accounts/${id}/sync`, { method: 'POST' })
    message.value = '同步完成'
    error.value = ''
  } catch (err) {
    error.value = err instanceof Error ? err.message : '同步失败'
  }
}

// openCreateModal 为新增场景重置表单并应用默认服务商配置。
function openCreateModal() {
  editingID.value = null
  resetForm()
  applyProviderPreset('custom')
  error.value = ''
  message.value = ''
  showModal.value = true
}

// openEditModal 把现有邮箱映射到表单，保证服务商和认证方式都可继续调整。
function openEditModal(account: MailAccount) {
  editingID.value = account.id
  Object.assign(form, {
    provider: account.provider || 'custom',
    provider_name: account.provider_name || '自定义',
    auth_type: account.auth_type || 'password',
    name: account.name,
    email: account.email,
    username: account.username,
    password: '',
    incoming_protocol: account.incoming_protocol,
    imap_host: account.imap_host,
    imap_port: account.imap_port,
    pop3_host: account.pop3_host,
    pop3_port: account.pop3_port,
    smtp_host: account.smtp_host,
    smtp_port: account.smtp_port,
    use_tls: account.use_tls,
    enabled: account.enabled,
  })
  error.value = ''
  message.value = ''
  showModal.value = true
}

// closeModal 关闭弹窗时同步清理编辑态，避免后续提交误走更新接口。
function closeModal() {
  showModal.value = false
  editingID.value = null
  error.value = ''
  resetForm()
}

// openImportModal 打开批量导入弹窗，并清理上次输入残留。
function openImportModal() {
  importText.value = ''
  error.value = ''
  message.value = ''
  showImportModal.value = true
}

// closeImportModal 关闭批量导入弹窗，避免无关文本继续停留在页面里。
function closeImportModal() {
  showImportModal.value = false
  importText.value = ''
}

// submitImport 把文本按行解析为批量导入请求，并明确限制为非 OAuth 邮箱。
async function submitImport() {
  error.value = ''
  message.value = ''
  try {
    const items = parseImportText(importText.value)
    await request<{ items: MailAccount[]; message: string }>('/api/accounts/import', {
      method: 'POST',
      body: JSON.stringify({ items }),
    })
    closeImportModal()
    message.value = `成功导入 ${items.length} 个邮箱`
    await loadAccounts()
  } catch (err) {
    error.value = err instanceof Error ? err.message : '批量导入失败'
  }
}

// parseImportText 兼容逗号和制表符分隔，尽量降低批量导入的准备成本。
function parseImportText(source: string) {
  const lines = source
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter(Boolean)
  if (lines.length === 0) {
    throw new Error('请输入至少一条导入记录')
  }

  return lines.map((line, index) => {
    const cells = splitImportLine(line)
    if (cells.length < 4) {
      throw new Error(`第 ${index + 1} 行格式不完整，至少需要名称、邮箱、用户名、密码`)
    }
    const protocol = normalizeProtocol(cells[5])
    const incomingHost = cells[6] ?? ''
    const incomingPort = Number(cells[7] ?? 0) || 0
    return {
      name: cells[0],
      email: cells[1],
      username: cells[2],
      password: cells[3],
      provider: cells[4] || 'custom',
      provider_name: '',
      auth_type: 'password',
      incoming_protocol: protocol,
      imap_host: protocol === 'imap' ? incomingHost : '',
      imap_port: protocol === 'imap' ? incomingPort : 0,
      pop3_host: protocol === 'pop3' ? incomingHost : '',
      pop3_port: protocol === 'pop3' ? incomingPort : 0,
      smtp_host: cells[8] ?? '',
      smtp_port: Number(cells[9] ?? 0) || 0,
      use_tls: normalizeBoolean(cells[10]),
      enabled: true,
    }
  })
}

// splitImportLine 优先支持制表符，其次回落到英文逗号，兼容常见复制来源。
function splitImportLine(line: string) {
  const separator = line.includes('\t') ? '\t' : ','
  return line.split(separator).map((item) => item.trim())
}

// normalizeProtocol 为批量导入提供最小兜底，避免协议字段空值导致后端拒绝。
function normalizeProtocol(value?: string): 'imap' | 'pop3' {
  return value?.toLowerCase() === 'pop3' ? 'pop3' : 'imap'
}

// normalizeBoolean 兼容常见真假值写法，避免导入时要求用户严格记忆大小写。
function normalizeBoolean(value?: string) {
  const lower = value?.trim().toLowerCase()
  if (!lower) {
    return true
  }
  return ['1', 'true', 'yes', 'y', 'on'].includes(lower)
}

// startMicrosoftOAuth 由前端生成 PKCE 参数并拉起弹窗，避免整页跳转打断当前表单。
async function startMicrosoftOAuth() {
  error.value = ''
  message.value = ''
  try {
    const loginHint = resolveOAuthLoginHint()
    const config = await request<MicrosoftOAuthConfigResponse>('/api/accounts/oauth/microsoft/config')
    if (!config.enabled) {
      throw new Error('微软 OAuth 未配置，请先设置 client_id 和 client_secret')
    }
    if (config.flow === 'legacy') {
      const legacyQuery = new URLSearchParams({ popup: '1' })
      if (loginHint) {
        legacyQuery.set('login_hint', loginHint)
      }
      const popup = window.open(`/api/accounts/oauth/microsoft/start?${legacyQuery.toString()}`, 'microsoft-oauth', 'popup=yes,width=640,height=760')
      if (!popup) {
        throw new Error('浏览器拦截了授权弹窗，请允许弹窗后重试')
      }
      oauthPopup = popup
      return
    }
    const session = createMicrosoftOAuthSession()
    const challenge = await createCodeChallenge(session.codeVerifier)
    saveMicrosoftOAuthSession(session)
    const query = new URLSearchParams({
      client_id: config.client_id,
      response_type: 'code',
      redirect_uri: config.redirect_uri,
      response_mode: 'query',
      scope: config.scope,
      state: session.state,
      prompt: 'select_account',
      code_challenge: challenge,
      code_challenge_method: 'S256',
    })
    if (loginHint) {
      query.set('login_hint', loginHint)
    }
    const popup = window.open(
      `https://login.microsoftonline.com/${config.tenant_id}/oauth2/v2.0/authorize?${query.toString()}`,
      'microsoft-oauth',
      'popup=yes,width=640,height=760',
    )
    if (!popup) {
      throw new Error('浏览器拦截了授权弹窗，请允许弹窗后重试')
    }
    oauthPopup = popup
  } catch (err) {
    error.value = err instanceof Error ? err.message : '发起微软 OAuth 失败'
  }
}

// resolveOAuthLoginHint 优先使用用户名，没有时退回邮箱，减少微软授权页重复输入账号。
function resolveOAuthLoginHint() {
  return form.username.trim() || form.email.trim()
}

// handleOAuthMessage 只接收同源回调页发来的结果，避免无关页面干扰当前列表状态。
async function handleOAuthMessage(event: MessageEvent<MicrosoftOAuthMessage>) {
  if (event.origin !== window.location.origin) {
    return
  }
  if (!event.data || event.data.type !== 'microsoft-oauth') {
    return
  }
  if (oauthPopup && !oauthPopup.closed) {
    oauthPopup.close()
  }
  oauthPopup = null
  if (event.data.success) {
    message.value = event.data.message
    error.value = ''
    await loadAccounts()
    closeModal()
    return
  }
  error.value = event.data.message
  message.value = ''
}

// incomingHost 统一根据协议显示当前实际收信服务器，避免表格重复判断分支。
function incomingHost(account: MailAccount) {
  return account.incoming_protocol === 'imap' ? account.imap_host : account.pop3_host
}

// incomingPort 统一根据协议显示当前实际收信端口，减少模板里的条件噪音。
function incomingPort(account: MailAccount) {
  return account.incoming_protocol === 'imap' ? account.imap_port : account.pop3_port
}

// buildAccountPayload 基于现有数据回放更新请求，以便批量启停复用单条更新接口。
function buildAccountPayload(account: MailAccount, enabled: boolean) {
  return {
    provider: account.provider,
    provider_name: account.provider_name,
    auth_type: account.auth_type,
    name: account.name,
    email: account.email,
    username: account.username,
    password: '',
    incoming_protocol: account.incoming_protocol,
    imap_host: account.imap_host,
    imap_port: account.imap_port,
    pop3_host: account.pop3_host,
    pop3_port: account.pop3_port,
    smtp_host: account.smtp_host,
    smtp_port: account.smtp_port,
    use_tls: account.use_tls,
    enabled,
  }
}

// batchUpdateEnabled 复用既有更新接口批量切换启用状态，避免额外扩展后端协议。
async function batchUpdateEnabled(enabled: boolean) {
  message.value = ''
  error.value = ''
  try {
    await Promise.all(
      selectedRows.value.map((item) =>
        request(`/api/accounts/${item.id}`, {
          method: 'PUT',
          body: JSON.stringify(buildAccountPayload(item, enabled)),
        }),
      ),
    )
    message.value = enabled ? '批量启用成功' : '批量禁用成功'
    await loadAccounts()
  } catch (err) {
    error.value = err instanceof Error ? err.message : '批量更新失败'
  }
}

// batchSync 对选中的邮箱逐一发起同步，保持现有后端接口不变。
async function batchSync() {
  message.value = ''
  error.value = ''
  try {
    await Promise.all(selectedRows.value.map((item) => request(`/api/accounts/${item.id}/sync`, { method: 'POST' })))
    message.value = '批量同步完成'
  } catch (err) {
    error.value = err instanceof Error ? err.message : '批量同步失败'
  }
}

// batchTest 逐一验证所选邮箱连接，避免单条点击重复操作过多。
async function batchTest() {
  message.value = ''
  error.value = ''
  try {
    await Promise.all(selectedRows.value.map((item) => request(`/api/accounts/${item.id}/test`, { method: 'POST' })))
    message.value = '批量测试成功'
  } catch (err) {
    error.value = err instanceof Error ? err.message : '批量测试失败'
  }
}

// batchDelete 复用删除接口处理多选删除，并在完成后刷新列表与勾选状态。
async function batchDelete() {
  message.value = ''
  error.value = ''
  try {
    await Promise.all(selectedRows.value.map((item) => request(`/api/accounts/${item.id}`, { method: 'DELETE' })))
    selectedRows.value = []
    message.value = '批量删除成功'
    await loadAccounts()
  } catch (err) {
    error.value = err instanceof Error ? err.message : '批量删除失败'
  }
}

// handleOAuthFeedback 把后端 OAuth 回跳结果映射到页面消息，并清理 URL 上的一次性参数。
async function handleOAuthFeedback() {
  const success = typeof route.query.oauth_success === 'string' ? route.query.oauth_success : ''
  const failure = typeof route.query.oauth_error === 'string' ? route.query.oauth_error : ''
  if (!success && !failure) {
    return
  }
  message.value = success
  error.value = failure
  await router.replace({ path: '/accounts' })
}

watch(
  () => form.incoming_protocol,
  (value) => {
    if (form.auth_type === 'oauth' && value !== 'imap') {
      form.incoming_protocol = 'imap'
    }
  },
)

watch(
  () => form.auth_type,
  (value) => {
    if (value === 'oauth' && !form.use_tls) {
      form.use_tls = true
    }
  },
)

onMounted(async () => {
  window.addEventListener('message', handleOAuthMessage)
  try {
    await loadProviders()
    await loadAccounts()
    await handleOAuthFeedback()
  } catch (err) {
    error.value = err instanceof Error ? err.message : '加载邮箱失败'
  }
})

onBeforeUnmount(() => {
  window.removeEventListener('message', handleOAuthMessage)
  oauthPopup = null
})
</script>
