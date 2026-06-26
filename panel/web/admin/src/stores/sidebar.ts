import { ref, computed } from 'vue'
import { defineStore } from 'pinia'

// ─── Types ──────────────────────────────────────────────────────────────────

/**
 * Persisted sidebar ordering state.
 * Saved to localStorage under key `koris-sidebar-order`.
 */
export interface SidebarOrder {
  version: number               // schema version for future migrations
  categories: string[]          // category IDs in display order
  items: Record<string, string[]>  // categoryId → item IDs in order
}

/**
 * A menu item available in the sidebar (provided by the app).
 */
export interface MenuItem {
  id: string
  categoryId: string
}

// ─── Constants ──────────────────────────────────────────────────────────────

const STORAGE_KEY = 'koris-sidebar-order'
const CURRENT_VERSION = 1

// ─── Store ──────────────────────────────────────────────────────────────────

/**
 * Sidebar ordering store — manages the display order of navigation categories
 * and items, with persistence to localStorage and merge logic for edition changes.
 *
 * Requirements: 6.4, 6.5, 6.6, 6.7
 */
export const useSidebarStore = defineStore('sidebar', () => {
  // ─── State ──────────────────────────────────────────────────────────────

  /** Current sidebar order (categories + items within each) */
  const order = ref<SidebarOrder>({
    version: CURRENT_VERSION,
    categories: [],
    items: {},
  })

  /** The default order derived from available menu items (used for reset) */
  const defaultOrder = ref<SidebarOrder>({
    version: CURRENT_VERSION,
    categories: [],
    items: {},
  })

  /** Whether the store has been initialized */
  const initialized = ref(false)

  // ─── Computed ───────────────────────────────────────────────────────────

  const categoryOrder = computed(() => order.value.categories)
  const itemOrder = computed(() => order.value.items)

  // ─── Persistence ────────────────────────────────────────────────────────

  /**
   * Save current order to localStorage.
   */
  function saveOrder(): void {
    try {
      const serialized = JSON.stringify(order.value)
      localStorage.setItem(STORAGE_KEY, serialized)
    } catch {
      // localStorage may be full or unavailable — fail silently
    }
  }

  /**
   * Load order from localStorage. Returns null if nothing is saved or
   * the stored data is invalid.
   */
  function loadOrder(): SidebarOrder | null {
    try {
      const raw = localStorage.getItem(STORAGE_KEY)
      if (!raw) return null

      const parsed = JSON.parse(raw)

      // Basic structure validation
      if (
        typeof parsed !== 'object' ||
        parsed === null ||
        typeof parsed.version !== 'number' ||
        !Array.isArray(parsed.categories) ||
        typeof parsed.items !== 'object' ||
        parsed.items === null
      ) {
        return null
      }

      // Validate categories are strings
      if (!parsed.categories.every((c: unknown) => typeof c === 'string')) {
        return null
      }

      // Validate items are Record<string, string[]>
      for (const key of Object.keys(parsed.items)) {
        if (!Array.isArray(parsed.items[key])) return null
        if (!parsed.items[key].every((i: unknown) => typeof i === 'string')) return null
      }

      return parsed as SidebarOrder
    } catch {
      return null
    }
  }

  // ─── Merge Logic ────────────────────────────────────────────────────────

  /**
   * Merge a saved order with the currently available menu items.
   *
   * Rules:
   * 1. Keep items that still exist in their saved positions
   * 2. Append new items (present in available but not in saved) to end of their default category
   * 3. Remove items no longer available (present in saved but absent from available)
   *
   * Categories follow the same logic: keep saved order for existing ones,
   * append new categories at the end, remove categories that no longer exist.
   */
  function mergeSavedOrder(saved: SidebarOrder, available: MenuItem[]): SidebarOrder {
    // Build lookup of available items by ID and default category membership
    const availableSet = new Set(available.map(item => item.id))
    const availableByCategoryDefault = new Map<string, string[]>()
    const allAvailableCategories = new Set<string>()

    for (const item of available) {
      allAvailableCategories.add(item.categoryId)
      if (!availableByCategoryDefault.has(item.categoryId)) {
        availableByCategoryDefault.set(item.categoryId, [])
      }
      availableByCategoryDefault.get(item.categoryId)!.push(item.id)
    }

    // --- Merge categories ---
    // Keep saved categories that still exist
    const mergedCategories = saved.categories.filter(cat => allAvailableCategories.has(cat))
    // Append new categories not in saved order
    for (const cat of allAvailableCategories) {
      if (!mergedCategories.includes(cat)) {
        mergedCategories.push(cat)
      }
    }

    // --- Merge items within each category ---
    const mergedItems: Record<string, string[]> = {}

    // Track which available items have been placed
    const placedItems = new Set<string>()

    for (const categoryId of mergedCategories) {
      const savedItemsForCategory = saved.items[categoryId] || []
      const defaultItemsForCategory = availableByCategoryDefault.get(categoryId) || []
      const availableInCategory = new Set(defaultItemsForCategory)

      // Step 1: Keep saved items that still exist in the available set
      const kept = savedItemsForCategory.filter(itemId => availableSet.has(itemId) && availableInCategory.has(itemId))
      kept.forEach(id => placedItems.add(id))

      // Step 2: Append new items (in available for this category but not in saved)
      const newItems = defaultItemsForCategory.filter(itemId => !kept.includes(itemId))
      newItems.forEach(id => placedItems.add(id))

      mergedItems[categoryId] = [...kept, ...newItems]
    }

    return {
      version: CURRENT_VERSION,
      categories: mergedCategories,
      items: mergedItems,
    }
  }

  // ─── Actions ────────────────────────────────────────────────────────────

  /**
   * Initialize the sidebar store with the available menu items.
   * Call this once on app mount with the full list of menu items.
   *
   * - Computes the default order from available items
   * - Loads any saved order from localStorage
   * - Merges saved order with available items
   */
  function initialize(available: MenuItem[], defaultCategories: string[]): void {
    // Build the default order from available items
    const defaultItems: Record<string, string[]> = {}
    for (const cat of defaultCategories) {
      defaultItems[cat] = available
        .filter(item => item.categoryId === cat)
        .map(item => item.id)
    }

    defaultOrder.value = {
      version: CURRENT_VERSION,
      categories: [...defaultCategories],
      items: defaultItems,
    }

    // Attempt to load saved order
    const saved = loadOrder()

    if (saved) {
      // Merge with current available items (handles edition changes)
      order.value = mergeSavedOrder(saved, available)
    } else {
      // No saved order — use default
      order.value = {
        version: CURRENT_VERSION,
        categories: [...defaultCategories],
        items: { ...defaultItems },
      }
    }

    initialized.value = true
  }

  /**
   * Update the category display order.
   * Persists to localStorage immediately.
   */
  function reorderCategories(newCategoryOrder: string[]): void {
    order.value = {
      ...order.value,
      categories: [...newCategoryOrder],
    }
    saveOrder()
  }

  /**
   * Update item order within a specific category.
   * Persists to localStorage immediately.
   */
  function reorderItems(categoryId: string, newItemOrder: string[]): void {
    order.value = {
      ...order.value,
      items: {
        ...order.value.items,
        [categoryId]: [...newItemOrder],
      },
    }
    saveOrder()
  }

  /**
   * Reset to Default: clear localStorage and restore original order.
   * Requirement 6.6
   */
  function resetToDefault(): void {
    localStorage.removeItem(STORAGE_KEY)
    order.value = {
      version: defaultOrder.value.version,
      categories: [...defaultOrder.value.categories],
      items: Object.fromEntries(
        Object.entries(defaultOrder.value.items).map(([k, v]) => [k, [...v]])
      ),
    }
  }

  // ─── Expose ─────────────────────────────────────────────────────────────

  return {
    // State
    order,
    defaultOrder,
    initialized,

    // Computed
    categoryOrder,
    itemOrder,

    // Actions
    initialize,
    reorderCategories,
    reorderItems,
    resetToDefault,
    saveOrder,
    loadOrder,
    mergeSavedOrder,
  }
})
