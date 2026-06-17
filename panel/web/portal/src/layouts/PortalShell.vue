<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { usePortalAuthStore } from '@/stores/auth'
import { useTheme } from '@koris/composables/useTheme'
import NotificationCenter from '@/components/NotificationCenter.vue'

const router = useRouter()
const auth = usePortalAuthStore()
const { isDark, toggle: toggleTheme } = useTheme()

const userMenuOpen = ref(false)
const mobileMenuOpen = ref(false)

function toggleUserMenu() {
  userMenuOpen.value = !userMenuOpen.value
}

function closeUserMenu() {
  userMenuOpen.value = false
}

function toggleMobileMenu() {
  mobileMenuOpen.value = !mobileMenuOpen.value
}

function closeMobileMenu() {
  mobileMenuOpen.value = false
}

function goToProfile() {
  closeUserMenu()
  router.push({ name: 'portal-profile' })
}

async function logout() {
  closeUserMenu()
  await auth.logout()
  router.push({ name: 'portal-login' })
}
</script>
<template>
  <div class="portal-shell">
    <header class="portal-nav">
      <div class="portal-nav__brand"><span class="portal-nav__logo">K</span><span class="portal-nav__title">KorisPanel</span></div>

      <!-- Hamburger button for mobile -->
      <button class="portal-nav__hamburger" @click="toggleMobileMenu" aria-label="Toggle menu">
        <svg v-if="!mobileMenuOpen" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" width="22" height="22">
          <path d="M3 6h18M3 12h18M3 18h18" stroke-linecap="round"/>
        </svg>
        <svg v-else viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" width="22" height="22">
          <path d="M6 6l12 12M6 18L18 6" stroke-linecap="round"/>
        </svg>
      </button>

      <!-- Desktop nav links -->
      <nav class="portal-nav__links">
        <router-link :to="{ name: 'portal-dashboard' }">
          <svg class="portal-nav__icon" viewBox="0 0 20 20" fill="currentColor" width="16" height="16"><path d="M3 3h6v6H3V3zm8 0h6v6h-6V3zm-8 8h6v6H3v-6zm8 0h6v6h-6v-6z"/></svg>
          Dashboard
        </router-link>
        <router-link :to="{ name: 'portal-support' }">
          <svg class="portal-nav__icon" viewBox="0 0 20 20" fill="currentColor" width="16" height="16"><path d="M2 5a2 2 0 012-2h12a2 2 0 012 2v7a2 2 0 01-2 2H6l-4 4V5z"/></svg>
          Support
        </router-link>
        <router-link :to="{ name: 'portal-vpn' }">
          <svg class="portal-nav__icon" viewBox="0 0 20 20" fill="currentColor" width="16" height="16"><path d="M10 2a6 6 0 00-6 6c0 2.21 1.2 4.14 3 5.18V17a1 1 0 001 1h4a1 1 0 001-1v-3.82A5.99 5.99 0 0016 8a6 6 0 00-6-6zm0 2a4 4 0 014 4c0 1.48-.8 2.77-2 3.46V16H8v-4.54A3.99 3.99 0 016 8a4 4 0 014-4z"/></svg>
          My VPN
        </router-link>
      </nav>

      <div class="portal-nav__actions">
        <NotificationCenter />
        <button @click="toggleTheme" class="portal-nav__btn">{{ isDark ? '☀️' : '🌙' }}</button>
        <div class="portal-nav__user-menu">
          <button class="portal-nav__user-toggle" @click="toggleUserMenu">
            <span class="portal-nav__user">{{ auth.user?.username }}</span>
            <svg class="portal-nav__chevron" :class="{ 'portal-nav__chevron--open': userMenuOpen }" viewBox="0 0 20 20" fill="currentColor" width="16" height="16">
              <path fill-rule="evenodd" d="M5.23 7.21a.75.75 0 011.06.02L10 11.168l3.71-3.938a.75.75 0 111.08 1.04l-4.25 4.5a.75.75 0 01-1.08 0l-4.25-4.5a.75.75 0 01.02-1.06z" clip-rule="evenodd" />
            </svg>
          </button>
          <div v-if="userMenuOpen" class="portal-nav__dropdown-backdrop" @click="closeUserMenu"></div>
          <div v-if="userMenuOpen" class="portal-nav__dropdown">
            <div class="portal-nav__dropdown-header">{{ auth.user?.username }}</div>
            <button class="portal-nav__dropdown-item" @click="goToProfile">Profile Settings</button>
            <button class="portal-nav__dropdown-item portal-nav__dropdown-item--danger" @click="logout">Logout</button>
          </div>
        </div>
      </div>
    </header>

    <!-- Mobile nav overlay -->
    <div v-if="mobileMenuOpen" class="portal-mobile-backdrop" @click="closeMobileMenu"></div>
    <nav v-if="mobileMenuOpen" class="portal-mobile-nav">
      <router-link :to="{ name: 'portal-dashboard' }" @click="closeMobileMenu">
        <svg class="portal-nav__icon" viewBox="0 0 20 20" fill="currentColor" width="18" height="18"><path d="M3 3h6v6H3V3zm8 0h6v6h-6V3zm-8 8h6v6H3v-6zm8 0h6v6h-6v-6z"/></svg>
        Dashboard
      </router-link>
      <router-link :to="{ name: 'portal-support' }" @click="closeMobileMenu">
        <svg class="portal-nav__icon" viewBox="0 0 20 20" fill="currentColor" width="18" height="18"><path d="M2 5a2 2 0 012-2h12a2 2 0 012 2v7a2 2 0 01-2 2H6l-4 4V5z"/></svg>
        Support
      </router-link>
      <router-link :to="{ name: 'portal-vpn' }" @click="closeMobileMenu">
        <svg class="portal-nav__icon" viewBox="0 0 20 20" fill="currentColor" width="18" height="18"><path d="M10 2a6 6 0 00-6 6c0 2.21 1.2 4.14 3 5.18V17a1 1 0 001 1h4a1 1 0 001-1v-3.82A5.99 5.99 0 0016 8a6 6 0 00-6-6zm0 2a4 4 0 014 4c0 1.48-.8 2.77-2 3.46V16H8v-4.54A3.99 3.99 0 016 8a4 4 0 014-4z"/></svg>
        My VPN
      </router-link>
    </nav>

    <!-- Mobile bottom tab bar -->
    <nav class="portal-bottom-tabs">
      <router-link :to="{ name: 'portal-dashboard' }" class="portal-bottom-tabs__item">
        <svg viewBox="0 0 20 20" fill="currentColor" width="20" height="20"><path d="M3 3h6v6H3V3zm8 0h6v6h-6V3zm-8 8h6v6H3v-6zm8 0h6v6h-6v-6z"/></svg>
        <span>Dashboard</span>
      </router-link>
      <router-link :to="{ name: 'portal-support' }" class="portal-bottom-tabs__item">
        <svg viewBox="0 0 20 20" fill="currentColor" width="20" height="20"><path d="M2 5a2 2 0 012-2h12a2 2 0 012 2v7a2 2 0 01-2 2H6l-4 4V5z"/></svg>
        <span>Support</span>
      </router-link>
      <router-link :to="{ name: 'portal-vpn' }" class="portal-bottom-tabs__item">
        <svg viewBox="0 0 20 20" fill="currentColor" width="20" height="20"><path d="M10 2a6 6 0 00-6 6c0 2.21 1.2 4.14 3 5.18V17a1 1 0 001 1h4a1 1 0 001-1v-3.82A5.99 5.99 0 0016 8a6 6 0 00-6-6zm0 2a4 4 0 014 4c0 1.48-.8 2.77-2 3.46V16H8v-4.54A3.99 3.99 0 016 8a4 4 0 014-4z"/></svg>
        <span>My VPN</span>
      </router-link>
    </nav>

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
.portal-nav__links a { display:flex;align-items:center;gap:var(--space-2);padding:var(--space-2) var(--space-3);border-radius:var(--radius-md);font-size:var(--text-sm);color:var(--color-muted);text-decoration:none;transition:all var(--duration-fast); }
.portal-nav__links a:hover { color:var(--color-text);background:var(--color-surface-2); }
.portal-nav__links a.router-link-active { color:var(--color-primary);background:rgba(37,99,235,0.08); }
.portal-nav__icon { flex-shrink:0; }
.portal-nav__actions { margin-left:auto;display:flex;align-items:center;gap:var(--space-3); }
.portal-nav__user { font-size:var(--text-sm);color:var(--color-muted); }
.portal-nav__btn { background:none;border:none;color:var(--color-muted);cursor:pointer;font-size:var(--text-sm);padding:var(--space-1) var(--space-2);border-radius:var(--radius-sm); }
.portal-nav__btn:hover { color:var(--color-text); }
.portal-nav__user-menu { position:relative; }
.portal-nav__user-toggle { display:flex;align-items:center;gap:var(--space-1);background:none;border:none;cursor:pointer;padding:var(--space-1) var(--space-2);border-radius:var(--radius-sm);transition:background var(--duration-fast); }
.portal-nav__user-toggle:hover { background:var(--color-surface-2); }
.portal-nav__chevron { transition:transform var(--duration-fast);color:var(--color-muted); }
.portal-nav__chevron--open { transform:rotate(180deg); }
.portal-nav__dropdown { position:absolute;top:calc(100% + var(--space-2));right:0;min-width:180px;background:var(--color-surface);border:1px solid var(--color-border);border-radius:var(--radius-md);box-shadow:0 4px 12px rgba(0,0,0,0.1);z-index:100;overflow:hidden; }
.portal-nav__dropdown-backdrop { position:fixed;inset:0;z-index:99; }
.portal-nav__dropdown-header { padding:var(--space-3) var(--space-4);font-size:var(--text-xs);color:var(--color-muted);border-bottom:1px solid var(--color-border);font-weight:500; }
.portal-nav__dropdown-item { display:block;width:100%;padding:var(--space-3) var(--space-4);font-size:var(--text-sm);color:var(--color-text);background:none;border:none;text-align:left;cursor:pointer;transition:background var(--duration-fast); }
.portal-nav__dropdown-item:hover { background:var(--color-surface-2); }
.portal-nav__dropdown-item--danger { color:var(--color-danger); }
.portal-nav__dropdown-item--danger:hover { background:var(--color-danger-bg, #fef2f2); }
.portal-main { padding:var(--space-6);max-width:1200px;margin:0 auto; }
.fade-enter-active, .fade-leave-active { transition:opacity 0.2s ease; }
.fade-enter-from, .fade-leave-to { opacity:0; }

/* Hamburger button - hidden on desktop */
.portal-nav__hamburger { display:none;background:none;border:none;color:var(--color-text);cursor:pointer;padding:var(--space-1);border-radius:var(--radius-sm);margin-left:auto; }
.portal-nav__hamburger:hover { background:var(--color-surface-2); }

/* Mobile nav overlay */
.portal-mobile-backdrop { position:fixed;inset:0;background:rgba(0,0,0,0.3);z-index:200; }
.portal-mobile-nav { display:none;position:fixed;top:60px;left:0;right:0;background:var(--color-surface);border-bottom:1px solid var(--color-border);z-index:201;padding:var(--space-3);flex-direction:column;gap:var(--space-1);box-shadow:0 4px 12px rgba(0,0,0,0.1); }
.portal-mobile-nav a { display:flex;align-items:center;gap:var(--space-3);padding:var(--space-3) var(--space-4);border-radius:var(--radius-md);font-size:var(--text-md);color:var(--color-text);text-decoration:none;transition:background var(--duration-fast); }
.portal-mobile-nav a:hover { background:var(--color-surface-2); }
.portal-mobile-nav a.router-link-active { color:var(--color-primary);background:rgba(37,99,235,0.08); }

/* Bottom tab bar - hidden on desktop */
.portal-bottom-tabs { display:none;position:fixed;bottom:0;left:0;right:0;background:var(--color-surface);border-top:1px solid var(--color-border);z-index:150;padding:var(--space-2) 0;padding-bottom:env(safe-area-inset-bottom, var(--space-2)); }
.portal-bottom-tabs__item { display:flex;flex-direction:column;align-items:center;gap:2px;flex:1;padding:var(--space-1) 0;color:var(--color-muted);text-decoration:none;font-size:11px;transition:color var(--duration-fast); }
.portal-bottom-tabs__item:hover,
.portal-bottom-tabs__item.router-link-active { color:var(--color-primary); }

/* Mobile responsive: < 768px */
@media (max-width: 767px) {
  .portal-nav__links { display:none; }
  .portal-nav__hamburger { display:flex; }
  .portal-mobile-nav { display:flex; }
  .portal-bottom-tabs { display:flex; }
  .portal-main { padding:var(--space-4);padding-bottom:80px; }
  .portal-nav__user { display:none; }
  .portal-nav__title { display:none; }
}
</style>
