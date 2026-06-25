<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { useRealtimeStore } from '@/stores/realtime'
import { useTheme } from '@koris/composables/useTheme'
import { useI18n } from '@koris/composables/useI18n'
import type { Locale } from '@koris/composables/useI18n'
import { openCommandPalette } from '@/composables/useCommandPalette'
import TheSidebar from '@/components/TheSidebar.vue'
import TheTopbar from '@/components/TheTopbar.vue'
import UpdateBanner from '@/components/UpdateBanner.vue'
import CommandPalette from '@/components/CommandPalette.vue'
import ToastProvider from '@/components/ToastProvider.vue'
import KConfirmDialog from '@koris/ui/KConfirmDialog.vue'

const router = useRouter()
const route = useRoute()
const authStore = useAuthStore()
const realtimeStore = useRealtimeStore()
const { toggle: toggleTheme } = useTheme()
const { t, setLocale } = useI18n()

// Panel version (fetched from API)
const panelVersion = ref('dev')

onMounted(async () => {
  try {
    const res = await fetch('/api/health')
    if (res.ok) {
      const data = await res.json()
      if (data.version) {
        panelVersion.value = data.version
      }
    }
  } catch {
    // Fallback to 'dev' silently
  }
})

// Sidebar state
const sidebarCollapsed = ref(false)
const mobileMenuOpen = ref(false)

function handleCollapseToggle() {
  sidebarCollapsed.value = !sidebarCollapsed.value
}

function toggleMobileMenu() {
  mobileMenuOpen.value = !mobileMenuOpen.value
}

function closeMobileMenu() {
  mobileMenuOpen.value = false
}

function handleChangeLang(locale: string) {
  setLocale(locale as Locale)
}

// Derive current route name for sidebar highlighting
const currentRoute = computed(() => (route.name as string) || 'overview')

// Breadcrumbs from route matched hierarchy
const breadcrumbs = computed(() => {
  const crumbs: { label: string; to?: string }[] = []
  const matched = route.matched

  for (const record of matched) {
    if (record.name && record.name !== route.name) {
      crumbs.push({
        label: String(record.name).charAt(0).toUpperCase() + String(record.name).slice(1),
        to: record.path || undefined,
      })
    }
  }

  // Current page (non-clickable)
  if (route.name) {
    const name = String(route.name)
    crumbs.push({
      label: name.charAt(0).toUpperCase() + name.slice(1).replace(/-/g, ' '),
    })
  }

  return crumbs
})

// Page title derived from current route
const pageTitle = computed(() => {
  const name = String(route.name || 'overview')
  return name.charAt(0).toUpperCase() + name.slice(1).replace(/-/g, ' ')
})

// Sidebar navigation
function handleNavigate(routeName: string) {
  router.push({ name: routeName })
  mobileMenuOpen.value = false
}

async function handleLogout() {
  realtimeStore.disconnect()
  await authStore.logout()
  router.push({ name: 'login' })
}

function handleNotifications() {
  realtimeStore.markAllRead()
  router.push({ name: 'notifications' })
}
</script>

<template>
  <div class="app-shell" :class="{ 'sidebar-collapsed': sidebarCollapsed, 'mobile-open': mobileMenuOpen }">
    <!-- Mobile hamburger button -->
    <button class="mobile-menu-btn" aria-label="Toggle menu" @click="toggleMobileMenu">
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
        <path v-if="!mobileMenuOpen" d="M4 6h16M4 12h16M4 18h16" />
        <path v-else d="M6 18L18 6M6 6l12 12" />
      </svg>
    </button>

    <!-- Mobile overlay -->
    <div v-if="mobileMenuOpen" class="mobile-overlay" @click="closeMobileMenu"></div>

    <TheSidebar
      :collapsed="sidebarCollapsed"
      :current-route="currentRoute"
      :version="panelVersion"
      :user="{ username: authStore.username, role: authStore.role }"
      class="sidebar-wrapper"
      @navigate="handleNavigate"
      @collapse-toggle="handleCollapseToggle"
      @logout="handleLogout"
      @toggle-theme="toggleTheme"
    />

    <main class="main" role="main">
      <TheTopbar
        :title="pageTitle"
        :breadcrumbs="breadcrumbs"
        :notification-count="realtimeStore.notificationCount"
        @toggle-theme="toggleTheme"
        @open-notifications="handleNotifications"
        @open-command-palette="openCommandPalette"
        @change-lang="handleChangeLang"
      />

      <UpdateBanner />

      <router-view v-slot="{ Component, route: viewRoute }">
        <Transition name="fade" mode="out-in">
          <Suspense>
            <component :is="Component" :key="viewRoute.path" />

            <template #fallback>
              <div class="page-skeleton">
                <div class="skeleton skeleton-card"></div>
                <div class="skeleton skeleton-card"></div>
                <div class="skeleton skeleton-card"></div>
              </div>
            </template>
          </Suspense>
        </Transition>
      </router-view>
    </main>

    <CommandPalette />
    <ToastProvider />
    <KConfirmDialog />
  </div>
</template>

<style scoped>
.app-shell {
  display: grid;
  grid-template-columns: auto 1fr;
  min-height: 100vh;
  width: 100%;
  background: var(--color-bg, #070a12);
}

.app-shell.sidebar-collapsed {
  grid-template-columns: 64px 1fr;
}

.main {
  display: flex;
  flex-direction: column;
  padding: var(--space-5, 20px) var(--space-6, 24px);
  overflow-y: auto;
  height: 100vh;
}

/* Fade transition for view changes */
.fade-enter-active,
.fade-leave-active {
  transition: opacity var(--duration-normal, 0.2s) var(--ease-default, ease),
              transform var(--duration-normal, 0.2s) var(--ease-default, ease);
}

.fade-enter-from {
  opacity: 0;
  transform: translateY(6px);
}

.fade-leave-to {
  opacity: 0;
  transform: translateY(-6px);
}

/* Skeleton fallback during lazy-load */
.page-skeleton {
  display: flex;
  flex-direction: column;
  gap: var(--space-4, 16px);
  padding: var(--space-4, 16px) 0;
}

.skeleton {
  background: linear-gradient(
    90deg,
    var(--color-surface, #0b1120) 25%,
    var(--color-surface-2, #1e2630) 50%,
    var(--color-surface, #0b1120) 75%
  );
  background-size: 200% 100%;
  animation: skeletonShimmer 1.5s ease-in-out infinite;
  border-radius: var(--radius-lg, 10px);
}

.skeleton-card {
  height: 120px;
  width: 100%;
}

.skeleton-card:first-child {
  height: 180px;
}

@keyframes skeletonShimmer {
  0% {
    background-position: 200% 0;
  }
  100% {
    background-position: -200% 0;
  }
}

/* Reduced motion preference */
@media (prefers-reduced-motion: reduce) {
  .fade-enter-active,
  .fade-leave-active {
    transition: none;
  }

  .skeleton {
    animation: none;
  }
}

/* Mobile hamburger button */
.mobile-menu-btn {
  display: none;
  position: fixed;
  top: 12px;
  left: 12px;
  z-index: 1100;
  width: 40px;
  height: 40px;
  border: none;
  border-radius: var(--radius-md, 8px);
  background: var(--color-surface, #0b1120);
  border: 1px solid var(--color-border, #28333f);
  color: var(--color-text, #e6edf3);
  cursor: pointer;
  align-items: center;
  justify-content: center;
}

.mobile-menu-btn svg {
  width: 20px;
  height: 20px;
}

/* Mobile overlay */
.mobile-overlay {
  display: none;
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.5);
  z-index: 999;
}

/* Mobile responsive: sidebar collapses to hamburger */
@media (max-width: 768px) {
  .app-shell {
    grid-template-columns: 1fr;
  }

  .app-shell.sidebar-collapsed {
    grid-template-columns: 1fr;
  }

  .mobile-menu-btn {
    display: flex;
  }

  .mobile-overlay {
    display: block;
  }

  .sidebar-wrapper {
    position: fixed;
    top: 0;
    left: 0;
    height: 100vh;
    z-index: 1000;
    transform: translateX(-100%);
    transition: transform 0.25s ease;
  }

  .mobile-open .sidebar-wrapper {
    transform: translateX(0);
  }

  .main {
    padding: var(--space-4, 16px);
    padding-top: 60px;
  }
}
</style>
