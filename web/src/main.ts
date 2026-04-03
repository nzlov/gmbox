import { createApp } from 'vue'
import { Quasar } from 'quasar'
import quasarLang from 'quasar/lang/zh-CN'
import App from './App.vue'
import router from './router'
import 'quasar/dist/quasar.css'
import '@quasar/extras/material-icons/material-icons.css'
import './styles.css'

// createApp 负责挂载前端入口并注入路由。
createApp(App)
  .use(router)
  .use(Quasar, {
    lang: quasarLang,
    config: {
      brand: {
        primary: '#2563eb',
        secondary: '#7c3aed',
        accent: '#06b6d4',
      },
    },
  })
  .mount('#app')
