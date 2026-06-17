<script setup lang="ts">
import { ref, computed } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { useRealtimeStore } from '@/stores/realtime'
import { useTheme } from '@koris/composables/useTheme'
import { useI18n } from '@koris/composables/useI18n'
import { openCommandPalette } from '@/composables/useCommandPalette'
import TheSidebar from '@/components/TheSidebar.vue'
import TheTopbar from '@/components/TheTopbar.vue'
import CommandPalette from '@/components/CommandPalette.vue'

const router = useRouter()
const route = useRoute()
const authStore = useAuthStore()
const realtimeStore = useRealtimeStore()
const { toggle: toggleTheme } = useTheme()
const { t } = useI18n()

// Sidebar state
const sidebarCollapsed = ref(false)

function handleCollapseToggle() {
  sidebarCollapsed.value = !sidebarCollapsed.value
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
}

async function handleLogout() {
  realtimeStore.disconnect()
  await authStore.logout()
  router.push({ name: 'login' })
}

function handleNotifications() {
  realtimeStore.markAllRead()
  router.push({ name: 'tickets' })
}
</script>

<template>
  <div class="app-shell" :class="{ 'sidebar-collapsed': sidebarCollapsed }">
    <TheSidebar
      :collapsed="sidebarCollapsed"
      :current-route="currentRoute"
      :user="{ username: authStore.username, role: authStore.role }"
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
      />

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
</style>
