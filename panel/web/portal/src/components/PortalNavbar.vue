<script setup lang="ts">
import { useRouter } from 'vue-router'
import { usePortalAuthStore } from '@/stores/auth'
import { useTheme } from '@koris/composables/useTheme'
import KButton from '@koris/ui/KButton.vue'

const router = useRouter()
const auth = usePortalAuthStore()
const { isDark, toggle: toggleTheme } = useTheme()

async function handleLogout() {
  await auth.logout()
  router.push({ name: 'portal-login' })
}
</script>
<template>
  <header class="portal-navbar">
    <div class="portal-navbar__brand">
      <span class="portal-navbar__logo">K</span>
      <div class="portal-navbar__brand-text">
        <span class="portal-navbar__title">KorisPanel</span>
        <span class="portal-navbar__subtitle">Client Portal</span>
      </div>
    </div>

    <nav class="portal-navbar__nav">
      <router-link :to="{ name: 'portal-dashboard' }" class="portal-navbar__link">Dashboard</router-link>
      <router-link :to="{ name: 'portal-billing' }" class="portal-navbar__link">Billing</router-link>
      <router-link :to="{ name: 'portal-usage' }" class="portal-navbar__link">Usage</router-link>
      <router-link :to="{ name: 'portal-support' }" class="portal-navbar__link">Support</router-link>
      <router-link :to="{ name: 'portal-vpn' }" class="portal-navbar__link">VPN Profiles</router-link>
      <router-link :to="{ name: 'portal-profile' }" class="portal-navbar__link">Profile</router-link>
    </nav>

    <div class="portal-navbar__actions">
      <KButton variant="ghost" size="sm" @click="toggleTheme" :aria-label="isDark ? 'Switch to light theme' : 'Switch to dark theme'">
        {{ isDark ? '☀️' : '🌙' }}
      </KButton>
      <span class="portal-navbar__user">{{ auth.displayName }}</span>
      <KButton variant="ghost" size="sm" @click="handleLogout">
        Logout
      </KButton>
    </div>
  </header>
</template>
<style scoped>
.portal-navbar {
  display: flex;
  align-items: center;
  gap: var(--space-4);
  padding: var(--space-3) var(--space-6);
  border-bottom: 1px solid var(--color-border);
  background: var(--color-surface);
}
.portal-navbar__brand {
  display: flex;
  align-items: center;
  gap: var(--space-2);
}
.portal-navbar__logo {
  width: 34px;
  height: 34px;
  border-radius: var(--radius-md);
  background: var(--gradient-brand);
  display: flex;
  align-items: center;
  justify-content: center;
  color: #fff;
  font-weight: 800;
  font-size: 14px;
}
.portal-navbar__brand-text {
  display: flex;
  flex-direction: column;
}
.portal-navbar__title {
  font-weight: 700;
  font-size: var(--text-sm);
  line-height: 1.2;
}
.portal-navbar__subtitle {
  font-size: var(--text-xs);
  color: var(--color-muted);
}
.portal-navbar__nav {
  display: flex;
  gap: var(--space-1);
  margin-left: var(--space-6);
}
.portal-navbar__link {
  padding: var(--space-2) var(--space-3);
  border-radius: var(--radius-md);
  font-size: var(--text-sm);
  color: var(--color-muted);
  text-decoration: none;
  transition: all var(--duration-fast);
}
.portal-navbar__link:hover {
  color: var(--color-text);
  background: var(--color-surface-2);
}
.portal-navbar__link.router-link-active {
  color: var(--color-primary);
  background: rgba(37, 99, 235, 0.08);
}
.portal-navbar__actions {
  margin-left: auto;
  display: flex;
  align-items: center;
  gap: var(--space-3);
}
.portal-navbar__user {
  font-size: var(--text-sm);
  color: var(--color-muted);
}
@media (max-width: 768px) {
  .portal-navbar { flex-wrap: wrap; }
  .portal-navbar__nav { order: 3; width: 100%; overflow-x: auto; }
}
</style>
