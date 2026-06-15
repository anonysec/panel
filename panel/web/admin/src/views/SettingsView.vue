<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useNodesStore } from '@/stores/nodes'
import KTabs from '@koris/ui/KTabs.vue'
import KFormField from '@koris/ui/KFormField.vue'
import KInput from '@koris/ui/KInput.vue'
import KSelect from '@koris/ui/KSelect.vue'
import KButton from '@koris/ui/KButton.vue'
import KStatusPill from '@koris/ui/KStatusPill.vue'

const props = defineProps<{ tab?: string }>()

const nodesStore = useNodesStore()
const activeTab = ref(props.tab || 'panel-status')
const saving = ref(false)

const tabs = [
  { key: 'panel-status', label: 'Panel Status' },
  { key: 'panel-settings', label: 'Panel Settings' },
  { key: 'telegram', label: 'Telegram Bot' },
  { key: 'certificates', label: 'Certificates' },
  { key: 'vpn-settings', label: 'VPN Settings' },
  { key: 'audit-logs', label: 'Audit Logs' },
  { key: 'backup', label: 'Backup' },
]

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
  await nodesStore.loadVpnSettings()
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

.text-muted { color: var(--color-muted); }
.text-sm { font-size: var(--text-sm); }
.mt-3 { margin-top: var(--space-3); }

@media (max-width: 768px) {
  .form-grid-2 { grid-template-columns: 1fr; }
}
</style>
