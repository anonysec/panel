import { ref, computed, watch } from 'vue'

export type ThemeMode = 'system' | 'dark' | 'light'
export type UITheme = 'midnight' | 'kiro' | 'github' | 'soft-dark' | 'corporate'

export interface ThemeInfo {
  id: UITheme
  name: string
  description: string
  colors: { bg: string; surface: string; primary: string; accent: string }
  forcedMode?: 'light' | 'dark'
}

const MODE_KEY = 'koris-mode'
const THEME_KEY = 'koris-ui-theme'
const MODE_ATTRIBUTE = 'data-theme'
const THEME_ATTRIBUTE = 'data-ui-theme'

export const availableThemes: ThemeInfo[] = [
  {
    id: 'midnight',
    name: 'Midnight',
    description: 'Dark command-center aesthetic',
    colors: { bg: '#070a12', surface: '#0b1120', primary: '#2563eb', accent: '#22d3ee' },
  },
  {
    id: 'kiro',
    name: 'Kiro',
    description: 'Teal-cyan with dark navy',
    colors: { bg: '#0c1222', surface: '#1a2332', primary: '#06b6d4', accent: '#a78bfa' },
  },
  {
    id: 'github',
    name: 'GitHub',
    description: 'GitHub-inspired dark palette',
    colors: { bg: '#0d1117', surface: '#161b22', primary: '#58a6ff', accent: '#7ee787' },
  },
  {
    id: 'soft-dark',
    name: 'Soft Dark',
    description: 'Warmer tones, softer contrast',
    colors: { bg: '#1a1b26', surface: '#24283b', primary: '#7aa2f7', accent: '#9ece6a' },
  },
  {
    id: 'corporate',
    name: 'Corporate',
    description: 'Clean professional light theme',
    colors: { bg: '#f8fafc', surface: '#ffffff', primary: '#4f46e5', accent: '#0891b2' },
    forcedMode: 'light',
  },
]

function getPersistedMode(): ThemeMode {
  try {
    const stored = localStorage.getItem(MODE_KEY)
    if (stored === 'system' || stored === 'dark' || stored === 'light') {
      return stored
    }
  } catch {
    // localStorage unavailable
  }
  return 'system'
}

function getPersistedTheme(): UITheme {
  try {
    const stored = localStorage.getItem(THEME_KEY)
    if (stored && availableThemes.some((t) => t.id === stored)) {
      return stored as UITheme
    }
  } catch {
    // localStorage unavailable
  }
  return 'midnight'
}

function getSystemPrefersDark(): boolean {
  if (typeof window === 'undefined') return true
  return window.matchMedia('(prefers-color-scheme: dark)').matches
}

function resolveEffectiveMode(mode: ThemeMode, theme: UITheme): 'dark' | 'light' {
  // Corporate theme always forces light mode
  const themeInfo = availableThemes.find((t) => t.id === theme)
  if (themeInfo?.forcedMode) {
    return themeInfo.forcedMode
  }
  if (mode === 'system') {
    return getSystemPrefersDark() ? 'dark' : 'light'
  }
  return mode
}

function applyToDocument(effectiveMode: 'dark' | 'light', theme: UITheme): void {
  document.documentElement.setAttribute(MODE_ATTRIBUTE, effectiveMode)
  document.documentElement.setAttribute(THEME_ATTRIBUTE, theme)
}

// Module-level singleton state
const mode = ref<ThemeMode>(getPersistedMode())
const theme = ref<UITheme>(getPersistedTheme())

// Apply immediately on load
applyToDocument(resolveEffectiveMode(mode.value, theme.value), theme.value)

// Listen for system preference changes
let mediaQuery: MediaQueryList | null = null
if (typeof window !== 'undefined') {
  mediaQuery = window.matchMedia('(prefers-color-scheme: dark)')
  mediaQuery.addEventListener('change', () => {
    // Only react if mode is 'system' and theme does not force a mode
    if (mode.value === 'system') {
      const themeInfo = availableThemes.find((t) => t.id === theme.value)
      if (!themeInfo?.forcedMode) {
        applyToDocument(getSystemPrefersDark() ? 'dark' : 'light', theme.value)
      }
    }
  })
}

/**
 * useTheme composable
 *
 * Two-level theme system:
 * - mode: controls dark/light/system preference
 * - theme: controls the full UI color palette restyle
 *
 * The admin saves these to the server via /api/panel-settings.
 * Both admin and portal read from server on startup to apply the admin-chosen theme.
 */
export function useTheme() {
  const isDark = computed(() => {
    return resolveEffectiveMode(mode.value, theme.value) === 'dark'
  })

  function setMode(newMode: ThemeMode): void {
    mode.value = newMode
    try {
      localStorage.setItem(MODE_KEY, newMode)
    } catch {
      // silent
    }
    applyToDocument(resolveEffectiveMode(newMode, theme.value), theme.value)
  }

  function setTheme(newTheme: UITheme): void {
    theme.value = newTheme
    try {
      localStorage.setItem(THEME_KEY, newTheme)
    } catch {
      // silent
    }
    applyToDocument(resolveEffectiveMode(mode.value, newTheme), newTheme)
  }

  /** Legacy toggle for backward compatibility */
  function toggle(): void {
    if (mode.value === 'dark') {
      setMode('light')
    } else {
      setMode('dark')
    }
  }

  return {
    /** Current mode setting */
    mode,
    /** Current UI theme */
    theme,
    /** Whether the resolved mode is dark */
    isDark,
    /** List of available themes with metadata */
    availableThemes,
    /** Set the dark/light/system mode */
    setMode,
    /** Set the UI theme */
    setTheme,
    /** Toggle between dark and light (legacy) */
    toggle,
  }
}
