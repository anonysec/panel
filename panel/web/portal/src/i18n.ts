import { registerMessages, useI18n } from '@koris/composables/useI18n'
import type { Locale } from '@koris/composables/useI18n'

export type { Locale }

const messages: Record<Locale, Record<string, string>> = {
  en: {
    'portal.nav.dashboard': 'Dashboard',
    'portal.nav.support': 'Support',
    'portal.nav.vpn': 'My VPN',
    'portal.nav.profile': 'Profile Settings',
    'portal.nav.logout': 'Logout',
  },
  fa: {
    'portal.nav.dashboard': 'داشبورد',
    'portal.nav.support': 'پشتیبانی',
    'portal.nav.vpn': 'وی‌پی‌ان من',
    'portal.nav.profile': 'تنظیمات پروفایل',
    'portal.nav.logout': 'خروج',
  },
  zh: {
    'portal.nav.dashboard': '仪表盘',
    'portal.nav.support': '客服支持',
    'portal.nav.vpn': '我的VPN',
    'portal.nav.profile': '个人设置',
    'portal.nav.logout': '退出登录',
  },
}

registerMessages(messages)

const { t, locale, setLocale } = useI18n()
export { t, locale, setLocale }
