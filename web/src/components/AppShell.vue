<template>
  <q-layout view="lHh Lpr lFf" class="app-shell-layout">
    <q-header class="app-shell-header text-white">
      <q-toolbar class="q-px-md q-py-sm">
        <q-btn flat round dense icon="menu" class="lt-md q-mr-sm" @click="drawerOpen = !drawerOpen" />
        <div class="row items-center no-wrap q-gutter-sm">
          <q-avatar color="primary" text-color="white" icon="mail" />
          <q-toolbar-title class="q-pa-none">
            <div class="text-weight-bold">gmbox</div>
            <div class="text-caption text-blue-1">统一邮箱工作台</div>
          </q-toolbar-title>
        </div>
        <q-space />
        <slot name="actions" />
      </q-toolbar>
    </q-header>

    <q-drawer v-model="drawerOpen" show-if-above bordered :width="272" class="app-shell-drawer">
      <div class="drawer-inner column no-wrap full-height q-pa-md">
        <div class="drawer-brand q-mb-md q-px-sm q-pt-sm q-pb-md">
          <div class="row items-center q-gutter-sm no-wrap">
            <q-avatar color="primary" text-color="white" icon="mail" />
            <div>
              <div class="text-subtitle1 text-weight-bold">gmbox</div>
              <div class="text-caption text-grey-7">邮件工作台导航</div>
            </div>
          </div>
        </div>

        <q-list padding>
          <q-item
            v-for="item in navItems"
            :key="item.key"
            clickable
            :to="item.to"
            :active="item.key === active"
            active-class="app-nav-active"
            @click="drawerOpen = false"
          >
            <q-item-section avatar>
              <q-icon :name="item.icon" />
            </q-item-section>
            <q-item-section>
              <q-item-label>{{ item.label }}</q-item-label>
              <q-item-label caption>{{ item.caption }}</q-item-label>
            </q-item-section>
          </q-item>
        </q-list>

        <q-space />

        <q-btn color="grey-2" text-color="dark" unelevated icon="logout" label="退出登录" @click="emit('logout')" />
      </div>
    </q-drawer>

    <q-page-container>
      <q-page class="app-page q-pa-md q-pa-lg-lg">
        <q-card flat class="app-page-hero q-mb-md q-mb-lg-lg">
          <q-card-section class="row items-start justify-between q-col-gutter-md">
            <div class="col-12 col-md">
              <div class="text-overline text-primary">{{ eyebrow || '工作台' }}</div>
              <div class="text-h4 text-weight-bold q-mt-xs">{{ title }}</div>
              <div v-if="subtitle" class="text-body1 text-grey-7 q-mt-sm">{{ subtitle }}</div>
            </div>
            <div class="col-12 col-md-auto row items-center justify-end q-gutter-sm">
              <slot name="hero-actions" />
            </div>
          </q-card-section>
        </q-card>

        <slot />
      </q-page>
    </q-page-container>
  </q-layout>
</template>

<script setup lang="ts">
import { ref } from 'vue'

type NavKey = 'inbox' | 'compose' | 'accounts'

defineProps<{
  title: string
  eyebrow?: string
  subtitle?: string
  active: NavKey
}>()

const emit = defineEmits<{
  logout: []
}>()

const drawerOpen = ref(false)

// navItems 统一维护工作台主导航，避免多个页面各自复制菜单配置。
const navItems: Array<{ key: NavKey; label: string; caption: string; to: string; icon: string }> = [
  { key: 'inbox', label: '聚合信息', caption: '查看聚合邮件与文件夹', to: '/inbox', icon: 'inbox' },
  { key: 'compose', label: '写信', caption: '通过已接入账户发送邮件', to: '/compose', icon: 'edit_square' },
  { key: 'accounts', label: '邮箱管理', caption: '维护账户、授权与同步', to: '/accounts', icon: 'manage_accounts' },
]
</script>

<style scoped>
.app-shell-layout {
  background:
    radial-gradient(circle at top left, rgba(89, 121, 255, 0.16), transparent 24%),
    radial-gradient(circle at top right, rgba(0, 188, 212, 0.12), transparent 22%),
    linear-gradient(180deg, #f3f7ff 0%, #f7f9fc 100%);
}

.app-shell-header {
  background: rgba(15, 23, 42, 0.92);
  backdrop-filter: blur(12px);
}

.app-shell-drawer {
  background: rgba(255, 255, 255, 0.88);
  backdrop-filter: blur(18px);
  border-right: 1px solid rgba(148, 163, 184, 0.16);
}

.drawer-inner {
  min-height: 100%;
}

.app-page-hero {
  background: rgba(255, 255, 255, 0.84);
  border: 1px solid rgba(148, 163, 184, 0.16);
  border-radius: 24px;
  box-shadow: 0 20px 50px rgba(15, 23, 42, 0.08);
}

.app-nav-active {
  color: #1d4ed8;
  background: rgba(59, 130, 246, 0.12);
  border-radius: 16px;
}

.drawer-brand {
  border-bottom: 1px solid rgba(148, 163, 184, 0.16);
}

.app-page {
  max-width: 1440px;
  margin: 0 auto;
}
</style>
