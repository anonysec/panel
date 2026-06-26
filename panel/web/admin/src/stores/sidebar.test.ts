import { describe, it, expect, beforeEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { useSidebarStore } from './sidebar'
import type { MenuItem, SidebarOrder } from './sidebar'

const STORAGE_KEY = 'koris-sidebar-order'

describe('sidebar store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    localStorage.clear()
  })

  const defaultCategories = ['overview', 'manage', 'system']
  const defaultItems: MenuItem[] = [
    { id: 'dashboard', categoryId: 'overview' },
    { id: 'metrics', categoryId: 'overview' },
    { id: 'users', categoryId: 'manage' },
    { id: 'nodes', categoryId: 'manage' },
    { id: 'plans', categoryId: 'manage' },
    { id: 'settings', categoryId: 'system' },
  ]

  describe('initialize', () => {
    it('sets default order when no saved state exists', () => {
      const store = useSidebarStore()
      store.initialize(defaultItems, defaultCategories)

      expect(store.order.version).toBe(1)
      expect(store.order.categories).toEqual(['overview', 'manage', 'system'])
      expect(store.order.items).toEqual({
        overview: ['dashboard', 'metrics'],
        manage: ['users', 'nodes', 'plans'],
        system: ['settings'],
      })
      expect(store.initialized).toBe(true)
    })

    it('restores saved order from localStorage', () => {
      const saved: SidebarOrder = {
        version: 1,
        categories: ['system', 'manage', 'overview'],
        items: {
          overview: ['metrics', 'dashboard'],
          manage: ['plans', 'nodes', 'users'],
          system: ['settings'],
        },
      }
      localStorage.setItem(STORAGE_KEY, JSON.stringify(saved))

      const store = useSidebarStore()
      store.initialize(defaultItems, defaultCategories)

      expect(store.order.categories).toEqual(['system', 'manage', 'overview'])
      expect(store.order.items.overview).toEqual(['metrics', 'dashboard'])
      expect(store.order.items.manage).toEqual(['plans', 'nodes', 'users'])
    })

    it('ignores invalid localStorage data', () => {
      localStorage.setItem(STORAGE_KEY, 'not valid json{{{')

      const store = useSidebarStore()
      store.initialize(defaultItems, defaultCategories)

      // Should fall back to default order
      expect(store.order.categories).toEqual(['overview', 'manage', 'system'])
    })

    it('ignores localStorage data with invalid structure', () => {
      localStorage.setItem(STORAGE_KEY, JSON.stringify({ foo: 'bar' }))

      const store = useSidebarStore()
      store.initialize(defaultItems, defaultCategories)

      expect(store.order.categories).toEqual(['overview', 'manage', 'system'])
    })
  })

  describe('saveOrder / persistence', () => {
    it('persists order to localStorage on reorderCategories', () => {
      const store = useSidebarStore()
      store.initialize(defaultItems, defaultCategories)

      store.reorderCategories(['system', 'overview', 'manage'])

      const stored = JSON.parse(localStorage.getItem(STORAGE_KEY)!)
      expect(stored.categories).toEqual(['system', 'overview', 'manage'])
    })

    it('persists order to localStorage on reorderItems', () => {
      const store = useSidebarStore()
      store.initialize(defaultItems, defaultCategories)

      store.reorderItems('manage', ['plans', 'users', 'nodes'])

      const stored = JSON.parse(localStorage.getItem(STORAGE_KEY)!)
      expect(stored.items.manage).toEqual(['plans', 'users', 'nodes'])
    })
  })

  describe('loadOrder', () => {
    it('returns null when localStorage is empty', () => {
      const store = useSidebarStore()
      expect(store.loadOrder()).toBeNull()
    })

    it('returns parsed order for valid data', () => {
      const saved: SidebarOrder = {
        version: 1,
        categories: ['overview'],
        items: { overview: ['dashboard'] },
      }
      localStorage.setItem(STORAGE_KEY, JSON.stringify(saved))

      const store = useSidebarStore()
      const result = store.loadOrder()
      expect(result).toEqual(saved)
    })

    it('returns null for data with non-string category entries', () => {
      localStorage.setItem(STORAGE_KEY, JSON.stringify({
        version: 1,
        categories: [123, 'valid'],
        items: {},
      }))

      const store = useSidebarStore()
      expect(store.loadOrder()).toBeNull()
    })

    it('returns null for data with non-array item values', () => {
      localStorage.setItem(STORAGE_KEY, JSON.stringify({
        version: 1,
        categories: ['overview'],
        items: { overview: 'not-an-array' },
      }))

      const store = useSidebarStore()
      expect(store.loadOrder()).toBeNull()
    })
  })

  describe('mergeSavedOrder', () => {
    it('preserves saved positions for items that still exist', () => {
      const store = useSidebarStore()
      const saved: SidebarOrder = {
        version: 1,
        categories: ['manage', 'overview', 'system'],
        items: {
          overview: ['metrics', 'dashboard'],
          manage: ['plans', 'nodes', 'users'],
          system: ['settings'],
        },
      }

      const result = store.mergeSavedOrder(saved, defaultItems)

      expect(result.categories).toEqual(['manage', 'overview', 'system'])
      expect(result.items.overview).toEqual(['metrics', 'dashboard'])
      expect(result.items.manage).toEqual(['plans', 'nodes', 'users'])
    })

    it('appends new items to end of their default category', () => {
      const store = useSidebarStore()
      const saved: SidebarOrder = {
        version: 1,
        categories: ['overview', 'manage', 'system'],
        items: {
          overview: ['dashboard'],
          manage: ['users', 'nodes'],
          system: ['settings'],
        },
      }
      // 'metrics' (overview) and 'plans' (manage) are new
      const result = store.mergeSavedOrder(saved, defaultItems)

      expect(result.items.overview).toEqual(['dashboard', 'metrics'])
      expect(result.items.manage).toEqual(['users', 'nodes', 'plans'])
    })

    it('removes items no longer available', () => {
      const store = useSidebarStore()
      const saved: SidebarOrder = {
        version: 1,
        categories: ['overview', 'manage', 'system'],
        items: {
          overview: ['dashboard', 'metrics'],
          manage: ['users', 'nodes', 'plans', 'tickets'], // tickets removed
          system: ['settings', 'backups'], // backups removed
        },
      }

      const result = store.mergeSavedOrder(saved, defaultItems)

      expect(result.items.manage).toEqual(['users', 'nodes', 'plans'])
      expect(result.items.system).toEqual(['settings'])
    })

    it('appends new categories not in saved order', () => {
      const store = useSidebarStore()
      const saved: SidebarOrder = {
        version: 1,
        categories: ['overview', 'manage'],
        items: {
          overview: ['dashboard', 'metrics'],
          manage: ['users', 'nodes', 'plans'],
        },
      }

      const result = store.mergeSavedOrder(saved, defaultItems)

      // 'system' category is new and should be appended
      expect(result.categories).toEqual(['overview', 'manage', 'system'])
      expect(result.items.system).toEqual(['settings'])
    })

    it('removes categories that no longer exist', () => {
      const store = useSidebarStore()
      const saved: SidebarOrder = {
        version: 1,
        categories: ['overview', 'manage', 'advanced', 'system'],
        items: {
          overview: ['dashboard', 'metrics'],
          manage: ['users', 'nodes', 'plans'],
          advanced: ['xray', 'telegram'],
          system: ['settings'],
        },
      }

      const result = store.mergeSavedOrder(saved, defaultItems)

      expect(result.categories).not.toContain('advanced')
      expect(result.items.advanced).toBeUndefined()
    })

    it('handles empty saved order gracefully', () => {
      const store = useSidebarStore()
      const saved: SidebarOrder = {
        version: 1,
        categories: [],
        items: {},
      }

      const result = store.mergeSavedOrder(saved, defaultItems)

      // All categories and items should appear as new
      expect(result.categories).toEqual(['overview', 'manage', 'system'])
      expect(result.items.overview).toEqual(['dashboard', 'metrics'])
      expect(result.items.manage).toEqual(['users', 'nodes', 'plans'])
      expect(result.items.system).toEqual(['settings'])
    })

    it('handles empty available items gracefully', () => {
      const store = useSidebarStore()
      const saved: SidebarOrder = {
        version: 1,
        categories: ['overview', 'manage', 'system'],
        items: {
          overview: ['dashboard', 'metrics'],
          manage: ['users', 'nodes', 'plans'],
          system: ['settings'],
        },
      }

      const result = store.mergeSavedOrder(saved, [])

      // Everything removed since nothing is available
      expect(result.categories).toEqual([])
      expect(result.items).toEqual({})
    })
  })

  describe('reorderCategories', () => {
    it('updates category order in state', () => {
      const store = useSidebarStore()
      store.initialize(defaultItems, defaultCategories)

      store.reorderCategories(['system', 'manage', 'overview'])

      expect(store.order.categories).toEqual(['system', 'manage', 'overview'])
    })

    it('does not mutate item order when reordering categories', () => {
      const store = useSidebarStore()
      store.initialize(defaultItems, defaultCategories)

      const originalItems = { ...store.order.items }
      store.reorderCategories(['system', 'manage', 'overview'])

      expect(store.order.items).toEqual(originalItems)
    })
  })

  describe('reorderItems', () => {
    it('updates item order within a category', () => {
      const store = useSidebarStore()
      store.initialize(defaultItems, defaultCategories)

      store.reorderItems('manage', ['plans', 'users', 'nodes'])

      expect(store.order.items.manage).toEqual(['plans', 'users', 'nodes'])
    })

    it('does not affect other categories', () => {
      const store = useSidebarStore()
      store.initialize(defaultItems, defaultCategories)

      store.reorderItems('manage', ['plans', 'users', 'nodes'])

      expect(store.order.items.overview).toEqual(['dashboard', 'metrics'])
      expect(store.order.items.system).toEqual(['settings'])
    })
  })

  describe('resetToDefault', () => {
    it('clears localStorage', () => {
      const store = useSidebarStore()
      store.initialize(defaultItems, defaultCategories)
      store.reorderCategories(['system', 'manage', 'overview'])

      store.resetToDefault()

      expect(localStorage.getItem(STORAGE_KEY)).toBeNull()
    })

    it('restores default category order', () => {
      const store = useSidebarStore()
      store.initialize(defaultItems, defaultCategories)
      store.reorderCategories(['system', 'manage', 'overview'])

      store.resetToDefault()

      expect(store.order.categories).toEqual(['overview', 'manage', 'system'])
    })

    it('restores default item order', () => {
      const store = useSidebarStore()
      store.initialize(defaultItems, defaultCategories)
      store.reorderItems('manage', ['plans', 'users', 'nodes'])

      store.resetToDefault()

      expect(store.order.items.manage).toEqual(['users', 'nodes', 'plans'])
    })
  })
})
