<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useApi } from '@koris/composables/useApi'
import { useClipboard } from '@koris/composables/useClipboard'
import KButton from '@koris/ui/KButton.vue'
import KStatusPill from '@koris/ui/KStatusPill.vue'
import KSkeleton from '@koris/ui/KSkeleton.vue'
import KEmptyState from '@koris/ui/KEmptyState.vue'
import { usePortalAuthStore } from '@/stores/auth'

interface VpnProfile {
  type: string
  name: string
  filename: string
  available: boolean
  remote: string
  port: number
  protocol: string
  node: string
  download: string
}

interface ProfilesResponse {
  ok: boolean
  profiles: VpnProfile[]
}

const { get, loading } = useApi()
const { copy, copied } = useClipboard()
const auth = usePortalAuthStore()

const profiles = ref<VpnProfile[]>([])

const subUrl = ref('')

onMounted(async () => {
  try {
    const res = await get<ProfilesResponse>('/api/portal/profiles')
    profiles.value = res.profiles || []
  } catch {
    // Preserve empty state
  }

  // Build subscription URL
  if (auth.user?.sub_token) {
    subUrl.value = `${window.location.origin}/sub/${auth.user.sub_token}`
  }
})

function handleCopySubUrl() {
  if (subUrl.value) {
    copy(subUrl.value)
  }
}

function getProfileIcon(type: string): string {
  switch (type) {
    case 'openvpn': return '🔐'
    case 'l2tp': return '🔒'
    case 'ikev2': return '🛡️'
    default: return '📄'
  }
}
</script>
<template>
  <div class="vpn-profiles">
    <h1 class="vpn-profiles__title">VPN Profiles</h1>

    <KSkeleton v-if="loading && !profiles.length" type="card" :count="3" />

    <template v-else>
      <!-- Subscription URL -->
      <section v-if="subUrl" class="vpn-profiles__section">
        <h2 class="vpn-profiles__section-title">Subscription URL</h2>
        <p class="vpn-profiles__section-desc">Use this URL to auto-configure VPN clients that support subscription links.</p>
        <div class="vpn-profiles__sub-url">
          <input
            type="text"
            :value="subUrl"
            class="vpn-profiles__url-input"
            readonly
          />
          <KButton variant="primary" size="sm" @click="handleCopySubUrl">
            {{ copied ? 'Copied!' : 'Copy' }}
          </KButton>
        </div>
      </section>

      <!-- Profiles Grid -->
      <section class="vpn-profiles__section">
        <h2 class="vpn-profiles__section-title">Available Profiles</h2>

        <KEmptyState
          v-if="!profiles.length"
          title="No profiles available"
          description="VPN configuration profiles will appear here when available."
          icon="📡"
        />

        <div v-else class="vpn-profiles__grid">
          <div v-for="profile in profiles" :key="profile.type" class="profile-card">
            <div class="profile-card__icon">{{ getProfileIcon(profile.type) }}</div>
            <div class="profile-card__info">
              <h3 class="profile-card__name">{{ profile.name }}</h3>
              <div class="profile-card__meta">
                <span>{{ profile.remote }}:{{ profile.port }}</span>
                <span>{{ profile.protocol }}</span>
                <span>{{ profile.node }}</span>
              </div>
            </div>
            <div class="profile-card__actions">
              <KStatusPill :status="profile.available ? 'active' : 'disabled'">
                {{ profile.available ? 'Available' : 'Unavailable' }}
              </KStatusPill>
              <a
                v-if="profile.available"
                :href="profile.download"
                download
                class="profile-card__download"
              >
                <KButton variant="primary" size="sm">
                  Download
                </KButton>
              </a>
              <KButton v-else variant="ghost" size="sm" :disabled="true">
                N/A
              </KButton>
            </div>
          </div>
        </div>
      </section>
    </template>
  </div>
</template>
<style scoped>
.vpn-profiles__title {
  font-size: var(--text-2xl);
  font-weight: 700;
  margin-bottom: var(--space-6);
}
.vpn-profiles__section {
  padding: var(--space-5);
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-lg);
  margin-bottom: var(--space-4);
}
.vpn-profiles__section-title {
  font-size: var(--text-md);
  font-weight: 600;
  margin-bottom: var(--space-2);
}
.vpn-profiles__section-desc {
  font-size: var(--text-sm);
  color: var(--color-muted);
  margin-bottom: var(--space-4);
}
.vpn-profiles__sub-url {
  display: flex;
  gap: var(--space-2);
  align-items: center;
}
.vpn-profiles__url-input {
  flex: 1;
  padding: var(--space-2) var(--space-3);
  background: var(--color-bg);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  color: var(--color-text);
  font-size: var(--text-sm);
  font-family: monospace;
}
.vpn-profiles__grid {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}
.profile-card {
  display: flex;
  align-items: center;
  gap: var(--space-4);
  padding: var(--space-4);
  background: var(--color-bg);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
}
.profile-card__icon {
  font-size: 1.5rem;
  width: 48px;
  height: 48px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--color-surface-2);
  border-radius: var(--radius-md);
}
.profile-card__info {
  flex: 1;
}
.profile-card__name {
  font-size: var(--text-sm);
  font-weight: 600;
  margin-bottom: var(--space-1);
}
.profile-card__meta {
  display: flex;
  gap: var(--space-3);
  font-size: var(--text-xs);
  color: var(--color-muted);
}
.profile-card__actions {
  display: flex;
  align-items: center;
  gap: var(--space-3);
}
.profile-card__download {
  text-decoration: none;
}
</style>
