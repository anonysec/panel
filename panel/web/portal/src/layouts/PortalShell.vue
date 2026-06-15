<script setup lang="ts">
import { useRouter } from 'vue-router'
import { usePortalAuthStore } from '@/stores/auth'
import { useTheme } from '@koris/composables/useTheme'

const router = useRouter()
const auth = usePortalAuthStore()
const { isDark, toggle: toggleTheme } = useTheme()

async function logout() {
  await auth.logout()
  router.push({ name: 'portal-login' })
}
</script>
<template>
  <div class="portal-shell">
    <header class="portal-nav">
      <div class="portal-nav__brand"><span class="portal-nav__logo">K</span><span class="portal-nav__title">KorisPanel</span></div>
      <nav class="portal-nav__links">
        <router-link :to="{ name: 'portal-dashboard' }">Dashboard</router-link>
        <router-link :to="{ name: 'portal-billing' }">Billing</router-link>
        <router-link :to="{ name: 'portal-usage' }">Usage</router-link>
        <router-link :to="{ name: 'portal-support' }">Support</router-link>
        <router-link :to="{ name: 'portal-vpn' }">VPN Profiles</router-link>
        <router-link :to="{ name: 'portal-profile' }">Profile</router-link>
      </nav>
      <div class="portal-nav__actions">
        <button @click="toggleTheme" class="portal-nav__btn">{{ isDark ? '☀️' : '🌙' }}</button>
        <span class="portal-nav__user">{{ auth.user?.username }}</span>
        <button @click="logout" class="portal-nav__btn portal-nav__btn--logout">Logout</button>
      </div>
    </header>
    <main class="portal-main">
      <router-view v-slot="{ Component }">
        <transition name="fade" mode="out-in">
          <component :is="Component" />
        </transition>
      </router-view>
    </main>
  </div>
</template>
<style scoped>
.portal-shell { min-height:100vh;background:var(--color-bg); }
.portal-nav { display:flex;align-items:center;gap:var(--space-4);padding:var(--space-3) var(--space-6);border-bottom:1px solid var(--color-border);background:var(--color-surface); }
.portal-nav__brand { display:flex;align-items:center;gap:var(--space-2); }
.portal-nav__logo { width:32px;height:32px;border-radius:var(--radius-md);background:var(--gradient-brand);display:flex;align-items:center;justify-content:center;color:#fff;font-weight:800;font-size:14px; }
.portal-nav__title { font-weight:700;font-size:var(--text-md); }
.portal-nav__links { display:flex;gap:var(--space-1);margin-left:var(--space-6); }
.portal-nav__links a { padding:var(--space-2) var(--space-3);border-radius:var(--radius-md);font-size:var(--text-sm);color:var(--color-muted);text-decoration:none;transition:all var(--duration-fast); }
.portal-nav__links a:hover { color:var(--color-text);background:var(--color-surface-2); }
.portal-nav__links a.router-link-active { color:var(--color-primary);background:rgba(37,99,235,0.08); }
.portal-nav__actions { margin-left:auto;display:flex;align-items:center;gap:var(--space-3); }
.portal-nav__user { font-size:var(--text-sm);color:var(--color-muted); }
.portal-nav__btn { background:none;border:none;color:var(--color-muted);cursor:pointer;font-size:var(--text-sm);padding:var(--space-1) var(--space-2);border-radius:var(--radius-sm); }
.portal-nav__btn:hover { color:var(--color-text); }
.portal-nav__btn--logout { color:var(--color-danger); }
.portal-main { padding:var(--space-6);max-width:1200px;margin:0 auto; }
.fade-enter-active, .fade-leave-active { transition:opacity 0.2s ease; }
.fade-enter-from, .fade-leave-to { opacity:0; }
</style>
