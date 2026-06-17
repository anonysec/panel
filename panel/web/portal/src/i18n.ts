import { registerMessages, useI18n } from '@koris/composables/useI18n'
import type { Locale } from '@koris/composables/useI18n'

export type { Locale }

const messages: Record<Locale, Record<string, string>> = {
  en: {
    // Nav / Shell
    'portal.nav.dashboard': 'Dashboard',
    'portal.nav.support': 'Support',
    'portal.nav.vpn': 'My VPN',
    'portal.nav.profile': 'Profile Settings',
    'portal.nav.logout': 'Logout',
    'portal.nav.language': 'Language',
    'portal.nav.theme': 'Toggle theme',

    // Welcome
    'portal.hello': 'Hello',
    'portal.welcome': 'Everything you need is right here.',

    // Account section
    'portal.account.title': 'My Account',
    'portal.account.plan': 'Plan',
    'portal.account.status': 'Status',
    'portal.account.status_active': 'Active',
    'portal.account.status_expired': 'Expired',
    'portal.account.status_disabled': 'Disabled',
    'portal.account.status_inactive': 'Inactive',
    'portal.account.expires': 'Expires',
    'portal.account.daysLeft': 'Days Left',
    'portal.account.days': 'days',
    'portal.account.connection': 'Connection',
    'portal.account.online': 'Online',
    'portal.account.offline': 'Offline',

    // Usage section
    'portal.usage.title': 'Data Usage',
    'portal.usage.remaining': 'remaining',
    'portal.usage.used': 'used',
    'portal.unlimitedData': 'Unlimited',
    'portal.noExpiry': 'No expiry',

    // VPN section
    'portal.vpn.title': 'My VPN',
    'portal.vpn.subUrl': 'Subscription Link',
    'portal.vpn.subUrlDesc': 'Paste this link in your VPN app to connect automatically.',
    'portal.vpn.copy': 'Copy',
    'portal.vpn.copied': 'Copied!',
    'portal.vpn.download': 'Download',
    'portal.vpn.unavailable': 'N/A',
    'portal.vpn.noProfiles': 'No VPN files yet',
    'portal.vpn.noProfilesDesc': 'Your connection files will show up here when ready.',

    // Support section
    'portal.support.title': 'Help & Support',

    // Apps section
    'portal.apps.title': 'Download Apps',
    'portal.apps.desc': 'Get the app for your device to connect easily.',
    'portal.support.newTicket': 'Ask for Help',
    'portal.support.subject': 'What is the problem?',
    'portal.support.subjectPlaceholder': 'Short description...',
    'portal.support.message': 'Tell us more',
    'portal.support.messagePlaceholder': 'Describe what happened...',
    'portal.support.cancel': 'Cancel',
    'portal.support.create': 'Send',
    'portal.support.ticketCreated': 'Your message was sent! We will reply soon.',
    'portal.support.replySent': 'Reply sent.',
    'portal.support.send': 'Send',
    'portal.support.yourReply': 'Your reply',
    'portal.support.replyPlaceholder': 'Type your message...',
    'portal.support.backToList': 'Back',
    'portal.support.open': 'Open',
    'portal.support.closed': 'Closed',
    'portal.support.noTickets': 'No messages yet',
    'portal.support.noTicketsDesc': 'If you need help, tap the button above.',
  },
  fa: {
    // Nav / Shell
    'portal.nav.dashboard': 'داشبورد',
    'portal.nav.support': 'پشتیبانی',
    'portal.nav.vpn': 'وی‌پی‌ان من',
    'portal.nav.profile': 'تنظیمات حساب',
    'portal.nav.logout': 'خروج',
    'portal.nav.language': 'زبان',
    'portal.nav.theme': 'تغییر تم',

    // Welcome
    'portal.hello': 'سلام',
    'portal.welcome': 'همه چیز اینجاست.',

    // Account section
    'portal.account.title': 'حساب من',
    'portal.account.plan': 'پلن',
    'portal.account.status': 'وضعیت',
    'portal.account.status_active': 'فعال',
    'portal.account.status_expired': 'منقضی',
    'portal.account.status_disabled': 'غیرفعال',
    'portal.account.status_inactive': 'غیرفعال',
    'portal.account.expires': 'انقضا',
    'portal.account.daysLeft': 'روز باقیمانده',
    'portal.account.days': 'روز',
    'portal.account.connection': 'اتصال',
    'portal.account.online': 'آنلاین',
    'portal.account.offline': 'آفلاین',

    // Usage section
    'portal.usage.title': 'مصرف دیتا',
    'portal.usage.remaining': 'باقیمانده',
    'portal.usage.used': 'مصرف شده',
    'portal.unlimitedData': 'نامحدود',
    'portal.noExpiry': 'بدون انقضا',

    // VPN section
    'portal.vpn.title': 'وی‌پی‌ان من',
    'portal.vpn.subUrl': 'لینک اشتراک',
    'portal.vpn.subUrlDesc': 'این لینک را در برنامه وی‌پی‌ان خود وارد کنید تا خودکار وصل شوید.',
    'portal.vpn.copy': 'کپی',
    'portal.vpn.copied': 'کپی شد!',
    'portal.vpn.download': 'دانلود',
    'portal.vpn.unavailable': 'ندارد',
    'portal.vpn.noProfiles': 'هنوز فایلی نیست',
    'portal.vpn.noProfilesDesc': 'فایل‌های اتصال شما وقتی آماده شوند اینجا نمایش داده می‌شوند.',

    // Support section
    'portal.support.title': 'کمک و پشتیبانی',

    // Apps section
    'portal.apps.title': 'دانلود اپلیکیشن',
    'portal.apps.desc': 'اپ مناسب دستگاه خود را دانلود کنید تا راحت وصل شوید.',
    'portal.support.newTicket': 'درخواست کمک',
    'portal.support.subject': 'مشکل چیست؟',
    'portal.support.subjectPlaceholder': 'توضیح کوتاه...',
    'portal.support.message': 'بیشتر بگویید',
    'portal.support.messagePlaceholder': 'توضیح دهید چه اتفاقی افتاده...',
    'portal.support.cancel': 'لغو',
    'portal.support.create': 'ارسال',
    'portal.support.ticketCreated': 'پیام شما ارسال شد! به زودی پاسخ می‌دهیم.',
    'portal.support.replySent': 'پاسخ ارسال شد.',
    'portal.support.send': 'ارسال',
    'portal.support.yourReply': 'پاسخ شما',
    'portal.support.replyPlaceholder': 'پیام خود را بنویسید...',
    'portal.support.backToList': 'بازگشت',
    'portal.support.open': 'باز',
    'portal.support.closed': 'بسته',
    'portal.support.noTickets': 'هنوز پیامی نیست',
    'portal.support.noTicketsDesc': 'اگر کمک نیاز دارید، دکمه بالا را بزنید.',
  },
  zh: {
    // Nav / Shell
    'portal.nav.dashboard': '仪表盘',
    'portal.nav.support': '客服支持',
    'portal.nav.vpn': '我的VPN',
    'portal.nav.profile': '账户设置',
    'portal.nav.logout': '退出登录',
    'portal.nav.language': '语言',
    'portal.nav.theme': '切换主题',

    // Welcome
    'portal.hello': '你好',
    'portal.welcome': '所有信息都在这里。',

    // Account section
    'portal.account.title': '我的账户',
    'portal.account.plan': '套餐',
    'portal.account.status': '状态',
    'portal.account.status_active': '活跃',
    'portal.account.status_expired': '已过期',
    'portal.account.status_disabled': '已禁用',
    'portal.account.status_inactive': '未激活',
    'portal.account.expires': '到期时间',
    'portal.account.daysLeft': '剩余天数',
    'portal.account.days': '天',
    'portal.account.connection': '连接',
    'portal.account.online': '在线',
    'portal.account.offline': '离线',

    // Usage section
    'portal.usage.title': '数据用量',
    'portal.usage.remaining': '剩余',
    'portal.usage.used': '已使用',
    'portal.unlimitedData': '不限流量',
    'portal.noExpiry': '无到期时间',

    // VPN section
    'portal.vpn.title': '我的VPN',
    'portal.vpn.subUrl': '订阅链接',
    'portal.vpn.subUrlDesc': '将此链接粘贴到VPN应用中即可自动连接。',
    'portal.vpn.copy': '复制',
    'portal.vpn.copied': '已复制！',
    'portal.vpn.download': '下载',
    'portal.vpn.unavailable': '不可用',
    'portal.vpn.noProfiles': '暂无VPN文件',
    'portal.vpn.noProfilesDesc': '连接文件准备好后会显示在这里。',

    // Support section
    'portal.support.title': '帮助与支持',

    // Apps section
    'portal.apps.title': '下载应用',
    'portal.apps.desc': '下载适合您设备的应用，轻松连接。',
    'portal.support.newTicket': '寻求帮助',
    'portal.support.subject': '问题是什么？',
    'portal.support.subjectPlaceholder': '简短描述...',
    'portal.support.message': '详细说明',
    'portal.support.messagePlaceholder': '描述发生了什么...',
    'portal.support.cancel': '取消',
    'portal.support.create': '发送',
    'portal.support.ticketCreated': '消息已发送！我们会尽快回复。',
    'portal.support.replySent': '回复已发送。',
    'portal.support.send': '发送',
    'portal.support.yourReply': '你的回复',
    'portal.support.replyPlaceholder': '输入消息...',
    'portal.support.backToList': '返回',
    'portal.support.open': '待处理',
    'portal.support.closed': '已关闭',
    'portal.support.noTickets': '暂无消息',
    'portal.support.noTicketsDesc': '如需帮助，请点击上方按钮。',
  },
  ru: {
    // Nav / Shell
    'portal.nav.dashboard': 'Панель',
    'portal.nav.support': 'Поддержка',
    'portal.nav.vpn': 'Мой VPN',
    'portal.nav.profile': 'Настройки профиля',
    'portal.nav.logout': 'Выход',
    'portal.nav.language': 'Язык',
    'portal.nav.theme': 'Сменить тему',

    // Welcome
    'portal.hello': 'Привет',
    'portal.welcome': 'Все что нужно - прямо здесь.',

    // Account section
    'portal.account.title': 'Мой аккаунт',
    'portal.account.plan': 'Тариф',
    'portal.account.status': 'Статус',
    'portal.account.status_active': 'Активен',
    'portal.account.status_expired': 'Истек',
    'portal.account.status_disabled': 'Отключен',
    'portal.account.status_inactive': 'Неактивен',
    'portal.account.expires': 'Истекает',
    'portal.account.daysLeft': 'Осталось дней',
    'portal.account.days': 'дней',
    'portal.account.connection': 'Подключение',
    'portal.account.online': 'Онлайн',
    'portal.account.offline': 'Офлайн',

    // Usage section
    'portal.usage.title': 'Использование данных',
    'portal.usage.remaining': 'осталось',
    'portal.usage.used': 'использовано',
    'portal.unlimitedData': 'Безлимит',
    'portal.noExpiry': 'Без срока',

    // VPN section
    'portal.vpn.title': 'Мой VPN',
    'portal.vpn.subUrl': 'Ссылка подписки',
    'portal.vpn.subUrlDesc': 'Вставьте эту ссылку в VPN-приложение для автоматического подключения.',
    'portal.vpn.copy': 'Копировать',
    'portal.vpn.copied': 'Скопировано!',
    'portal.vpn.download': 'Скачать',
    'portal.vpn.unavailable': 'Н/Д',
    'portal.vpn.noProfiles': 'Пока нет файлов VPN',
    'portal.vpn.noProfilesDesc': 'Файлы подключения появятся здесь, когда будут готовы.',

    // Apps section
    'portal.apps.title': 'Скачать приложения',
    'portal.apps.desc': 'Скачайте приложение для вашего устройства.',

    // Support section
    'portal.support.title': 'Помощь и поддержка',
    'portal.support.newTicket': 'Задать вопрос',
    'portal.support.subject': 'В чем проблема?',
    'portal.support.subjectPlaceholder': 'Краткое описание...',
    'portal.support.message': 'Расскажите подробнее',
    'portal.support.messagePlaceholder': 'Опишите что произошло...',
    'portal.support.cancel': 'Отмена',
    'portal.support.create': 'Отправить',
    'portal.support.ticketCreated': 'Сообщение отправлено! Мы скоро ответим.',
    'portal.support.replySent': 'Ответ отправлен.',
    'portal.support.send': 'Отправить',
    'portal.support.yourReply': 'Ваш ответ',
    'portal.support.replyPlaceholder': 'Введите сообщение...',
    'portal.support.backToList': 'Назад',
    'portal.support.open': 'Открыт',
    'portal.support.closed': 'Закрыт',
    'portal.support.noTickets': 'Пока нет сообщений',
    'portal.support.noTicketsDesc': 'Если нужна помощь, нажмите кнопку выше.',
  },
}

registerMessages(messages)

const { t, locale, setLocale } = useI18n()
export { t, locale, setLocale }
