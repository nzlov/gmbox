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
  </q-layout>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useQuasar } from 'quasar'
import { request } from '@/api'

type NavKey = 'inbox' | 'compose' | 'accounts'

const route = useRoute()
const router = useRouter()
const $q = useQuasar()
const drawerOpen = ref(true)

// navItems 统一维护工作台主导航，避免布局和页面各自维护菜单。
const navItems: Array<{ key: NavKey; label: string; to: string; icon: string }> = [
  { key: 'inbox', label: '聚合信息', to: '/inbox', icon: 'inbox' },
  { key: 'compose', label: '写信', to: '/compose', icon: 'edit_square' },
  { key: 'accounts', label: '邮箱管理', to: '/accounts', icon: 'manage_accounts' },
]

// activeKey 根据当前路由路径决定激活菜单，避免详情页丢失收件箱导航状态。
const activeKey = computed<NavKey>(() => {
  if (route.path.startsWith('/compose')) {
    return 'compose'
  }
  if (route.path.startsWith('/accounts')) {
    return 'accounts'
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
</script>
