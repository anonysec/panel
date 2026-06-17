import { ref, computed, onMounted, onUnmounted, type Ref, type ComputedRef } from 'vue'

export interface CommandAction {
  id: string
  label: string
  description?: string
  icon?: string
  shortcut?: string
  action: () => void
  section?: string
  keywords?: string[]
}

export interface UseCommandPaletteOptions {
  actions: ComputedRef<CommandAction[]> | Ref<CommandAction[]>
  shortcut?: string // default: 'ctrl+k'
}

export interface UseCommandPaletteReturn {
  isOpen: Ref<boolean>
  query: Ref<string>
  filteredActions: ComputedRef<CommandAction[]>
  selectedIndex: Ref<number>
  open(): void
  close(): void
  execute(action: CommandAction): void
}

function fuzzyMatch(text: string, pattern: string): boolean {
  const lower = text.toLowerCase()
  const search = pattern.toLowerCase()
  let j = 0
  for (let i = 0; i < lower.length && j < search.length; i++) {
    if (lower[i] === search[j]) {
      j++
    }
  }
  return j === search.length
}

function matchesAction(action: CommandAction, pattern: string): boolean {
  if (fuzzyMatch(action.label, pattern)) return true
  if (action.description && fuzzyMatch(action.description, pattern)) return true
  if (action.keywords) {
    for (const keyword of action.keywords) {
      if (fuzzyMatch(keyword, pattern)) return true
    }
  }
  return false
}

// Module-level shared state (singleton pattern)
// All consumers share the same isOpen/query/selectedIndex refs
const isOpen = ref(false)
const query = ref('')
const selectedIndex = ref(0)

/**
 * Open the command palette from anywhere.
 * Can be called outside of a Vue component context.
 */
export function openCommandPalette(): void {
  isOpen.value = true
  query.value = ''
  selectedIndex.value = 0
}

/**
 * Close the command palette from anywhere.
 */
export function closeCommandPalette(): void {
  isOpen.value = false
  query.value = ''
  selectedIndex.value = 0
}

export function useCommandPalette(options: UseCommandPaletteOptions): UseCommandPaletteReturn {
  const { actions, shortcut = 'ctrl+k' } = options

  const filteredActions = computed<CommandAction[]>(() => {
    const q = query.value.trim()
    if (!q) return actions.value
    return actions.value.filter(action => matchesAction(action, q))
  })

  function open() {
    openCommandPalette()
  }

  function close() {
    closeCommandPalette()
  }

  function execute(action: CommandAction) {
    action.action()
    close()
  }

  function parseShortcut(shortcutStr: string) {
    const parts = shortcutStr.toLowerCase().split('+')
    return {
      ctrl: parts.includes('ctrl'),
      meta: parts.includes('meta'),
      shift: parts.includes('shift'),
      alt: parts.includes('alt'),
      key: parts[parts.length - 1]
    }
  }

  function handleKeydown(e: KeyboardEvent) {
    const parsed = parseShortcut(shortcut)

    // Check shortcut to open/close
    const ctrlMatch = parsed.ctrl ? (e.ctrlKey || e.metaKey) : true
    const shiftMatch = parsed.shift ? e.shiftKey : true
    const altMatch = parsed.alt ? e.altKey : true
    const keyMatch = e.key.toLowerCase() === parsed.key

    if (ctrlMatch && shiftMatch && altMatch && keyMatch) {
      e.preventDefault()
      if (isOpen.value) {
        close()
      } else {
        open()
      }
      return
    }

    if (!isOpen.value) return

    switch (e.key) {
      case 'ArrowDown':
        e.preventDefault()
        if (filteredActions.value.length > 0) {
          selectedIndex.value = (selectedIndex.value + 1) % filteredActions.value.length
        }
        break
      case 'ArrowUp':
        e.preventDefault()
        if (filteredActions.value.length > 0) {
          selectedIndex.value = (selectedIndex.value - 1 + filteredActions.value.length) % filteredActions.value.length
        }
        break
      case 'Enter':
        e.preventDefault()
        if (filteredActions.value.length > 0 && selectedIndex.value < filteredActions.value.length) {
          execute(filteredActions.value[selectedIndex.value])
        }
        break
      case 'Escape':
        e.preventDefault()
        close()
        break
    }
  }

  onMounted(() => {
    document.addEventListener('keydown', handleKeydown)
  })

  onUnmounted(() => {
    document.removeEventListener('keydown', handleKeydown)
  })

  return {
    isOpen,
    query,
    filteredActions,
    selectedIndex,
    open,
    close,
    execute
  }
}
