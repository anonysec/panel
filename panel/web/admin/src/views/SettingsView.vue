<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useNodesStore } from '@/stores/nodes'
import { useApi } from '@koris/composables/useApi'
import { useToast } from '@koris/composables/useToast'
import KTabs from '@koris/ui/KTabs.vue'
import KFormField from '@koris/ui/KFormField.vue'
import KInput from '@koris/ui/KInput.vue'
import KSelect from '@koris/ui/KSelect.vue'
import KButton from '@koris/ui/KButton.vue'
import KStatusPill from '@koris/ui/KStatusPill.vue'

const props = defineProps<{ tab?: string }>()

const nodesStore = useNodesStore()
const { get, put } = useApi()
const toast = useToast()
const activeTab = ref(props.tab || 'panel-status')
const saving = ref(false)

const tabs = [
  { key: 'panel-status', label: 'Panel Status' },
  { key: 'panel-settings', label: 'Panel Settings' },
  { key: 'data-warnings', label: 'Data Warnings' },
  { key: 'telegram', label: 'Telegram Bot' },
  { key: 'certificates', label: 'Certificates' },
  { key: 'vpn-settings', label: 'VPN Settings' },
  { key: 'audit-logs', label: 'Audit Logs' },
  { key: 'backup', label: 'Backup' },
]

// ─── Data Warning Thresholds ────────────────────────────────────────────────
const thresholds = ref<number[]>([80, 95])
const savingThresholds = ref(false)
const loadingThresholds = ref(false)

async function loadThresholds(): Promise<void> {
  loadingThresholds.value = true
  try {
    const res = await get<{ ok: boolean; thresholds: number[] }>('/api/settings/data-warning-thresholds')
    if (res.thresholds && res.thresholds.length > 0) {
      thresholds.value = res.thresholds
    }
  } catch {
    // Use defaults on error
  } finally {
    loadingThresholds.value = false
  }
}

function addThreshold(): void {
  thresholds.value.push(50)
}

function removeThreshold(index: number): void {
  if (thresholds.value.length > 1) {
    thresholds.value.splice(index, 1)
  }
}

function updateThreshold(index: number, value: string | number): void {
  const num = typeof value === 'number' ? value : parseInt(value, 10)
  if (!isNaN(num)) {
    thresholds.value[index] = Math.min(100, Math.max(0, num))
  }
}

async function saveThresholds(): Promise<void> {
  savingThresholds.value = true
  try {
    // Sort and deduplicate before saving
    const sorted = [...new Set(thresholds.value)].sort((a, b) => a - b)
    thresholds.value = sorted
    await put<{ ok: boolean }>('/api/settings/data-warning-thresholds', { thresholds: sorted })
    toast.success('Data warning thresholds saved successfully.')
  } catch {
    toast.error('Failed to save data warning thresholds.')
  } finally {
    savingThresholds.value = false
  }
}

// VPN Settings form
const vpnForm = ref({
  openvpn_port: '',
  openvpn_protocol: 'udp',
  openvpn_network: '',
  l2tp_network: '',
  ikev2_network: '',
  dns_1: '',
  dns_2: '',
})

function populateVpnForm() {
  const s = nodesStore.vpnSettings
  if (s) {
    vpnForm.value = {
      openvpn_port: String(s.openvpn_port || ''),
      openvpn_protocol: s.openvpn_protocol || 'udp',
      openvpn_network: s.openvpn_network || '',
      l2tp_network: s.l2tp_network || '',
      ikev2_network: s.ikev2_network || '',
      dns_1: s.dns_1 || '',
      dns_2: s.dns_2 || '',
    }
  }
}

async function saveVpnSettings() {
  saving.value = true
  await nodesStore.updateVpnSettings({
    openvpn_port: Number(vpnForm.value.openvpn_port),
    openvpn_protocol: vpnForm.value.openvpn_protocol,
    openvpn_network: vpnForm.value.openvpn_network,
    l2tp_network: vpnForm.value.l2tp_network,
    ikev2_network: vpnForm.value.ikev2_network,
    dns_1: vpnForm.value.dns_1,
    dns_2: vpnForm.value.dns_2,
  })
  saving.value = false
}

onMounted(async () => {
  await Promise.all([
    nodesStore.loadVpnSettings(),
    loadThresholds(),
  ])
  populateVpnForm()
})
</script>

<template>
  <div class="page settings-view">
    <header class="page-header">
      <h2 class="page-title">Settings</h2>
    </header>

    <KTabs v-model="activeTab" :tabs="tabs" aria-label="Settings sections">
      <!-- Panel Status -->
      <template #panel-status>
        <div class="settings-panel">
          <h4 class="section-title">Panel Status</h4>
          <div class="status-grid">
            <div class="status-item">
              <span class="status-item__label">WebSocket</span>
              <KStatusPill :status="'active'" size="sm" />
            </div>
            <div class="status-item">
              <span class="status-item__label">Version</span>
              <span class="status-item__value">1.0.0</span>
            </div>
            <div class="status-item">
              <span class="status-item__label">Uptime</span>
              <span class="status-item__value">Running</span>
            </div>
          </div>
        </div>
      </template>

      <!-- Panel Settings -->
      <template #panel-settings>
        <div class="settings-panel">
          <h4 class="section-title">Panel Settings</h4>
          <form class="settings-form">
            <KFormField name="panel-name" label="Panel Name">
              <template #default="{ fieldId }">
                <KInput :id="fieldId" placeholder="KorisPanel" />
              </template>
            </KFormField>
            <KFormField name="panel-lang" label="Language">
              <template #default="{ fieldId }">
                <KSelect
                  :id="fieldId"
                  :options="[
                    { label: 'English', value: 'en' },
                    { label: 'Persian', value: 'fa' },
                    { label: 'Chinese', value: 'zh' },
                  ]"
                  model-value="en"
                />
              </template>
            </KFormField>
          </form>
        </div>
      </template>

      <!-- Data Usage Warnings -->
      <template #data-warnings>
        <div class="settings-panel">
          <h4 class="section-title">Data Usage Warnings</h4>
          <p class="text-muted text-sm">
            Configure percentage thresholds at which customers receive data usage warnings.
            When a customer's traffic reaches any of these thresholds, a warning notification will be sent.
          </p>
          <form class="settings-form" @submit.prevent="saveThresholds">
            <div class="thresholds-list">
              <div
                v-for="(threshold, index) in thresholds"
                :key="index"
                class="threshold-row"
              >
                <KFormField :name="`threshold-${index}`" :label="`Threshold ${index + 1}`">
                  <template #default="{ fieldId }">
                    <div class="threshold-input-group">
                      <KInput
                        :id="fieldId"
                        :model-value="String(threshold)"
                        type="number"
                        min="0"
                        max="100"
                        placeholder="e.g. 80"
                        @update:model-value="updateThreshold(index, $event)"
                      />
                      <span class="threshold-unit">%</span>
                      <KButton
                        variant="ghost"
                        size="sm"
                        type="button"
                        :disabled="thresholds.length <= 1"
                        @click="removeThreshold(index)"
                      >
                        Remove
                      </KButton>
                    </div>
                  </template>
                </KFormField>
              </div>
            </div>
            <div class="threshold-actions">
              <KButton variant="ghost" size="sm" type="button" @click="addThreshold">
                + Add Threshold
              </KButton>
            </div>
            <KButton type="submit" variant="primary" :loading="savingThresholds">
              Save Thresholds
            </KButton>
          </form>
        </div>
      </template>

      <!-- Telegram Bot -->
      <template #telegram>
        <div class="settings-panel">
          <h4 class="section-title">Telegram Bot</h4>
          <form class="settings-form">
            <KFormField name="tg-token" label="Bot Token" hint="Get this from @BotFather">
              <template #default="{ fieldId }">
                <KInput :id="fieldId" placeholder="123456:ABC-DEF..." type="password" />
              </template>
            </KFormField>
            <KFormField name="tg-chat" label="Chat ID">
              <template #default="{ fieldId }">
                <KInput :id="fieldId" placeholder="-1001234567890" />
              </template>
            </KFormField>
            <KButton variant="primary" size="sm">Save Telegram Settings</KButton>
          </form>
        </div>
      </template>

      <!-- Certificates -->
      <template #certificates>
        <div class="settings-panel">
          <h4 class="section-title">SSL/TLS Certificates</h4>
          <div class="cert-info">
            <div class="cert-item">
              <span class="cert-item__label">CA Certificate</span>
              <KStatusPill :status="nodesStore.vpnSettings?.ca_exists ? 'active' : 'disabled'" size="sm" />
            </div>
            <div class="cert-item">
              <span class="cert-item__label">TLS Crypt Key</span>
              <KStatusPill :status="nodesStore.vpnSettings?.tls_crypt_exists ? 'active' : 'disabled'" size="sm" />
            </div>
          </div>
          <KButton variant="primary" size="sm" class="mt-3">Regenerate Certificates</KButton>
        </div>
      </template>

      <!-- VPN Settings -->
      <template #vpn-settings>
        <div class="settings-panel">
          <h4 class="section-title">VPN Settings</h4>
          <form class="settings-form" @submit.prevent="saveVpnSettings">
            <div class="form-grid-2">
              <KFormField name="vpn-port" label="OpenVPN Port">
                <template #default="{ fieldId }">
                  <KInput :id="fieldId" v-model="vpnForm.openvpn_port" type="number" placeholder="1194" />
                </template>
              </KFormField>
              <KFormField name="vpn-proto" label="OpenVPN Protocol">
                <template #default="{ fieldId }">
                  <KSelect
                    :id="fieldId"
                    v-model="vpnForm.openvpn_protocol"
                    :options="[
                      { label: 'UDP', value: 'udp' },
                      { label: 'TCP', value: 'tcp' },
                    ]"
                  />
                </template>
              </KFormField>
              <KFormField name="vpn-ovpn-net" label="OpenVPN Network">
                <template #default="{ fieldId }">
                  <KInput :id="fieldId" v-model="vpnForm.openvpn_network" placeholder="10.8.0.0/24" />
                </template>
              </KFormField>
              <KFormField name="vpn-l2tp-net" label="L2TP Network">
                <template #default="{ fieldId }">
                  <KInput :id="fieldId" v-model="vpnForm.l2tp_network" placeholder="10.9.0.0/24" />
                </template>
              </KFormField>
              <KFormField name="vpn-ikev2-net" label="IKEv2 Network">
                <template #default="{ fieldId }">
                  <KInput :id="fieldId" v-model="vpnForm.ikev2_network" placeholder="10.10.0.0/24" />
                </template>
              </KFormField>
              <KFormField name="vpn-dns1" label="DNS 1">
                <template #default="{ fieldId }">
                  <KInput :id="fieldId" v-model="vpnForm.dns_1" placeholder="8.8.8.8" />
                </template>
              </KFormField>
              <KFormField name="vpn-dns2" label="DNS 2">
                <template #default="{ fieldId }">
                  <KInput :id="fieldId" v-model="vpnForm.dns_2" placeholder="8.8.4.4" />
                </template>
              </KFormField>
            </div>
            <KButton type="submit" variant="primary" :loading="saving">Save VPN Settings</KButton>
          </form>
        </div>
      </template>

      <!-- Audit Logs -->
      <template #audit-logs>
        <div class="settings-panel">
          <h4 class="section-title">Audit Logs</h4>
          <p class="text-muted">Recent administrative actions will be displayed here.</p>
          <div class="audit-placeholder">
            <p class="text-muted text-sm">No audit logs available.</p>
          </div>
        </div>
      </template>

      <!-- Backup -->
      <template #backup>
        <div class="settings-panel">
          <h4 class="section-title">Backup &amp; Restore</h4>
          <p class="text-muted text-sm">Export or import your panel configuration and data.</p>
          <div class="backup-actions">
            <KButton variant="primary" size="sm">Export Backup</KButton>
            <KButton variant="ghost" size="sm">Import Backup</KButton>
          </div>
        </div>
      </template>
    </KTabs>
  </div>
</template>

<style scoped>
.settings-view { display: flex; flex-direction: column; gap: var(--space-5); }
.page-header { display: flex; align-items: center; justify-content: space-between; }
.page-title { margin: 0; font-size: var(--text-xl); font-weight: var(--font-bold); }

.settings-panel { padding: var(--space-5) 0; display: flex; flex-direction: column; gap: var(--space-4); }
.section-title { margin: 0; font-size: var(--text-base); font-weight: var(--font-semibold); }

.settings-form { display: flex; flex-direction: column; gap: var(--space-3); max-width: 480px; }
.form-grid-2 { display: grid; grid-template-columns: 1fr 1fr; gap: var(--space-3); }

.status-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(180px, 1fr)); gap: var(--space-3); }
.status-item { display: flex; justify-content: space-between; align-items: center; padding: var(--space-3); background: var(--color-surface); border: 1px solid var(--color-border); border-radius: var(--radius-md); }
.status-item__label { font-size: var(--text-sm); color: var(--color-muted); }
.status-item__value { font-size: var(--text-sm); font-weight: var(--font-medium); }

.cert-info { display: flex; flex-direction: column; gap: var(--space-2); }
.cert-item { display: flex; justify-content: space-between; align-items: center; padding: var(--space-3); background: var(--color-surface); border: 1px solid var(--color-border); border-radius: var(--radius-md); max-width: 400px; }
.cert-item__label { font-size: var(--text-sm); }

.backup-actions { display: flex; gap: var(--space-2); }

.thresholds-list { display: flex; flex-direction: column; gap: var(--space-2); }
.threshold-row { display: flex; align-items: flex-end; gap: var(--space-2); }
.threshold-input-group { display: flex; align-items: center; gap: var(--space-2); }
.threshold-unit { font-size: var(--text-sm); color: var(--color-muted); font-weight: var(--font-medium); }
.threshold-actions { display: flex; align-items: center; }

.text-muted { color: var(--color-muted); }
.text-sm { font-size: var(--text-sm); }
.mt-3 { margin-top: var(--space-3); }

@media (max-width: 768px) {
  .form-grid-2 { grid-template-columns: 1fr; }
}
</style>
