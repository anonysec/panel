import { ref, computed } from 'vue'
import { defineStore } from 'pinia'
import {
  useTheme,
  availableThemes as sharedAvailableThemes,
  getCSSVariables,
  type ThemeMode,
  type UITheme,
  type ThemeConfig as SharedThemeConfig,
  type ThemeInfo,
} from '@koris/composables/useTheme'

/**
 * ThemeConfig interface matching the design document specification.
 * Maps to the shared ThemeInfo structure for interop.
 */
export interface ThemeConfig {
  id: string
  name: string
  mode: 'light' | 'dark'
  tokens: {
    primary: string
    primaryHover: string
    secondary: string
    background: string
    surface: string
    surfaceHover: string
    text: string
    textMuted: string
    border: string
    success: string
    warning: string
    error: string
    info: string
    accent: string
    borderRadius: string
    shadowSm: string
    shadowMd: string
    shadowLg: string
  }
}

const THEME_KEY = 'koris-ui-theme'
const MODE_KEY = 'koris-mode'

/**
 * Convert a shared ThemeInfo to the design document's ThemeConfig format.
 */
function toThemeConfig(info: ThemeInfo): ThemeConfig {
  return {
    id: info.id,
    name: info.name,
    mode: info.mode,
    tokens: {
      primary: info.config.colors.primary,
      primaryHover: info.config.colors.primaryHover,
      secondary: info.config.colors.secondary,
      background: info.config.colors.background,
      surface: info.config.colors.surface,
      surfaceHover: info.config.colors.surfaceHover,
      text: info.config.colors.text,
      textMuted: info.config.colors.textMuted,
      border: info.config.colors.border,
      success: info.config.colors.success,
      warning: info.config.colors.warning,
      error: info.config.colors.error,
      info: info.config.colors.info,
      accent: info.config.colors.accent,
      borderRadius: info.config.borderRadius,
      shadowSm: info.config.shadows.sm,
      shadowMd: info.config.shadows.md,
      shadowLg: info.config.shadows.lg,
    },
  }
}

/** All available themes in ThemeConfig format */
export const availableThemes: ThemeConfig[] = sharedAvailableThemes.map(toThemeConfig)

/**
 * Theme store — manages theme selection, application, and persistence.
 *
 * Application Flow:
 * 1. App.vue on mount: calls initTheme()
 * 2. initTheme reads "koris-ui-theme" from localStorage
 * 3. Validates themeId against availableThemes
 * 4. If invalid/missing: fallback to "default-dark", remove stale localStorage key
 * 5. Calls applyTheme() to set CSS vars and data attributes
 *
 * Requirements: 4.1, 4.2, 4.4, 4.5, 4.7, 4.8
 */
export const useThemeStore = defineStore('theme', () => {
  // ─── Shared composable (handles DOM + persistence internals) ──────────────
  const themeComposable = useTheme()

  // ─── State ────────────────────────────────────────────────────────────────
  const currentThemeId = ref<string>(themeComposable.theme.value)
  const currentMode = ref<ThemeMode>(themeComposable.mode.value)
  const initialized = ref(false)

  // ─── Computed ─────────────────────────────────────────────────────────────

  /** The active ThemeConfig object */
  const activeTheme = computed<ThemeConfig>(() => {
    return (
      availableThemes.find((t) => t.id === currentThemeId.value) ||
      availableThemes.find((t) => t.id === 'default-dark')!
    )
  })

  /** Whether the current effective mode is dark */
  const isDark = computed(() => themeComposable.isDark.value)

  // ─── Actions ──────────────────────────────────────────────────────────────

  /**
   * Initialize the theme system.
   * Reads localStorage, validates against available themes, falls back to default-dark.
   * Then applies the theme to the document.
   *
   * @param overrideThemeId - Optional theme ID from server settings (takes priority)
   */
  function initTheme(overrideThemeId?: string): void {
    let themeId: string | null = overrideThemeId || null

    // If no override provided, read from localStorage
    if (!themeId) {
      try {
        themeId = localStorage.getItem(THEME_KEY)
      } catch {
        themeId = null
      }
    }

    // Validate against available themes
    const isValid = themeId && availableThemes.some((t) => t.id === themeId)

    if (!isValid) {
      // Fallback to default-dark, remove stale localStorage key
      themeId = 'default-dark'
      try {
        localStorage.removeItem(THEME_KEY)
      } catch {
        // silent
      }
    }

    currentThemeId.value = themeId!
    applyTheme(themeId!)
    initialized.value = true
  }

  /**
   * Apply a theme by ID.
   * Sets all 18 --koris-* CSS variables on document.documentElement,
   * and sets data-ui-theme and data-theme attributes.
   *
   * @param themeId - The theme ID to apply
   */
  function applyTheme(themeId: string): void {
    const themeConfig = availableThemes.find((t) => t.id === themeId)
    if (!themeConfig) return

    currentThemeId.value = themeId

    // Set data attributes on documentElement
    document.documentElement.setAttribute('data-ui-theme', themeId)
    document.documentElement.setAttribute('data-theme', themeConfig.mode)

    // Apply all 18 CSS variables
    const root = document.documentElement
    root.style.setProperty('--koris-primary', themeConfig.tokens.primary)
    root.style.setProperty('--koris-primary-hover', themeConfig.tokens.primaryHover)
    root.style.setProperty('--koris-secondary', themeConfig.tokens.secondary)
    root.style.setProperty('--koris-background', themeConfig.tokens.background)
    root.style.setProperty('--koris-surface', themeConfig.tokens.surface)
    root.style.setProperty('--koris-surface-hover', themeConfig.tokens.surfaceHover)
    root.style.setProperty('--koris-text', themeConfig.tokens.text)
    root.style.setProperty('--koris-text-muted', themeConfig.tokens.textMuted)
    root.style.setProperty('--koris-border', themeConfig.tokens.border)
    root.style.setProperty('--koris-success', themeConfig.tokens.success)
    root.style.setProperty('--koris-warning', themeConfig.tokens.warning)
    root.style.setProperty('--koris-error', themeConfig.tokens.error)
    root.style.setProperty('--koris-info', themeConfig.tokens.info)
    root.style.setProperty('--koris-accent', themeConfig.tokens.accent)
    root.style.setProperty('--koris-border-radius', themeConfig.tokens.borderRadius)
    root.style.setProperty('--koris-shadow-sm', themeConfig.tokens.shadowSm)
    root.style.setProperty('--koris-shadow-md', themeConfig.tokens.shadowMd)
    root.style.setProperty('--koris-shadow-lg', themeConfig.tokens.shadowLg)

    // Persist to localStorage
    try {
      localStorage.setItem(THEME_KEY, themeId)
    } catch {
      // silent
    }

    // Sync with shared composable
    themeComposable.setTheme(themeId as UITheme)
  }

  /**
   * Set the theme mode (dark, light, system).
   * When mode is 'system', listens to prefers-color-scheme changes.
   */
  function setMode(newMode: ThemeMode): void {
    currentMode.value = newMode
    themeComposable.setMode(newMode)

    try {
      localStorage.setItem(MODE_KEY, newMode)
    } catch {
      // silent
    }
  }

  /**
   * Apply effective mode based on system preference.
   * Called by the system mode media query listener.
   */
  function applyEffectiveMode(effectiveMode: 'dark' | 'light'): void {
    document.documentElement.setAttribute('data-theme', effectiveMode)
  }

  // ─── System Mode Detection ────────────────────────────────────────────────
  if (typeof window !== 'undefined') {
    const mediaQuery = window.matchMedia('(prefers-color-scheme: dark)')
    mediaQuery.addEventListener('change', (e) => {
      if (currentMode.value === 'system') {
        applyEffectiveMode(e.matches ? 'dark' : 'light')
      }
    })
  }

  // ─── Expose ───────────────────────────────────────────────────────────────
  return {
    // State
    currentThemeId,
    currentMode,
    initialized,
    // Computed
    activeTheme,
    isDark,
    // Actions
    initTheme,
    applyTheme,
    setMode,
    applyEffectiveMode,
    // Static
    availableThemes,
  }
})
