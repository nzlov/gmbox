<template>
  <q-layout view="lHh Lpr lFf">
    <q-header elevated>
      <q-toolbar>
        <q-btn flat round dense icon="menu" class="lt-md q-mr-sm" @click="drawerOpen = !drawerOpen" />
        <q-avatar color="primary" text-color="white" icon="mail" class="q-mr-sm" />
        <q-toolbar-title>
          <div class="text-weight-bold">gmbox</div>
          <div class="text-caption">统一邮箱工作台</div>
        </q-toolbar-title>
        <slot name="actions" />
      </q-toolbar>
    </q-header>

    <q-drawer v-model="drawerOpen" show-if-above bordered :width="260">
      <q-list padding>
        <q-item-label header>导航</q-item-label>
        <q-item
          v-for="item in navItems"
          :key="item.key"
          clickable
          :to="item.to"
          :active="item.key === active"
          active-class="bg-primary text-white"
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

      <div class="q-pa-md">
        <q-btn color="primary" outline class="full-width" icon="logout" label="退出登录" @click="emit('logout')" />
      </div>
    </q-drawer>

    <q-page-container>
      <q-page class="q-pa-md">
        <q-card flat bordered class="q-mb-md">
          <q-card-section class="row items-start justify-between q-col-gutter-md">
            <div class="col-12 col-md">
              <div class="text-overline text-primary">{{ eyebrow || '工作台' }}</div>
              <div class="text-h5 text-weight-bold q-mt-xs">{{ title }}</div>
              <div v-if="subtitle" class="text-body2 text-grey-7 q-mt-sm">{{ subtitle }}</div>
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
