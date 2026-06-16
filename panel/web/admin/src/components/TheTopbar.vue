<script setup lang="ts">
import type { Breadcrumb } from '@koris/types/components'

export interface Props {
  title: string
  subtitle?: string
  breadcrumbs?: Breadcrumb[]
  realtimeConnected: boolean
  notificationCount: number
}

withDefaults(defineProps<Props>(), {
  subtitle: '',
  breadcrumbs: () => [],
  realtimeConnected: false,
  notificationCount: 0,
})

const emit = defineEmits<{
  (e: 'open-command-palette'): void
  (e: 'open-notifications'): void
  (e: 'toggle-theme'): void
  (e: 'search', query: string): void
}>()

const searchQuery = defineModel<string>('searchQuery', { default: '' })

function handleSearchKeyup(event: KeyboardEvent) {
  if (event.key === 'Enter') {
    emit('search', searchQuery.value)
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

      <!-- Title and subtitle -->
      <h2 class="topbar-title">{{ title }}</h2>
      <p v-if="subtitle" class="topbar-subtitle">{{ subtitle }}</p>
    </div>

    <div class="topbar-right">
      <!-- Search box -->
      <div class="search-box">
        <svg
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
          aria-hidden="true"
        >
          <circle cx="11" cy="11" r="7" />
          <path d="M21 21l-4-4" />
        </svg>
        <input
          v-model="searchQuery"
          type="text"
          placeholder="Search..."
          aria-label="Search"
          @keyup="handleSearchKeyup"
          @focus="emit('open-command-palette')"
        />
        <kbd class="search-shortcut">Ctrl+K</kbd>
      </div>

      <!-- Realtime connection status -->
      <div
        :class="['status-dot', { offline: !realtimeConnected }]"
        :title="realtimeConnected ? 'Realtime connected' : 'Realtime disconnected'"
        :aria-label="realtimeConnected ? 'Realtime connected' : 'Realtime disconnected'"
        role="status"
      />

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

/* Search box */
.search-box {
  display: flex;
  align-items: center;
  gap: var(--space-2, 8px);
  background: var(--color-surface, #0b1120);
  border: 1px solid var(--color-border, #28333f);
  padding: var(--space-2, 8px) var(--space-3, 12px);
  border-radius: var(--radius-lg, 10px);
  width: 220px;
  transition: border-color var(--duration-normal, 0.15s) var(--ease-default, ease);
}

.search-box:focus-within {
  border-color: rgba(37, 99, 235, 0.5);
  box-shadow: 0 0 0 3px rgba(37, 99, 235, 0.1);
}

.search-box svg {
  width: 15px;
  height: 15px;
  color: var(--color-muted, #8b98a5);
  flex-shrink: 0;
}

.search-box input {
  background: none;
  border: none;
  outline: none;
  color: var(--color-text, #e6edf3);
  font-size: var(--text-sm, 12.5px);
  width: 100%;
  min-height: unset;
  padding: 0;
}

.search-box input::placeholder {
  color: rgba(139, 152, 165, 0.5);
}

.search-shortcut {
  font-size: 10px;
  color: var(--color-muted, #8b98a5);
  background: var(--color-surface-2, #1e2630);
  border: 1px solid var(--color-border, #28333f);
  border-radius: var(--radius-sm, 6px);
  padding: 2px 5px;
  font-family: var(--font-mono, monospace);
  white-space: nowrap;
  flex-shrink: 0;
}

/* Status dot */
.status-dot {
  width: 8px;
  height: 8px;
  border-radius: var(--radius-full, 9999px);
  background: var(--color-success, #22c55e);
  box-shadow: 0 0 6px rgba(34, 197, 94, 0.5);
  animation: livePulse 2s ease-in-out infinite;
  flex-shrink: 0;
}

.status-dot.offline {
  background: var(--color-danger, #ef4444);
  box-shadow: 0 0 6px rgba(239, 68, 68, 0.5);
  animation: none;
}

@keyframes livePulse {
  0%, 100% {
    box-shadow: 0 0 6px rgba(34, 197, 94, 0.5);
  }
  50% {
    box-shadow: 0 0 12px rgba(34, 197, 94, 0.8), 0 0 4px rgba(34, 197, 94, 1);
  }
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

</style>
