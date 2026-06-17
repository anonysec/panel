<script setup lang="ts">
import { computed } from 'vue'
import type { Breadcrumb } from '@koris/types/components'
import { useI18n } from '@koris/composables/useI18n'
import type { Locale } from '@koris/composables/useI18n'

const { locale, setLocale } = useI18n()

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
}>()

// Platform-aware keyboard shortcut display
const shortcutLabel = computed(() => {
  const isMac = typeof navigator !== 'undefined' &&
    (/mac/i.test(navigator.platform) || /macintosh/i.test(navigator.userAgent))
  return isMac ? 'Cmd+K' : 'Ctrl+K'
})
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

      <!-- Title and subtitle -->
      <h2 class="topbar-title">{{ title }}</h2>
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
        <span class="search-box-text">Search...</span>
        <kbd class="search-shortcut">{{ shortcutLabel }}</kbd>
      </button>

      <!-- Language Switcher -->
      <div class="lang-switcher" role="group" aria-label="Language switcher">
        <button
          v-for="lang in (['en', 'fa', 'zh'] as Locale[])"
          :key="lang"
          :class="['lang-btn', { 'lang-btn--active': locale === lang }]"
          @click="setLocale(lang)"
        >
          {{ lang === 'en' ? 'EN' : lang === 'fa' ? 'FA' : 'ZH' }}
        </button>
      </div>

      <!-- Notification bell -->
      <button
        class="icon-btn"
        title="Notifications"
        aria-label="Notifications"
        @click="emit('open-notifications')"
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
      </button>
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

/* Language Switcher */
.lang-switcher {
  display: flex;
  align-items: center;
  border: 1px solid var(--color-border, #28333f);
  border-radius: var(--radius-md, 8px);
  overflow: hidden;
}

.lang-btn {
  padding: var(--space-1, 4px) var(--space-2, 8px);
  border: none;
  background: var(--color-surface, #0b1120);
  color: var(--color-muted, #8b98a5);
  font-size: var(--text-xs, 11px);
  font-weight: var(--font-medium, 500);
  cursor: pointer;
  transition: all var(--duration-fast, 0.12s);
}

.lang-btn:not(:last-child) {
  border-right: 1px solid var(--color-border, #28333f);
}

.lang-btn:hover {
  color: var(--color-text, #e6edf3);
  background: var(--color-surface-2, #1e2630);
}

.lang-btn--active {
  color: var(--color-primary, #2563eb);
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
}
</style>
