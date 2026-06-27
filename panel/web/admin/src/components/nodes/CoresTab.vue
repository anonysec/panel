<script setup lang="ts">
import type { CoreInfo } from './types'
import CoreCard from './CoreCard.vue'

const props = defineProps<{
  nodeId: number
  cores: CoreInfo[]
}>()

const emit = defineEmits<{
  (e: 'enable', coreType: string, port: number): void
  (e: 'disable', coreType: string): void
  (e: 'restart', coreType: string): void
}>()
</script>

<template>
  <div class="cores-tab">
    <div v-if="cores.length === 0" class="cores-tab__empty" role="status">
      <p class="cores-tab__empty-text">No cores configured</p>
    </div>

    <div v-else class="cores-tab__grid">
      <CoreCard
        v-for="core in cores"
        :key="core.type"
        :core="core"
        :node-id="nodeId"
        @enable="(port) => emit('enable', core.type, port)"
        @disable="emit('disable', core.type)"
        @restart="emit('restart', core.type)"
      />
    </div>
  </div>
</template>

<style scoped>
.cores-tab {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}

.cores-tab__empty {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: var(--space-6);
  background: var(--color-surface);
  border: 1px dashed var(--color-border);
  border-radius: var(--radius-md);
}

.cores-tab__empty-text {
  margin: 0;
  font-size: var(--text-sm);
  color: var(--color-muted);
}

.cores-tab__grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
  gap: var(--space-3);
}
</style>
