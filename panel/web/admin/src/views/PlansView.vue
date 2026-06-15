<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { usePlansStore } from '@/stores/plans'
import KButton from '@koris/ui/KButton.vue'
import KFormField from '@koris/ui/KFormField.vue'
import KInput from '@koris/ui/KInput.vue'
import KSkeleton from '@koris/ui/KSkeleton.vue'
import KEmptyState from '@koris/ui/KEmptyState.vue'

const store = usePlansStore()
const showForm = ref(false)
const editingId = ref<number | null>(null)
const saving = ref(false)

const form = ref({
  name: '',
  data_gb: '',
  speed_mbps: '',
  duration_days: '',
  price: '',
})

function resetForm() {
  form.value = { name: '', data_gb: '', speed_mbps: '', duration_days: '', price: '' }
  editingId.value = null
  showForm.value = false
}

function openCreate() {
  resetForm()
  showForm.value = true
}

function openEdit(plan: any) {
  form.value = {
    name: plan.name,
    data_gb: String(plan.data_gb),
    speed_mbps: String(plan.speed_mbps),
    duration_days: String(plan.duration_days),
    price: String(plan.price),
  }
  editingId.value = plan.id
  showForm.value = true
}

async function handleSubmit() {
  saving.value = true
  const payload = {
    name: form.value.name,
    data_gb: Number(form.value.data_gb),
    speed_mbps: Number(form.value.speed_mbps),
    duration_days: Number(form.value.duration_days),
    price: Number(form.value.price),
    is_active: true,
    sort_order: 0,
  }
  if (editingId.value) {
    await store.updatePlan(editingId.value, payload)
  } else {
    await store.createPlan(payload)
  }
  saving.value = false
  resetForm()
}

async function deactivatePlan(id: number) {
  await store.deletePlan(id)
}

onMounted(() => {
  store.loadPlans()
})
</script>

<template>
  <div class="page plans-view">
    <!-- Header -->
    <header class="page-header">
      <h2 class="page-title">Plans</h2>
      <KButton variant="primary" icon="+" @click="openCreate">Create Plan</KButton>
    </header>

    <!-- Create/Edit Form -->
    <div v-if="showForm" class="plan-form-panel">
      <h4 class="form-title">{{ editingId ? 'Edit Plan' : 'New Plan' }}</h4>
      <form class="plan-form" @submit.prevent="handleSubmit">
        <div class="form-grid">
          <KFormField name="plan-name" label="Name" required>
            <template #default="{ fieldId }">
              <KInput :id="fieldId" v-model="form.name" placeholder="Plan name" />
            </template>
          </KFormField>
          <KFormField name="plan-data" label="Data (GB)" required>
            <template #default="{ fieldId }">
              <KInput :id="fieldId" v-model="form.data_gb" type="number" placeholder="50" />
            </template>
          </KFormField>
          <KFormField name="plan-speed" label="Speed (Mbps)" required>
            <template #default="{ fieldId }">
              <KInput :id="fieldId" v-model="form.speed_mbps" type="number" placeholder="100" />
            </template>
          </KFormField>
          <KFormField name="plan-duration" label="Duration (Days)" required>
            <template #default="{ fieldId }">
              <KInput :id="fieldId" v-model="form.duration_days" type="number" placeholder="30" />
            </template>
          </KFormField>
          <KFormField name="plan-price" label="Price ($)" required>
            <template #default="{ fieldId }">
              <KInput :id="fieldId" v-model="form.price" type="number" placeholder="9.99" />
            </template>
          </KFormField>
        </div>
        <div class="form-actions">
          <KButton variant="ghost" @click="resetForm">Cancel</KButton>
          <KButton type="submit" variant="primary" :loading="saving">
            {{ editingId ? 'Update' : 'Create' }}
          </KButton>
        </div>
      </form>
    </div>

    <!-- Loading -->
    <div v-if="store.loading && store.list.length === 0" class="plans-grid">
      <KSkeleton v-for="i in 4" :key="i" variant="rect" :width="'100%'" :height="160" />
    </div>

    <!-- Empty -->
    <KEmptyState
      v-else-if="store.list.length === 0"
      icon="📋"
      title="No Plans"
      description="Create your first subscription plan to get started."
    />

    <!-- Plans Grid -->
    <div v-else class="plans-grid">
      <div v-for="plan in store.list" :key="plan.id" class="plan-card" :class="{ 'plan-card--inactive': !plan.is_active }">
        <div class="plan-card__header">
          <h4 class="plan-card__name">{{ plan.name }}</h4>
          <span v-if="!plan.is_active" class="plan-card__badge">Inactive</span>
        </div>
        <div class="plan-card__price">${{ plan.price }}</div>
        <div class="plan-card__specs">
          <div class="plan-spec">
            <span class="plan-spec__label">Data</span>
            <span class="plan-spec__value">{{ plan.data_gb }} GB</span>
          </div>
          <div class="plan-spec">
            <span class="plan-spec__label">Speed</span>
            <span class="plan-spec__value">{{ plan.speed_mbps }} Mbps</span>
          </div>
          <div class="plan-spec">
            <span class="plan-spec__label">Duration</span>
            <span class="plan-spec__value">{{ plan.duration_days }} days</span>
          </div>
        </div>
        <div class="plan-card__actions">
          <KButton variant="ghost" size="sm" @click="openEdit(plan)">Edit</KButton>
          <KButton v-if="plan.is_active" variant="danger" size="sm" @click="deactivatePlan(plan.id)">Deactivate</KButton>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.plans-view { display: flex; flex-direction: column; gap: var(--space-5); }
.page-header { display: flex; align-items: center; justify-content: space-between; }
.page-title { margin: 0; font-size: var(--text-xl); font-weight: var(--font-bold); }

.plan-form-panel { padding: var(--space-4); background: var(--color-surface); border: 1px solid var(--color-border); border-radius: var(--radius-lg); }
.form-title { margin: 0 0 var(--space-3); font-size: var(--text-base); font-weight: var(--font-semibold); }
.plan-form { display: flex; flex-direction: column; gap: var(--space-4); }
.form-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(180px, 1fr)); gap: var(--space-3); }
.form-actions { display: flex; justify-content: flex-end; gap: var(--space-2); }

.plans-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(260px, 1fr)); gap: var(--space-4); }

.plan-card { padding: var(--space-4); background: var(--color-surface); border: 1px solid var(--color-border); border-radius: var(--radius-lg); display: flex; flex-direction: column; gap: var(--space-3); transition: border-color var(--duration-fast); }
.plan-card:hover { border-color: var(--color-primary); }
.plan-card--inactive { opacity: 0.6; }

.plan-card__header { display: flex; justify-content: space-between; align-items: center; }
.plan-card__name { margin: 0; font-size: var(--text-base); font-weight: var(--font-semibold); }
.plan-card__badge { font-size: var(--text-xs); color: var(--color-warning); background: rgba(245, 158, 11, 0.1); padding: 2px 8px; border-radius: var(--radius-full); }
.plan-card__price { font-size: var(--text-2xl); font-weight: var(--font-bold); color: var(--color-primary); }

.plan-card__specs { display: flex; flex-direction: column; gap: var(--space-1); }
.plan-spec { display: flex; justify-content: space-between; font-size: var(--text-sm); }
.plan-spec__label { color: var(--color-muted); }
.plan-spec__value { font-weight: var(--font-medium); }

.plan-card__actions { display: flex; gap: var(--space-2); border-top: 1px solid var(--color-border); padding-top: var(--space-3); }
</style>
