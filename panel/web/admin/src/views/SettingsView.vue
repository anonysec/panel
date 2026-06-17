<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'
import { useNodesStore } from '@/stores/nodes'
import { useApi } from '@koris/composables/useApi'
import { useToast } from '@koris/composables/useToast'
import { useI18n } from '@koris/composables/useI18n'
import { useTheme, availableThemes } from '@koris/composables/useTheme'
import type { ThemeMode, UITheme } from '@koris/composables/useTheme'
import type { Locale } from '@koris/composables/useI18n'
import KTabs from '@koris/ui/KTabs.vue'
import KFormField from '@koris/ui/KFormField.vue'
import KInput from '@koris/ui/KInput.vue'
import KSelect from '@koris/ui/KSelect.vue'
import KButton from '@koris/ui/KButton.vue'
import KStatusPill from '@koris/ui/KStatusPill.vue'

const props = defineProps<{ tab?: string }>()

const { t, locale: currentLocale, setLocale } = useI18n()
const { mode: currentMode, theme: currentTheme, setMode, setTheme } = useTheme()
const nodesStore = useNodesStore()
const { get, put, patch } = useApi()
const toast = useToast()
const activeTab = ref(props.tab || 'panel-settings')
const saving = ref(false)

const tabs = computed(() => [
  { key: 'panel-settings', label: t('settings.panel_settings') },
  { key: 'data-warnings', label: t('settings.data_warnings') },
  { key: 'app-links', label: t('settings.app_links') },
  { key: 'telegram', label: t('settings.telegram') },
  { key: 'certificates', label: t('settings.certificates') },
  { key: 'backup', label: t('settings.backup') },
])

// ─── Panel Settings ─────────────────────────────────────────────────────────
const panelName = ref('')
const panelLang = ref<string>(currentLocale.value)
const loadingSettings = ref(false)
const savingSettings = ref(false)

// Theme settings (local copies for save)
const selectedTheme = ref<UITheme>(currentTheme.value)
const selectedMode = ref<ThemeMode>(currentMode.value)

const modeOptions: { value: ThemeMode; labelKey: string }[] = [
  { value: 'system', labelKey: 'settings.mode_system' },
  { value: 'dark', labelKey: 'settings.mode_dark' },
  { value: 'light', labelKey: 'settings.mode_light' },
]

// Apply theme/mode immediately when user changes in UI (live preview)
watch(selectedTheme, (v) => { setTheme(v) })
watch(selectedMode, (v) => { setMode(v) })

// Sync panelLang with global locale (bidirectional)
watch(panelLang, (newLang) => {
  if (newLang !== currentLocale.value) {
    setLocale(newLang as Locale)
  }
})

watch(currentLocale, (newLocale) => {
  if (newLocale !== panelLang.value) {
    panelLang.value = newLocale
  }
})

async function loadPanelSettings(): Promise<void> {
  loadingSettings.value = true
  try {
    const res = await get<{ ok: boolean; settings: Record<string, string> }>('/api/panel-settings')
    if (res.settings) {
      panelName.value = res.settings.panel_name || ''
      // Only apply server language if user has NOT explicitly chosen a language via sidebar
      // (localStorage takes priority as the most recent user choice)
      const storedLang = typeof window !== 'undefined' ? localStorage.getItem('koris-lang') : null
      if (!storedLang) {
        panelLang.value = res.settings.language || 'en'
      }
      // Load theme settings from server
      if (res.settings.ui_theme && availableThemes.some((t) => t.id === res.settings.ui_theme)) {
        selectedTheme.value = res.settings.ui_theme as UITheme
        setTheme(res.settings.ui_theme as UITheme)
      }
      if (res.settings.ui_mode && ['system', 'dark', 'light'].includes(res.settings.ui_mode)) {
        selectedMode.value = res.settings.ui_mode as ThemeMode
        setMode(res.settings.ui_mode as ThemeMode)
      }
    }
  } catch {
    // Use defaults on error
  } finally {
    loadingSettings.value = false
  }
}

async function savePanelSettings(): Promise<void> {
  savingSettings.value = true
  try {
    await patch<{ ok: boolean }>('/api/panel-settings', {
      panel_name: panelName.value,
      language: panelLang.value,
      ui_theme: selectedTheme.value,
      ui_mode: selectedMode.value,
    })
    toast.success(t('settings.save_success'))
  } catch {
    toast.error(t('settings.save_error'))
  } finally {
    savingSettings.value = false
  }
}

// ─── Data Warning Thresholds ────────────────────────────────────────────────
const thresholds = ref<number[]>([80, 95])
const savingThresholds = ref(false)
const loadingThresholds = ref(false)

// ─── Expiry Warnings ────────────────────────────────────────────────────────
const expiryDays = ref<number[]>([7, 3, 1])
const connThresholds = ref<number[]>([80, 95])
const webhookUrl = ref('')
const savingWarningConfig = ref(false)

async function loadWarningConfig(): Promise<void> {
  try {
    const res = await get<{ ok: boolean; config: { expiry_days?: number[]; conn_thresholds?: number[]; webhook_url?: string } }>('/api/settings/warning-config')
    if (res.config) {
      if (res.config.expiry_days?.length) expiryDays.value = res.config.expiry_days
      if (res.config.conn_thresholds?.length) connThresholds.value = res.config.conn_thresholds
      if (res.config.webhook_url) webhookUrl.value = res.config.webhook_url
    }
  } catch {
    // Use defaults
  }
}

function addExpiryDay(): void { expiryDays.value.push(7) }
function removeExpiryDay(i: number): void { if (expiryDays.value.length > 1) expiryDays.value.splice(i, 1) }
function updateExpiryDay(i: number, v: string | number): void {
  const num = typeof v === 'number' ? v : parseInt(v, 10)
  if (!isNaN(num)) expiryDays.value[i] = Math.max(1, num)
}

function addConnThreshold(): void { connThresholds.value.push(80) }
function removeConnThreshold(i: number): void { if (connThresholds.value.length > 1) connThresholds.value.splice(i, 1) }
function updateConnThreshold(i: number, v: string | number): void {
  const num = typeof v === 'number' ? v : parseInt(v, 10)
  if (!isNaN(num)) connThresholds.value[i] = Math.min(100, Math.max(1, num))
}

async function saveWarningConfig(): Promise<void> {
  savingWarningConfig.value = true
  try {
    await put<{ ok: boolean }>('/api/settings/warning-config', {
      expiry_days: [...new Set(expiryDays.value)].sort((a, b) => b - a),
      conn_thresholds: [...new Set(connThresholds.value)].sort((a, b) => a - b),
      webhook_url: webhookUrl.value.trim(),
    })
    toast.success(t('settings.warning_config_save_success'))
  } catch {
    toast.error(t('settings.warning_config_save_error'))
  } finally {
    savingWarningConfig.value = false
  }
}

// ─── App Links ──────────────────────────────────────────────────────────────
interface AppLink {
  name: string
  url: string
  platform: string
  icon: string
}
const appLinks = ref<AppLink[]>([])
const savingAppLinks = ref(false)
const loadingAppLinks = ref(false)

const platformOptions = computed(() => [
  { label: 'iOS', value: 'ios' },
  { label: 'Android', value: 'android' },
  { label: 'Windows', value: 'windows' },
  { label: 'Mac', value: 'mac' },
  { label: t('settings.app_platform_other'), value: 'other' },
])

const platformIcons: Record<string, string> = {
  ios: '🍎',
  android: '🤖',
  windows: '🪟',
  mac: '💻',
  other: '🔗',
}

async function loadAppLinks(): Promise<void> {
  loadingAppLinks.value = true
  try {
    const res = await get<{ ok: boolean; settings: Record<string, string> }>('/api/panel-settings')
    if (res.settings?.app_links) {
      try {
        const parsed = JSON.parse(res.settings.app_links)
        if (Array.isArray(parsed)) appLinks.value = parsed
      } catch { /* keep default */ }
    }
  } catch { /* use defaults */ }
  finally { loadingAppLinks.value = false }
}

function addAppLink(): void {
  appLinks.value.push({ name: '', url: '', platform: 'other', icon: '🔗' })
}

function removeAppLink(i: number): void {
  appLinks.value.splice(i, 1)
}

function updatePlatformIcon(i: number): void {
  appLinks.value[i].icon = platformIcons[appLinks.value[i].platform] || '🔗'
}

async function saveAppLinks(): Promise<void> {
  savingAppLinks.value = true
  try {
    await patch<{ ok: boolean }>('/api/panel-settings', {
      app_links: JSON.stringify(appLinks.value.filter(l => l.name && l.url)),
    })
    toast.success(t('settings.app_links_save_success'))
  } catch {
    toast.error(t('settings.app_links_save_error'))
  } finally {
    savingAppLinks.value = false
  }
}

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
    const sorted = [...new Set(thresholds.value)].sort((a, b) => a - b)
    thresholds.value = sorted
    await put<{ ok: boolean }>('/api/settings/data-warning-thresholds', { thresholds: sorted })
    toast.success(t('settings.thresholds_save_success'))
  } catch {
    toast.error(t('settings.thresholds_save_error'))
  } finally {
    savingThresholds.value = false
  }
}

// ─── Telegram Bot Settings ───────────────────────────────────────────────────
const telegramToken = ref('')
const telegramChatId = ref('')
const savingTelegram = ref(false)

async function loadTelegramSettings(): Promise<void> {
  try {
    const res = await get<{ ok: boolean; settings: Record<string, string> }>('/api/panel-settings')
    if (res.settings) {
      telegramToken.value = res.settings.telegram_token || ''
      telegramChatId.value = res.settings.telegram_chat_id || ''
    }
  } catch {
    // Use defaults on error
  }
}

async function saveTelegramSettings(): Promise<void> {
  savingTelegram.value = true
  try {
    await patch<{ ok: boolean }>('/api/panel-settings', {
      telegram_token: telegramToken.value,
      telegram_chat_id: telegramChatId.value,
    })
    toast.success(t('settings.telegram_save_success'))
  } catch {
    toast.error(t('settings.telegram_save_error'))
  } finally {
    savingTelegram.value = false
  }
}

// ─── Backup ─────────────────────────────────────────────────────────────────
const importFileInput = ref<HTMLInputElement | null>(null)
const exporting = ref(false)
const importing = ref(false)

async function downloadBackup(): Promise<void> {
  exporting.value = true
  try {
    const res = await fetch('/api/backup/export', { credentials: 'include' })
    if (!res.ok) throw new Error('Export failed')
    const blob = await res.blob()
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    const ts = new Date().toISOString().replace(/[:.]/g, '-').slice(0, 19)
    a.download = `panel-backup-${ts}.json`
    document.body.appendChild(a)
    a.click()
    document.body.removeChild(a)
    URL.revokeObjectURL(url)
    toast.success(t('settings.export_success'))
  } catch {
    toast.error(t('settings.export_error'))
  } finally {
    exporting.value = false
  }
}

function triggerImport(): void {
  importFileInput.value?.click()
}

async function handleImportFile(event: Event): Promise<void> {
  const target = event.target as HTMLInputElement
  if (!target.files || target.files.length === 0) return
  const file = target.files[0]
  importing.value = true
  try {
    const formData = new FormData()
    formData.append('file', file)
    const res = await fetch('/api/backup/import', {
      method: 'POST',
      credentials: 'include',
      body: formData,
    })
    const data = await res.json()
    if (data.ok) {
      toast.success(t('settings.import_success'))
    } else {
      toast.error(t('settings.import_error') + (data.error ? `: ${data.error}` : ''))
    }
  } catch {
    toast.error(t('settings.import_error'))
  } finally {
    importing.value = false
    target.value = ''
  }
}

onMounted(async () => {
  await Promise.all([loadPanelSettings(), loadThresholds(), loadTelegramSettings(), loadWarningConfig(), loadAppLinks()])
})
</script>

<template>
  <div class="page settings-view">
    <KTabs v-model="activeTab" :tabs="tabs" aria-label="Settings sections">
      <!-- Panel Settings -->
      <template #panel-settings>
        <div class="settings-panel">
          <h4 class="section-title">{{ t('settings.panel_settings') }}</h4>
          <form class="settings-form" @submit.prevent="savePanelSettings">
            <KFormField name="panel-name" :label="t('settings.panel_name')">
              <template #default="{ fieldId }">
                <KInput
                  :id="fieldId"
                  v-model="panelName"
                  placeholder="KorisPanel"
                  :disabled="loadingSettings"
                />
              </template>
            </KFormField>
            <KFormField name="panel-lang" :label="t('settings.language')">
              <template #default="{ fieldId }">
                <KSelect
                  :id="fieldId"
                  v-model="panelLang"
                  :options="[
                    { label: 'English', value: 'en' },
                    { label: 'Persian', value: 'fa' },
                    { label: 'Chinese', value: 'zh' },
                    { label: 'Russian', value: 'ru' },
                  ]"
                  :disabled="loadingSettings"
                />
              </template>
            </KFormField>

            <!-- Theme Section -->
            <div class="theme-section">
              <h5 class="subsection-title">{{ t('settings.ui_theme') }}</h5>
              <p class="text-muted text-sm">{{ t('settings.ui_theme_desc') }}</p>
              <div class="theme-cards">
                <button
                  v-for="themeItem in availableThemes"
                  :key="themeItem.id"
                  type="button"
                  class="theme-card"
                  :class="{ active: selectedTheme === themeItem.id }"
                  @click="selectedTheme = themeItem.id"
                >
                  <div class="theme-card__swatches">
                    <span class="swatch" :style="{ background: themeItem.colors.bg }"></span>
                    <span class="swatch" :style="{ background: themeItem.colors.surface }"></span>
                    <span class="swatch" :style="{ background: themeItem.colors.primary }"></span>
                    <span class="swatch" :style="{ background: themeItem.colors.accent }"></span>
                  </div>
                  <div class="theme-card__info">
                    <span class="theme-card__name">{{ t('settings.theme_' + themeItem.id) }}</span>
                    <span class="theme-card__desc">{{ t('settings.theme_' + themeItem.id + '_desc') }}</span>
                  </div>
                </button>
              </div>
            </div>

            <!-- Mode Section -->
            <div class="mode-section">
              <h5 class="subsection-title">{{ t('settings.ui_mode') }}</h5>
              <div class="mode-radios">
                <label
                  v-for="opt in modeOptions"
                  :key="opt.value"
                  class="mode-radio"
                  :class="{ active: selectedMode === opt.value }"
                >
                  <input
                    type="radio"
                    name="ui-mode"
                    :value="opt.value"
                    v-model="selectedMode"
                    class="mode-radio__input"
                  />
                  <span class="mode-radio__label">{{ t(opt.labelKey) }}</span>
                </label>
              </div>
            </div>

            <KButton type="submit" variant="primary" :loading="savingSettings" :disabled="loadingSettings">
              {{ t('settings.save') }}
            </KButton>
          </form>
        </div>
      </template>

      <!-- Data Usage Warnings -->
      <template #data-warnings>
        <div class="settings-panel">
          <h4 class="section-title">{{ t('settings.thresholds') }}</h4>
          <p class="text-muted text-sm">
            {{ t('settings.thresholds_desc') }}
          </p>
          <form class="settings-form" @submit.prevent="saveThresholds">
            <div class="thresholds-list">
              <div
                v-for="(threshold, index) in thresholds"
                :key="index"
                class="threshold-row"
              >
                <KFormField :name="`threshold-${index}`" :label="`${t('label.threshold')} ${index + 1}`">
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
                        {{ t('label.remove') }}
                      </KButton>
                    </div>
                  </template>
                </KFormField>
              </div>
            </div>
            <div class="threshold-actions">
              <KButton variant="ghost" size="sm" type="button" @click="addThreshold">
                {{ t('settings.add_threshold') }}
              </KButton>
            </div>
            <KButton type="submit" variant="primary" :loading="savingThresholds">
              {{ t('settings.save_thresholds') }}
            </KButton>
          </form>

          <!-- Expiry Warnings -->
          <div class="warning-subsection">
            <h5 class="subsection-title">{{ t('settings.expiry_warnings') }}</h5>
            <p class="text-muted text-sm">{{ t('settings.expiry_warnings_desc') }}</p>
            <form class="settings-form" @submit.prevent="saveWarningConfig">
              <div class="thresholds-list">
                <div v-for="(day, index) in expiryDays" :key="'exp-'+index" class="threshold-row">
                  <KFormField :name="`expiry-${index}`" :label="`${t('settings.days_before_expiry')} ${index + 1}`">
                    <template #default="{ fieldId }">
                      <div class="threshold-input-group">
                        <KInput
                          :id="fieldId"
                          :model-value="String(day)"
                          type="number"
                          min="1"
                          max="365"
                          placeholder="7"
                          @update:model-value="updateExpiryDay(index, $event)"
                        />
                        <span class="threshold-unit">{{ t('settings.days_unit') }}</span>
                        <KButton variant="ghost" size="sm" type="button" :disabled="expiryDays.length <= 1" @click="removeExpiryDay(index)">
                          {{ t('label.remove') }}
                        </KButton>
                      </div>
                    </template>
                  </KFormField>
                </div>
              </div>
              <div class="threshold-actions">
                <KButton variant="ghost" size="sm" type="button" @click="addExpiryDay">{{ t('settings.add_threshold') }}</KButton>
              </div>

              <!-- Connection Limit Warnings -->
              <h5 class="subsection-title">{{ t('settings.conn_warnings') }}</h5>
              <p class="text-muted text-sm">{{ t('settings.conn_warnings_desc') }}</p>
              <div class="thresholds-list">
                <div v-for="(ct, index) in connThresholds" :key="'conn-'+index" class="threshold-row">
                  <KFormField :name="`conn-${index}`" :label="`${t('label.threshold')} ${index + 1}`">
                    <template #default="{ fieldId }">
                      <div class="threshold-input-group">
                        <KInput
                          :id="fieldId"
                          :model-value="String(ct)"
                          type="number"
                          min="1"
                          max="100"
                          placeholder="80"
                          @update:model-value="updateConnThreshold(index, $event)"
                        />
                        <span class="threshold-unit">%</span>
                        <KButton variant="ghost" size="sm" type="button" :disabled="connThresholds.length <= 1" @click="removeConnThreshold(index)">
                          {{ t('label.remove') }}
                        </KButton>
                      </div>
                    </template>
                  </KFormField>
                </div>
              </div>
              <div class="threshold-actions">
                <KButton variant="ghost" size="sm" type="button" @click="addConnThreshold">{{ t('settings.add_threshold') }}</KButton>
              </div>

              <!-- Webhook URL -->
              <h5 class="subsection-title">{{ t('settings.webhook_url') }}</h5>
              <p class="text-muted text-sm">{{ t('settings.webhook_url_desc') }}</p>
              <KFormField name="webhook-url" :label="t('settings.webhook_url')">
                <template #default="{ fieldId }">
                  <KInput :id="fieldId" v-model="webhookUrl" placeholder="https://example.com/webhook" />
                </template>
              </KFormField>

              <KButton type="submit" variant="primary" :loading="savingWarningConfig">
                {{ t('settings.save_warning_config') }}
              </KButton>
            </form>
          </div>
        </div>
      </template>

      <!-- App Links -->
      <template #app-links>
        <div class="settings-panel">
          <h4 class="section-title">{{ t('settings.app_links') }}</h4>
          <p class="text-muted text-sm">{{ t('settings.app_links_desc') }}</p>
          <form class="settings-form app-links-form" @submit.prevent="saveAppLinks">
            <div class="app-links-list">
              <div v-for="(link, index) in appLinks" :key="index" class="app-link-item">
                <div class="app-link-item__icon">{{ link.icon }}</div>
                <div class="app-link-item__fields">
                  <KInput v-model="appLinks[index].name" :placeholder="t('settings.app_link_name')" />
                  <KInput v-model="appLinks[index].url" placeholder="https://..." />
                  <KSelect
                    v-model="appLinks[index].platform"
                    :options="platformOptions"
                    @update:model-value="updatePlatformIcon(index)"
                  />
                </div>
                <KButton variant="ghost" size="sm" type="button" @click="removeAppLink(index)">{{ t('label.remove') }}</KButton>
              </div>
            </div>
            <div class="threshold-actions">
              <KButton variant="ghost" size="sm" type="button" @click="addAppLink">{{ t('settings.add_app_link') }}</KButton>
            </div>
            <KButton type="submit" variant="primary" :loading="savingAppLinks">{{ t('settings.save_app_links') }}</KButton>
          </form>
        </div>
      </template>

      <!-- Telegram Bot -->
      <template #telegram>
        <div class="settings-panel">
          <h4 class="section-title">{{ t('settings.telegram') }}</h4>
          <form class="settings-form" @submit.prevent="saveTelegramSettings">
            <KFormField name="tg-token" :label="t('settings.telegram_token')" hint="Get this from @BotFather">
              <template #default="{ fieldId }">
                <KInput :id="fieldId" v-model="telegramToken" placeholder="123456:ABC-DEF..." type="password" />
              </template>
            </KFormField>
            <KFormField name="tg-chat" :label="t('settings.telegram_chat')">
              <template #default="{ fieldId }">
                <KInput :id="fieldId" v-model="telegramChatId" placeholder="-1001234567890" />
              </template>
            </KFormField>
            <KButton type="submit" variant="primary" size="sm" :loading="savingTelegram">{{ t('settings.save_telegram') }}</KButton>
          </form>
        </div>
      </template>

      <!-- Certificates -->
      <template #certificates>
        <div class="settings-panel">
          <h4 class="section-title">{{ t('settings.ssl_certs') }}</h4>
          <div class="cert-info">
            <div class="cert-item">
              <span class="cert-item__label">{{ t('settings.ca_cert') }}</span>
              <KStatusPill :status="nodesStore.vpnSettings?.ca_exists ? 'active' : 'disabled'" size="sm" />
            </div>
            <div class="cert-item">
              <span class="cert-item__label">{{ t('settings.tls_key') }}</span>
              <KStatusPill :status="nodesStore.vpnSettings?.tls_crypt_exists ? 'active' : 'disabled'" size="sm" />
            </div>
          </div>
          <KButton variant="primary" size="sm" class="mt-3">{{ t('settings.regen_certs') }}</KButton>
        </div>
      </template>

      <!-- Backup -->
      <template #backup>
        <div class="settings-panel">
          <h4 class="section-title">{{ t('settings.backup') }}</h4>
          <p class="text-muted text-sm">{{ t('settings.backup_desc') }}</p>

          <div class="backup-section">
            <h5 class="subsection-title">{{ t('settings.export_backup') }}</h5>
            <p class="text-muted text-sm">{{ t('settings.backup_format') }}</p>
            <KButton variant="primary" size="sm" :loading="exporting" @click="downloadBackup">
              {{ t('settings.export_backup') }}
            </KButton>
          </div>

          <div class="backup-section">
            <h5 class="subsection-title">{{ t('settings.import_backup') }}</h5>
            <p class="text-muted text-sm">{{ t('settings.import_backup_desc') }}</p>
            <input
              ref="importFileInput"
              type="file"
              accept=".json"
              class="hidden-input"
              @change="handleImportFile"
            />
            <KButton variant="ghost" size="sm" :loading="importing" @click="triggerImport">
              {{ t('settings.import_backup') }}
            </KButton>
          </div>
        </div>
      </template>
    </KTabs>
  </div>
</template>

<style scoped>
.settings-view { display: flex; flex-direction: column; gap: var(--space-5); }

.settings-panel { padding: var(--space-5) 0; display: flex; flex-direction: column; gap: var(--space-4); }
.section-title { margin: 0; font-size: var(--text-base); font-weight: var(--font-semibold); }
.subsection-title { margin: 0; font-size: var(--text-sm); font-weight: var(--font-semibold); }

.settings-form { display: flex; flex-direction: column; gap: var(--space-3); max-width: 480px; }

.cert-info { display: flex; flex-direction: column; gap: var(--space-2); }
.cert-item { display: flex; justify-content: space-between; align-items: center; padding: var(--space-3); background: var(--color-surface); border: 1px solid var(--color-border); border-radius: var(--radius-md); max-width: 400px; }
.cert-item__label { font-size: var(--text-sm); }

.thresholds-list { display: flex; flex-direction: column; gap: var(--space-2); }
.threshold-row { display: flex; align-items: flex-end; gap: var(--space-2); }
.threshold-input-group { display: flex; align-items: center; gap: var(--space-2); }
.threshold-unit { font-size: var(--text-sm); color: var(--color-muted); font-weight: var(--font-medium); }
.threshold-actions { display: flex; align-items: center; }

.backup-section { display: flex; flex-direction: column; gap: var(--space-2); padding-top: var(--space-3); border-top: 1px solid var(--color-border); }
.backup-section:first-of-type { border-top: none; padding-top: 0; }

.export-list { display: flex; flex-direction: column; gap: var(--space-2); max-width: 400px; }
.export-item { display: flex; justify-content: space-between; align-items: center; padding: var(--space-2) var(--space-3); background: var(--color-surface); border: 1px solid var(--color-border); border-radius: var(--radius-md); }
.export-item__label { font-size: var(--text-sm); font-weight: var(--font-medium); }

.hidden-input { display: none; }

.text-muted { color: var(--color-muted); }
.text-sm { font-size: var(--text-sm); }
.mt-3 { margin-top: var(--space-3); }

/* Theme Section */
.theme-section { display: flex; flex-direction: column; gap: var(--space-2); margin-top: var(--space-3); padding-top: var(--space-3); border-top: 1px solid var(--color-border); }
.theme-cards { display: grid; grid-template-columns: repeat(auto-fill, minmax(180px, 1fr)); gap: var(--space-2); max-width: 600px; }
.theme-card { display: flex; flex-direction: column; gap: var(--space-2); padding: var(--space-3); background: var(--color-surface); border: 2px solid var(--color-border); border-radius: var(--radius-lg); cursor: pointer; transition: border-color 0.15s, transform 0.15s; text-align: left; }
.theme-card:hover { border-color: var(--color-primary); transform: translateY(-1px); }
.theme-card.active { border-color: var(--color-primary); box-shadow: 0 0 0 3px rgba(37, 99, 235, 0.15); }
.theme-card__swatches { display: flex; gap: 4px; }
.theme-card__swatches .swatch { width: 20px; height: 20px; border-radius: 50%; border: 1px solid rgba(255, 255, 255, 0.1); }
.theme-card__info { display: flex; flex-direction: column; gap: 2px; }
.theme-card__name { font-size: var(--text-sm); font-weight: var(--font-semibold); color: var(--color-text); }
.theme-card__desc { font-size: var(--text-xs); color: var(--color-muted); }

/* Mode Section */
.mode-section { display: flex; flex-direction: column; gap: var(--space-2); margin-top: var(--space-2); }
.mode-radios { display: flex; gap: var(--space-2); flex-wrap: wrap; }
.mode-radio { display: flex; align-items: center; gap: var(--space-2); padding: var(--space-2) var(--space-3); background: var(--color-surface); border: 1px solid var(--color-border); border-radius: var(--radius-md); cursor: pointer; transition: border-color 0.15s; font-size: var(--text-sm); font-weight: var(--font-medium); color: var(--color-text); }
.mode-radio:hover { border-color: var(--color-primary); }
.mode-radio.active { border-color: var(--color-primary); background: rgba(37, 99, 235, 0.05); }
.mode-radio__input { display: none; }
.mode-radio__label { pointer-events: none; }

@media (max-width: 768px) {
  .settings-form { max-width: 100%; }
  .cert-item { max-width: 100%; }
  .export-list { max-width: 100%; }
  .export-item { flex-direction: column; align-items: flex-start; gap: var(--space-2); }
  .threshold-input-group { flex-wrap: wrap; }
  .theme-cards { grid-template-columns: 1fr; max-width: 100%; }
  .mode-radios { flex-direction: column; }
  .app-link-item { flex-direction: column; align-items: stretch; }
  .app-link-item__fields { flex-direction: column; }
}

.warning-subsection { margin-top: var(--space-5); padding-top: var(--space-4); border-top: 1px solid var(--color-border); }

/* App Links */
.app-links-form { max-width: 600px; }
.app-links-list { display: flex; flex-direction: column; gap: var(--space-3); }
.app-link-item { display: flex; align-items: center; gap: var(--space-2); padding: var(--space-3); background: var(--color-surface); border: 1px solid var(--color-border); border-radius: var(--radius-md); }
.app-link-item__icon { font-size: 1.5rem; width: 40px; text-align: center; flex-shrink: 0; }
.app-link-item__fields { display: flex; gap: var(--space-2); flex: 1; min-width: 0; flex-wrap: wrap; }

</style>
