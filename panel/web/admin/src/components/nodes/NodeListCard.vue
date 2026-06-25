<script setup lang="ts">
import { computed } from 'vue'
import type { KnodeNode } from '@/stores/nodes'
import KStatusPill from '@koris/ui/KStatusPill.vue'

const props = defineProps<{
  node: KnodeNode
}>()

const emit = defineEmits<{
  (e: 'select', nodeId: number): void
}>()

const addressDisplay = computed(() => `${props.node.address}:${props.node.port}`)

const lastSeenDisplay = computed(() => {
  if (!props.node.lastSeenAt) return 'Never'
  const date = new Date(props.node.lastSeenAt)
  const now = new Date()
  const diffMs = now.getTime() - date.getTime()
  const diffSec = Math.floor(diffMs / 1000)

  if (diffSec < 60) return 'Just now'
  if (diffSec < 3600) return `${Math.floor(diffSec / 60)}m ago`
  if (diffSec < 86400) return `${Math.floor(diffSec / 3600)}h ago`
  return `${Math.floor(diffSec / 86400)}d ago`
})
</script>

<template>
  <div
    class="node-list-card"
    role="button"
    tabindex="0"
    :aria-label="`Node ${node.name}, status ${node.status}`"
    @click="emit('select', node.id)"
    @keydown.enter="emit('select', node.id)"
    @keydown.space.prevent="emit('select', node.id)"
  >
    <div class="node-list-card__main">
      <span class="node-list-card__name">{{ node.name || 'Unnamed' }}</span>
      <span class="node-list-card__address">{{ addressDisplay }}</span>
    </div>

    <div class="node-list-card__meta">
      <KStatusPill :status="node.status" size="sm" />
      <span class="node-list-card__last-seen" :title="node.lastSeenAt">
        {{ lastSeenDisplay }}
      </span>
    </div>
  </div>
</template>

<style scoped>
.node-list-card {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-3);
  padding: var(--space-3) var(--space-4);
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  cursor: pointer;
  transition:
    background var(--duration-fast) var(--ease-default),
    border-color var(--duration-fast) var(--ease-default);
}

.node-list-card:hover {
  background: var(--color-surface-2);
  border-color: var(--color-muted);
}

.node-list-card:focus-visible {
  outline: 2px solid var(--color-accent);
  outline-offset: 2px;
}

.node-list-card__main {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.node-list-card__name {
  font-size: var(--text-base);
  font-weight: var(--font-medium);
  color: var(--color-text);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.node-list-card__address {
  font-size: var(--text-sm);
  color: var(--color-muted);
  font-family: monospace;
}

.node-list-card__meta {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  flex-shrink: 0;
}

.node-list-card__last-seen {
  font-size: var(--text-xs);
  color: var(--color-muted);
  white-space: nowrap;
}
</style>
