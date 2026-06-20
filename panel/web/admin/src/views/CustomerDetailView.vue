<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useCustomersStore } from '@/stores/customers'
import { useToast } from '@koris/composables/useToast'
import { useI18n } from '@koris/composables/useI18n'
import { useApi } from '@koris/composables/useApi'
import { formatDate, formatDateTime } from '@koris/composables/useFormatDate'
import KTabs from '@koris/ui/KTabs.vue'
import KFormField from '@koris/ui/KFormField.vue'
import KInput from '@koris/ui/KInput.vue'
import KSelect from '@koris/ui/KSelect.vue'
import KTextarea from '@koris/ui/KTextarea.vue'
import KButton from '@koris/ui/KButton.vue'
import KStatusPill from '@koris/ui/KStatusPill.vue'
import KAvatar from '@koris/ui/KAvatar.vue'
import KSkeleton from '@koris/ui/KSkeleton.vue'

const props = defineProps<{ id: string }>()

const { t } = useI18n()
const router = useRouter()
const store = useCustomersStore()
const toast = useToast()
const { get } = useApi()
const activeTab = ref('profile')
const saving = ref(false)

// ─── Traffic Reset State (Requirement 3.4) ───────────────────────────────────
const resettingTraffic = ref(false)

// ─── Connection Limit State (Requirement 4.3) ────────────────────────────────
const editingConnectionLimit = ref(false)
const connectionLimitInput = ref(0)
const savingConnectionLimit = ref(false)

/**
 * Extracts the current connection limit from the customer's radius_checks.
 * Looks for the Simultaneous-Use attribute. Returns 0 (unlimited) if not found.
 * Requirement 4.3
 */
const currentConnectionLimit = computed(() => {
  if (!store.detail?.radius_checks) return 0
  const check = store.detail.radius_checks.find(
    (rc) => rc.attribute === 'Simultaneous-Use'
  )
  return check ? Number(check.value) || 0 : 0
})

/**
 * Reset traffic counters for this customer.
 * Requirement 3.4
 */
async function handleTrafficReset() {
  if (!store.detail) return
  resettingTraffic.value = true
  const success = await store.trafficReset(store.detail.id)
  resettingTraffic.value = false
  if (success) {
    toast.success(t('customer.traffic_reset_success'))
  } else {
    toast.error(t('customer.traffic_reset_error'))
  }
}

/**
 * Start editing the connection limit inline.
 */
function startEditConnectionLimit() {
  connectionLimitInput.value = currentConnectionLimit.value
  editingConnectionLimit.value = true
}

/**
 * Cancel connection limit editing.
 */
function cancelEditConnectionLimit() {
  editingConnectionLimit.value = false
}

/**
 * Save the new connection limit.
 * Requirement 4.3
 */
async function saveConnectionLimit() {
  if (!store.detail) return
  savingConnectionLimit.value = true
  const limit = Math.max(0, Math.floor(connectionLimitInput.value))
  const success = await store.setConnectionLimit(store.detail.id, limit)
  savingConnectionLimit.value = false
  if (success) {
    editingConnectionLimit.value = false
    toast.success(
      limit === 0
        ? t('customer.conn_limit_removed')
        : t('customer.conn_limit_set') + ' ' + limit
    )
  } else {
    toast.error(t('customer.conn_limit_error'))
  }
}

const tabs = computed(() => [
  { key: 'profile', label: t('customer.tab_profile') },
  { key: 'usage', label: t('customer.tab_usage') },
  { key: 'history', label: t('customer.tab_history') },
])

// Edit form state
const form = ref({
  username: '',
  password: '',
  display_name: '',
  status: '',
  plan_id: '',
  data_gb: '',
  speed_mbps: '',
  days: '',
  notes: '',
})

const customer = computed(() => store.detail)
const usage = computed(() => store.usage)
const isNew = computed(() => props.id === 'new')

function populateForm() {
  if (customer.value) {
    form.value = {
      username: customer.value.username || '',
      password: '',
      display_name: customer.value.display_name || '',
      status: customer.value.status || '',
      plan_id: String(customer.value.plan_id ?? ''),
      data_gb: '',
      speed_mbps: '',
      days: '',
      notes: customer.value.notes || '',
    }
  }
}

watch(customer, populateForm)

async function saveProfile() {
  if (!customer.value) return
  saving.value = true
  await store.updateCustomer(customer.value.id, {
    display_name: form.value.display_name,
    status: form.value.status,
    notes: form.value.notes,
  })
  saving.value = false
}

async function createCustomer() {
  saving.value = true
  const created = await store.createCustomer({
    username: form.value.username,
    password: form.value.password,
    display_name: form.value.display_name,
    plan_id: Number(form.value.plan_id) || 1,
    data_gb: Number(form.value.data_gb) || 0,
    speed_mbps: Number(form.value.speed_mbps) || 0,
    days: Number(form.value.days) || 30,
  })
  saving.value = false
  if (created) {
    router.push({ name: 'customers' })
  }
}

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1048576) return `${(bytes / 1024).toFixed(1)} KB`
  if (bytes < 1073741824) return `${(bytes / 1048576).toFixed(1)} MB`
  return `${(bytes / 1073741824).toFixed(2)} GB`
}

// ---- Plan Change ----
interface Plan {
  id: number
  name: string
  data_gb: number
  speed_mbps: number
  duration_days: number
  price: number
  is_active: boolean
}
const plans = ref<Plan[]>([])
const selectedPlanId = ref<number>(0)
const applyingPlan = ref(false)

async function loadPlans() {
  try {
    const res = await get<{ ok: boolean; plans: Plan[] }>('/api/plans')
    plans.value = (res.plans || []).filter(p => p.is_active)
  } catch { /* ignore */ }
}

async function handleApplyPlan() {
  if (!customer.value || !selectedPlanId.value) return
  applyingPlan.value = true
  try {
    const { post: postApi } = useApi()
    const res = await postApi<{ ok: boolean; error?: string }>(`/api/customers/${customer.value.id}/renew`, {
      plan_id: selectedPlanId.value,
    })
    if (res.ok) {
      toast.success('Plan applied successfully')
      await store.loadDetail(customer.value.id)
    } else {
      toast.error(res.error || 'Failed to apply plan')
    }
  } catch (err: any) {
    toast.error(err?.message || 'Failed to apply plan')
  } finally {
    applyingPlan.value = false
  }
}

onMounted(() => {
  if (props.id && props.id !== 'new') {
    store.loadDetail(Number(props.id))
    loadPlans()
  }
})
</script>

<template>
  <div class="page customer-detail">
    <!-- Create New Customer Form -->
    <template v-if="isNew">
      <header class="detail-header">
        <div class="detail-header__left">
          <div class="detail-header__info">
            <h2 class="detail-header__username">{{ t('customer.new_customer') }}</h2>
          </div>
        </div>
        <KButton variant="ghost" @click="router.back()">{{ t('customer.back') }}</KButton>
      </header>

      <form class="profile-form" @submit.prevent="createCustomer">
        <div class="form-grid">
          <KFormField name="username" :label="t('login.username')" required>
            <template #default="{ fieldId }">
              <KInput :id="fieldId" v-model="form.username" placeholder="username" />
            </template>
          </KFormField>

          <KFormField name="password" :label="t('login.password')" required>
            <template #default="{ fieldId }">
              <KInput :id="fieldId" v-model="form.password" type="password" :placeholder="t('customer.placeholder_password')" />
            </template>
          </KFormField>

          <KFormField name="display_name" :label="t('customer.display_name')" required>
            <template #default="{ fieldId }">
              <KInput :id="fieldId" v-model="form.display_name" :placeholder="t('customer.placeholder_display_name')" />
            </template>
          </KFormField>

          <KFormField name="days" :label="t('customer.duration_days')">
            <template #default="{ fieldId }">
              <KInput :id="fieldId" v-model="form.days" type="number" placeholder="30" />
            </template>
          </KFormField>

          <KFormField name="data_gb" :label="t('customer.data_gb')">
            <template #default="{ fieldId }">
              <KInput :id="fieldId" v-model="form.data_gb" type="number" :placeholder="t('customer.placeholder_plan_default')" />
            </template>
          </KFormField>

          <KFormField name="speed_mbps" :label="t('customer.speed_mbps')">
            <template #default="{ fieldId }">
              <KInput :id="fieldId" v-model="form.speed_mbps" type="number" :placeholder="t('customer.placeholder_plan_default')" />
            </template>
          </KFormField>
        </div>

        <KFormField name="notes" :label="t('customer.notes')">
          <template #default="{ fieldId }">
            <KTextarea :id="fieldId" v-model="form.notes" rows="3" />
          </template>
        </KFormField>

        <div class="form-actions">
          <KButton variant="ghost" @click="router.back()">{{ t('btn.cancel') }}</KButton>
          <KButton type="submit" variant="primary" :loading="saving">{{ t('customer.create_customer') }}</KButton>
        </div>
      </form>
    </template>

    <!-- Loading State -->
    <div v-else-if="store.detailLoading" class="loading-state">
      <KSkeleton variant="rect" :width="'100%'" :height="80" />
      <KSkeleton variant="rect" :width="'100%'" :height="300" />
    </div>

    <template v-else-if="customer">
      <!-- Header -->
      <header class="detail-header">
        <div class="detail-header__left">
          <KAvatar :name="customer.display_name || customer.username" size="lg" />
          <div class="detail-header__info">
            <h2 class="detail-header__username">{{ customer.username }}</h2>
            <div class="detail-header__meta">
              <KStatusPill :status="customer.status" />
              <span class="detail-header__balance">${{ customer.credit.toFixed(2) }}</span>
            </div>
          </div>
        </div>
        <KButton variant="ghost" @click="router.back()">{{ t('customer.back') }}</KButton>
      </header>

      <!-- Tabs -->
      <KTabs v-model="activeTab" :tabs="tabs" aria-label="Customer details">
        <!-- Profile Tab -->
        <template #profile>
          <form class="profile-form" @submit.prevent="saveProfile">
            <div class="form-grid">
              <KFormField name="display_name" :label="t('customer.display_name')" required>
                <template #default="{ fieldId, describedBy }">
                  <KInput :id="fieldId" v-model="form.display_name" :aria-describedby="describedBy" />
                </template>
              </KFormField>

              <KFormField name="status" :label="t('customer.status')">
                <template #default="{ fieldId }">
                  <KSelect
                    :id="fieldId"
                    v-model="form.status"
                    :options="[
                      { label: t('status.active'), value: 'active' },
                      { label: t('status.disabled'), value: 'disabled' },
                      { label: t('status.limited'), value: 'limited' },
                      { label: t('status.expired'), value: 'expired' },
                    ]"
                  />
                </template>
              </KFormField>

              <KFormField name="data_gb" :label="t('customer.data_gb')">
                <template #default="{ fieldId }">
                  <KInput :id="fieldId" v-model="form.data_gb" type="number" :placeholder="t('customer.placeholder_plan_default')" />
                </template>
              </KFormField>

              <KFormField name="speed_mbps" :label="t('customer.speed_mbps')">
                <template #default="{ fieldId }">
                  <KInput :id="fieldId" v-model="form.speed_mbps" type="number" :placeholder="t('customer.placeholder_plan_default')" />
                </template>
              </KFormField>
            </div>

            <KFormField name="notes" :label="t('customer.notes')">
              <template #default="{ fieldId }">
                <KTextarea :id="fieldId" v-model="form.notes" rows="3" />
              </template>
            </KFormField>

            <div class="form-actions">
              <KButton type="submit" variant="primary" :loading="saving">{{ t('customer.save_changes') }}</KButton>
            </div>
          </form>

          <!-- Change Plan Section -->
          <div v-if="!isNew && customer" class="plan-change-section">
            <h4 class="section-title">Change Plan</h4>
            <div class="plan-change-row">
              <select v-model="selectedPlanId" class="plan-change-select">
                <option value="0" disabled>Select a plan...</option>
                <option v-for="plan in plans" :key="plan.id" :value="plan.id">
                  {{ plan.name }} — {{ plan.data_gb }}GB · {{ plan.duration_days }}d · ${{ plan.price }}
                </option>
              </select>
              <KButton
                variant="primary"
                size="sm"
                :loading="applyingPlan"
                :disabled="!selectedPlanId"
                @click="handleApplyPlan"
              >
                Apply Plan
              </KButton>
            </div>
            <p class="plan-change-note">This will activate the selected plan, create a subscription, and deduct from wallet.</p>
          </div>
        </template>

        <!-- Usage Tab -->
        <template #usage>
          <div class="usage-tab">
            <div v-if="usage" class="usage-stats">
              <div class="usage-stat">
                <span class="usage-stat__label">{{ t('customer.status') }}</span>
                <KStatusPill :status="usage.online ? 'online' : 'offline'" size="sm" />
              </div>
              <div class="usage-stat">
                <span class="usage-stat__label">{{ t('customer.active_sessions') }}</span>
                <span class="usage-stat__value">{{ usage.active_sessions }}</span>
              </div>
              <div class="usage-stat">
                <span class="usage-stat__label">{{ t('customer.total_download') }}</span>
                <span class="usage-stat__value">{{ formatBytes(usage.total_input_bytes) }}</span>
              </div>
              <div class="usage-stat">
                <span class="usage-stat__label">{{ t('customer.total_upload') }}</span>
                <span class="usage-stat__value">{{ formatBytes(usage.total_output_bytes) }}</span>
              </div>
              <div class="usage-stat">
                <span class="usage-stat__label">{{ t('customer.data_used') }}</span>
                <span class="usage-stat__value">{{ formatBytes(usage.total_usage_bytes) }}</span>
              </div>
            </div>

            <!-- Traffic Management Section (Requirements 3.4, 4.3) -->
            <div class="traffic-management">
              <!-- Traffic Reset Button (Requirement 3.4) -->
              <div class="traffic-management__row">
                <div class="traffic-management__info">
                  <h4 class="section-title">{{ t('customer.traffic_reset') }}</h4>
                  <p class="traffic-management__desc">{{ t('customer.traffic_reset_desc') }}</p>
                </div>
                <KButton
                  variant="ghost"
                  size="sm"
                  :loading="resettingTraffic"
                  @click="handleTrafficReset"
                >
                  {{ t('customer.reset_traffic') }}
                </KButton>
              </div>

              <!-- Connection Limit Inline Editor (Requirement 4.3) -->
              <div class="traffic-management__row">
                <div class="traffic-management__info">
                  <h4 class="section-title">{{ t('customer.connection_limit') }}</h4>
                  <p class="traffic-management__desc">{{ t('customer.connection_limit_desc') }}</p>
                </div>
                <div class="connection-limit-editor">
                  <template v-if="!editingConnectionLimit">
                    <span class="connection-limit-editor__value">
                      {{ currentConnectionLimit === 0 ? t('templates.unlimited') : currentConnectionLimit }}
                    </span>
                    <KButton variant="ghost" size="sm" @click="startEditConnectionLimit">
                      {{ t('btn.edit') }}
                    </KButton>
                  </template>
                  <template v-else>
                    <input
                      v-model.number="connectionLimitInput"
                      type="number"
                      min="0"
                      class="connection-limit-editor__input"
                      :aria-label="t('customer.connection_limit')"
                    />
                    <KButton
                      variant="primary"
                      size="sm"
                      :loading="savingConnectionLimit"
                      @click="saveConnectionLimit"
                    >
                      {{ t('btn.save') }}
                    </KButton>
                    <KButton variant="ghost" size="sm" @click="cancelEditConnectionLimit">
                      {{ t('btn.cancel') }}
                    </KButton>
                  </template>
                </div>
              </div>
            </div>

            <!-- Sessions Table -->
            <h4 class="section-title">{{ t('customer.sessions') }}</h4>
            <table class="mini-table" role="table">
              <thead>
                <tr><th>IP</th><th>{{ t('customer.th_start') }}</th><th>{{ t('customer.th_duration') }}</th><th>{{ t('customer.th_traffic') }}</th><th>{{ t('customer.th_status') }}</th></tr>
              </thead>
              <tbody>
                <tr v-for="s in usage?.sessions?.slice(0, 10)" :key="s.id">
                  <td>{{ s.framed_ip }}</td>
                  <td class="text-muted">{{ formatDateTime(s.start_time) }}</td>
                  <td>{{ Math.floor(s.session_seconds / 60) }}m</td>
                  <td>{{ formatBytes(s.total_bytes) }}</td>
                  <td><KStatusPill :status="s.online ? 'online' : 'offline'" size="sm" /></td>
                </tr>
              </tbody>
            </table>
          </div>
        </template>

        <!-- History Tab -->
        <template #history>
          <div class="history-tab">
            <h4 class="section-title">{{ t('customer.wallet_transactions') }}</h4>
            <table class="mini-table" role="table">
              <thead>
                <tr><th>{{ t('customer.th_date') }}</th><th>{{ t('customer.th_type') }}</th><th>{{ t('customer.th_amount') }}</th><th>{{ t('customer.th_description') }}</th></tr>
              </thead>
              <tbody>
                <tr v-for="tx in customer.wallet_transactions" :key="tx.id">
                  <td class="text-muted">{{ formatDate(tx.created_at) }}</td>
                  <td>{{ tx.type }}</td>
                  <td :class="{ 'text-success': tx.amount > 0, 'text-danger': tx.amount < 0 }">
                    ${{ tx.amount.toFixed(2) }}
                  </td>
                  <td>{{ tx.description }}</td>
                </tr>
              </tbody>
            </table>

            <h4 class="section-title">{{ t('customer.subscriptions') }}</h4>
            <table class="mini-table" role="table">
              <thead>
                <tr><th>{{ t('customer.th_plan') }}</th><th>{{ t('customer.th_start') }}</th><th>{{ t('customer.th_end') }}</th><th>{{ t('customer.th_status') }}</th></tr>
              </thead>
              <tbody>
                <tr v-for="sub in customer.subscriptions" :key="sub.id">
                  <td>{{ sub.plan_name }}</td>
                  <td class="text-muted">{{ formatDate(sub.start_date) }}</td>
                  <td class="text-muted">{{ formatDate(sub.end_date) }}</td>
                  <td><KStatusPill :status="sub.status" size="sm" /></td>
                </tr>
              </tbody>
            </table>
          </div>
        </template>

      </KTabs>
    </template>

    <!-- Not Found -->
    <div v-else class="empty-state">
      <p class="text-muted">Customer not found.</p>
      <KButton variant="ghost" @click="router.back()">Go Back</KButton>
    </div>
  </div>
</template>

<style scoped>
.customer-detail { display: flex; flex-direction: column; gap: var(--space-5); }
.loading-state { display: flex; flex-direction: column; gap: var(--space-4); }

.detail-header { display: flex; justify-content: space-between; align-items: center; padding: var(--space-4); background: var(--color-surface); border: 1px solid var(--color-border); border-radius: var(--radius-lg); }
.detail-header__left { display: flex; align-items: center; gap: var(--space-4); }
.detail-header__info { display: flex; flex-direction: column; gap: var(--space-1); }
.detail-header__username { margin: 0; font-size: var(--text-lg); font-weight: var(--font-bold); }
.detail-header__meta { display: flex; align-items: center; gap: var(--space-3); }
.detail-header__balance { font-size: var(--text-sm); font-weight: var(--font-semibold); color: var(--color-accent); }

.profile-form { display: flex; flex-direction: column; gap: var(--space-4); padding: var(--space-4) 0; }
.form-grid { display: grid; grid-template-columns: 1fr 1fr; gap: var(--space-4); }
.form-actions { display: flex; justify-content: flex-end; padding-top: var(--space-3); }

.usage-tab { display: flex; flex-direction: column; gap: var(--space-4); padding: var(--space-4) 0; }
.usage-stats { display: grid; grid-template-columns: repeat(auto-fit, minmax(150px, 1fr)); gap: var(--space-3); }
.usage-stat { display: flex; flex-direction: column; gap: var(--space-1); padding: var(--space-3); background: var(--color-surface-2); border-radius: var(--radius-md); }
.usage-stat__label { font-size: var(--text-xs); color: var(--color-muted); text-transform: uppercase; }
.usage-stat__value { font-size: var(--text-lg); font-weight: var(--font-bold); }

.history-tab { display: flex; flex-direction: column; gap: var(--space-4); padding: var(--space-4) 0; }
.section-title { margin: 0; font-size: var(--text-sm); font-weight: var(--font-semibold); color: var(--color-text); }

.mini-table { width: 100%; border-collapse: collapse; font-size: var(--text-sm); }
.mini-table th { text-align: left; padding: var(--space-2) var(--space-3); color: var(--color-muted); font-size: var(--text-xs); text-transform: uppercase; border-bottom: 1px solid var(--color-border); }
.mini-table td { padding: var(--space-2) var(--space-3); border-bottom: 1px solid var(--color-border); color: var(--color-text); }

.text-muted { color: var(--color-muted); }
.text-success { color: var(--color-success); }
.text-danger { color: var(--color-danger); }
.empty-state { text-align: center; padding: var(--space-12); }

.traffic-management { display: flex; flex-direction: column; gap: var(--space-4); padding: var(--space-4); background: var(--color-surface); border: 1px solid var(--color-border); border-radius: var(--radius-md); }
.traffic-management__row { display: flex; align-items: center; justify-content: space-between; gap: var(--space-4); }
.traffic-management__info { display: flex; flex-direction: column; gap: var(--space-1); }
.traffic-management__desc { margin: 0; font-size: var(--text-xs); color: var(--color-muted); }

.connection-limit-editor { display: flex; align-items: center; gap: var(--space-2); }
.connection-limit-editor__value { font-size: var(--text-sm); font-weight: var(--font-semibold); color: var(--color-text); min-width: 60px; }
.connection-limit-editor__input { width: 80px; padding: var(--space-1) var(--space-2); background: var(--color-surface); border: 1px solid var(--color-border); border-radius: var(--radius-sm); color: var(--color-text); font-size: var(--text-sm); outline: none; transition: border-color var(--duration-normal); }
.connection-limit-editor__input:focus { border-color: var(--color-primary); }

@media (max-width: 768px) {
  .form-grid { grid-template-columns: 1fr; }
  .traffic-management__row { flex-direction: column; align-items: flex-start; }
}

.plan-change-section {
  margin-top: var(--space-6);
  padding-top: var(--space-4);
  border-top: 1px solid var(--color-border);
}
.plan-change-row {
  display: flex;
  gap: var(--space-3);
  align-items: center;
  margin-top: var(--space-3);
}
.plan-change-select {
  flex: 1;
  max-width: 400px;
  padding: var(--space-2) var(--space-3);
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  color: var(--color-text);
  font-size: var(--text-sm);
}
.plan-change-note {
  margin-top: var(--space-2);
  font-size: var(--text-xs);
  color: var(--color-muted);
}
</style>
