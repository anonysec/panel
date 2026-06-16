<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, type Ref } from 'vue'
import { onClickOutside } from '@vueuse/core'
import { useApi } from '@koris/composables/useApi'

/**
 * NotificationCenter - Bell icon with badge that opens a dropdown panel
 * showing recent data_warning events and account status changes.
 *
 * Fetches warnings from GET /api/portal/warnings on mount and at
 * a configurable polling interval (default: 60 seconds).
 *
 * Validates: Requirements 5.4, 6.7
 */

interface Warning {
  id: number
  type: string
  severity: 'warning' | 'error' | 'info'
  message: string
  created_at: string
  seen?: boolean
}

interface WarningsResponse {
  ok: boolean
  warnings: Warning[]
}

const { get } = useApi()

const warnings = ref<Warning[]>([])
const isOpen = ref(false)
const seenIds = ref<Set<number>>(new Set())
const loading = ref(false)
const containerRef = ref<HTMLElement | null>(null) as Ref<HTMLElement | null>

// Close panel when clicking outside
onClickOutside(containerRef, () => {
  if (isOpen.value) {
    isOpen.value = false
  }
})

// Poll interval in ms (60 seconds)
const POLL_INTERVAL = 60_000
let pollTimer: ReturnType<typeof setInterval> | null = null

const unseenCount = computed(() => {
  return warnings.value.filter(w => !seenIds.value.has(w.id)).length
})

const sortedWarnings = computed(() => {
  return [...warnings.value].sort(
    (a, b) => new Date(b.created_at).getTime() - new Date(a.created_at).getTime()
  )
})

async function fetchWarnings() {
  loading.value = true
  try {
    const res = await get<WarningsResponse>('/api/portal/warnings')
    if (res.ok && Array.isArray(res.warnings)) {
      warnings.value = res.warnings
    }
  } catch {
    // Silently fail — do not disrupt user flow for notifications
  } finally {
    loading.value = false
  }
}

function togglePanel() {
  isOpen.value = !isOpen.value
  if (isOpen.value) {
    // Mark all current warnings as seen when panel is opened
    warnings.value.forEach(w => seenIds.value.add(w.id))
  }
}

function formatTime(dateStr: string): string {
  const date = new Date(dateStr)
  const now = new Date()
  const diffMs = now.getTime() - date.getTime()
  const diffMins = Math.floor(diffMs / 60_000)

  if (diffMins < 1) return 'Just now'
  if (diffMins < 60) return `${diffMins}m ago`
  const diffHours = Math.floor(diffMins / 60)
  if (diffHours < 24) return `${diffHours}h ago`
  const diffDays = Math.floor(diffHours / 24)
  return `${diffDays}d ago`
}

function severityIcon(severity: string): string {
  switch (severity) {
    case 'error': return '!'
    case 'warning': return '⚠'
    default: return 'i'
  }
}

onMounted(() => {
  fetchWarnings()
  pollTimer = setInterval(fetchWarnings, POLL_INTERVAL)
})

onUnmounted(() => {
  if (pollTimer) {
    clearInterval(pollTimer)
    pollTimer = null
  }
})
</script>

<template>
  <div class="notification-center" ref="containerRef">
    <button
      class="notification-center__trigger"
      :aria-label="`Notifications${unseenCount > 0 ? ` (${unseenCount} unseen)` : ''}`"
      @click="togglePanel"
    >
      <svg class="notification-center__icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
        <path d="M18 8A6 6 0 0 0 6 8c0 7-3 9-3 9h18s-3-2-3-9" />
        <path d="M13.73 21a2 2 0 0 1-3.46 0" />
      </svg>
      <span v-if="unseenCount > 0" class="notification-center__badge">
        {{ unseenCount > 99 ? '99+' : unseenCount }}
      </span>
    </button>

    <Transition name="dropdown">
      <div v-if="isOpen" class="notification-center__panel">
        <div class="notification-center__header">
          <span class="notification-center__title">Notifications</span>
          <span class="notification-center__count">{{ warnings.length }} total</span>
        </div>

        <div v-if="loading && warnings.length === 0" class="notification-center__loading">
          Loading...
        </div>

        <div v-else-if="warnings.length === 0" class="notification-center__empty">
          No notifications
        </div>

        <ul v-else class="notification-center__list">
          <li
            v-for="warning in sortedWarnings"
            :key="warning.id"
            class="notification-center__item"
            :class="`notification-center__item--${warning.severity}`"
          >
            <span class="notification-center__item-icon" :class="`notification-center__item-icon--${warning.severity}`">
              {{ severityIcon(warning.severity) }}
            </span>
            <div class="notification-center__item-content">
              <p class="notification-center__item-message">{{ warning.message }}</p>
              <span class="notification-center__item-time">{{ formatTime(warning.created_at) }}</span>
            </div>
          </li>
        </ul>
      </div>
    </Transition>
  </div>
</template>

<style scoped>
.notification-center {
  position: relative;
}

.notification-center__trigger {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: center;
  background: none;
  border: none;
  color: var(--color-muted);
  cursor: pointer;
  padding: var(--space-1) var(--space-2);
  border-radius: var(--radius-sm);
  transition: color var(--duration-fast);
}

.notification-center__trigger:hover {
  color: var(--color-text);
}

.notification-center__icon {
  width: 20px;
  height: 20px;
}

.notification-center__badge {
  position: absolute;
  top: -2px;
  right: 0;
  min-width: 16px;
  height: 16px;
  padding: 0 4px;
  border-radius: 999px;
  background: var(--color-danger, #ef4444);
  color: #fff;
  font-size: 10px;
  font-weight: 700;
  display: flex;
  align-items: center;
  justify-content: center;
  line-height: 1;
}

.notification-center__panel {
  position: absolute;
  top: calc(100% + var(--space-2, 8px));
  right: 0;
  width: 320px;
  max-height: 400px;
  background: var(--color-surface, #0b1120);
  border: 1px solid var(--color-border, rgba(148, 163, 184, 0.12));
  border-radius: var(--radius-lg, 12px);
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.3);
  overflow: hidden;
  z-index: 1000;
  display: flex;
  flex-direction: column;
}

.notification-center__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: var(--space-3) var(--space-4);
  border-bottom: 1px solid var(--color-border, rgba(148, 163, 184, 0.12));
}

.notification-center__title {
  font-weight: 600;
  font-size: var(--text-sm, 14px);
  color: var(--color-text);
}

.notification-center__count {
  font-size: var(--text-xs, 12px);
  color: var(--color-muted);
}

.notification-center__loading,
.notification-center__empty {
  padding: var(--space-6) var(--space-4);
  text-align: center;
  font-size: var(--text-sm, 14px);
  color: var(--color-muted);
}

.notification-center__list {
  list-style: none;
  margin: 0;
  padding: 0;
  overflow-y: auto;
  max-height: 340px;
}

.notification-center__item {
  display: flex;
  gap: var(--space-3, 12px);
  padding: var(--space-3) var(--space-4);
  border-bottom: 1px solid var(--color-border, rgba(148, 163, 184, 0.06));
  transition: background var(--duration-fast, 150ms);
}

.notification-center__item:last-child {
  border-bottom: none;
}

.notification-center__item:hover {
  background: var(--color-surface-2, rgba(148, 163, 184, 0.04));
}

.notification-center__item-icon {
  flex-shrink: 0;
  width: 24px;
  height: 24px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 11px;
  font-weight: 700;
}

.notification-center__item-icon--error {
  background: rgba(239, 68, 68, 0.15);
  color: var(--color-danger, #ef4444);
}

.notification-center__item-icon--warning {
  background: rgba(245, 158, 11, 0.15);
  color: var(--color-warning, #f59e0b);
}

.notification-center__item-icon--info {
  background: rgba(37, 99, 235, 0.15);
  color: var(--color-primary, #2563eb);
}

.notification-center__item-content {
  flex: 1;
  min-width: 0;
}

.notification-center__item-message {
  margin: 0;
  font-size: var(--text-sm, 14px);
  color: var(--color-text);
  line-height: 1.4;
}

.notification-center__item-time {
  font-size: var(--text-xs, 12px);
  color: var(--color-muted);
  margin-top: 2px;
  display: block;
}

/* Dropdown transition */
.dropdown-enter-active,
.dropdown-leave-active {
  transition: opacity 0.15s ease, transform 0.15s ease;
}

.dropdown-enter-from,
.dropdown-leave-to {
  opacity: 0;
  transform: translateY(-4px);
}
</style>
