<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useTemplatesStore, type UserTemplate, type CreateTemplatePayload, type UpdateTemplatePayload } from '@/stores/templates'
import { usePlansStore } from '@/stores/plans'
import { useI18n } from '@koris/composables/useI18n'
import KDataTable from '@koris/ui/KDataTable.vue'
import KButton from '@koris/ui/KButton.vue'
import KFormField from '@koris/ui/KFormField.vue'
import KInput from '@koris/ui/KInput.vue'
import KSelect from '@koris/ui/KSelect.vue'
import KTextarea from '@koris/ui/KTextarea.vue'
import KStatusPill from '@koris/ui/KStatusPill.vue'
import KEmptyState from '@koris/ui/KEmptyState.vue'

const { t } = useI18n()
const store = useTemplatesStore()
const plansStore = usePlansStore()

const showForm = ref(false)
const editingId = ref<number | null>(null)
const saving = ref(false)
const showDeleteConfirm = ref(false)
const deleteTargetId = ref<number | null>(null)
const deleteTargetName = ref('')

const form = ref({
  name: '',
  plan_id: '',
  status: 'active',
  connection_limit: '0',
  radius_checks: '[]',
  radius_replies: '[]',
})

const tableColumns = computed(() => [
  { key: 'name', label: t('templates.col_name'), sortable: true },
  { key: 'plan_id', label: t('templates.col_plan'), sortable: true },
  { key: 'status', label: t('templates.col_status'), sortable: true },
  { key: 'connection_limit', label: t('templates.col_conn_limit'), sortable: true, align: 'right' as const },
  { key: 'created_by', label: t('templates.col_created_by'), sortable: true },
  { key: 'actions', label: t('payments.col_actions'), align: 'center' as const },
])

const statusOptions = computed(() => [
  { label: t('status.active'), value: 'active' },
  { label: t('status.disabled'), value: 'disabled' },
])

function resetForm() {
  form.value = {
    name: '',
    plan_id: '',
    status: 'active',
    connection_limit: '0',
    radius_checks: '[]',
    radius_replies: '[]',
  }
  editingId.value = null
  showForm.value = false
}

function openCreate() {
  resetForm()
  showForm.value = true
}

function openEdit(template: UserTemplate) {
  form.value = {
    name: template.name,
    plan_id: template.plan_id != null ? String(template.plan_id) : '',
    status: template.status,
    connection_limit: String(template.connection_limit),
    radius_checks: JSON.stringify(template.radius_checks || [], null, 2),
    radius_replies: JSON.stringify(template.radius_replies || [], null, 2),
  }
  editingId.value = template.id
  showForm.value = true
}

function confirmDelete(template: UserTemplate) {
  deleteTargetId.value = template.id
  deleteTargetName.value = template.name
  showDeleteConfirm.value = true
}

function cancelDelete() {
  showDeleteConfirm.value = false
  deleteTargetId.value = null
  deleteTargetName.value = ''
}

async function executeDelete() {
  if (deleteTargetId.value !== null) {
    await store.deleteTemplate(deleteTargetId.value)
  }
  cancelDelete()
}

function parseRadiusJson(text: string): any[] {
  try {
    const parsed = JSON.parse(text)
    return Array.isArray(parsed) ? parsed : []
  } catch {
    return []
  }
}

async function handleSubmit() {
  saving.value = true

  if (editingId.value) {
    const payload: UpdateTemplatePayload = {
      name: form.value.name,
      plan_id: form.value.plan_id ? Number(form.value.plan_id) : null,
      status: form.value.status,
      connection_limit: Number(form.value.connection_limit),
      radius_checks: parseRadiusJson(form.value.radius_checks),
      radius_replies: parseRadiusJson(form.value.radius_replies),
    }
    await store.updateTemplate(editingId.value, payload)
  } else {
    const payload: CreateTemplatePayload = {
      name: form.value.name,
      plan_id: form.value.plan_id ? Number(form.value.plan_id) : null,
      status: form.value.status,
      connection_limit: Number(form.value.connection_limit),
      radius_checks: parseRadiusJson(form.value.radius_checks),
      radius_replies: parseRadiusJson(form.value.radius_replies),
    }
    await store.createTemplate(payload)
  }

  saving.value = false
  resetForm()
}

function getPlanName(planId: number | null): string {
  if (planId == null) return '\u2014'
  const plan = plansStore.list.find((p) => p.id === planId)
  return plan ? plan.name : `#${planId}`
}

onMounted(() => {
  store.loadTemplates()
  plansStore.loadPlans()
})
</script>

<template>
  <div class="page templates-view">
    <!-- Header -->
    <header class="page-header">
      <KButton variant="primary" icon="+" @click="openCreate">{{ t('templates.create_template') }}</KButton>
    </header>

    <!-- Create/Edit Form -->
    <div v-if="showForm" class="template-form-panel">
      <h4 class="form-title">{{ editingId ? t('templates.edit_template') : t('templates.new_template') }}</h4>
      <form class="template-form" @submit.prevent="handleSubmit">
        <div class="form-grid">
          <KFormField name="tpl-name" :label="t('templates.name')" required>
            <template #default="{ fieldId }">
              <KInput :id="fieldId" v-model="form.name" :placeholder="t('templates.name_placeholder')" />
            </template>
          </KFormField>
          <KFormField name="tpl-plan" :label="t('templates.plan')">
            <template #default="{ fieldId }">
              <KSelect
                :id="fieldId"
                v-model="form.plan_id"
                :options="plansStore.list.map(p => ({ label: p.name, value: String(p.id) }))"
                :placeholder="t('templates.select_plan')"
              />
            </template>
          </KFormField>
          <KFormField name="tpl-status" :label="t('templates.status')" required>
            <template #default="{ fieldId }">
              <KSelect
                :id="fieldId"
                v-model="form.status"
                :options="statusOptions"
                :placeholder="t('templates.select_status')"
              />
            </template>
          </KFormField>
          <KFormField name="tpl-conn-limit" :label="t('templates.conn_limit')">
            <template #default="{ fieldId }">
              <KInput :id="fieldId" v-model="form.connection_limit" type="number" :placeholder="t('templates.unlimited_placeholder')" />
            </template>
          </KFormField>
        </div>
        <KFormField name="tpl-radius-checks" :label="t('templates.radius_checks')">
          <template #default="{ fieldId }">
            <KTextarea
              :id="fieldId"
              v-model="form.radius_checks"
              placeholder='[{"attribute":"Simultaneous-Use","op":":=","value":"1"}]'
              :rows="4"
            />
          </template>
        </KFormField>
        <KFormField name="tpl-radius-replies" :label="t('templates.radius_replies')">
          <template #default="{ fieldId }">
            <KTextarea
              :id="fieldId"
              v-model="form.radius_replies"
              placeholder='[{"attribute":"Framed-Pool","op":":=","value":"main_pool"}]'
              :rows="4"
            />
          </template>
        </KFormField>
        <div class="form-actions">
          <KButton variant="ghost" @click="resetForm">{{ t('btn.cancel') }}</KButton>
          <KButton type="submit" variant="primary" :loading="saving">
            {{ editingId ? t('templates.update') : t('btn.create') }}
          </KButton>
        </div>
      </form>
    </div>

    <!-- Delete Confirmation Dialog -->
    <div v-if="showDeleteConfirm" class="confirm-overlay" @click.self="cancelDelete">
      <div class="confirm-dialog">
        <h4 class="confirm-title">{{ t('templates.delete_template') }}</h4>
        <p class="confirm-message">
          {{ t('templates.delete_confirm_msg') }} <strong>{{ deleteTargetName }}</strong>
        </p>
        <div class="confirm-actions">
          <KButton variant="ghost" @click="cancelDelete">{{ t('btn.cancel') }}</KButton>
          <KButton variant="danger" @click="executeDelete">{{ t('btn.delete') }}</KButton>
        </div>
      </div>
    </div>

    <!-- Empty State -->
    <KEmptyState
      v-if="!store.loading && store.list.length === 0"
      icon="📋"
      :title="t('templates.no_templates')"
      :description="t('templates.no_templates_desc')"
    />

    <!-- Data Table -->
    <KDataTable
      v-else
      :columns="tableColumns"
      :data="store.list"
      :loading="store.loading"
      row-key="id"
    >
      <template #cell-plan_id="{ value }">
        {{ getPlanName(value) }}
      </template>
      <template #cell-status="{ value }">
        <KStatusPill :status="value" size="sm" />
      </template>
      <template #cell-connection_limit="{ value }">
        {{ value === 0 ? t('templates.unlimited') : value }}
      </template>
      <template #cell-actions="{ row }">
        <div class="action-btns">
          <KButton variant="ghost" size="sm" @click.stop="openEdit(row)">{{ t('btn.edit') }}</KButton>
          <KButton variant="danger" size="sm" @click.stop="confirmDelete(row)">{{ t('btn.delete') }}</KButton>
        </div>
      </template>
    </KDataTable>
  </div>
</template>

<style scoped>
.templates-view { display: flex; flex-direction: column; gap: var(--space-5); }
.page-header { display: flex; align-items: center; justify-content: flex-end; }

.template-form-panel { padding: var(--space-4); background: var(--color-surface); border: 1px solid var(--color-border); border-radius: var(--radius-lg); }
.form-title { margin: 0 0 var(--space-3); font-size: var(--text-base); font-weight: var(--font-semibold); }
.template-form { display: flex; flex-direction: column; gap: var(--space-4); }
.form-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: var(--space-3); }
.form-actions { display: flex; justify-content: flex-end; gap: var(--space-2); }

.action-btns { display: flex; gap: var(--space-1); }

/* Confirm Dialog */
.confirm-overlay { position: fixed; inset: 0; z-index: 1000; display: flex; align-items: center; justify-content: center; background: rgba(0, 0, 0, 0.6); backdrop-filter: blur(2px); }
.confirm-dialog { width: 100%; max-width: 420px; padding: var(--space-5); background: var(--color-surface); border: 1px solid var(--color-border); border-radius: var(--radius-lg); box-shadow: 0 8px 32px rgba(0, 0, 0, 0.4); }
.confirm-title { margin: 0 0 var(--space-2); font-size: var(--text-base); font-weight: var(--font-semibold); }
.confirm-message { margin: 0 0 var(--space-4); font-size: var(--text-sm); color: var(--color-muted); line-height: 1.5; }
.confirm-actions { display: flex; justify-content: flex-end; gap: var(--space-2); }
</style>
