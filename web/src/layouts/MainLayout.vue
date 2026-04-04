<template>
  <q-layout view="lHh Lpr lFf">
    <q-header elevated>
      <q-toolbar>
        <q-btn flat round dense icon="menu" class="q-mr-sm" @click="drawerOpen = !drawerOpen" />
        <q-avatar color="primary" text-color="white" icon="mail" class="q-mr-sm" />
        <q-toolbar-title>
          <div class="text-weight-bold">gmbox</div>
          <div class="text-caption">统一邮箱工作台</div>
        </q-toolbar-title>
        <q-btn flat round dense icon="palette" @click="showThemeDialog = true">
          <q-tooltip>切换主题</q-tooltip>
        </q-btn>
      </q-toolbar>
    </q-header>

    <q-drawer v-model="drawerOpen" show-if-above bordered :width="260">
      <div class="column no-wrap full-height">
        <q-list padding>
          <q-item-label header>导航</q-item-label>
          <q-item
            v-for="item in navItems"
            :key="item.key"
            clickable
            :to="item.to"
            :active="item.key === activeKey"
            active-class="bg-primary text-white"
            @click="handleNavClick"
          >
            <q-item-section avatar>
              <q-icon :name="item.icon" />
            </q-item-section>
            <q-item-section>
              <q-item-label>{{ item.label }}</q-item-label>
            </q-item-section>
          </q-item>
        </q-list>

        <q-space />

        <div class="q-pa-md">
          <q-btn color="primary" outline class="full-width" icon="logout" label="退出登录" @click="logout" />
        </div>
      </div>
    </q-drawer>

    <q-page-container>
      <router-view />
    </q-page-container>

    <q-dialog v-model="showThemeDialog" @hide="handleThemeDialogHide">
      <q-card class="full-width" style="max-width: 720px">
        <q-card-section class="row items-start justify-between">
          <div>
            <div class="text-h6 text-weight-bold">切换主题</div>
            <div class="text-body2 text-grey-7 q-mt-xs">主题会保存到数据库，登录同一账号的其他设备也会同步应用。</div>
          </div>
          <q-btn flat round dense icon="close" v-close-popup />
        </q-card-section>

        <q-separator />

        <q-card-section>
          <div class="row q-col-gutter-md">
            <div v-for="preset in presets" :key="preset.name" class="col-12 col-md-6">
              <q-card bordered flat class="theme-option cursor-pointer" :class="draftTheme.theme_name === preset.name ? 'theme-option-active' : ''" @click="applyPreset(preset.name)">
                <q-card-section>
                  <div class="row items-center justify-between">
                    <div class="text-subtitle1 text-weight-medium">{{ preset.label }}</div>
                    <q-badge :color="preset.theme_mode === 'dark' ? 'dark' : 'grey-4'" :text-color="preset.theme_mode === 'dark' ? 'white' : 'dark'">{{ preset.theme_mode === 'dark' ? '深色' : '浅色' }}</q-badge>
                  </div>
                  <div class="row q-gutter-sm q-mt-md">
                    <div class="theme-swatch" :style="{ background: preset.primary_color }"></div>
                    <div class="theme-swatch" :style="{ background: preset.secondary_color }"></div>
                    <div class="theme-swatch" :style="{ background: preset.accent_color }"></div>
                  </div>
                </q-card-section>
              </q-card>
            </div>
          </div>

          <div class="row q-col-gutter-md q-mt-md">
            <div class="col-12 col-md-4">
              <q-select v-model="draftTheme.theme_mode" outlined emit-value map-options :options="themeModeOptions" label="主题模式" />
            </div>
            <div class="col-12 col-md-4">
              <q-input v-model="draftTheme.primary_color" outlined label="主色">
                <template #append>
                  <q-icon name="colorize" class="cursor-pointer">
                    <q-popup-proxy cover transition-show="scale" transition-hide="scale">
                      <q-color v-model="draftTheme.primary_color" format-model="hex" />
                    </q-popup-proxy>
                  </q-icon>
                </template>
              </q-input>
            </div>
            <div class="col-12 col-md-4">
              <q-input v-model="draftTheme.secondary_color" outlined label="辅助色">
                <template #append>
                  <q-icon name="colorize" class="cursor-pointer">
                    <q-popup-proxy cover transition-show="scale" transition-hide="scale">
                      <q-color v-model="draftTheme.secondary_color" format-model="hex" />
                    </q-popup-proxy>
                  </q-icon>
                </template>
              </q-input>
            </div>
            <div class="col-12 col-md-4">
              <q-input v-model="draftTheme.accent_color" outlined label="强调色">
                <template #append>
                  <q-icon name="colorize" class="cursor-pointer">
                    <q-popup-proxy cover transition-show="scale" transition-hide="scale">
                      <q-color v-model="draftTheme.accent_color" format-model="hex" />
                    </q-popup-proxy>
                  </q-icon>
                </template>
              </q-input>
            </div>
          </div>
        </q-card-section>

        <q-card-actions align="right">
          <q-btn flat no-caps label="取消" v-close-popup />
          <q-btn color="primary" unelevated no-caps label="保存主题" @click="saveTheme" />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </q-layout>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useQuasar } from 'quasar'
import { request, type ThemePreference } from '@/api'
import { applyThemePreference, defaultThemePreference, themePresets, themeState } from '@/theme'

type NavKey = 'aggregated' | 'contacts' | 'inbox' | 'accounts' | 'sync-logs'

const route = useRoute()
const router = useRouter()
const $q = useQuasar()
const drawerOpen = ref(true)
const showThemeDialog = ref(false)
const draftTheme = reactive<ThemePreference>(defaultThemePreference())
const committedTheme = reactive<ThemePreference>({ ...themeState })
const themeSaveCommitted = ref(false)
const themeDraftDirty = ref(false)
const syncingThemeDraft = ref(false)
const presets = themePresets
const themeModeOptions = [
  { label: '浅色', value: 'light' },
  { label: '深色', value: 'dark' },
]

// navItems 统一维护工作台主导航，避免布局和页面各自维护菜单。
const navItems: Array<{ key: NavKey; label: string; to: string; icon: string }> = [
  { key: 'aggregated', label: '聚合消息', to: '/aggregated', icon: 'all_inbox' },
  { key: 'contacts', label: '联系人', to: '/contacts', icon: 'groups' },
  { key: 'inbox', label: '邮件列表', to: '/inbox', icon: 'inbox' },
  { key: 'accounts', label: '邮箱管理', to: '/accounts', icon: 'manage_accounts' },
  { key: 'sync-logs', label: '同步日志', to: '/sync-logs', icon: 'history' },
]

// activeKey 根据当前路由路径决定激活菜单，避免详情页丢失收件箱导航状态。
const activeKey = computed<NavKey>(() => {
  if (route.path.startsWith('/aggregated')) {
    return 'aggregated'
  }
  if (route.path.startsWith('/contacts')) {
    return 'contacts'
  }
  if (route.path.startsWith('/accounts')) {
    return 'accounts'
  }
  if (route.path.startsWith('/sync-logs')) {
    return 'sync-logs'
  }
  return 'inbox'
})

// handleNavClick 只在移动端点击导航后收起抽屉，避免桌面端切页时闪烁或消失。
function handleNavClick() {
  if ($q.screen.lt.md) {
    drawerOpen.value = false
  }
}

// logout 统一清理登录态，避免每个业务页重复实现同样逻辑。
async function logout() {
  await request('/api/auth/logout', { method: 'POST' })
  await router.push('/login')
}

// applyPreset 允许用户从常用主题开始，再按需继续微调颜色。
function applyPreset(name: string) {
  const preset = presets.find((item) => item.name === name)
  if (!preset) {
    return
  }
  Object.assign(draftTheme, preset)
}

// handleThemeDialogHide 在未保存关闭弹窗时恢复原主题，避免预览状态污染全局界面。
function handleThemeDialogHide() {
  if (themeSaveCommitted.value) {
    themeSaveCommitted.value = false
    themeDraftDirty.value = false
    return
  }
  syncingThemeDraft.value = true
  Object.assign(draftTheme, committedTheme)
  syncingThemeDraft.value = false
  themeDraftDirty.value = false
  applyThemePreference(committedTheme)
}

async function loadTheme() {
  try {
    const response = await request<ThemePreference>('/api/preferences/theme')
    Object.assign(committedTheme, response)
    if (!showThemeDialog.value || !themeDraftDirty.value) {
      syncingThemeDraft.value = true
      Object.assign(draftTheme, response)
      syncingThemeDraft.value = false
      applyThemePreference(response)
    }
  } catch {
    Object.assign(committedTheme, themeState)
    syncingThemeDraft.value = true
    Object.assign(draftTheme, committedTheme)
    syncingThemeDraft.value = false
    applyThemePreference(committedTheme)
  }
}

async function saveTheme() {
  const response = await request<ThemePreference>('/api/preferences/theme', {
    method: 'PUT',
    body: JSON.stringify(draftTheme),
  })
  themeSaveCommitted.value = true
  themeDraftDirty.value = false
  applyThemePreference(response)
  Object.assign(committedTheme, response)
  syncingThemeDraft.value = true
  Object.assign(draftTheme, response)
  syncingThemeDraft.value = false
  showThemeDialog.value = false
}

watch(showThemeDialog, (value) => {
  if (!value) {
    return
  }
  themeSaveCommitted.value = false
  themeDraftDirty.value = false
  syncingThemeDraft.value = true
  Object.assign(draftTheme, committedTheme)
  syncingThemeDraft.value = false
})

watch(draftTheme, (value) => {
  if (!showThemeDialog.value) {
    return
  }
  if (!syncingThemeDraft.value) {
    themeDraftDirty.value = true
  }
  applyThemePreference(value, { persist: false })
}, { deep: true })

void loadTheme()
</script>

<style scoped>
.theme-option {
  transition: border-color 0.18s ease, transform 0.18s ease;
}

.theme-option-active {
  border-color: var(--gmbox-primary, #2563eb);
  box-shadow: inset 0 0 0 2px var(--gmbox-primary, #2563eb);
  transform: translateY(-1px);
}

.theme-swatch {
  width: 24px;
  height: 24px;
  border-radius: 999px;
}
</style>
