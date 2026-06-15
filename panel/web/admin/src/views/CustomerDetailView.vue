<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useCustomersStore } from '@/stores/customers'
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

const router = useRouter()
const store = useCustomersStore()
const activeTab = ref('profile')
const saving = ref(false)

const tabs = [
  { key: 'profile', label: 'Profile' },
  { key: 'usage', label: 'Usage' },
  { key: 'history', label: 'History' },
]

// Edit form state
const form = ref({
  display_name: '',
  status: '',
  plan_id: '',
  data_gb: '',
  speed_mbps: '',
  notes: '',
})

const customer = computed(() => store.detail)
const usage = computed(() => store.usage)

function populateForm() {
  if (customer.value) {
    form.value = {
      display_name: customer.value.display_name || '',
      status: customer.value.status || '',
      plan_id: String(customer.value.plan_id ?? ''),
      data_gb: '',
      speed_mbps: '',
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

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1048576) return `${(bytes / 1024).toFixed(1)} KB`
  if (bytes < 1073741824) return `${(bytes / 1048576).toFixed(1)} MB`
  return `${(bytes / 1073741824).toFixed(2)} GB`
}

onMounted(() => {
  if (props.id && props.id !== 'new') {
    store.loadDetail(Number(props.id))
  }
})
</script>

<template>
  <div class="page customer-detail">
    <!-- Loading State -->
    <div v-if="store.detailLoading" class="loading-state">
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
        <KButton variant="ghost" @click="router.back()">Back</KButton>
      </header>

      <!-- Tabs -->
      <KTabs v-model="activeTab" :tabs="tabs" aria-label="Customer details">
        <!-- Profile Tab -->
        <template #profile>
          <form class="profile-form" @submit.prevent="saveProfile">
            <div class="form-grid">
              <KFormField name="display_name" label="Display Name" required>
                <template #default="{ fieldId, describedBy }">
                  <KInput :id="fieldId" v-model="form.display_name" :aria-describedby="describedBy" />
                </template>
              </KFormField>

              <KFormField name="status" label="Status">
                <template #default="{ fieldId }">
                  <KSelect
                    :id="fieldId"
                    v-model="form.status"
                    :options="[
                      { label: 'Active', value: 'active' },
                      { label: 'Disabled', value: 'disabled' },
                      { label: 'Limited', value: 'limited' },
                      { label: 'Expired', value: 'expired' },
                    ]"
                  />
                </template>
              </KFormField>

              <KFormField name="data_gb" label="Data (GB)">
                <template #default="{ fieldId }">
                  <KInput :id="fieldId" v-model="form.data_gb" type="number" placeholder="Plan default" />
                </template>
              </KFormField>

              <KFormField name="speed_mbps" label="Speed (Mbps)">
                <template #default="{ fieldId }">
                  <KInput :id="fieldId" v-model="form.speed_mbps" type="number" placeholder="Plan default" />
                </template>
              </KFormField>
            </div>

            <KFormField name="notes" label="Notes">
              <template #default="{ fieldId }">
                <KTextarea :id="fieldId" v-model="form.notes" rows="3" />
              </template>
            </KFormField>

            <div class="form-actions">
              <KButton type="submit" variant="primary" :loading="saving">Save Changes</KButton>
            </div>
          </form>
        </template>

        <!-- Usage Tab -->
        <template #usage>
          <div class="usage-tab">
            <div v-if="usage" class="usage-stats">
              <div class="usage-stat">
                <span class="usage-stat__label">Status</span>
                <KStatusPill :status="usage.online ? 'online' : 'offline'" size="sm" />
              </div>
              <div class="usage-stat">
                <span class="usage-stat__label">Active Sessions</span>
                <span class="usage-stat__value">{{ usage.active_sessions }}</span>
              </div>
              <div class="usage-stat">
                <span class="usage-stat__label">Total Download</span>
                <span class="usage-stat__value">{{ formatBytes(usage.total_input_bytes) }}</span>
              </div>
              <div class="usage-stat">
                <span class="usage-stat__label">Total Upload</span>
                <span class="usage-stat__value">{{ formatBytes(usage.total_output_bytes) }}</span>
              </div>
              <div class="usage-stat">
                <span class="usage-stat__label">Data Used</span>
                <span class="usage-stat__value">{{ formatBytes(usage.total_usage_bytes) }}</span>
              </div>
            </div>

            <!-- Sessions Table -->
            <h4 class="section-title">Sessions</h4>
            <table class="mini-table" role="table">
              <thead>
                <tr><th>IP</th><th>Start</th><th>Duration</th><th>Traffic</th><th>Status</th></tr>
              </thead>
              <tbody>
                <tr v-for="s in usage?.sessions?.slice(0, 10)" :key="s.id">
                  <td>{{ s.framed_ip }}</td>
                  <td class="text-muted">{{ s.start_time?.slice(0, 16) }}</td>
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
            <h4 class="section-title">Wallet Transactions</h4>
            <table class="mini-table" role="table">
              <thead>
                <tr><th>Date</th><th>Type</th><th>Amount</th><th>Description</th></tr>
              </thead>
              <tbody>
                <tr v-for="tx in customer.wallet_transactions" :key="tx.id">
                  <td class="text-muted">{{ tx.created_at?.slice(0, 10) }}</td>
                  <td>{{ tx.type }}</td>
                  <td :class="{ 'text-success': tx.amount > 0, 'text-danger': tx.amount < 0 }">
                    ${{ tx.amount.toFixed(2) }}
                  </td>
                  <td>{{ tx.description }}</td>
                </tr>
              </tbody>
            </table>

            <h4 class="section-title">Subscriptions</h4>
            <table class="mini-table" role="table">
              <thead>
                <tr><th>Plan</th><th>Start</th><th>End</th><th>Status</th></tr>
              </thead>
              <tbody>
                <tr v-for="sub in customer.subscriptions" :key="sub.id">
                  <td>{{ sub.plan_name }}</td>
                  <td class="text-muted">{{ sub.start_date?.slice(0, 10) }}</td>
                  <td class="text-muted">{{ sub.end_date?.slice(0, 10) }}</td>
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

@media (max-width: 768px) {
  .form-grid { grid-template-columns: 1fr; }
}
</style>
