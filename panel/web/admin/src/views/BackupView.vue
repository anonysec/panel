<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useBackups, type BackupRecord } from '@/composables/useBackups'
import { useToast } from '@koris/composables/useToast'
import { useConfirm } from '@koris/composables/useConfirm'
import KButton from '@koris/ui/KButton.vue'
import KStatusPill from '@koris/ui/KStatusPill.vue'
import KSkeleton from '@koris/ui/KSkeleton.vue'
import KEmptyState from '@koris/ui/KEmptyState.vue'
import BackupSettings from '@/components/BackupSettings.vue'
import BackupRestoreDialog from '@/components/BackupRestoreDialog.vue'

const toast = useToast()
const { confirm } = useConfirm()
const {
  backups,
  loading,
  fetchBackups,
  createBackup,
  downloadBackup,
  verifyBackup,
  deleteBackup,
  restoreBackup,
} = useBackups()

const creating = ref(false)
const verifying = ref<number | null>(null)
const deleting = ref<number | null>(null)
const showRestoreDialog = ref(false)
const restoring = ref(false)
let pollInterval: ReturnType<typeof setInterval> | null = null

const hasInProgress = computed(() =>
  backups.value.some(b => b.status === 'in_progress')
)

function formatSize(bytes: number | null): string {
  if (!bytes) return '—'
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
  return `${(bytes / (1024 * 1024 * 1024)).toFixed(2)} GB`
}

function formatDate(iso: string | null): string {
  if (!iso) return '—'
  const d = new Date(iso)
  return d.toLocaleString()
}

function statusLabel(status: string): string {
  switch (status) {
    case 'in_progress': return 'pending'
    case 'completed': return 'active'
    case 'failed': return 'failed'
    default: return status
  }
}

async function handleCreate() {
  creating.value = true
  try {
    const id = await createBackup()
    if (id) {
      toast.success('Backup started')
      await fetchBackups()
      startPolling()
    }
  } catch {
    toast.error('Failed to start backup')
  } finally {
    creating.value = false
  }
}

async function handleVerify(backup: BackupRecord) {
  verifying.value = backup.id
  try {
    const valid = await verifyBackup(backup.id)
    if (valid) {
      toast.success(`Backup "${backup.filename}" integrity verified`)
    } else {
      toast.error(`Backup "${backup.filename}" integrity check failed`)
    }
  } catch {
    toast.error('Verification failed')
  } finally {
    verifying.value = null
  }
}

async function handleDelete(backup: BackupRecord) {
  const confirmed = await confirm({
    title: 'Delete Backup',
    message: `Are you sure you want to delete "${backup.filename}"? This cannot be undone.`,
    variant: 'danger',
    icon: '⚠',
    confirmText: 'Delete',
    cancelText: 'Cancel',
  })
  if (!confirmed) return

  deleting.value = backup.id
  try {
    const ok = await deleteBackup(backup.id)
    if (ok) {
      toast.success('Backup deleted')
      await fetchBackups()
    } else {
      toast.error('Failed to delete backup')
    }
  } catch {
    toast.error('Failed to delete backup')
  } finally {
    deleting.value = null
  }
}

async function handleRestore(file: File) {
  restoring.value = true
  try {
    const ok = await restoreBackup(file)
    if (ok) {
      toast.success('Restore started successfully')
      showRestoreDialog.value = false
      await fetchBackups()
      startPolling()
    } else {
      toast.error('Restore failed')
    }
  } catch {
    toast.error('Restore failed')
  } finally {
    restoring.value = false
  }
}

function startPolling() {
  if (pollInterval) return
  pollInterval = setInterval(async () => {
    await fetchBackups()
    if (!hasInProgress.value) {
      stopPolling()
    }
  }, 3000)
}

function stopPolling() {
  if (pollInterval) {
    clearInterval(pollInterval)
    pollInterval = null
  }
}

onMounted(async () => {
  await fetchBackups()
  if (hasInProgress.value) {
    startPolling()
  }
})

onUnmounted(() => {
  stopPolling()
})
</script>

<template>
  <div class="page backup-view">
    <header class="page-header">
      <KButton variant="primary" :loading="creating" @click="handleCreate">
        Create Backup Now
      </KButton>
    </header>

    <!-- Settings -->
    <BackupSettings />

    <!-- Backup List -->
    <section class="backup-list-section">
      <h3 class="section-title">Backup History</h3>

      <div v-if="loading && backups.length === 0" class="backup-skeleton">
        <KSkeleton v-for="i in 3" :key="i" variant="rect" width="100%" :height="48" />
      </div>

      <KEmptyState
        v-else-if="backups.length === 0"
        icon="💾"
        title="No backups yet"
        description="Create your first backup to get started."
      />

      <div v-else class="backup-table-wrap">
        <table class="backup-table">
          <thead>
            <tr>
              <th>Filename</th>
              <th>Date</th>
              <th>Size</th>
              <th>Status</th>
              <th>Nodes</th>
              <th>Actions</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="backup in backups" :key="backup.id">
              <td class="cell-filename">{{ backup.filename }}</td>
              <td class="cell-date">{{ formatDate(backup.started_at) }}</td>
              <td class="cell-size">{{ formatSize(backup.size_bytes) }}</td>
              <td class="cell-status">
                <KStatusPill :status="statusLabel(backup.status)" size="sm" />
                <span v-if="backup.status === 'in_progress'" class="spinner" aria-label="In progress" />
              </td>
              <td class="cell-nodes">
                {{ backup.nodes_included?.length ?? 0 }}
              </td>
              <td class="cell-actions">
                <KButton
                  v-if="backup.status === 'completed'"
                  variant="ghost"
                  size="sm"
                  @click="downloadBackup(backup.id)"
                >
                  Download
                </KButton>
                <KButton
                  v-if="backup.status === 'completed'"
                  variant="ghost"
                  size="sm"
                  :loading="verifying === backup.id"
                  @click="handleVerify(backup)"
                >
                  Verify
                </KButton>
                <KButton
                  variant="ghost"
                  size="sm"
                  :loading="deleting === backup.id"
                  :disabled="backup.status === 'in_progress'"
                  @click="handleDelete(backup)"
                >
                  Delete
                </KButton>
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <!-- Restore Button -->
      <div class="restore-section">
        <KButton variant="ghost" @click="showRestoreDialog = true">
          Restore from File
        </KButton>
      </div>
    </section>

    <!-- Restore Dialog -->
    <BackupRestoreDialog
      :open="showRestoreDialog"
      :loading="restoring"
      @confirm="handleRestore"
      @cancel="showRestoreDialog = false"
    />
  </div>
</template>

<style scoped>
.backup-view {
  display: flex;
  flex-direction: column;
  gap: var(--space-5);
}

.page-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-3);
}

.section-title {
  margin: 0 0 var(--space-3);
  font-size: var(--text-base);
  font-weight: var(--font-semibold);
}

.backup-list-section {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}

.backup-skeleton {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}

.backup-table-wrap {
  overflow-x: auto;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-lg);
}

.backup-table {
  width: 100%;
  border-collapse: collapse;
  font-size: var(--text-sm);
}

.backup-table th {
  text-align: left;
  padding: var(--space-3) var(--space-3);
  background: var(--color-surface);
  border-bottom: 1px solid var(--color-border);
  font-weight: var(--font-semibold);
  color: var(--color-muted);
  font-size: var(--text-xs);
  text-transform: uppercase;
  letter-spacing: 0.5px;
}

.backup-table td {
  padding: var(--space-3) var(--space-3);
  border-bottom: 1px solid var(--color-border);
  color: var(--color-text);
}

.backup-table tr:last-child td {
  border-bottom: none;
}

.cell-filename {
  font-family: var(--font-mono, monospace);
  font-size: var(--text-xs);
}

.cell-date {
  white-space: nowrap;
}

.cell-size {
  white-space: nowrap;
}

.cell-status {
  display: flex;
  align-items: center;
  gap: var(--space-2);
}

.cell-actions {
  display: flex;
  gap: var(--space-1);
  flex-wrap: wrap;
}

.spinner {
  display: inline-block;
  width: 14px;
  height: 14px;
  border: 2px solid var(--color-border);
  border-top-color: var(--color-primary);
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

.restore-section {
  padding-top: var(--space-3);
  border-top: 1px solid var(--color-border);
}

@media (max-width: 768px) {
  .page-header {
    flex-direction: column;
    align-items: stretch;
  }

  .cell-actions {
    flex-direction: column;
  }
}
</style>
