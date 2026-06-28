<template>
  <div class="profile-fields">
    <!-- Row 1: Username + Status side by side -->
    <div class="profile-fields__row">
      <KFormField name="username" :label="t('customer.username')">
        <template #default="{ fieldId, describedBy }">
          <KInput
            :id="fieldId"
            :model-value="modelValue.username"
            :aria-describedby="describedBy"
            disabled
          />
        </template>
      </KFormField>

      <KFormField name="status" :label="t('customer.status')">
        <template #default="{ fieldId, describedBy }">
          <KSelect
            :id="fieldId"
            :model-value="modelValue.status"
            :options="statusOptions"
            :aria-describedby="describedBy"
            @update:model-value="updateField('status', $event)"
          />
        </template>
      </KFormField>
    </div>

    <!-- Row 2: Data Limit + Expiry Date side by side -->
    <div class="profile-fields__row">
      <!-- Data Limit (GB only, decimals supported) -->
      <KFormField name="data-limit" :label="t('customer.data_limit')">
        <template #default="{ fieldId }">
          <div class="profile-fields__data-limit">
            <KInput
              :id="fieldId"
              :model-value="modelValue.data_limit"
              type="number"
              step="0.1"
              min="0"
              placeholder="0 = unlimited"
              @update:model-value="updateField('data_limit', $event)"
            />
            <span class="profile-fields__data-limit-suffix">GB</span>
          </div>
        </template>
      </KFormField>

      <!-- Expiry Date -->
      <KFormField name="expiry" :label="t('customer.expiry_date')">
        <template #default="{ fieldId }">
          <div class="profile-fields__expiry">
            <!-- Date input with calendar icon -->
            <div class="profile-fields__date-wrapper">
              <input
                ref="dateInputRef"
                :id="fieldId"
                type="date"
                class="profile-fields__date-input"
                :value="expiryDateValue"
                @input="onDateInput"
              />
              <button
                type="button"
                class="profile-fields__calendar-icon"
                aria-label="Open calendar"
                @click="openDatePicker"
              >
                <svg width="16" height="16" viewBox="0 0 16 16" fill="none" aria-hidden="true">
                  <path d="M5 1v2M11 1v2M1.5 6h13M2.5 3h11a1 1 0 011 1v10a1 1 0 01-1 1h-11a1 1 0 01-1-1V4a1 1 0 011-1z" stroke="currentColor" stroke-width="1.2" stroke-linecap="round" stroke-linejoin="round"/>
                </svg>
              </button>
            </div>

            <!-- Quick-set chips (always visible, minimal style) -->
            <div class="profile-fields__quick-chips">
              <button
                v-for="chip in expiryChips"
                :key="chip"
                type="button"
                class="profile-fields__chip"
                @click="applyChip(chip)"
              >
                {{ chip }}
              </button>
            </div>

            <!-- "Expires in X days" display -->
            <p v-if="expiresInDays !== null" class="profile-fields__expiry-info">
              <span v-if="expiresInDays > 0">
                Expires in {{ expiresInDays }} day{{ expiresInDays === 1 ? '' : 's' }}
              </span>
              <span v-else-if="expiresInDays === 0" class="profile-fields__expiry-info--warning">
                Expires today
              </span>
              <span v-else class="profile-fields__expiry-info--expired">
                Expired {{ Math.abs(expiresInDays) }} day{{ Math.abs(expiresInDays) === 1 ? '' : 's' }} ago
              </span>
            </p>
          </div>
        </template>
      </KFormField>
    </div>

    <!-- Billing Toggle -->
    <div class="profile-fields__toggle-field">
      <label class="profile-fields__toggle-label">
        <input
          type="checkbox"
          class="profile-fields__toggle-checkbox"
          :checked="modelValue.billing_enabled"
          @change="updateField('billing_enabled', ($event.target as HTMLInputElement).checked)"
        />
        <span class="profile-fields__toggle-text">Enable Billing</span>
      </label>
    </div>

    <!-- Note -->
    <KFormField name="note" :label="t('customer.note')">
      <template #default="{ fieldId, describedBy }">
        <KTextarea
          :id="fieldId"
          :model-value="modelValue.note"
          :aria-describedby="describedBy"
          placeholder="Note..."
          :rows="2"
          @update:model-value="updateField('note', $event)"
        />
      </template>
    </KFormField>

    <!-- Proxy settings — styled cards (no checkboxes) -->
    <KFormField name="proxy-settings" :label="t('customer.proxy_settings')">
      <template #default>
        <div class="profile-fields__proxy-list">
          <button
            v-for="protocol in availableProtocols"
            :key="protocol.value"
            type="button"
            class="profile-fields__protocol-card"
            :class="{ 'profile-fields__protocol-card--active': isProtocolEnabled(protocol.value) }"
            @click="toggleProtocol(protocol.value)"
          >
            <span class="profile-fields__protocol-label">{{ protocol.label }}</span>
          </button>
        </div>
        <!-- Protocol sub-options (shown inline for enabled protocols that have options) -->
        <div
          v-for="protocol in enabledProtocolsWithOptions"
          :key="'opts-' + protocol.value"
          class="profile-fields__protocol-options"
        >
          <span class="profile-fields__protocol-options-label">{{ protocol.label }}:</span>
          <label
            v-for="opt in protocol.options"
            :key="opt.value"
            class="profile-fields__protocol-option"
          >
            <input
              type="radio"
              :name="`proto-opt-${protocol.value}`"
              :value="opt.value"
              :checked="getProtocolOption(protocol.value) === opt.value"
              @change="setProtocolOption(protocol.value, opt.value)"
            />
            <span>{{ opt.label }}</span>
          </label>
        </div>
      </template>
    </KFormField>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { useI18n } from '@koris/composables/useI18n'
import KFormField from '@koris/ui/KFormField.vue'
import KInput from '@koris/ui/KInput.vue'
import KSelect from '@koris/ui/KSelect.vue'
import KTextarea from '@koris/ui/KTextarea.vue'
import { computeExpiryDate, type ExpiryOffset } from '@/utils/computeExpiryDate'

const { t } = useI18n()

// ─── Date input ref for calendar icon click ─────────────────────────────────
const dateInputRef = ref<HTMLInputElement | null>(null)

function openDatePicker() {
  dateInputRef.value?.showPicker?.()
}

/**
 * Form data shape for user profile editing.
 */
export interface ProfileFormData {
  username: string
  status: string
  data_limit: string
  expiry_date: string
  note: string
  allowed_protocols: string[]
  protocol_options: Record<string, string>
  billing_enabled: boolean
}

export interface ProfileFieldsProps {
  modelValue: ProfileFormData
}

const props = defineProps<ProfileFieldsProps>()

const emit = defineEmits<{
  'update:modelValue': [value: ProfileFormData]
}>()

// ─── Status Options ─────────────────────────────────────────────────────────
const statusOptions = [
  { label: t('customer.status_active'), value: 'active' },
  { label: t('customer.status_disabled'), value: 'disabled' },
  { label: t('customer.status_expired'), value: 'expired' },
  { label: t('customer.status_limited'), value: 'limited' },
]

// ─── Expiry Chips ───────────────────────────────────────────────────────────
const expiryChips: ExpiryOffset[] = ['+7d', '+1m', '+2m', '+3m', '+6m', '+1y']

// ─── Available Protocols with sub-options ───────────────────────────────────
const availableProtocols = [
  {
    label: 'OpenVPN',
    value: 'openvpn',
    options: [
      { label: 'Password + Certificate', value: 'auth' },
      { label: 'Passwordless (cert only)', value: 'noauth' },
    ],
  },
  { label: 'WireGuard', value: 'wireguard', options: null },
  { label: 'IKEv2', value: 'ikev2', options: null },
  {
    label: 'L2TP',
    value: 'l2tp',
    options: [
      { label: 'L2TP/IPsec', value: 'ipsec' },
      { label: 'L2TP (plain)', value: 'plain' },
    ],
  },
  { label: 'SSH Tunnel', value: 'ssh', options: null },
  { label: 'MTProto', value: 'mtproto', options: null },
]

// ─── Computed ───────────────────────────────────────────────────────────────

/** Protocols that are enabled and have sub-options to show */
const enabledProtocolsWithOptions = computed(() => {
  return availableProtocols.filter(
    (p) => p.options && isProtocolEnabled(p.value)
  )
})

const expiryDateValue = computed(() => {
  if (!props.modelValue.expiry_date) return ''
  try {
    const date = new Date(props.modelValue.expiry_date)
    if (isNaN(date.getTime())) return ''
    return date.toISOString().split('T')[0]
  } catch {
    return ''
  }
})

const expiresInDays = computed<number | null>(() => {
  if (!props.modelValue.expiry_date) return null
  try {
    const expiry = new Date(props.modelValue.expiry_date)
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

// ─── Methods ────────────────────────────────────────────────────────────────

function updateField(field: keyof ProfileFormData, value: any) {
  emit('update:modelValue', { ...props.modelValue, [field]: value })
}

function onDateInput(event: Event) {
  const target = event.target as HTMLInputElement
  if (target.value) {
    const date = new Date(target.value + 'T00:00:00')
    updateField('expiry_date', date.toISOString())
  } else {
    updateField('expiry_date', '')
  }
}

function applyChip(chip: ExpiryOffset) {
  const result = computeExpiryDate(new Date(), chip)
  updateField('expiry_date', result.toISOString())
}

function isProtocolEnabled(protocol: string): boolean {
  return props.modelValue.allowed_protocols.includes(protocol)
}

function toggleProtocol(protocol: string) {
  const current = [...props.modelValue.allowed_protocols]
  const index = current.indexOf(protocol)
  if (index >= 0) {
    current.splice(index, 1)
  } else {
    current.push(protocol)
  }
  updateField('allowed_protocols', current)
}

function getProtocolOption(protocol: string): string {
  return props.modelValue.protocol_options?.[protocol] ?? ''
}

function setProtocolOption(protocol: string, value: string) {
  const opts = { ...(props.modelValue.protocol_options || {}), [protocol]: value }
  updateField('protocol_options', opts)
}
</script>

<style scoped>
.profile-fields {
  display: flex;
  flex-direction: column;
  gap: var(--space-3, 12px);
}

/* ─── Side-by-side Row ────────────────────────────────────────────────────── */
.profile-fields__row {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: var(--space-3, 12px);
}

@media (max-width: 480px) {
  .profile-fields__row {
    grid-template-columns: 1fr;
  }
}

/* ─── Data Limit ──────────────────────────────────────────────────────────── */
.profile-fields__data-limit {
  display: flex;
  align-items: center;
  gap: var(--space-2, 8px);
}

.profile-fields__data-limit-suffix {
  font-size: var(--text-sm, 13px);
  font-weight: 500;
  color: var(--color-muted, #8b98a5);
  white-space: nowrap;
  padding-right: var(--space-1, 4px);
}

/* ─── Expiry Section ──────────────────────────────────────────────────────── */
.profile-fields__expiry {
  display: flex;
  flex-direction: column;
  gap: var(--space-2, 8px);
}

.profile-fields__date-wrapper {
  position: relative;
}

.profile-fields__date-input {
  display: block;
  width: 100%;
  height: 36px;
  padding: 0 var(--space-8, 32px) 0 var(--space-3, 12px);
  background: var(--color-surface, #0b1120);
  border: 1px solid var(--color-border, #28333f);
  border-radius: var(--radius-md, 6px);
  color: var(--color-text, #e6edf3);
  font-family: var(--font-family);
  font-size: var(--text-base, 14px);
  line-height: var(--leading-normal);
  outline: none;
  cursor: pointer;
  transition: border-color 150ms ease, box-shadow 150ms ease;
}

.profile-fields__date-input:focus-visible {
  border-color: var(--color-primary, #2563eb);
  box-shadow: 0 0 0 2px rgba(37, 99, 235, 0.25);
}

/* Hide native calendar icon in dark theme and provide our own */
.profile-fields__date-input::-webkit-calendar-picker-indicator {
  opacity: 0;
  position: absolute;
  right: 0;
  top: 0;
  width: 100%;
  height: 100%;
  cursor: pointer;
}

.profile-fields__calendar-icon {
  position: absolute;
  right: var(--space-2, 8px);
  top: 50%;
  transform: translateY(-50%);
  display: flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  padding: 0;
  border: none;
  border-radius: var(--radius-sm, 4px);
  background: transparent;
  color: var(--color-muted, #8b98a5);
  cursor: pointer;
  pointer-events: auto;
}

.profile-fields__calendar-icon:hover {
  color: var(--color-primary, #2563eb);
}

/* Quick-set chips — minimal, no bg/border */
.profile-fields__quick-chips {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-1, 4px);
}

.profile-fields__chip {
  padding: 2px 8px;
  border: none;
  background: transparent;
  color: var(--color-primary, #2563eb);
  font-size: var(--text-sm, 13px);
  font-family: var(--font-family);
  font-weight: 500;
  cursor: pointer;
  border-radius: var(--radius-sm, 4px);
  transition: background 100ms ease;
}

.profile-fields__chip:hover {
  background: rgba(37, 99, 235, 0.1);
}

.profile-fields__expiry-info {
  margin: 0;
  font-size: var(--text-xs, 12px);
  color: var(--color-muted, #6b7280);
}

.profile-fields__expiry-info--warning {
  color: var(--color-warning, #d97706);
  font-weight: 500;
}

.profile-fields__expiry-info--expired {
  color: var(--color-danger, #dc2626);
  font-weight: 500;
}

/* ─── Billing Toggle ─────────────────────────────────────────────────────── */
.profile-fields__toggle-field {
  display: flex;
  align-items: center;
}

.profile-fields__toggle-label {
  display: inline-flex;
  align-items: center;
  gap: var(--space-2, 8px);
  cursor: pointer;
}

.profile-fields__toggle-checkbox {
  width: 16px;
  height: 16px;
  accent-color: var(--color-primary, #2563eb);
  cursor: pointer;
  margin: 0;
}

.profile-fields__toggle-text {
  font-size: var(--text-sm, 13px);
  font-weight: 500;
  color: var(--color-text, #e6edf3);
}

/* ─── Proxy Settings — Styled Cards (no checkboxes) ───────────────────────── */
.profile-fields__proxy-list {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2, 8px);
}

.profile-fields__protocol-card {
  display: inline-flex;
  align-items: center;
  padding: 6px 12px;
  border: 1px solid var(--color-border, #28333f);
  border-radius: var(--radius-md, 6px);
  background: var(--color-surface, #0b1120);
  cursor: pointer;
  font-size: var(--text-sm, 13px);
  font-family: var(--font-family);
  color: var(--color-muted, #8b98a5);
  transition: border-color 100ms ease, background 100ms ease, color 100ms ease;
  user-select: none;
}

.profile-fields__protocol-card:hover {
  border-color: var(--color-primary, #2563eb);
  color: var(--color-text, #e6edf3);
}

.profile-fields__protocol-card--active {
  border-color: var(--color-primary, #2563eb);
  background: rgba(37, 99, 235, 0.08);
  color: var(--color-primary, #2563eb);
  font-weight: 500;
}

.profile-fields__protocol-label {
  pointer-events: none;
}

/* Protocol sub-options */
.profile-fields__protocol-options {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-2, 8px);
  padding-left: var(--space-2, 8px);
  margin-top: var(--space-1, 4px);
}

.profile-fields__protocol-options-label {
  font-size: 12px;
  color: var(--color-muted, #8b98a5);
  font-weight: 500;
}

.profile-fields__protocol-option {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  cursor: pointer;
  font-size: 12px;
  color: var(--color-muted, #8b98a5);
}

.profile-fields__protocol-option input[type="radio"] {
  width: 14px;
  height: 14px;
  accent-color: var(--color-primary, #2563eb);
  cursor: pointer;
  margin: 0;
}
</style>
