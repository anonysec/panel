<script setup lang="ts">
/**
 * UserEditModal — Modal dialog for editing user (customer) details.
 *
 * Uses KModal centered with blurred overlay.
 * Fields:
 *   - Username (read-only) + Status dropdown (side by side)
 *   - Data Limit (numeric + unit selector)
 *   - Periodic Usage Reset toggle
 *   - Expiry Date + KExpiryChips + calendar picker
 *   - HWID Limit (numeric)
 *   - Note (text)
 * Bottom bar: ThreeDotMenu (left), Cancel + Modify (right)
 *
 * Submits via PATCH /api/customers/:id
 * Shows inline error on failure, preserves form values for retry.
 * Fade-in 200ms, fade-out 150ms (handled by KModal).
 *
 * Requirements: 7.1, 7.2, 7.3, 7.4, 7.5, 7.6, 7.7
 */
import { ref, watch, computed } from 'vue'
import { useApi } from '@koris/composables/useApi'
import { useI18n } from '@koris/composables/useI18n'
import { usePlansStore } from '@/stores/plans'
import KModal from '@koris/ui/KModal.vue'
import KFormField from '@koris/ui/KFormField.vue'
import KInput from '@koris/ui/KInput.vue'
import KSelect from '@koris/ui/KSelect.vue'
import KButton from '@koris/ui/KButton.vue'
import KExpiryChips from '@koris/ui/KExpiryChips.vue'
import KThreeDotMenu from '@koris/ui/KThreeDotMenu.vue'
import type { MenuItem } from '@koris/ui/KThreeDotMenu.vue'
import type { CustomerDetail } from '@koris/types'

export interface UserEditModalProps {
  open: boolean
  userId: number | null
}

const props = defineProps<UserEditModalProps>()

const emit = defineEmits<{
  close: []
  saved: []
}>()

const { t } = useI18n()
const { get, patch, loading: apiLoading } = useApi({ showErrorToast: false })
const plansStore = usePlansStore()

// ─── Form State ─────────────────────────────────────────────────────────────
const username = ref('')
const status = ref<string>('active')
const planId = ref<string>('')
const dataLimitValue = ref<string>('')
const dataLimitUnit = ref<string>('GB')
const periodicUsageReset = ref(false)
const expiryDate = ref<string>('')
const hwidLimit = ref<string>('')
const note = ref('')

// ─── UI State ───────────────────────────────────────────────────────────────
const fetchLoading = ref(false)
const submitting = ref(false)
const inlineError = ref<string | null>(null)
const expiryMode = ref<'date' | 'days'>('date')

// ─── Status Options ─────────────────────────────────────────────────────────
const statusOptions = [
  { label: 'Active', value: 'active' },
  { label: 'Disabled', value: 'disabled' },
  { label: 'Expired', value: 'expired' },
  { label: 'Limited', value: 'limited' },
]

// ─── Data Limit Unit Options ────────────────────────────────────────────────
const unitOptions = [
  { label: 'MB', value: 'MB' },
  { label: 'GB', value: 'GB' },
  { label: 'TB', value: 'TB' },
]

// ─── Plan Options ───────────────────────────────────────────────────────────
const editPlanOptions = computed(() => {
  return plansStore.activePlans.map((p) => ({
    value: String(p.id),
    label: `${p.name} (${p.data_gb}GB / ${p.duration_days}d)`,
  }))
})

// ─── Three-dot menu items ───────────────────────────────────────────────────
const menuItems: MenuItem[] = [
  { key: 'reset-traffic', label: 'Reset Traffic', icon: '🔄' },
  { key: 'reset-usage', label: 'Reset Usage', icon: '📊' },
  { key: 'copy-sub-link', label: 'Copy Subscription Link', icon: '🔗' },
  { key: 'qr-code', label: 'Generate QR Code', icon: '📱' },
]

// ─── Computed: expiry date as YYYY-MM-DD for native date input ──────────────
const expiryDateFormatted = computed(() => {
  if (!expiryDate.value) return ''
  try {
    const date = new Date(expiryDate.value)
    if (isNaN(date.getTime())) return ''
    return date.toISOString().split('T')[0]
  } catch {
    return ''
  }
})

// ─── Computed: "Expires in X days" ──────────────────────────────────────────
const expiresInDays = computed<number | null>(() => {
  if (!expiryDate.value) return null
  try {
    const expiry = new Date(expiryDate.value)
    if (isNaN(expiry.getTime())) return null
    const now = new Date()
    const expiryDay = new Date(expiry.getFullYear(), expiry.getMonth(), expiry.getDate())
    const today = new Date(now.getFullYear(), now.getMonth(), now.getDate())
    const diffMs = expiryDay.getTime() - today.getTime()
    return Math.round(diffMs / (1000 * 60 * 60 * 24))
  } catch {
    return null
  }
})

// ─── Fetch user data when modal opens ───────────────────────────────────────

interface CustomerDetailResponse {
  ok: boolean
  customer: CustomerDetail
}

async function fetchUserData(userId: number): Promise<void> {
  fetchLoading.value = true
  inlineError.value = null

  try {
    const res = await get<CustomerDetailResponse>(`/api/customers/${userId}`)
    const c = res.customer

    username.value = c.username
    status.value = c.status
    planId.value = c.plan_id ? String(c.plan_id) : ''

    // Extract data limit from subscription or radius attributes
    if (c.subscription?.data_limit_gb) {
      const limitGb = c.subscription.data_limit_gb
      if (limitGb >= 1024) {
        dataLimitValue.value = String(Math.round(limitGb / 1024))
        dataLimitUnit.value = 'TB'
      } else if (limitGb < 1) {
        dataLimitValue.value = String(Math.round(limitGb * 1024))
        dataLimitUnit.value = 'MB'
      } else {
        dataLimitValue.value = String(limitGb)
        dataLimitUnit.value = 'GB'
      }
    } else {
      dataLimitValue.value = ''
      dataLimitUnit.value = 'GB'
    }

    // Periodic usage reset — check radius attribute
    const resetCheck = c.radius_checks?.find(
      (r) => r.attribute === 'Periodic-Usage-Reset'
    )
    periodicUsageReset.value = resetCheck?.value === '1' || resetCheck?.value === 'true'

    // Expiry date from radius checks
    const expirationCheck = c.radius_checks?.find(
      (r) => r.attribute === 'Expiration'
    )
    expiryDate.value = expirationCheck?.value ?? ''

    // HWID limit from radius checks
    const hwidCheck = c.radius_checks?.find(
      (r) => r.attribute === 'HWID-Limit'
    )
    hwidLimit.value = hwidCheck?.value ?? ''

    // Note
    note.value = c.notes ?? ''
  } catch (e: any) {
    inlineError.value = e?.message || 'Failed to load user data'
  } finally {
    fetchLoading.value = false
  }
}

// ─── Watch open/userId to fetch data ────────────────────────────────────────
watch(
  () => [props.open, props.userId] as const,
  ([isOpen, userId]) => {
    if (isOpen && userId) {
      inlineError.value = null
      fetchUserData(userId)
    }
  },
  { immediate: true }
)

// ─── Form Submission ────────────────────────────────────────────────────────

async function handleSubmit(): Promise<void> {
  if (!props.userId) return

  submitting.value = true
  inlineError.value = null

  // Convert data limit to GB for the API
  let dataLimitGb: number | null = null
  if (dataLimitValue.value) {
    const numVal = Number(dataLimitValue.value)
    if (!isNaN(numVal) && numVal > 0) {
      switch (dataLimitUnit.value) {
        case 'MB':
          dataLimitGb = numVal / 1024
          break
        case 'GB':
          dataLimitGb = numVal
          break
        case 'TB':
          dataLimitGb = numVal * 1024
          break
      }
    }
  }

  const payload: Record<string, unknown> = {
    status: status.value,
    note: note.value,
    expiry_date: expiryDate.value || null,
    periodic_usage_reset: periodicUsageReset.value,
    hwid_limit: hwidLimit.value ? Number(hwidLimit.value) : 0,
    plan_id: planId.value ? Number(planId.value) : undefined,
  }

  if (dataLimitGb !== null) {
    payload.data_limit_gb = dataLimitGb
  }

  try {
    await patch(`/api/customers/${props.userId}`, payload)
    emit('saved')
    emit('close')
  } catch (e: any) {
    inlineError.value = e?.message || 'Failed to save changes. Please try again.'
  } finally {
    submitting.value = false
  }
}

// ─── Handlers ───────────────────────────────────────────────────────────────

function handleClose(): void {
  emit('close')
}

function handleCancel(): void {
  emit('close')
}

function onDateInput(event: Event): void {
  const target = event.target as HTMLInputElement
  if (target.value) {
    const date = new Date(target.value + 'T00:00:00')
    expiryDate.value = date.toISOString()
  } else {
    expiryDate.value = ''
  }
}

function onExpiryChipUpdate(value: string): void {
  expiryDate.value = value
}

function handleMenuSelect(key: string): void {
  // Menu actions are handled by the parent — close modal and let the parent handle
  // For now, these are placeholder actions within the edit modal context
  // The parent component listens for 'saved' or 'close' and handles the three-dot actions
}
</script>

<template>
  <KModal
    :open="open"
    title="Edit User"
    width="560px"
    @close="handleClose"
  >
    <!-- Loading state -->
    <div v-if="fetchLoading" class="user-edit-modal__loading">
      <div class="user-edit-modal__skeleton" />
      <div class="user-edit-modal__skeleton user-edit-modal__skeleton--short" />
      <div class="user-edit-modal__skeleton user-edit-modal__skeleton--medium" />
    </div>

    <!-- Form -->
    <form v-else class="user-edit-modal__form" @submit.prevent="handleSubmit">
      <!-- Inline error -->
      <div v-if="inlineError" class="user-edit-modal__error" role="alert">
        <p class="user-edit-modal__error-text">{{ inlineError }}</p>
      </div>

      <!-- Row 1: Username (read-only) + Status (side by side) -->
      <div class="user-edit-modal__row">
        <KFormField name="username" label="Username" class="user-edit-modal__field">
          <template #default="{ fieldId, describedBy }">
            <KInput
              :id="fieldId"
              :model-value="username"
              :aria-describedby="describedBy"
              disabled
            />
          </template>
        </KFormField>

        <KFormField name="status" label="Status" class="user-edit-modal__field">
          <template #default="{ fieldId, describedBy }">
            <KSelect
              :id="fieldId"
              :model-value="status"
              :options="statusOptions"
              :aria-describedby="describedBy"
              @update:model-value="status = String($event)"
            />
          </template>
        </KFormField>
      </div>

      <!-- Plan -->
      <KFormField name="plan" label="Plan">
        <template #default="{ fieldId }">
          <KSelect
            :id="fieldId"
            :model-value="planId"
            :options="editPlanOptions"
            placeholder="Select plan"
            @update:model-value="planId = String($event)"
          />
        </template>
      </KFormField>

      <!-- Data Limit (numeric + unit) -->
      <KFormField name="data-limit" label="Data Limit">
        <template #default="{ fieldId }">
          <div class="user-edit-modal__data-limit">
            <KInput
              :id="fieldId"
              :model-value="dataLimitValue"
              type="number"
              placeholder="Unlimited"
              @update:model-value="dataLimitValue = String($event)"
            />
            <KSelect
              :model-value="dataLimitUnit"
              :options="unitOptions"
              class="user-edit-modal__unit-select"
              @update:model-value="dataLimitUnit = String($event)"
            />
          </div>
        </template>
      </KFormField>

      <!-- Periodic Usage Reset toggle -->
      <div class="user-edit-modal__toggle-field">
        <label class="user-edit-modal__toggle-label" for="periodic-reset">
          <input
            id="periodic-reset"
            v-model="periodicUsageReset"
            type="checkbox"
            class="user-edit-modal__toggle-checkbox"
          />
          <span class="user-edit-modal__toggle-text">Periodic Usage Reset</span>
        </label>
        <p class="user-edit-modal__toggle-hint">Automatically reset usage counter each billing period</p>
      </div>

      <!-- Expiry Date -->
      <KFormField name="expiry" label="Expiry Date">
        <template #default="{ fieldId }">
          <div class="user-edit-modal__expiry">
            <!-- Input mode toggle -->
            <div class="user-edit-modal__expiry-toggle" role="group" aria-label="Expiry input mode">
              <button
                type="button"
                :class="['user-edit-modal__toggle-btn', { 'user-edit-modal__toggle-btn--active': expiryMode === 'date' }]"
                :aria-pressed="expiryMode === 'date'"
                @click="expiryMode = 'date'"
              >
                Date
              </button>
              <button
                type="button"
                :class="['user-edit-modal__toggle-btn', { 'user-edit-modal__toggle-btn--active': expiryMode === 'days' }]"
                :aria-pressed="expiryMode === 'days'"
                @click="expiryMode = 'days'"
              >
                Duration
              </button>
            </div>

            <!-- Calendar picker -->
            <div v-if="expiryMode === 'date'" class="user-edit-modal__expiry-date">
              <input
                :id="fieldId"
                type="date"
                class="user-edit-modal__date-input"
                :value="expiryDateFormatted"
                @input="onDateInput"
              />
            </div>

            <!-- Shortcut chips -->
            <div v-else class="user-edit-modal__expiry-chips">
              <KExpiryChips
                :model-value="expiryDate"
                @update:model-value="onExpiryChipUpdate"
              />
            </div>

            <!-- "Expires in X days" info -->
            <p v-if="expiresInDays !== null" class="user-edit-modal__expiry-info">
              <span v-if="expiresInDays > 0">
                Expires in {{ expiresInDays }} day{{ expiresInDays === 1 ? '' : 's' }}
              </span>
              <span v-else-if="expiresInDays === 0" class="user-edit-modal__expiry-info--warning">
                Expires today
              </span>
              <span v-else class="user-edit-modal__expiry-info--expired">
                Expired {{ Math.abs(expiresInDays) }} day{{ Math.abs(expiresInDays) === 1 ? '' : 's' }} ago
              </span>
            </p>
          </div>
        </template>
      </KFormField>

      <!-- HWID Limit -->
      <KFormField name="hwid-limit" label="HWID Limit">
        <template #default="{ fieldId, describedBy }">
          <KInput
            :id="fieldId"
            :model-value="hwidLimit"
            type="number"
            placeholder="0 (unlimited)"
            :aria-describedby="describedBy"
            @update:model-value="hwidLimit = String($event)"
          />
        </template>
      </KFormField>

      <!-- Note -->
      <KFormField name="note" label="Note">
        <template #default="{ fieldId, describedBy }">
          <textarea
            :id="fieldId"
            v-model="note"
            :aria-describedby="describedBy"
            class="user-edit-modal__textarea"
            placeholder="Add a note about this user..."
            rows="3"
          />
        </template>
      </KFormField>
    </form>

    <!-- Footer: ThreeDotMenu (left), Cancel + Modify (right) -->
    <template #footer>
      <div class="user-edit-modal__footer">
        <div class="user-edit-modal__footer-left">
          <KThreeDotMenu
            :items="menuItems"
            placement="bottom-start"
            @select="handleMenuSelect"
          />
        </div>
        <div class="user-edit-modal__footer-right">
          <KButton variant="ghost" @click="handleCancel">
            Cancel
          </KButton>
          <KButton
            variant="primary"
            :loading="submitting"
            :disabled="fetchLoading"
            @click="handleSubmit"
          >
            Modify
          </KButton>
        </div>
      </div>
    </template>
  </KModal>
</template>

<style scoped>
/* ─── Loading State ───────────────────────────────────────────────────────── */
.user-edit-modal__loading {
  display: flex;
  flex-direction: column;
  gap: var(--space-4, 16px);
  padding: var(--space-2, 8px) 0;
}

.user-edit-modal__skeleton {
  height: 20px;
  width: 100%;
  border-radius: var(--radius-sm, 6px);
  background: linear-gradient(
    90deg,
    var(--color-surface-2, #1e2630) 25%,
    var(--color-surface-3, #2a3544) 50%,
    var(--color-surface-2, #1e2630) 75%
  );
  background-size: 200% 100%;
  animation: shimmer 1.5s infinite;
}

.user-edit-modal__skeleton--short {
  width: 60%;
}

.user-edit-modal__skeleton--medium {
  width: 80%;
}

@keyframes shimmer {
  0% { background-position: 200% 0; }
  100% { background-position: -200% 0; }
}

/* ─── Form ────────────────────────────────────────────────────────────────── */
.user-edit-modal__form {
  display: flex;
  flex-direction: column;
  gap: var(--space-4, 16px);
}

/* ─── Inline Error ────────────────────────────────────────────────────────── */
.user-edit-modal__error {
  padding: var(--space-3, 12px);
  border-radius: var(--radius-md, 8px);
  background: rgba(239, 68, 68, 0.1);
  border: 1px solid rgba(239, 68, 68, 0.3);
}

.user-edit-modal__error-text {
  margin: 0;
  font-size: var(--text-sm, 13px);
  color: var(--color-danger, #ef4444);
}

/* ─── Side-by-side Row ────────────────────────────────────────────────────── */
.user-edit-modal__row {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: var(--space-3, 12px);
}

.user-edit-modal__field {
  min-width: 0;
}

/* ─── Data Limit ──────────────────────────────────────────────────────────── */
.user-edit-modal__data-limit {
  display: grid;
  grid-template-columns: 1fr auto;
  gap: var(--space-2, 8px);
}

.user-edit-modal__unit-select {
  width: 80px;
}

/* ─── Toggle Field ────────────────────────────────────────────────────────── */
.user-edit-modal__toggle-field {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.user-edit-modal__toggle-label {
  display: inline-flex;
  align-items: center;
  gap: var(--space-2, 8px);
  cursor: pointer;
}

.user-edit-modal__toggle-checkbox {
  width: 16px;
  height: 16px;
  accent-color: var(--color-primary, #2563eb);
  cursor: pointer;
}

.user-edit-modal__toggle-text {
  font-size: var(--text-sm, 13px);
  font-weight: var(--font-medium, 500);
  color: var(--color-text, #e6edf3);
}

.user-edit-modal__toggle-hint {
  margin: 0;
  padding-left: 24px;
  font-size: 12px;
  color: var(--color-muted, #8b98a5);
}

/* ─── Expiry Section ──────────────────────────────────────────────────────── */
.user-edit-modal__expiry {
  display: flex;
  flex-direction: column;
  gap: var(--space-2, 8px);
}

.user-edit-modal__expiry-toggle {
  display: inline-flex;
  border-radius: var(--radius-md, 6px);
  border: 1px solid var(--color-border, #28333f);
  overflow: hidden;
}

.user-edit-modal__toggle-btn {
  padding: var(--space-1, 4px) var(--space-3, 12px);
  border: none;
  background: var(--color-surface-2, #1e2630);
  color: var(--color-text, #e6edf3);
  font-size: var(--text-sm, 13px);
  font-family: var(--font-family);
  cursor: pointer;
  transition: background var(--duration-fast, 100ms) ease-out,
    color var(--duration-fast, 100ms) ease-out;
}

.user-edit-modal__toggle-btn:not(:last-child) {
  border-right: 1px solid var(--color-border, #28333f);
}

.user-edit-modal__toggle-btn:hover:not(.user-edit-modal__toggle-btn--active) {
  background: var(--color-surface-3, #2a3544);
}

.user-edit-modal__toggle-btn--active {
  background: var(--color-primary, #2563eb);
  color: #fff;
}

.user-edit-modal__expiry-date,
.user-edit-modal__expiry-chips {
  margin-top: var(--space-1, 4px);
}

.user-edit-modal__date-input {
  display: block;
  width: 100%;
  height: 36px;
  padding: 0 var(--space-3, 12px);
  background: var(--color-surface, #0b1120);
  border: 1px solid var(--color-border, #28333f);
  border-radius: var(--radius-md, 6px);
  color: var(--color-text, #e6edf3);
  font-family: var(--font-family);
  font-size: var(--text-base, 14px);
  line-height: var(--leading-normal);
  outline: none;
  transition: border-color var(--duration-normal, 150ms) ease,
    box-shadow var(--duration-normal, 150ms) ease;
}

.user-edit-modal__date-input:focus-visible {
  border-color: var(--color-primary, #2563eb);
  box-shadow: 0 0 0 2px rgba(37, 99, 235, 0.25);
}

.user-edit-modal__expiry-info {
  margin: 0;
  font-size: var(--text-sm, 13px);
  color: var(--color-muted, #8b98a5);
}

.user-edit-modal__expiry-info--warning {
  color: var(--color-warning, #d97706);
  font-weight: var(--font-medium, 500);
}

.user-edit-modal__expiry-info--expired {
  color: var(--color-danger, #dc2626);
  font-weight: var(--font-medium, 500);
}

/* ─── Textarea ────────────────────────────────────────────────────────────── */
.user-edit-modal__textarea {
  display: block;
  width: 100%;
  padding: var(--space-2, 8px) var(--space-3, 12px);
  background: var(--color-surface, #0b1120);
  border: 1px solid var(--color-border, #28333f);
  border-radius: var(--radius-md, 6px);
  color: var(--color-text, #e6edf3);
  font-family: var(--font-family);
  font-size: var(--text-base, 14px);
  line-height: var(--leading-normal);
  outline: none;
  resize: vertical;
  transition: border-color var(--duration-normal, 150ms) ease,
    box-shadow var(--duration-normal, 150ms) ease;
}

.user-edit-modal__textarea::placeholder {
  color: var(--color-muted, #8b98a5);
}

.user-edit-modal__textarea:focus-visible {
  border-color: var(--color-primary, #2563eb);
  box-shadow: 0 0 0 2px rgba(37, 99, 235, 0.25);
}

/* ─── Footer ──────────────────────────────────────────────────────────────── */
.user-edit-modal__footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.user-edit-modal__footer-left {
  display: flex;
  align-items: center;
}

.user-edit-modal__footer-right {
  display: flex;
  align-items: center;
  gap: var(--space-2, 8px);
}

/* ─── Reduced motion ──────────────────────────────────────────────────────── */
@media (prefers-reduced-motion: reduce) {
  .user-edit-modal__skeleton {
    animation: none;
  }
}
</style>
