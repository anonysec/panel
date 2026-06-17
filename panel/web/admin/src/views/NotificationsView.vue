<script setup lang="ts">
import { computed } from 'vue'
import { useRealtimeStore } from '@/stores/realtime'
import { useI18n } from '@koris/composables/useI18n'

const { t } = useI18n()
const realtimeStore = useRealtimeStore()

const notifications = computed(() => realtimeStore.notifications)

function markRead(id: string) {
  const idx = realtimeStore.notifications.findIndex(n => n.id === id)
  if (idx !== -1) {
    realtimeStore.notifications[idx] = { ...realtimeStore.notifications[idx], read: true }
  }
}

function formatTime(timestamp: string): string {
  try {
    const date = new Date(timestamp)
    return date.toLocaleString()
  } catch {
    return ''
  }
}

function notifIcon(type: string): string {
  switch (type) {
    case 'ticket': return 'M21 15a2 2 0 01-2 2H7l-4 4V5a2 2 0 012-2h14a2 2 0 012 2z'
    case 'payment': return 'M12 2v20M17 5H9.5a3.5 3.5 0 000 7h5a3.5 3.5 0 010 7H6'
    case 'user': return 'M20 21v-2a4 4 0 00-4-4H8a4 4 0 00-4 4v2M12 3a4 4 0 100 8 4 4 0 000-8'
    default: return 'M18 8a6 6 0 10-12 0c0 7-3 9-3 9h18s-3-2-3-9'
  }
}
</script>

<template>
  <div class="notifications-page">
    <div class="notifications-header">
      <h2 class="notifications-title">{{ t('notifications.title') }}</h2>
      <button
        v-if="realtimeStore.notificationCount > 0"
        class="btn-mark-all"
        @click="realtimeStore.markAllRead()"
      >
        {{ t('notifications.mark_all_read') }}
      </button>
    </div>

    <div v-if="notifications.length === 0" class="notifications-empty">
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" class="empty-icon">
        <path d="M18 8a6 6 0 10-12 0c0 7-3 9-3 9h18s-3-2-3-9" />
        <path d="M13.7 21a2 2 0 01-3.4 0" />
      </svg>
      <p>{{ t('notifications.empty') }}</p>
    </div>

    <ul v-else class="notifications-list">
      <li
        v-for="notif in notifications"
        :key="notif.id"
        class="notification-card"
        :class="{ 'notification-card--unread': !notif.read }"
      >
        <div class="notification-icon">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" width="16" height="16">
            <path :d="notifIcon(notif.type)" />
          </svg>
        </div>
        <div class="notification-body">
          <p class="notification-message">{{ notif.message }}</p>
          <span class="notification-time">{{ formatTime(notif.timestamp) }}</span>
        </div>
        <button
          v-if="!notif.read"
          class="btn-mark-read"
          :title="t('notifications.mark_read')"
          @click="markRead(notif.id)"
        >
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" width="14" height="14">
            <polyline points="20 6 9 17 4 12" />
          </svg>
        </button>
      </li>
    </ul>
  </div>
</template>

<style scoped>
.notifications-page {
  max-width: 700px;
}

.notifications-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: var(--space-4, 16px);
}

.notifications-title {
  font-size: var(--text-xl, 18px);
  font-weight: var(--font-bold, 700);
  color: var(--color-text, #e6edf3);
}

.btn-mark-all {
  font-size: 12px;
  color: var(--color-primary, #2563eb);
  background: rgba(37, 99, 235, 0.1);
  border: 1px solid rgba(37, 99, 235, 0.2);
  border-radius: var(--radius-md, 8px);
  padding: 6px 12px;
  cursor: pointer;
  transition: background 0.15s;
}

.btn-mark-all:hover {
  background: rgba(37, 99, 235, 0.2);
}

.notifications-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 60px 20px;
  text-align: center;
  color: var(--color-muted, #8b98a5);
}

.empty-icon {
  width: 48px;
  height: 48px;
  margin-bottom: var(--space-3, 12px);
  opacity: 0.4;
}

.notifications-empty p {
  font-size: var(--text-sm, 13px);
}

.notifications-list {
  list-style: none;
  padding: 0;
  margin: 0;
  display: flex;
  flex-direction: column;
  gap: 1px;
}

.notification-card {
  display: flex;
  align-items: flex-start;
  gap: var(--space-3, 12px);
  padding: var(--space-3, 12px) var(--space-4, 16px);
  background: var(--color-surface, #0b1120);
  border: 1px solid var(--color-border, #28333f);
  border-radius: var(--radius-md, 8px);
  margin-bottom: var(--space-2, 8px);
  transition: background 0.1s;
}

.notification-card:hover {
  background: var(--color-surface-2, #1e2630);
}

.notification-card--unread {
  border-left: 3px solid var(--color-primary, #2563eb);
  background: rgba(37, 99, 235, 0.03);
}

.notification-icon {
  width: 32px;
  height: 32px;
  border-radius: 8px;
  background: var(--color-surface-2, #1e2630);
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  color: var(--color-muted, #8b98a5);
}

.notification-body {
  flex: 1;
  min-width: 0;
}

.notification-message {
  font-size: 13px;
  font-weight: 500;
  color: var(--color-text, #e6edf3);
  margin: 0 0 4px;
}

.notification-time {
  font-size: 11px;
  color: var(--color-muted, #8b98a5);
}

.btn-mark-read {
  width: 28px;
  height: 28px;
  border-radius: 6px;
  background: none;
  border: 1px solid var(--color-border, #28333f);
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--color-muted, #8b98a5);
  cursor: pointer;
  flex-shrink: 0;
  transition: color 0.15s, border-color 0.15s, background 0.15s;
}

.btn-mark-read:hover {
  color: var(--color-primary, #2563eb);
  border-color: var(--color-primary, #2563eb);
  background: rgba(37, 99, 235, 0.1);
}

@media (max-width: 640px) {
  .notifications-header {
    flex-direction: column;
    align-items: flex-start;
    gap: var(--space-2, 8px);
  }

  .notification-card {
    padding: var(--space-2, 8px) var(--space-3, 12px);
  }
}
</style>
