<script setup lang="ts">
import { computed, onMounted, onBeforeUnmount, ref, watch, nextTick } from 'vue'
import Sortable from 'sortablejs'
import { useI18n } from '@koris/composables/useI18n'
import { useTheme } from '@koris/composables/useTheme'
import { useEditionStore } from '@/stores/edition'
import { useSidebarStore } from '@/stores/sidebar'
import type { MenuItem } from '@/stores/sidebar'

export interface Props {
  collapsed?: boolean
  currentRoute: string
  user: { username: string; role: string }
  version?: string
  notificationCount?: number
}

const props = withDefaults(defineProps<Props>(), {
  collapsed: false,
  version: 'dev',
  notificationCount: 0,
})

const emit = defineEmits<{
  navigate: [route: string]
  'collapse-toggle': []
  logout: []
  'toggle-theme': []
}>()

const { t, locale } = useI18n()
const { isDark } = useTheme()
const editionStore = useEditionStore()
const sidebarStore = useSidebarStore()
const isFull = computed(() => editionStore.isFull)

// ─── SortableJS instances ─────────────────────────────────────────────────
const categoriesContainerRef = ref<HTMLElement | null>(null)
let categorySortable: Sortable | null = null
const itemSortables: Sortable[] = []

onMounted(() => {
  editionStore.fetchEdition()
  // Initialize sidebar store after navGroups is computed
  nextTick(() => {
    initSidebarStore()
    initSortable()
  })
})

onBeforeUnmount(() => {
  destroySortables()
})

// Watch collapsed state — disable/enable drag
watch(() => props.collapsed, (isCollapsed) => {
  if (isCollapsed) {
    destroySortables()
  } else {
    nextTick(() => initSortable())
  }
})

// ─── SortableJS Helpers ───────────────────────────────────────────────────

function destroySortables() {
  categorySortable?.destroy()
  categorySortable = null
  itemSortables.forEach(s => s.destroy())
  itemSortables.length = 0
}

function initSidebarStore() {
  if (sidebarStore.initialized) return

  const groups = navGroups.value
  const defaultCategories = groups.map(g => g.id)
  const menuItems: MenuItem[] = []

  for (const group of groups) {
    for (const item of group.items) {
      menuItems.push({ id: item.route, categoryId: group.id })
    }
  }

  sidebarStore.initialize(menuItems, defaultCategories)
}

function initSortable() {
  destroySortables()

  // Don't enable drag when collapsed
  if (props.collapsed) return

  const container = categoriesContainerRef.value
  if (!container) return

  // Category-level sortable (drag entire category groups)
  categorySortable = Sortable.create(container, {
    animation: 150,
    handle: '.nav-group-handle',
    ghostClass: 'sortable-ghost',
    chosenClass: 'sortable-chosen',
    dragClass: 'sortable-drag',
    onEnd(evt) {
      if (evt.oldIndex == null || evt.newIndex == null) return
      const categoryIds = orderedGroups.value.map(g => g.id)
      const moved = categoryIds.splice(evt.oldIndex, 1)[0]
      categoryIds.splice(evt.newIndex, 0, moved)
      sidebarStore.reorderCategories(categoryIds)
    },
  })

  // Item-level sortables (one per category)
  const itemContainers = container.querySelectorAll<HTMLElement>('[data-category-id]')
  itemContainers.forEach(el => {
    const categoryId = el.getAttribute('data-category-id')!
    const sortable = Sortable.create(el, {
      animation: 150,
      group: 'sidebar-items',  // Allow cross-category drag
      ghostClass: 'sortable-ghost',
      chosenClass: 'sortable-chosen',
      dragClass: 'sortable-drag',
      onEnd(evt) {
        if (evt.oldIndex == null || evt.newIndex == null) return

        const fromCategoryId = evt.from.getAttribute('data-category-id')!
        const toCategoryId = evt.to.getAttribute('data-category-id')!

        if (fromCategoryId === toCategoryId) {
          // Same category reorder
          const items = getItemsForCategory(fromCategoryId)
          const itemIds = items.map(i => i.route)
          const moved = itemIds.splice(evt.oldIndex, 1)[0]
          itemIds.splice(evt.newIndex, 0, moved)
          sidebarStore.reorderItems(fromCategoryId, itemIds)
        } else {
          // Cross-category move
          const fromItems = getItemsForCategory(fromCategoryId)
          const fromIds = fromItems.map(i => i.route)
          const [movedId] = fromIds.splice(evt.oldIndex, 1)
          sidebarStore.reorderItems(fromCategoryId, fromIds)

          const toItems = getItemsForCategory(toCategoryId)
          const toIds = toItems.map(i => i.route)
          toIds.splice(evt.newIndex, 0, movedId)
          sidebarStore.reorderItems(toCategoryId, toIds)
        }
      },
    })
    itemSortables.push(sortable)
  })
}

function getItemsForCategory(categoryId: string): NavItem[] {
  const group = navGroups.value.find(g => g.id === categoryId)
  if (!group) return []
  const savedOrder = sidebarStore.itemOrder[categoryId]
  if (!savedOrder) return group.items
  // Sort items according to saved order
  const orderMap = new Map(savedOrder.map((id, idx) => [id, idx]))
  return [...group.items].sort((a, b) => {
    const ai = orderMap.get(a.route) ?? Infinity
    const bi = orderMap.get(b.route) ?? Infinity
    return ai - bi
  })
}

/** Derive user initials from username */
const initials = computed(() =>
  (props.user.username || 'K').slice(0, 2).toUpperCase()
)

/** Navigation items organized by section */
interface NavItem {
  route: string
  label: string
  badge?: number
  icon: string
}

interface NavGroup {
  id: string
  title: string
  items: NavItem[]
}

const navGroups = computed<NavGroup[]>(() => {
  const isReseller = props.user.role === 'reseller'

  const groups: NavGroup[] = []

  if (isReseller) {
    // Reseller navigation
    groups.push({
      id: 'overview',
      title: t('nav.group_overview'),
      items: [
        { route: 'reseller-dashboard', label: t('nav.dashboard'), icon: 'dashboard' },
      ],
    })
    groups.push({
      id: 'manage',
      title: t('nav.group_manage'),
      items: [
        { route: 'users', label: t('nav.users'), icon: 'users' },
        { route: 'reseller-plans', label: t('nav.plans'), icon: 'plans' },
        { route: 'reseller-transactions', label: t('nav.transactions'), icon: 'transactions' },
        { route: 'reseller-tickets', label: t('nav.tickets'), icon: 'tickets' },
      ],
    })
    groups.push({
      id: 'system',
      title: t('nav.group_system'),
      items: [
        { route: 'reseller-settings', label: t('nav.settings'), icon: 'settings' },
      ],
    })
    return groups
  }

  // Admin navigation
  groups.push({
    id: 'overview',
    title: t('nav.group_overview'),
    items: [
      {
        route: 'overview',
        label: t('nav.dashboard'),
        icon: 'dashboard',
      },
      ...(!isReseller ? [{
        route: 'metrics',
        label: t('nav.metrics'),
        icon: 'metrics',
      }] : []),
    ],
  })

  const manageItems: NavItem[] = [
    {
      route: 'users',
      label: t('nav.users'),
      icon: 'users',
      badge: props.notificationCount > 0 ? props.notificationCount : undefined,
    },
  ]

  if (!isReseller) {
    manageItems.push(
      {
        route: 'services',
        label: t('nav.services'),
        icon: 'services',
      },
      {
        route: 'domains',
        label: 'Domains',
        icon: 'domains',
      },
    )
  }

  if (isFull.value) {
    manageItems.push({
      route: 'plans',
      label: t('nav.plans'),
      icon: 'plans',
    })
  }

  if (!isReseller && isFull.value) {
    manageItems.push({
      route: 'tickets',
      label: t('nav.tickets'),
      icon: 'tickets',
    })
  }

  if (isFull.value) {
    manageItems.push({
      route: 'payments',
      label: t('nav.transactions'),
      icon: 'transactions',
    })
  }

  groups.push({
    id: 'manage',
    title: t('nav.group_manage'),
    items: manageItems,
  })

  if (!isReseller) {
    groups.push({
      id: 'system',
      title: t('nav.group_system'),
      items: [
        {
          route: 'settings',
          label: t('nav.settings'),
          icon: 'settings',
        },
      ],
    })
  }

  return groups
})

/**
 * Computed ordered nav groups — applies saved category and item order from the store.
 */
const orderedGroups = computed<NavGroup[]>(() => {
  if (!sidebarStore.initialized) return navGroups.value

  const categoryOrder = sidebarStore.categoryOrder
  const itemOrderMap = sidebarStore.itemOrder
  const groupMap = new Map(navGroups.value.map(g => [g.id, g]))

  // Order categories by store order
  const ordered: NavGroup[] = []
  for (const catId of categoryOrder) {
    const group = groupMap.get(catId)
    if (!group) continue

    // Order items within this category
    const savedItemOrder = itemOrderMap[catId]
    let items = group.items
    if (savedItemOrder && savedItemOrder.length > 0) {
      const orderMap = new Map(savedItemOrder.map((id, idx) => [id, idx]))
      items = [...group.items].sort((a, b) => {
        const ai = orderMap.get(a.route) ?? Infinity
        const bi = orderMap.get(b.route) ?? Infinity
        return ai - bi
      })
    }

    ordered.push({ ...group, items })
  }

  // Append any groups not in the store order (safety)
  for (const group of navGroups.value) {
    if (!ordered.find(g => g.id === group.id)) {
      ordered.push(group)
    }
  }

  return ordered
})

/** Determine if a nav item is active based on current route */
function isActive(route: string): boolean {
  if (route === 'users') {
    return ['users', 'user-detail', 'customers', 'customer-detail', 'resellers'].includes(props.currentRoute)
  }
  if (route === 'services') {
    return ['services', 'nodes', 'node-detail', 'node-compare'].includes(props.currentRoute)
  }
  if (route === 'tickets') {
    return ['tickets', 'ticket-detail'].includes(props.currentRoute)
  }
  if (route === 'reseller-tickets') {
    return ['reseller-tickets', 'reseller-ticket-detail'].includes(props.currentRoute)
  }
  return props.currentRoute === route
}

function handleNavigate(route: string) {
  emit('navigate', route)
}

function handleCollapseToggle() {
  emit('collapse-toggle')
}

function handleLogout() {
  emit('logout')
}

function handleToggleTheme() {
  emit('toggle-theme')
}
</script>

<template>
  <aside class="sidebar" :class="{ collapsed }">
    <!-- Brand -->
    <div class="brand">
      <div class="logo">K</div>
      <div v-if="!collapsed" class="brand-text">
        <h1>KorisPanel</h1>
        <span>v{{ version }}</span>
      </div>
      <button
        class="collapse-btn"
        :title="collapsed ? t('sidebar.expand') : t('sidebar.collapse')"
        @click="handleCollapseToggle"
      >
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path v-if="!collapsed" d="M11 19l-7-7 7-7M18 19l-7-7 7-7" />
          <path v-else d="M13 5l7 7-7 7M6 5l7 7-7 7" />
        </svg>
      </button>
    </div>

    <!-- Navigation Groups (drag-and-drop enabled) -->
    <div ref="categoriesContainerRef" class="nav-categories">
      <div v-for="group in orderedGroups" :key="group.id" class="nav-category-wrapper">
        <div v-if="!collapsed" class="nav-group nav-group-handle">{{ group.title }}</div>
        <div :data-category-id="group.id" class="nav-items-container">
          <button
            v-for="item in group.items"
            :key="item.route"
            class="nav-item"
            :class="{ active: isActive(item.route) }"
            :data-id="item.route"
            :title="collapsed ? item.label : undefined"
            @click="handleNavigate(item.route)"
          >
            <!-- Dashboard icon -->
            <svg v-if="item.icon === 'dashboard'" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <rect x="3" y="3" width="7" height="9" rx="1" />
              <rect x="14" y="3" width="7" height="5" rx="1" />
              <rect x="14" y="12" width="7" height="9" rx="1" />
              <rect x="3" y="16" width="7" height="5" rx="1" />
            </svg>
            <!-- Transactions icon -->
            <svg v-else-if="item.icon === 'transactions'" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M12 2v20M17 5H9.5a3.5 3.5 0 000 7h5a3.5 3.5 0 010 7H6" />
            </svg>
            <!-- Users icon -->
            <svg v-else-if="item.icon === 'users'" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <circle cx="9" cy="8" r="3.5" />
              <path d="M2.5 20a6.5 6.5 0 0113 0" />
              <circle cx="17" cy="9" r="2.5" />
              <path d="M16 14.5A5 5 0 0121.5 20" />
            </svg>
            <!-- Services icon -->
            <svg v-else-if="item.icon === 'services'" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <rect x="3" y="4" width="18" height="6" rx="1" />
              <rect x="3" y="14" width="18" height="6" rx="1" />
              <circle cx="7" cy="7" r="1" fill="currentColor" />
              <circle cx="7" cy="17" r="1" fill="currentColor" />
            </svg>
            <!-- Domains icon (globe with link) -->
            <svg v-else-if="item.icon === 'domains'" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <circle cx="12" cy="12" r="10" />
              <path d="M2 12h20" />
              <path d="M12 2a15.3 15.3 0 014 10 15.3 15.3 0 01-4 10 15.3 15.3 0 01-4-10 15.3 15.3 0 014-10z" />
            </svg>
            <!-- Cores icon (shield with network) -->
            <svg v-else-if="item.icon === 'cores'" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z" />
              <circle cx="12" cy="11" r="2" />
              <path d="M12 9V6M12 13v3M10 11H7M14 11h3" />
            </svg>
            <!-- Plans icon -->
            <svg v-else-if="item.icon === 'plans'" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <rect x="2" y="5" width="20" height="14" rx="2" />
              <path d="M2 10h20" />
            </svg>
            <!-- Tickets icon -->
            <svg v-else-if="item.icon === 'tickets'" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M21 15a2 2 0 01-2 2H7l-4 4V5a2 2 0 012-2h14a2 2 0 012 2z" />
              <path d="M8 9h8M8 13h4" />
            </svg>
            <!-- Backups icon -->
            <svg v-else-if="item.icon === 'backups'" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M21 15v4a2 2 0 01-2 2H5a2 2 0 01-2-2v-4" />
              <polyline points="7 10 12 15 17 10" />
              <line x1="12" y1="15" x2="12" y2="3" />
            </svg>
            <!-- WireGuard icon (shield/lock) -->
            <svg v-else-if="item.icon === 'wireguard'" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z" />
            </svg>
            <!-- Settings icon -->
            <svg v-else-if="item.icon === 'settings'" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <circle cx="12" cy="12" r="3" />
              <path d="M19.4 15a1.7 1.7 0 00.3 1.9l.1.1a2 2 0 11-2.8 2.8l-.1-.1a1.7 1.7 0 00-1.9-.3 1.7 1.7 0 00-1 1.5V21a2 2 0 11-4 0v-.1a1.7 1.7 0 00-1.1-1.5 1.7 1.7 0 00-1.9.3l-.1.1a2 2 0 11-2.8-2.8l.1-.1a1.7 1.7 0 00.3-1.9 1.7 1.7 0 00-1.5-1H3a2 2 0 110-4h.1a1.7 1.7 0 001.5-1.1 1.7 1.7 0 00-.3-1.9l-.1-.1a2 2 0 112.8-2.8l.1.1a1.7 1.7 0 001.9.3H10a1.7 1.7 0 001-1.5V3a2 2 0 114 0v.1a1.7 1.7 0 001 1.5 1.7 1.7 0 001.9-.3l.1-.1a2 2 0 112.8 2.8l-.1.1a1.7 1.7 0 00-.3 1.9V10a1.7 1.7 0 001.5 1H21a2 2 0 110 4h-.1a1.7 1.7 0 00-1.5 1z" />
            </svg>
            <!-- Telegram icon -->
            <svg v-else-if="item.icon === 'telegram'" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M22 2L11 13" />
              <path d="M22 2L15 22L11 13L2 9L22 2Z" />
            </svg>
            <!-- Xray icon (lightning/bolt) -->
            <svg v-else-if="item.icon === 'xray'" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M13 2L3 14h9l-1 8 10-12h-9l1-8z" />
            </svg>
            <!-- Metrics icon (chart) -->
            <svg v-else-if="item.icon === 'metrics'" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M3 3v18h18" />
              <path d="M7 14l4-4 4 4 5-5" />
            </svg>

            <span v-if="!collapsed" class="nav-label">{{ item.label }}</span>
            <span v-if="!collapsed && item.badge" class="badge">{{ item.badge }}</span>
          </button>
        </div>
      </div>
    </div>

    <!-- Sidebar Footer -->
    <div class="sidebar-foot">
      <div
        class="avatar"
        :style="{ background: 'linear-gradient(135deg, var(--color-primary), var(--color-brand-2))' }"
      >
        {{ initials }}
      </div>
      <template v-if="!collapsed">
        <div class="meta">
          {{ user.username }}
          <small>{{ user.role }}</small>
        </div>
        <button
          class="icon-btn"
          @click="handleToggleTheme"
          :title="isDark ? t('label.light_mode') : t('label.dark_mode')"
        >
          <!-- Sun icon (dark mode active, click for light) -->
          <svg v-if="isDark" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <circle cx="12" cy="12" r="5" />
            <path d="M12 1v2M12 21v2M4.22 4.22l1.42 1.42M18.36 18.36l1.42 1.42M1 12h2M21 12h2M4.22 19.78l1.42-1.42M18.36 5.64l1.42-1.42" />
          </svg>
          <!-- Moon icon (light mode active, click for dark) -->
          <svg v-else viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M21 12.79A9 9 0 1111.21 3 7 7 0 0021 12.79z" />
          </svg>
        </button>
        <button
          class="icon-btn"
          :title="t('label.logout')"
          @click="handleLogout"
        >
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M9 21H5a2 2 0 01-2-2V5a2 2 0 012-2h4M16 17l5-5-5-5M21 12H9" />
          </svg>
        </button>
      </template>
    </div>
  </aside>
</template>

<style scoped>
.sidebar {
  width: 240px;
  flex-shrink: 0;
  background: rgba(23, 29, 36, 0.95);
  border-right: 1px solid var(--color-border, var(--border, #28333f));
  padding: var(--space-5, 20px) var(--space-3, 14px);
  display: flex;
  flex-direction: column;
  gap: 2px;
  height: 100vh;
  overflow-y: auto;
  transition: width var(--duration-slow, 0.2s) var(--ease-default, ease);
}

.sidebar.collapsed {
  width: 64px;
  padding: var(--space-5, 20px) var(--space-2, 8px);
  align-items: center;
}

/* Brand */
.brand {
  display: flex;
  align-items: center;
  gap: var(--space-3, 12px);
  padding: var(--space-1, 4px) var(--space-2, 8px) var(--space-5, 20px);
  position: relative;
}

.logo {
  width: 38px;
  height: 38px;
  border-radius: var(--radius-lg, 10px);
  background: var(--gradient-brand, linear-gradient(135deg, var(--color-primary, #2563eb), var(--color-brand-2, #7c5cff)));
  display: flex;
  align-items: center;
  justify-content: center;
  font-weight: var(--font-extrabold, 800);
  font-size: var(--text-lg, 16px);
  color: #fff;
  box-shadow: var(--shadow-brand, 0 4px 14px rgba(91, 157, 255, 0.3));
  flex-shrink: 0;
}

.brand-text h1 {
  font-size: var(--text-lg, 16px);
  font-weight: var(--font-bold, 700);
  margin: 0;
}

.brand-text span {
  font-size: var(--text-xs, 10.5px);
  color: var(--color-muted, #8b98a5);
}

.collapse-btn {
  position: absolute;
  right: -4px;
  top: 50%;
  transform: translateY(-50%);
  width: 20px;
  height: 20px;
  border-radius: var(--radius-sm, 6px);
  background: var(--color-surface-2, #1e2630);
  border: 1px solid var(--color-border, #28333f);
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--color-muted, #8b98a5);
  cursor: pointer;
  opacity: 0;
  transition: opacity var(--duration-normal, 0.15s);
}

.collapse-btn svg {
  width: 10px;
  height: 10px;
}

.sidebar:hover .collapse-btn {
  opacity: 1;
}

/* Navigation Groups */
.nav-group {
  font-size: var(--text-xs, 10px);
  text-transform: uppercase;
  letter-spacing: var(--tracking-wider, 1.4px);
  color: #4a5568;
  padding: var(--space-4, 16px) var(--space-2, 10px) var(--space-1, 6px);
  font-weight: var(--font-semibold, 600);
}

.nav-item {
  display: flex;
  align-items: center;
  gap: var(--space-2, 10px);
  padding: 9px 10px;
  border-radius: 9px;
  color: var(--color-muted, #8b98a5);
  font-size: var(--text-base, 13.5px);
  font-weight: var(--font-medium, 500);
  transition: all var(--duration-normal, 0.15s);
  width: 100%;
  text-align: left;
  background: none;
  border: none;
  cursor: pointer;
}

.nav-item svg {
  width: 17px;
  height: 17px;
  flex-shrink: 0;
  opacity: 0.7;
}

.nav-item:hover {
  background: var(--color-surface-2, #1e2630);
  color: var(--color-text, #e6edf3);
}

.nav-item:hover svg {
  opacity: 1;
}

.nav-item.active {
  background: linear-gradient(135deg, rgba(91, 157, 255, 0.15), rgba(124, 92, 255, 0.12));
  color: #fff;
  box-shadow: inset 0 0 0 1px rgba(91, 157, 255, 0.25);
}

.nav-item.active svg {
  opacity: 1;
}

.nav-label {
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.badge {
  margin-left: auto;
  font-size: 10px;
  background: var(--color-primary, #2563eb);
  color: #fff;
  padding: 2px 6px;
  border-radius: 10px;
  font-weight: var(--font-bold, 700);
  min-width: 18px;
  text-align: center;
}

/* Sidebar Footer */
.sidebar-foot {
  margin-top: auto;
  padding: var(--space-3, 12px) var(--space-2, 8px) 0;
  border-top: 1px solid var(--color-border, var(--border, #28333f));
  display: flex;
  align-items: center;
  gap: 9px;
}

.avatar {
  width: 32px;
  height: 32px;
  border-radius: var(--radius-full, 50%);
  display: flex;
  align-items: center;
  justify-content: center;
  font-weight: var(--font-bold, 700);
  font-size: var(--text-sm, 12px);
  color: #fff;
  flex-shrink: 0;
}

.meta {
  font-size: var(--text-sm, 12px);
  font-weight: var(--font-semibold, 600);
  line-height: var(--leading-snug, 1.3);
  color: var(--color-text, #e6edf3);
}

.meta small {
  display: block;
  color: var(--color-muted, #8b98a5);
  font-weight: var(--font-normal, 400);
  font-size: 11px;
}

.icon-btn {
  width: 28px;
  height: 28px;
  border-radius: 7px;
  background: none;
  border: none;
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--color-muted, #8b98a5);
  cursor: pointer;
  transition: color var(--duration-normal, 0.15s);
}

.icon-btn:hover {
  color: var(--color-text, #e6edf3);
}

.icon-btn svg {
  width: 13px;
  height: 13px;
}

/* Collapsed state adjustments */
.collapsed .nav-item {
  justify-content: center;
  padding: 9px;
}

.collapsed .sidebar-foot {
  justify-content: center;
}

/* Light theme */
:deep([data-theme="light"]) .sidebar,
:global([data-theme="light"]) .sidebar {
  background: rgba(255, 255, 255, 0.97);
}

/* SortableJS drag-and-drop styles */
.nav-categories {
  display: flex;
  flex-direction: column;
  gap: 2px;
  flex: 1;
  min-height: 0;
}

.nav-category-wrapper {
  /* Wrapper for each category group — sortable at category level */
}

.nav-items-container {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.nav-group-handle {
  cursor: grab;
}

.nav-group-handle:active {
  cursor: grabbing;
}

/* Ghost element: 50% opacity during drag */
.sortable-ghost {
  opacity: 0.5;
}

/* Chosen element style */
.sortable-chosen {
  background: var(--color-surface-2, #1e2630);
  border-radius: 9px;
}

/* Drag element (follows cursor) */
.sortable-drag {
  opacity: 0.9;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.3);
  border-radius: 9px;
}

/* Collapsed state: hide drag handles */
.collapsed .nav-group-handle {
  cursor: default;
}

</style>
