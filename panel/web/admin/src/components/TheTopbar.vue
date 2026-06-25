<script setup lang="ts">
import { ref, computed } from 'vue'
import { useRouter } from 'vue-router'
import type { Breadcrumb } from '@koris/types/components'
import { useI18n } from '@koris/composables/useI18n'
import { useRealtimeStore } from '@/stores/realtime'

const { t, locale } = useI18n()
const router = useRouter()
const realtimeStore = useRealtimeStore()

export interface Props {
  title: string
  subtitle?: string
  breadcrumbs?: Breadcrumb[]
  notificationCount: number
}

withDefaults(defineProps<Props>(), {
  subtitle: '',
  breadcrumbs: () => [],
  notificationCount: 0,
})

const emit = defineEmits<{
  (e: 'open-command-palette'): void
  (e: 'open-notifications'): void
  (e: 'toggle-theme'): void
  (e: 'change-lang', locale: string): void
}>()

// Platform-aware keyboard shortcut display
const shortcutLabel = computed(() => {
  const isMac = typeof navigator !== 'undefined' &&
    (/mac/i.test(navigator.platform) || /macintosh/i.test(navigator.userAgent))
  return isMac ? 'Cmd+K' : 'Ctrl+K'
})

// Notification dropdown hover state
const showNotifDropdown = ref(false)
let hideTimeout: ReturnType<typeof setTimeout> | null = null

// Language dropdown state
const showLangDropdown = ref(false)
let langHideTimeout: ReturnType<typeof setTimeout> | null = null

const langOptions = [
  { code: 'en', label: 'EN' },
  { code: 'fa', label: 'FA' },
  { code: 'zh', label: 'ZH' },
  { code: 'ru', label: 'RU' },
]

function onLangEnter() {
  if (langHideTimeout) {
    clearTimeout(langHideTimeout)
    langHideTimeout = null
  }
  showLangDropdown.value = true
}

function onLangLeave() {
  langHideTimeout = setTimeout(() => {
    showLangDropdown.value = false
  }, 200)
}

function selectLang(code: string) {
  emit('change-lang', code)
  showLangDropdown.value = false
}

function onBellEnter() {
  if (hideTimeout) {
    clearTimeout(hideTimeout)
    hideTimeout = null
  }
  showNotifDropdown.value = true
}

function onBellLeave() {
  hideTimeout = setTimeout(() => {
    showNotifDropdown.value = false
  }, 200)
}

function onBellClick() {
  showNotifDropdown.value = false
  router.push({ name: 'notifications' })
}

// Get recent notifications (last 5)
const recentNotifications = computed(() => realtimeStore.notifications.slice(0, 5))

function formatTime(timestamp: string): string {
  try {
    const date = new Date(timestamp)
    const now = new Date()
    const diff = now.getTime() - date.getTime()
    const seconds = Math.floor(diff / 1000)
    const minutes = Math.floor(seconds / 60)
    const hours = Math.floor(minutes / 60)
    const days = Math.floor(hours / 24)

    // Map locale codes to BCP 47 tags for Intl.RelativeTimeFormat
    const localeMap: Record<string, string> = { en: 'en', fa: 'fa', zh: 'zh' }
    const bcp47 = localeMap[locale.value] || 'en'

    const rtf = new Intl.RelativeTimeFormat(bcp47, { numeric: 'auto', style: 'short' })

    if (minutes < 1) return rtf.format(0, 'second')
    if (minutes < 60) return rtf.format(-minutes, 'minute')
    if (hours < 24) return rtf.format(-hours, 'hour')
    return rtf.format(-days, 'day')
  } catch {
    return ''
  }
}
</script>

<template>
  <div class="topbar">
    <div class="topbar-left">
      <!-- Breadcrumb navigation -->
      <nav
        v-if="breadcrumbs && breadcrumbs.length > 0"
        class="topbar-breadcrumb"
        aria-label="Breadcrumb"
      >
        <ol class="breadcrumb-list">
          <li
            v-for="(crumb, index) in breadcrumbs"
            :key="index"
            class="breadcrumb-item"
          >
            <router-link
              v-if="crumb.to && index < breadcrumbs.length - 1"
              :to="crumb.to"
              class="breadcrumb-link"
            >
              {{ crumb.label }}
            </router-link>
            <span
              v-else
              class="breadcrumb-current"
              :aria-current="index === breadcrumbs.length - 1 ? 'page' : undefined"
            >
              {{ crumb.label }}
            </span>
            <svg
              v-if="index < breadcrumbs.length - 1"
              class="breadcrumb-separator"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              stroke-width="2"
              aria-hidden="true"
            >
              <path d="M9 18l6-6-6-6" />
            </svg>
          </li>
        </ol>
      </nav>

      <!-- Title and subtitle (hide title when breadcrumbs already show the page name) -->
      <h2 v-if="!breadcrumbs || breadcrumbs.length === 0" class="topbar-title">{{ title }}</h2>
      <p v-if="subtitle" class="topbar-subtitle">{{ subtitle }}</p>
    </div>

    <div class="topbar-right">
      <!-- Search box - always visible, opens command palette on click -->
      <button
        class="search-box"
        type="button"
        aria-label="Search"
        @click="emit('open-command-palette')"
      >
        <svg class="search-box-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true">
          <circle cx="11" cy="11" r="7" />
          <path d="M21 21l-4-4" />
        </svg>
        <span class="search-box-text">{{ t('topbar.search') }}</span>
        <kbd class="search-shortcut">{{ shortcutLabel }}</kbd>
      </button>

      <!-- Language selector -->
      <div
        class="lang-dropdown-wrapper"
        @mouseenter="onLangEnter"
        @mouseleave="onLangLeave"
      >
        <button
          class="icon-btn"
          :title="t('label.language')"
          :aria-label="t('label.language')"
        >
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true">
            <circle cx="12" cy="12" r="10" />
            <path d="M2 12h20" />
            <path d="M12 2a15.3 15.3 0 014 10 15.3 15.3 0 01-4 10 15.3 15.3 0 01-4-10 15.3 15.3 0 014-10z" />
          </svg>
          <span class="lang-badge">{{ locale.toUpperCase() }}</span>
        </button>

        <!-- Language dropdown -->
        <div v-if="showLangDropdown" class="lang-dropdown">
          <button
            v-for="lang in langOptions"
            :key="lang.code"
            class="lang-option"
            :class="{ 'lang-option--active': locale === lang.code }"
            @click="selectLang(lang.code)"
          >
            {{ lang.label }}
          </button>
        </div>
      </div>

      <!-- Notification bell with badge and dropdown -->
      <div
        class="notif-bell-wrapper"
        @mouseenter="onBellEnter"
        @mouseleave="onBellLeave"
      >
        <button
          class="icon-btn"
          :title="t('nav.notifications')"
          :aria-label="t('nav.notifications')"
          @click="onBellClick"
        >
          <svg
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            aria-hidden="true"
          >
            <path d="M18 8a6 6 0 10-12 0c0 7-3 9-3 9h18s-3-2-3-9" />
            <path d="M13.7 21a2 2 0 01-3.4 0" />
          </svg>
          <span v-if="notificationCount > 0" class="notif-badge">
            {{ notificationCount > 99 ? '99+' : notificationCount }}
          </span>
        </button>

        <!-- Hover dropdown -->
        <div v-if="showNotifDropdown" class="notif-dropdown">
          <div class="notif-head">
            <b>{{ t('nav.notifications') }}</b>
            <button
              v-if="notificationCount > 0"
              class="notif-mark-all"
              @click.stop="realtimeStore.markAllRead()"
            >
              {{ t('notifications.mark_all_read') }}
            </button>
          </div>
          <div class="notif-list">
            <div
              v-for="notif in recentNotifications"
              :key="notif.id"
              class="notif-item"
              :class="{ 'notif-item--unread': !notif.read }"
            >
              <div class="notif-icon">
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" width="14" height="14">
                  <path d="M18 8a6 6 0 10-12 0c0 7-3 9-3 9h18s-3-2-3-9" />
                </svg>
              </div>
              <div class="notif-text">
                <b>{{ notif.message }}</b>
                <span>{{ formatTime(notif.timestamp) }}</span>
              </div>
            </div>
            <div v-if="recentNotifications.length === 0" class="notif-empty">
              {{ t('notifications.empty') }}
            </div>
          </div>
          <div v-if="recentNotifications.length > 0" class="notif-footer">
            <button class="notif-view-all" @click="onBellClick">
              {{ t('btn.view_all') }}
            </button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.topbar {
  display: flex;
  align-items: center;
  gap: var(--space-4, 16px);
  margin-bottom: var(--space-5, 20px);
  min-height: 48px;
}

.topbar-left {
  display: flex;
  flex-direction: column;
  gap: var(--space-1, 4px);
  min-width: 0;
}

.topbar-title {
  font-size: var(--text-2xl, 20px);
  font-weight: var(--font-bold, 700);
  letter-spacing: var(--tracking-tight, -0.02em);
  color: var(--color-text, #e6edf3);
  line-height: var(--leading-tight, 1.1);
}

.topbar-subtitle {
  font-size: var(--text-sm, 12.5px);
  color: var(--color-muted, #8b98a5);
  margin-top: 2px;
}

/* Breadcrumb */
.topbar-breadcrumb {
  margin-bottom: var(--space-1, 4px);
}

.breadcrumb-list {
  display: flex;
  align-items: center;
  gap: var(--space-1, 4px);
  list-style: none;
  padding: 0;
  margin: 0;
}

.breadcrumb-item {
  display: flex;
  align-items: center;
  gap: var(--space-1, 4px);
  font-size: var(--text-sm, 12.5px);
}

.breadcrumb-link {
  color: var(--color-muted, #8b98a5);
  text-decoration: none;
  transition: color var(--duration-fast, 0.12s) var(--ease-default, ease);
}

.breadcrumb-link:hover {
  color: var(--color-primary, #2563eb);
}

.breadcrumb-current {
  color: var(--color-text, #e6edf3);
  font-weight: var(--font-medium, 500);
}

.breadcrumb-separator {
  width: 12px;
  height: 12px;
  color: var(--color-muted, #8b98a5);
  opacity: 0.5;
  flex-shrink: 0;
}

/* Right section */
.topbar-right {
  margin-left: auto;
  display: flex;
  align-items: center;
  gap: var(--space-3, 12px);
}

/* Search box - always visible, clickable to open command palette */
.search-box {
  display: flex;
  align-items: center;
  gap: var(--space-2, 8px);
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  padding: 0 var(--space-3, 12px);
  border-radius: var(--radius-md);
  height: 36px;
  cursor: pointer;
  transition: border-color var(--duration-normal, 0.15s) var(--ease-default, ease),
              box-shadow var(--duration-normal, 0.15s) var(--ease-default, ease);
  color: var(--color-muted, #8b98a5);
  font-family: var(--font-family);
}

.search-box:hover {
  border-color: var(--color-primary, #2563eb);
  box-shadow: 0 0 0 2px rgba(37, 99, 235, 0.15);
}

.search-box:focus-visible {
  outline: 2px solid var(--color-primary, #2563eb);
  outline-offset: 2px;
}

.search-box-icon {
  width: 16px;
  height: 16px;
  min-width: 16px;
  color: var(--color-muted);
  flex-shrink: 0;
}

.search-box-text {
  font-size: var(--text-sm, 12.5px);
  color: var(--color-muted, #8b98a5);
  white-space: nowrap;
}

.search-shortcut {
  font-size: 10px;
  color: var(--color-muted);
  background: var(--color-surface-2, #1e2630);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-sm);
  padding: 2px 5px;
  font-family: var(--font-mono, monospace);
  white-space: nowrap;
  flex-shrink: 0;
}

/* Notification bell wrapper */
.notif-bell-wrapper {
  position: relative;
}

/* Icon button */
.icon-btn {
  width: 38px;
  height: 38px;
  border-radius: var(--radius-lg, 10px);
  background: var(--color-surface, #0b1120);
  border: 1px solid var(--color-border, #28333f);
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--color-muted, #8b98a5);
  position: relative;
  transition: color var(--duration-normal, 0.15s), border-color var(--duration-normal, 0.15s);
  cursor: pointer;
}

.icon-btn:hover {
  color: var(--color-text, #e6edf3);
  border-color: #3a4756;
}

.icon-btn:focus-visible {
  outline: 2px solid var(--color-primary, #2563eb);
  outline-offset: 2px;
}

.icon-btn svg {
  width: 16px;
  height: 16px;
}

/* Notification badge */
.notif-badge {
  position: absolute;
  top: 4px;
  right: 4px;
  min-width: 16px;
  height: 16px;
  padding: 0 4px;
  border-radius: 8px;
  background: #ef4444;
  color: #fff;
  font-size: 10px;
  font-weight: 700;
  display: flex;
  align-items: center;
  justify-content: center;
  line-height: 1;
  border: 2px solid var(--color-surface, #0b1120);
}

/* Notification dropdown (scoped overrides for positioning) */
.notif-dropdown {
  position: absolute;
  top: calc(100% + 8px);
  right: 0;
  width: 320px;
  background: var(--color-surface, #0b1120);
  border: 1px solid var(--color-border, #28333f);
  border-radius: 12px;
  box-shadow: 0 20px 60px rgba(0, 0, 0, 0.4);
  z-index: 100;
  overflow: hidden;
}

.notif-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 16px;
  border-bottom: 1px solid var(--color-border, #28333f);
}

.notif-head b {
  font-size: 13px;
  color: var(--color-text, #e6edf3);
}

.notif-mark-all {
  font-size: 11px;
  color: var(--color-primary, #2563eb);
  background: none;
  border: none;
  cursor: pointer;
  padding: 0;
}

.notif-mark-all:hover {
  text-decoration: underline;
}

.notif-list {
  max-height: 300px;
  overflow-y: auto;
}

.notif-item {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  padding: 12px 16px;
  cursor: pointer;
  transition: background 0.1s;
}

.notif-item:hover {
  background: var(--color-surface-2, #1e2630);
}

.notif-item + .notif-item {
  border-top: 1px solid var(--color-border, #28333f);
}

.notif-item--unread {
  background: rgba(37, 99, 235, 0.05);
}

.notif-icon {
  width: 28px;
  height: 28px;
  border-radius: 7px;
  background: var(--color-surface-2, #1e2630);
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  color: var(--color-muted, #8b98a5);
}

.notif-text {
  flex: 1;
  min-width: 0;
}

.notif-text b {
  display: block;
  font-size: 12.5px;
  font-weight: 600;
  color: var(--color-text, #e6edf3);
}

.notif-text span {
  display: block;
  font-size: 11px;
  color: var(--color-muted, #8b98a5);
  margin-top: 2px;
}

.notif-empty {
  padding: 24px 16px;
  text-align: center;
  color: var(--color-muted, #8b98a5);
  font-size: 12.5px;
}

.notif-footer {
  border-top: 1px solid var(--color-border, #28333f);
  padding: 8px 16px;
  text-align: center;
}

.notif-view-all {
  font-size: 12px;
  color: var(--color-primary, #2563eb);
  background: none;
  border: none;
  cursor: pointer;
  padding: 4px 8px;
  border-radius: var(--radius-sm, 4px);
}

.notif-view-all:hover {
  background: rgba(37, 99, 235, 0.1);
}

/* Mobile responsive: hide text and kbd on small screens */
@media (max-width: 640px) {
  .search-box-text,
  .search-shortcut {
    display: none;
  }

  .search-box {
    width: 36px;
    padding: 0;
    justify-content: center;
  }

  .notif-dropdown {
    width: 280px;
    right: -8px;
  }
}

/* Language dropdown */
.lang-dropdown-wrapper {
  position: relative;
}

.lang-badge {
  position: absolute;
  bottom: 2px;
  right: 2px;
  font-size: 8px;
  font-weight: 700;
  color: var(--color-text, #e6edf3);
  background: var(--color-surface-2, #1e2630);
  border-radius: 3px;
  padding: 0 2px;
  line-height: 1.2;
}

.lang-dropdown {
  position: absolute;
  top: calc(100% + 8px);
  right: 0;
  min-width: 80px;
  background: var(--color-surface, #0b1120);
  border: 1px solid var(--color-border, #28333f);
  border-radius: var(--radius-md, 8px);
  box-shadow: 0 12px 40px rgba(0, 0, 0, 0.35);
  z-index: 100;
  overflow: hidden;
  padding: 4px;
}

.lang-option {
  display: block;
  width: 100%;
  padding: 8px 12px;
  font-size: 12.5px;
  font-weight: 500;
  color: var(--color-muted, #8b98a5);
  background: none;
  border: none;
  border-radius: var(--radius-sm, 4px);
  cursor: pointer;
  text-align: left;
  transition: background 0.1s, color 0.1s;
}

.lang-option:hover {
  background: var(--color-surface-2, #1e2630);
  color: var(--color-text, #e6edf3);
}

.lang-option--active {
  color: var(--color-primary, #2563eb);
  font-weight: 600;
}

</style>
