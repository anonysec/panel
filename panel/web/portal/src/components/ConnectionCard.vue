<script setup lang="ts">
import { computed } from 'vue'
import { formatBytes } from '@/composables/useUsageDisplay'

export interface ConnectionCardProps {
  protocol: string
  nodeName: string
  assignedIp: string
  duration: number
  rxBytes: number
  txBytes: number
}

const props = defineProps<ConnectionCardProps>()

const protocolBadgeClass = computed(() => {
  const map: Record<string, string> = {
    openvpn: 'badge--openvpn',
    wireguard: 'badge--wireguard',
    l2tp: 'badge--l2tp',
    ikev2: 'badge--ikev2',
    ssh: 'badge--ssh',
  }
  return map[props.protocol.toLowerCase()] || 'badge--default'
})

function formatDuration(seconds: number): string {
  if (seconds < 60) return `${seconds}s`
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m`
  const h = Math.floor(seconds / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  return `${h}h ${m}m`
}
</script>

<template>
  <div class="connection-card">
    <div class="connection-card__header">
      <span class="protocol-badge" :class="protocolBadgeClass">{{ protocol }}</span>
      <span class="node-name">{{ nodeName }}</span>
    </div>
    <div class="connection-card__body">
      <div class="info-row">
        <span class="info-row__label">IP</span>
        <code class="info-row__value">{{ assignedIp }}</code>
      </div>
      <div class="info-row">
        <span class="info-row__label">Duration</span>
        <span class="info-row__value">{{ formatDuration(duration) }}</span>
      </div>
      <div class="info-row">
        <span class="info-row__label">↓ RX</span>
        <span class="info-row__value">{{ formatBytes(rxBytes) }}</span>
      </div>
      <div class="info-row">
        <span class="info-row__label">↑ TX</span>
        <span class="info-row__value">{{ formatBytes(txBytes) }}</span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.connection-card {
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-lg);
  padding: var(--space-4);
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}

.connection-card__header {
  display: flex;
  align-items: center;
  gap: var(--space-2);
}

.protocol-badge {
  padding: 2px 8px;
  border-radius: var(--radius-full);
  font-size: var(--text-xs);
  font-weight: var(--font-bold);
  text-transform: uppercase;
}

.badge--openvpn { background: rgba(34, 197, 94, 0.15); color: #22c55e; }
.badge--wireguard { background: rgba(168, 85, 247, 0.15); color: #a855f7; }
.badge--l2tp { background: rgba(59, 130, 246, 0.15); color: #3b82f6; }
.badge--ikev2 { background: rgba(245, 158, 11, 0.15); color: #f59e0b; }
.badge--ssh { background: rgba(107, 114, 128, 0.15); color: #6b7280; }
.badge--default { background: rgba(107, 114, 128, 0.15); color: #6b7280; }

.node-name {
  font-size: var(--text-sm);
  font-weight: var(--font-medium);
  color: var(--color-text);
}

.connection-card__body {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: var(--space-2);
}

.info-row {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.info-row__label {
  font-size: var(--text-xs);
  color: var(--color-muted);
}

.info-row__value {
  font-size: var(--text-sm);
  font-weight: var(--font-medium);
  color: var(--color-text);
}

@media (max-width: 480px) {
  .connection-card__body {
    grid-template-columns: 1fr;
  }
}
</style>
