<script setup lang="ts">
/**
 * ConnectedClients — Displays currently connected devices for a user.
 *
 * Uses the useConnectedClients composable to fetch and display active connections.
 * Each connection shows IP address, device (or "Unknown device"), and user-agent.
 * Shows a placeholder when there are no active connections.
 * Provides a manual refresh button.
 *
 * Requirements: 4.1, 4.2, 4.3, 4.4, 4.5
 */
import { ref, watch } from 'vue'
import { useConnectedClients } from '@/composables/useConnectedClients'
import type { ConnectedClient } from '@koris/types/entities'
import KButton from '@koris/ui/KButton.vue'

const props = withDefaults(defineProps<{
  userId: number | null
  showTitle?: boolean
}>(), {
  showTitle: true,
})

// Create a stable ref for the composable that stays in sync with the prop
const internalUserId = ref<number | null>(props.userId)

watch(() => props.userId, (v) => {
  internalUserId.value = v
})

const { clients, loading, refresh } = useConnectedClients(internalUserId)

function getDeviceLabel(client: ConnectedClient): string {
  return client.device || 'Unknown device'
}

function getUserAgentLabel(client: ConnectedClient): string {
  return client.user_agent || 'Unknown device'
}
</script>

<template>
  <section class="connected-clients" aria-label="Connected Clients">
    <div class="connected-clients__header">
      <h3 v-if="showTitle" class="connected-clients__title">Connected Clients</h3>
      <KButton
        variant="ghost"
        size="sm"
        :disabled="loading"
        aria-label="Refresh connected clients"
        @click="refresh"
      >
        <span class="connected-clients__refresh-icon" :class="{ 'connected-clients__refresh-icon--spinning': loading }">↻</span>
      </KButton>
    </div>

    <!-- Loading state -->
    <div v-if="loading && clients.length === 0" class="connected-clients__loading">
      <span class="connected-clients__loading-text">Loading connections…</span>
    </div>

    <!-- Empty state -->
    <div v-else-if="clients.length === 0" class="connected-clients__empty">
      <p class="connected-clients__empty-text">No active connections</p>
    </div>

    <!-- Client list -->
    <ul v-else class="connected-clients__list" role="list">
      <li
        v-for="(client, index) in clients"
        :key="`${client.ip}-${index}`"
        class="connected-clients__item"
      >
        <div class="connected-clients__item-ip">
          <span class="connected-clients__dot" />
          {{ client.ip }}
        </div>
        <div class="connected-clients__item-device">
          {{ getDeviceLabel(client) }}
        </div>
        <div class="connected-clients__item-ua">
          {{ getUserAgentLabel(client) }}
        </div>
      </li>
    </ul>
  </section>
</template>

<style scoped>
.connected-clients {
  display: flex;
  flex-direction: column;
  gap: var(--space-2, 8px);
  padding: var(--space-4, 16px);
  border-top: 1px solid var(--color-border, #e5e7eb);
}

.connected-clients__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.connected-clients__title {
  margin: 0;
  font-size: var(--text-sm, 0.875rem);
  font-weight: var(--font-semibold, 600);
  color: var(--color-text, #1f2937);
}

.connected-clients__refresh-icon {
  display: inline-block;
  font-size: 1rem;
  line-height: 1;
  transition: transform 200ms ease-out;
}

.connected-clients__refresh-icon--spinning {
  animation: spin 600ms linear infinite;
}

@keyframes spin {
  from { transform: rotate(0deg); }
  to { transform: rotate(360deg); }
}

.connected-clients__loading {
  padding: var(--space-3, 12px) 0;
}

.connected-clients__loading-text {
  font-size: var(--text-sm, 0.875rem);
  color: var(--color-muted, #6b7280);
}

.connected-clients__empty {
  padding: var(--space-3, 12px) 0;
}

.connected-clients__empty-text {
  margin: 0;
  font-size: var(--text-sm, 0.875rem);
  color: var(--color-muted, #6b7280);
  text-align: center;
}

.connected-clients__list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: var(--space-2, 8px);
}

.connected-clients__item {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: var(--space-2, 8px) var(--space-3, 12px);
  border-radius: var(--radius-sm, 6px);
  background: var(--color-surface, #f9fafb);
  border: 1px solid var(--color-border, #e5e7eb);
}

.connected-clients__item-ip {
  display: flex;
  align-items: center;
  gap: var(--space-2, 8px);
  font-size: var(--text-sm, 0.875rem);
  font-weight: var(--font-medium, 500);
  color: var(--color-text, #1f2937);
  font-family: var(--font-mono, monospace);
}

.connected-clients__dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--color-success, #22c55e);
  flex-shrink: 0;
}

.connected-clients__item-device {
  font-size: var(--text-xs, 0.75rem);
  color: var(--color-muted, #6b7280);
  padding-left: 14px;
}

.connected-clients__item-ua {
  font-size: var(--text-xs, 0.75rem);
  color: var(--color-muted, #6b7280);
  padding-left: 14px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

/* Respect reduced motion preference */
@media (prefers-reduced-motion: reduce) {
  .connected-clients__refresh-icon--spinning {
    animation: none;
  }

  .connected-clients__refresh-icon {
    transition-duration: 0ms;
  }
}
</style>
