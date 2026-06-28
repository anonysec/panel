<template>
  <div class="detail-header">
    <div class="detail-header__top">
      <h2 class="detail-header__name">{{ displayName }}</h2>
      <KStatusPill :status="status" size="sm" />
    </div>

    <div class="detail-header__usage">
      <KUsageBar :used="usedBytes" :limit="limitBytes" size="sm" />
    </div>

    <div v-if="billingEnabled" class="detail-header__wallet">
      <span class="detail-header__balance">{{ formattedBalance }}</span>
      <div class="detail-header__wallet-actions">
        <KButton variant="ghost" size="sm" @click="$emit('top-up')">
          Top Up
        </KButton>
        <KButton variant="ghost" size="sm" @click="$emit('deduct')">
          Deduct
        </KButton>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import KStatusPill from '@koris/ui/KStatusPill.vue'
import KUsageBar from '@koris/ui/KUsageBar.vue'
import KButton from '@koris/ui/KButton.vue'
import { formatCurrency } from '@/utils/formatCurrency'

export interface DetailHeaderProps {
  displayName: string
  status: string
  usedBytes: number
  limitBytes: number
  walletBalance: number
  currencySymbol?: string
  billingEnabled?: boolean
}

const props = withDefaults(defineProps<DetailHeaderProps>(), {
  currencySymbol: '$',
  billingEnabled: true,
})

defineEmits<{
  'top-up': []
  'deduct': []
}>()

const formattedBalance = computed(() =>
  formatCurrency(props.walletBalance, props.currencySymbol)
)
</script>

<style scoped>
.detail-header {
  display: flex;
  flex-direction: column;
  gap: var(--space-3, 12px);
  padding: var(--space-4, 16px);
  border-bottom: 1px solid var(--color-border, #e5e7eb);
}

.detail-header__top {
  display: flex;
  align-items: center;
  gap: var(--space-2, 8px);
}

.detail-header__name {
  margin: 0;
  font-size: var(--text-lg, 1.125rem);
  font-weight: var(--font-semibold, 600);
  color: var(--color-text, #1f2937);
  line-height: 1.3;
}

.detail-header__usage {
  width: 100%;
}

.detail-header__wallet {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-2, 8px);
}

.detail-header__balance {
  font-size: var(--text-base, 1rem);
  font-weight: var(--font-semibold, 600);
  color: var(--color-text, #1f2937);
  font-family: var(--font-family);
}

.detail-header__wallet-actions {
  display: flex;
  gap: var(--space-1, 4px);
}
</style>
