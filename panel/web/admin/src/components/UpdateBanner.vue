<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { useApi } from '@koris/composables/useApi'
import { useConfirm } from '@koris/composables/useConfirm'
import { useI18n } from '@koris/composables/useI18n'

interface UpdateInfo {
  available: boolean
  current_version: string
  latest_version: string
  changelog?: string
}

interface UpdateCheckResponse {
  ok: boolean
  update: UpdateInfo
}

interface UpdateApplyResponse {
  ok: boolean
  message?: string
}

const { get, post } = useApi({ showErrorToast: false })
const { confirm } = useConfirm()
const { t } = useI18n()

const SESSION_KEY = 'koris-update-banner-dismissed'
const CHECK_INTERVAL_MS = 6 * 60 * 60 * 1000 // 6 hours

const visible = ref(false)
const currentVersion = ref('')
const latestVersion = ref('')
const applying = ref(false)
const applySuccess = ref(false)
const applyError = ref('')

let checkTimer: ReturnType<typeof setInterval> | null = null

function isDismissed(): boolean {
  try {
    return sessionStorage.getItem(SESSION_KEY) === 'true'
  } catch {
    return false
  }
}

function dismiss() {
  visible.value = false
  try {
    sessionStorage.setItem(SESSION_KEY, 'true')
  } catch {
    // sessionStorage unavailable
  }
}

async function checkForUpdate() {
  if (isDismissed()) return

  try {
    const res = await get<UpdateCheckResponse>('/api/admin/update/check')
    if (res.ok && res.update?.available) {
      currentVersion.value = res.update.current_version
      latestVersion.value = res.update.latest_version
      visible.value = true
    } else {
      visible.value = false
    }
  } catch {
    // Silently fail — don't show banner on error
  }
}

async function handleUpdateNow() {
  const confirmed = await confirm({
    title: t('update.confirm_title') || 'Apply Update',
    message: t('update.confirm_message') || 'This will download and install the update. The panel will restart. Continue?',
    variant: 'info',
    confirmText: t('update.confirm_btn') || 'Update',
    cancelText: t('btn.cancel') || 'Cancel',
  })

  if (!confirmed) return

  applying.value = true
  applyError.value = ''

  try {
    const res = await post<UpdateApplyResponse>('/api/admin/update/apply')
    if (res.ok) {
      applySuccess.value = true
    } else {
      applyError.value = t('update.apply_error') || 'Update failed. Please try again.'
      applying.value = false
    }
  } catch {
    applyError.value = t('update.apply_error') || 'Update failed. Please try again.'
    applying.value = false
  }
}

onMounted(() => {
  checkForUpdate()
  checkTimer = setInterval(checkForUpdate, CHECK_INTERVAL_MS)
})

onUnmounted(() => {
  if (checkTimer) {
    clearInterval(checkTimer)
    checkTimer = null
  }
})
</script>

<template>
  <Transition name="banner-slide">
    <div
      v-if="visible"
      class="update-banner"
      role="alert"
      aria-live="polite"
      aria-atomic="true"
    >
      <!-- Success state -->
      <template v-if="applySuccess">
        <span class="update-banner__icon" aria-hidden="true">✓</span>
        <span class="update-banner__text">
          {{ t('update.success') || 'Update applied! Panel is restarting...' }}
        </span>
      </template>

      <!-- Applying state -->
      <template v-else-if="applying">
        <span class="update-banner__spinner" aria-hidden="true">
          <svg viewBox="0 0 24 24" fill="none">
            <circle
              cx="12" cy="12" r="10"
              stroke="currentColor"
              stroke-width="3"
              stroke-linecap="round"
              stroke-dasharray="50 20"
            />
          </svg>
        </span>
        <span class="update-banner__text">
          {{ t('update.applying') || 'Applying update...' }}
        </span>
      </template>

      <!-- Default state -->
      <template v-else>
        <span class="update-banner__icon" aria-hidden="true">⬆</span>
        <span class="update-banner__text">
          {{ t('update.available') || 'Update available' }}:
          v{{ currentVersion }} → v{{ latestVersion }}
        </span>

        <span v-if="applyError" class="update-banner__error" role="alert">
          {{ applyError }}
        </span>

        <div class="update-banner__actions">
          <button
            class="update-banner__btn update-banner__btn--primary"
            type="button"
            @click="handleUpdateNow"
          >
            {{ t('update.update_now') || 'Update Now' }}
          </button>
          <button
            class="update-banner__btn update-banner__btn--dismiss"
            type="button"
            :aria-label="t('update.dismiss') || 'Dismiss update notification'"
            @click="dismiss"
          >
            {{ t('update.dismiss_label') || 'Dismiss' }}
          </button>
        </div>
      </template>
    </div>
  </Transition>
</template>

<style scoped>
.update-banner {
  display: flex;
  align-items: center;
  gap: var(--space-3, 12px);
  padding: var(--space-2, 8px) var(--space-4, 16px);
  background: var(--color-surface, #0b1120);
  border: 1px solid var(--color-primary, #2563eb);
  border-radius: var(--radius-md, 8px);
  margin-bottom: var(--space-4, 16px);
  font-family: var(--font-family);
  font-size: var(--text-sm, 13px);
  color: var(--color-text, #e6edf3);
  flex-wrap: wrap;
}

.update-banner__icon {
  font-size: 16px;
  flex-shrink: 0;
  color: var(--color-primary, #2563eb);
}

.update-banner__text {
  flex: 1;
  min-width: 0;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.update-banner__error {
  color: var(--color-danger, #ef4444);
  font-size: var(--text-xs, 11px);
  flex-basis: 100%;
  margin-top: var(--space-1, 4px);
}

.update-banner__actions {
  display: flex;
  align-items: center;
  gap: var(--space-2, 8px);
  margin-left: auto;
  flex-shrink: 0;
}

.update-banner__btn {
  border: none;
  border-radius: var(--radius-sm, 4px);
  font-family: var(--font-family);
  font-size: var(--text-xs, 12px);
  font-weight: var(--font-medium, 500);
  cursor: pointer;
  padding: 5px 12px;
  line-height: 1;
  white-space: nowrap;
  transition:
    background var(--duration-fast, 0.12s) var(--ease-default, ease),
    box-shadow var(--duration-fast, 0.12s) var(--ease-default, ease);
}

.update-banner__btn:focus-visible {
  outline: 2px solid var(--color-accent, #2563eb);
  outline-offset: 2px;
}

.update-banner__btn--primary {
  background: var(--gradient-brand, linear-gradient(135deg, #2563eb, #3b82f6));
  color: #fff;
  box-shadow: var(--shadow-brand, 0 2px 8px rgba(37, 99, 235, 0.25));
}

.update-banner__btn--primary:hover {
  box-shadow: 0 4px 14px rgba(37, 99, 235, 0.35);
}

.update-banner__btn--dismiss {
  background: transparent;
  color: var(--color-muted, #8b98a5);
  border: 1px solid var(--color-border, #28333f);
}

.update-banner__btn--dismiss:hover {
  background: var(--color-surface-2, #1e2630);
  color: var(--color-text, #e6edf3);
}

/* Spinner */
.update-banner__spinner {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 16px;
  height: 16px;
  flex-shrink: 0;
  color: var(--color-primary, #2563eb);
}

.update-banner__spinner svg {
  width: 16px;
  height: 16px;
  animation: update-spin 0.75s linear infinite;
}

@keyframes update-spin {
  from { transform: rotate(0deg); }
  to { transform: rotate(360deg); }
}

/* Transition */
.banner-slide-enter-active,
.banner-slide-leave-active {
  transition:
    opacity var(--duration-slow, 0.3s) var(--ease-out, ease-out),
    transform var(--duration-slow, 0.3s) var(--ease-out, ease-out),
    max-height var(--duration-slow, 0.3s) var(--ease-out, ease-out);
  overflow: hidden;
}

.banner-slide-enter-from,
.banner-slide-leave-to {
  opacity: 0;
  transform: translateY(-8px);
  max-height: 0;
  margin-bottom: 0;
  padding-top: 0;
  padding-bottom: 0;
}

@media (prefers-reduced-motion: reduce) {
  .banner-slide-enter-active,
  .banner-slide-leave-active {
    transition: opacity var(--duration-fast, 0.12s) var(--ease-default, ease);
  }

  .banner-slide-enter-from,
  .banner-slide-leave-to {
    transform: none;
  }

  .update-banner__spinner svg {
    animation-duration: 2s;
  }
}

/* Mobile: stack text and actions */
@media (max-width: 640px) {
  .update-banner {
    flex-wrap: wrap;
    gap: var(--space-2, 8px);
  }

  .update-banner__text {
    flex-basis: calc(100% - 32px);
    white-space: normal;
  }

  .update-banner__actions {
    flex-basis: 100%;
    margin-left: 0;
    justify-content: flex-end;
  }
}
</style>
