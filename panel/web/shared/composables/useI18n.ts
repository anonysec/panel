import { ref, watch } from 'vue'

export type Locale = 'en' | 'fa' | 'zh' | 'ru'

const STORAGE_KEY = 'koris-lang'

/** Global message registry: locale -> flat key-value translations */
const messages: Record<Locale, Record<string, string>> = {
  en: {},
  fa: {},
  zh: {},
  ru: {},
}

/** Read persisted locale from localStorage, defaulting to 'en' */
function getPersistedLocale(): Locale {
  if (typeof window === 'undefined') return 'en'
  const stored = localStorage.getItem(STORAGE_KEY)
  if (stored === 'en' || stored === 'fa' || stored === 'zh' || stored === 'ru') {
    return stored
  }
  return 'en'
}

/** Shared reactive locale state (singleton across all useI18n calls) */
const currentLocale = ref<Locale>(getPersistedLocale())

// Watch for locale changes: persist to localStorage
watch(currentLocale, (newLocale) => {
  if (typeof window !== 'undefined') {
    localStorage.setItem(STORAGE_KEY, newLocale)
  }
})

/**
 * Register a message bundle for one or more locales.
 * This allows different apps (admin, portal) to register their own translation keys.
 * Messages are merged into the global registry -- later registrations override earlier ones
 * for the same key.
 */
export function registerMessages(
  bundle: Partial<Record<Locale, Record<string, string>>>
): void {
  for (const locale of Object.keys(bundle) as Locale[]) {
    const localeMessages = bundle[locale]
    if (localeMessages) {
      Object.assign(messages[locale], localeMessages)
    }
  }
}

/**
 * Translate a key using the active locale with English fallback.
 * Never returns a raw key string -- if the key is missing in both the active locale
 * and English, returns an empty string.
 */
function translate(key: string): string {
  const activeTranslation = messages[currentLocale.value]?.[key]
  if (activeTranslation !== undefined && activeTranslation !== '') {
    return activeTranslation
  }

  const englishFallback = messages.en[key]
  if (englishFallback !== undefined && englishFallback !== '') {
    return englishFallback
  }

  // Never return raw key string
  return ''
}

/**
 * Set the active locale. Persists to localStorage.
 */
function setLocale(locale: Locale): void {
  currentLocale.value = locale
}

/**
 * Composable for internationalization.
 * Returns reactive locale, translation function, and locale setter.
 *
 * @example
 * ```ts
 * const { t, locale, setLocale } = useI18n()
 * const greeting = t('label.welcome_back') // "Welcome back" or translated
 * setLocale('fa') // switches to Persian, text changes but layout stays LTR
 * ```
 */
export function useI18n() {
  return {
    t: translate,
    locale: currentLocale,
    setLocale,
  }
}
