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
      <header class="topbar accounts-topbar">
        <div>
          <p class="eyebrow">多邮箱管理</p>
          <h1>邮箱账户</h1>
        </div>
        <div class="toolbar-actions wrap-actions">
          <button class="ghost-btn" @click="openImportModal">批量导入</button>
          <button class="primary-btn" @click="openCreateModal">添加邮箱</button>
        </div>
      </header>

      <section class="panel table-panel">
        <div class="panel-head panel-tools">
          <div>
            <h3>邮箱列表</h3>
            <span class="muted">支持服务商自动配置、微软 OAuth 和非 OAuth 批量导入。</span>
          </div>
          <div class="toolbar-actions wrap-actions">
            <button class="ghost-btn" :disabled="!hasSelection" @click="batchUpdateEnabled(true)">启用</button>
            <button class="ghost-btn" :disabled="!hasSelection" @click="batchUpdateEnabled(false)">禁用</button>
            <button class="ghost-btn" :disabled="!hasSelection" @click="batchSync">同步</button>
            <button class="ghost-btn" :disabled="!hasSelection" @click="batchTest">测试</button>
            <button class="ghost-btn danger-btn" :disabled="!hasSelection" @click="batchDelete">删除</button>
          </div>
        </div>

        <p v-if="message" :class="messageClass">{{ message }}</p>
        <p v-if="error" class="error-text">{{ error }}</p>

        <div class="table-wrapper">
          <table class="data-table">
            <thead>
              <tr>
                <th><input :checked="allSelected" type="checkbox" @change="toggleAll($event)" /></th>
                <th>名称</th>
                <th>邮箱</th>
                <th>服务商</th>
                <th>认证</th>
                <th>协议</th>
                <th>收信配置</th>
                <th>SMTP</th>
                <th>状态</th>
                <th>操作</th>
              </tr>
            </thead>
            <tbody>
              <tr v-if="accounts.length === 0">
                <td colspan="10" class="empty-cell">暂无邮箱，请先添加。</td>
              </tr>
              <tr v-for="item in accounts" :key="item.id">
                <td>
                  <input v-model="selectedIDs" type="checkbox" :value="item.id" />
                </td>
                <td>{{ item.name }}</td>
                <td>{{ item.email }}</td>
                <td>{{ item.provider_name }}</td>
                <td>
                  <span class="status-badge" :class="item.auth_type === 'oauth' ? 'status-enabled' : 'status-disabled'">
                    {{ item.auth_type === 'oauth' ? 'OAuth' : '密码' }}
                  </span>
                </td>
                <td>{{ item.incoming_protocol.toUpperCase() }}</td>
                <td>{{ incomingHost(item) }}:{{ incomingPort(item) }}</td>
                <td>{{ item.smtp_host }}:{{ item.smtp_port }}</td>
                <td>
                  <span class="status-badge" :class="item.enabled ? 'status-enabled' : 'status-disabled'">
                    {{ item.enabled ? '已启用' : '已禁用' }}
                  </span>
                </td>
                <td>
                  <div class="toolbar-actions wrap-actions">
                    <button class="ghost-btn" @click="openEditModal(item)">编辑</button>
                    <button class="ghost-btn" @click="test(item.id)">测试</button>
                    <button class="ghost-btn" @click="sync(item.id)">同步</button>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>

      <div v-if="showModal" class="modal-mask" @click.self="closeModal">
        <section class="modal-card account-modal-card">
          <div class="panel-head">
            <div>
              <h3>{{ editingID ? '修改邮箱' : '添加邮箱' }}</h3>
              <span class="muted">选择常见服务商后自动填充 IMAP、POP3、SMTP，仍可继续修改。</span>
            </div>
            <button class="ghost-btn" @click="closeModal">关闭</button>
          </div>

          <form class="form-grid account-form-grid" @submit.prevent="submit">
            <label class="form-block span-2">
              <span>服务商</span>
              <select v-model="form.provider" @change="handleProviderChange()">
                <option v-for="item in providers" :key="item.key" :value="item.key">{{ item.name }}</option>
              </select>
            </label>

            <label class="form-block span-2" v-if="form.provider === 'custom'">
              <span>自定义服务商名称</span>
              <input v-model="form.provider_name" placeholder="例如：公司邮箱" />
            </label>

            <label class="form-block">
              <span>展示名称</span>
              <input v-model="form.name" placeholder="展示名称" />
            </label>
            <label class="form-block">
              <span>邮箱地址</span>
              <input v-model="form.email" placeholder="邮箱地址" />
            </label>
            <label class="form-block">
              <span>登录用户名</span>
              <input v-model="form.username" placeholder="登录用户名，默认建议填邮箱地址" />
            </label>
            <label class="form-block">
              <span>认证方式</span>
              <select v-model="form.auth_type" @change="handleAuthTypeChange()">
                <option value="password">密码 / 授权码</option>
                <option :disabled="!oauthAvailable" value="oauth">微软 OAuth</option>
              </select>
            </label>

            <label class="form-block span-2" v-if="form.auth_type === 'password'">
              <span>密码或授权码</span>
              <input v-model="form.password" type="password" :placeholder="editingID ? '留空则保持原密码' : '密码或授权码'" />
            </label>

            <div v-else class="oauth-panel span-2">
              <div>
                <strong>微软 OAuth 授权</strong>
                <p class="muted">完成授权后会自动接入 Outlook / Microsoft 365 邮箱，并可继续修改 IMAP、SMTP 配置。</p>
              </div>
              <button class="ghost-btn" type="button" :disabled="!microsoftOAuthEnabled" @click="startMicrosoftOAuth">
                {{ microsoftOAuthEnabled ? '连接微软邮箱' : '未配置微软 OAuth' }}
              </button>
            </div>

            <label class="form-block">
              <span>收信协议</span>
              <select v-model="form.incoming_protocol" :disabled="form.auth_type === 'oauth'">
                <option value="imap">IMAP</option>
                <option value="pop3" :disabled="form.auth_type === 'oauth'">POP3</option>
              </select>
            </label>
            <label class="switch-row form-block">
              <span>启用 TLS</span>
              <input v-model="form.use_tls" :disabled="form.auth_type === 'oauth'" type="checkbox" />
            </label>

            <template v-if="form.incoming_protocol === 'imap'">
              <label class="form-block">
                <span>IMAP Host</span>
                <input v-model="form.imap_host" placeholder="IMAP Host" />
              </label>
              <label class="form-block">
                <span>IMAP Port</span>
                <input v-model.number="form.imap_port" type="number" placeholder="IMAP Port" />
              </label>
            </template>
            <template v-else>
              <label class="form-block">
                <span>POP3 Host</span>
                <input v-model="form.pop3_host" placeholder="POP3 Host" />
              </label>
              <label class="form-block">
                <span>POP3 Port</span>
                <input v-model.number="form.pop3_port" type="number" placeholder="POP3 Port" />
              </label>
            </template>

            <label class="form-block">
              <span>SMTP Host</span>
              <input v-model="form.smtp_host" placeholder="SMTP Host" />
            </label>
            <label class="form-block">
              <span>SMTP Port</span>
              <input v-model.number="form.smtp_port" type="number" placeholder="SMTP Port" />
            </label>
            <label class="switch-row form-block span-2">
              <span>启用账户</span>
              <input v-model="form.enabled" type="checkbox" />
            </label>
            <button class="primary-btn span-2">{{ editingID ? '保存修改' : '保存邮箱' }}</button>
          </form>
        </section>
      </div>

      <div v-if="showImportModal" class="modal-mask" @click.self="closeImportModal">
        <section class="modal-card account-modal-card">
          <div class="panel-head">
            <div>
              <h3>批量导入非 OAuth 邮箱</h3>
              <span class="muted">每行一个邮箱，支持英文逗号或制表符分隔，已知服务商可自动补齐服务器配置。</span>
            </div>
            <button class="ghost-btn" @click="closeImportModal">关闭</button>
          </div>

          <div class="form-grid account-form-grid">
            <label class="form-block span-2">
              <span>导入内容</span>
              <textarea
                v-model="importText"
                class="import-textarea"
                rows="10"
                placeholder="name,email,username,password,provider,protocol,incoming_host,incoming_port,smtp_host,smtp_port,use_tls"
              />
            </label>

            <div class="import-help span-2 muted">
              <p>列顺序：名称、邮箱、用户名、密码、服务商、协议、收信主机、收信端口、SMTP 主机、SMTP 端口、是否 TLS。</p>
              <p>服务商可填：gmail、qq、163、126、aliyun、outlook、yahoo、custom。已知服务商留空主机和端口时会自动补齐。</p>
              <p>示例：张三,zs@example.com,zs@example.com,authcode123,gmail,imap,,,,,true</p>
            </div>

            <div class="toolbar-actions wrap-actions span-2">
              <button class="ghost-btn" type="button" @click="closeImportModal">取消</button>
              <button class="primary-btn" type="button" @click="submitImport">开始导入</button>
            </div>
          </div>
        </section>
      </div>
    </main>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { request, type AccountProvidersResponse, type MailAccount, type ProviderPreset } from '@/api'

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
const selectedIDs = ref<number[]>([])
const importText = ref('')
const form = reactive<AccountForm>(createDefaultForm())

const messageClass = computed(() => 'success-text')
const hasSelection = computed(() => selectedIDs.value.length > 0)
const allSelected = computed(() => accounts.value.length > 0 && selectedIDs.value.length === accounts.value.length)
const currentProviderPreset = computed(() => providers.value.find((item) => item.key === form.provider) ?? null)
const oauthAvailable = computed(() => form.provider === 'outlook' && microsoftOAuthEnabled.value)

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
  selectedIDs.value = selectedIDs.value.filter((id) => accounts.value.some((item) => item.id === id))
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

// startMicrosoftOAuth 直接跳转到后端授权入口，把换码和落库留给服务端处理。
function startMicrosoftOAuth() {
  window.location.href = '/api/accounts/oauth/microsoft/start'
}

// toggleAll 让表格多选支持一键选择当前页全部邮箱。
function toggleAll(event: Event) {
  const checked = (event.target as HTMLInputElement).checked
  selectedIDs.value = checked ? accounts.value.map((item) => item.id) : []
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
    const selectedAccounts = accounts.value.filter((item) => selectedIDs.value.includes(item.id))
    await Promise.all(
      selectedAccounts.map((item) =>
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
    await Promise.all(selectedIDs.value.map((id) => request(`/api/accounts/${id}/sync`, { method: 'POST' })))
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
    await Promise.all(selectedIDs.value.map((id) => request(`/api/accounts/${id}/test`, { method: 'POST' })))
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
    await Promise.all(selectedIDs.value.map((id) => request(`/api/accounts/${id}`, { method: 'DELETE' })))
    selectedIDs.value = []
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

// logout 统一清理登录态，保持所有业务页退出行为一致。
async function logout() {
  await request('/api/auth/logout', { method: 'POST' })
  await router.push('/login')
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
  try {
    await loadProviders()
    await loadAccounts()
    await handleOAuthFeedback()
  } catch (err) {
    error.value = err instanceof Error ? err.message : '加载邮箱失败'
  }
})
</script>

<style scoped>
.accounts-topbar {
  gap: 16px;
}

.account-modal-card {
  width: min(820px, calc(100vw - 32px));
}

.account-form-grid {
  grid-template-columns: repeat(2, minmax(0, 1fr));
}

.form-block {
  display: grid;
  gap: 8px;
}

.form-block span {
  font-size: 13px;
  color: #667085;
}

.span-2 {
  grid-column: span 2;
}

.oauth-panel,
.import-help {
  display: grid;
  gap: 8px;
  padding: 14px;
  border: 1px solid rgba(15, 23, 42, 0.08);
  border-radius: 16px;
  background: rgba(248, 250, 252, 0.9);
}

.import-help p {
  margin: 0;
}

.import-textarea {
  min-height: 220px;
  resize: vertical;
}

@media (max-width: 860px) {
  .account-form-grid {
    grid-template-columns: minmax(0, 1fr);
  }

  .span-2 {
    grid-column: span 1;
  }
}
</style>
