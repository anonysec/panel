import { createApp } from 'vue'
import { createPinia } from 'pinia'
import router from './router'
import App from './App.vue'
import { registerMessages } from '@koris/composables/useI18n'
import '@koris/styles/reset.css'
import '@koris/styles/tokens.css'
import '@koris/styles/utilities.css'
import './style.css'

// Register admin translation messages
registerMessages({
  en: {
    'nav.dashboard': 'Dashboard',
    'nav.transactions': 'Transactions',
    'nav.users': 'Users',
    'nav.services': 'Services',
    'nav.plans': 'Plans',
    'nav.settings': 'Settings',
    'stat.revenue': 'Revenue',
    'stat.active_users': 'Active Users',
    'stat.nodes_online': 'Nodes Online',
    'stat.open_tickets': 'Open Tickets',
    'label.search': 'Search...',
    'label.notifications': 'Notifications',
    'label.welcome_back': 'Welcome back',
    'label.connected': 'Connected',
    'label.disconnected': 'Disconnected',
    'btn.new_user': '+ New User',
    'btn.view_all': 'View All',
  },
  fa: {
    'nav.dashboard': 'داشبورد',
    'nav.transactions': 'تراکنش‌ها',
    'nav.users': 'کاربران',
    'nav.services': 'سرویس‌ها',
    'nav.plans': 'پلن‌ها',
    'nav.settings': 'تنظیمات',
    'stat.revenue': 'درآمد',
    'stat.active_users': 'کاربران فعال',
    'stat.nodes_online': 'نودهای آنلاین',
    'stat.open_tickets': 'تیکت‌های باز',
    'label.search': 'جستجو...',
    'label.notifications': 'اعلان‌ها',
    'label.welcome_back': 'خوش آمدید',
    'label.connected': 'متصل',
    'label.disconnected': 'قطع شده',
    'btn.new_user': '+ کاربر جدید',
    'btn.view_all': 'مشاهده همه',
  },
  zh: {
    'nav.dashboard': '仪表盘',
    'nav.transactions': '交易记录',
    'nav.users': '用户',
    'nav.services': '服务',
    'nav.plans': '套餐',
    'nav.settings': '设置',
    'stat.revenue': '收入',
    'stat.active_users': '活跃用户',
    'stat.nodes_online': '在线节点',
    'stat.open_tickets': '待处理工单',
    'label.search': '搜索...',
    'label.notifications': '通知',
    'label.welcome_back': '欢迎回来',
    'label.connected': '已连接',
    'label.disconnected': '已断开',
    'btn.new_user': '+ 新用户',
    'btn.view_all': '查看全部',
  },
})

const app = createApp(App)
app.use(createPinia())
app.use(router)
app.mount('#app')
