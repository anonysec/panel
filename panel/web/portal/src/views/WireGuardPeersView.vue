<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useWireGuardPortal } from '@/composables/useWireGuardPortal'
import { useI18n } from '@koris/composables/useI18n'
import KButton from '@koris/ui/KButton.vue'
import KStatusPill from '@koris/ui/KStatusPill.vue'
import KSkeleton from '@koris/ui/KSkeleton.vue'
import KEmptyState from '@koris/ui/KEmptyState.vue'

const { t } = useI18n()
const { peers, loading, fetchMyPeers, downloadConfig, getQRCodeUrl } = useWireGuardPortal()

const qrModalOpen = ref(false)
const qrModalUrl = ref('')
const qrModalPeerName = ref('')

function openQRModal(peerId: number, nodeName: string) {
  qrModalUrl.value = getQRCodeUrl(peerId)
  qrModalPeerName.value = nodeName
  qrModalOpen.value = true
}

function closeQRModal() {
  qrModalOpen.value = false
  qrModalUrl.value = ''
  qrModalPeerName.value = ''
}

function statusVariant(status: string): string {
  switch (status) {
    case 'active': return 'active'
    case 'revoked': return 'disabled'
    default: return 'expired'
  }
}

onMounted(() => {
  fetchMyPeers()
})
</script>

<template>
  <div class="wg-peers">
    <h1 class="wg-peers__title">{{ t('portal.wireguard.title') }}</h1>
    <p class="wg-peers__subtitle">{{ t('portal.wireguard.subtitle') }}</p>

    <KSkeleton v-if="loading && !peers.length" type="card" :count="2" />

    <KEmptyState
      v-else-if="!peers.length"
      :title="t('portal.wireguard.noPeers')"
      :description="t('portal.wireguard.noPeersDesc')"
      icon="🔒"
    />

    <div v-else class="wg-peers__list">
      <div v-for="peer in peers" :key="peer.id" class="wg-peers__card">
        <div class="wg-peers__card-info">
          <div class="wg-peers__card-header">
            <span class="wg-peers__node-name">{{ peer.node_name }}</span>
            <KStatusPill :status="statusVariant(peer.status)">
              {{ t(`portal.wireguard.status_${peer.status}`) }}
            </KStatusPill>
          </div>
          <div class="wg-peers__card-meta">
            <span class="wg-peers__ip">{{ peer.allowed_ips }}</span>
          </div>
        </div>
        <div class="wg-peers__card-actions">
          <KButton variant="primary" size="sm" @click="downloadConfig(peer.id)">
            {{ t('portal.wireguard.download') }}
          </KButton>
          <KButton variant="ghost" size="sm" @click="openQRModal(peer.id, peer.node_name)">
            {{ t('portal.wireguard.qrCode') }}
          </KButton>
        </div>
      </div>
    </div>

    <!-- QR Code Modal -->
    <Teleport to="body">
      <Transition name="wg-modal">
        <div v-if="qrModalOpen" class="wg-modal-overlay" @click.self="closeQRModal">
          <div class="wg-modal" role="dialog" aria-modal="true" :aria-label="t('portal.wireguard.qrTitle')">
            <div class="wg-modal__header">
              <h2 class="wg-modal__title">{{ t('portal.wireguard.qrTitle') }}</h2>
              <button class="wg-modal__close" aria-label="Close" @click="closeQRModal">
                <svg width="20" height="20" viewBox="0 0 20 20" fill="none">
                  <path d="M15 5L5 15M5 5l10 10" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
                </svg>
              </button>
            </div>
            <div class="wg-modal__body">
              <p class="wg-modal__peer-name">{{ qrModalPeerName }}</p>
              <img :src="qrModalUrl" :alt="t('portal.wireguard.qrAlt')" class="wg-modal__qr-img" />
              <p class="wg-modal__hint">{{ t('portal.wireguard.qrHint') }}</p>
            </div>
          </div>
        </div>
      </Transition>
    </Teleport>
  </div>
</template>

<style scoped>
.wg-peers {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}

.wg-peers__title {
  font-size: var(--text-xl);
  font-weight: 700;
}

.wg-peers__subtitle {
  color: var(--color-muted);
  font-size: var(--text-sm);
}

.wg-peers__list {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}

.wg-peers__card {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-4);
  padding: var(--space-4) var(--space-5);
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-lg);
}

.wg-peers__card-info {
  flex: 1;
  min-width: 0;
}

.wg-peers__card-header {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  margin-bottom: var(--space-1);
}

.wg-peers__node-name {
  font-size: var(--text-sm);
  font-weight: 600;
}

.wg-peers__card-meta {
  font-size: var(--text-xs);
  color: var(--color-muted);
}

.wg-peers__ip {
  font-family: monospace;
}

.wg-peers__card-actions {
  display: flex;
  gap: var(--space-2);
  flex-shrink: 0;
}

/* Modal */
.wg-modal-overlay {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.6);
  z-index: var(--z-modal, 200);
  display: flex;
  align-items: center;
  justify-content: center;
  padding: var(--space-4);
}

.wg-modal {
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-lg);
  width: 100%;
  max-width: 360px;
  box-shadow: var(--shadow-xl, 0 30px 80px rgba(0, 0, 0, 0.6));
}

.wg-modal__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: var(--space-4) var(--space-5);
  border-bottom: 1px solid var(--color-border);
}

.wg-modal__title {
  font-size: var(--text-md);
  font-weight: 600;
  margin: 0;
}

.wg-modal__close {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  border: none;
  border-radius: var(--radius-sm);
  background: transparent;
  color: var(--color-muted);
  cursor: pointer;
  transition: background 0.12s, color 0.12s;
}

.wg-modal__close:hover {
  background: var(--color-surface-2);
  color: var(--color-text);
}

.wg-modal__body {
  padding: var(--space-5);
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--space-3);
}

.wg-modal__peer-name {
  font-size: var(--text-sm);
  font-weight: 500;
  color: var(--color-muted);
}

.wg-modal__qr-img {
  width: 220px;
  height: 220px;
  border-radius: var(--radius-md);
  background: #fff;
  padding: 8px;
}

.wg-modal__hint {
  font-size: var(--text-xs);
  color: var(--color-muted);
  text-align: center;
}

/* Modal transitions */
.wg-modal-enter-active,
.wg-modal-leave-active {
  transition: opacity 0.2s ease;
}

.wg-modal-enter-from,
.wg-modal-leave-to {
  opacity: 0;
}

/* Mobile */
@media (max-width: 640px) {
  .wg-peers__card {
    flex-direction: column;
    align-items: stretch;
  }

  .wg-peers__card-actions {
    justify-content: flex-end;
  }
}
</style>
