import { createRouter, createWebHistory } from 'vue-router'
import MainLayout from '@/layouts/MainLayout.vue'
import LoginView from '@/views/LoginView.vue'
import AggregatedMessagesView from '@/views/AggregatedMessagesView.vue'
import ContactsView from '@/views/ContactsView.vue'
import InboxView from '@/views/InboxView.vue'
import AccountsView from '@/views/AccountsView.vue'
import SyncLogsView from '@/views/SyncLogsView.vue'
import MicrosoftOAuthCallbackView from '@/views/MicrosoftOAuthCallbackView.vue'

// router 统一维护页面切换和登录前置校验。
const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/login', component: LoginView },
    { path: '/oauth/microsoft/callback', component: MicrosoftOAuthCallbackView },
    {
      path: '/',
      component: MainLayout,
      meta: { auth: true },
      children: [
        { path: '', redirect: '/aggregated' },
        { path: 'aggregated', component: AggregatedMessagesView },
        { path: 'contacts', component: ContactsView },
        { path: 'inbox', component: InboxView },
        { path: 'accounts', component: AccountsView },
        { path: 'sync-logs', component: SyncLogsView },
      ],
    },
  ],
})

// beforeEach 通过后端登录态接口判断当前页面是否允许进入。
router.beforeEach(async (to) => {
  if (!to.meta.auth) {
    return true
  }

  try {
    const response = await fetch('/api/auth/me', { credentials: 'include' })
    if (response.ok) {
      return true
    }
  } catch {
    return '/login'
  }

  return '/login'
})

export default router
