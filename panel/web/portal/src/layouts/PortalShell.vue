<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { usePortalAuthStore } from '@/stores/auth'
import { useTheme } from '@koris/composables/useTheme'
import { useI18n } from '@koris/composables/useI18n'
import type { Locale } from '@koris/composables/useI18n'

const router = useRouter()
const auth = usePortalAuthStore()
const { isDark, toggle: toggleTheme } = useTheme()
const { t, locale, setLocale } = useI18n()

const userMenuOpen = ref(false)
const langMenuOpen = ref(false)

function toggleUserMenu() {
  userMenuOpen.value = !userMenuOpen.value
  langMenuOpen.value = false
}

function closeUserMenu() {
  userMenuOpen.value = false
}

function toggleLangMenu() {
  langMenuOpen.value = !langMenuOpen.value
  userMenuOpen.value = false
}

function closeLangMenu() {
  langMenuOpen.value = false
}

function switchLang(lang: Locale) {
  setLocale(lang)
  closeLangMenu()
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

const langLabels: Record<Locale, string> = {
  en: 'EN',
  fa: 'FA',
  zh: 'ZH',
}
</script>
<template>
  <div class="portal-shell">
    <header class="portal-header">
      <div class="portal-header__brand">
        <span class="portal-header__logo">K</span>
        <span class="portal-header__title">KorisPanel</span>
      </div>

      <div class="portal-header__actions">
        <!-- Language switcher -->
        <div class="portal-header__lang">
          <button class="portal-header__btn" @click="toggleLangMenu" :aria-label="t('portal.nav.language')">
            🌐 {{ langLabels[locale] }}
          </button>
          <div v-if="langMenuOpen" class="portal-header__dropdown-backdrop" @click="closeLangMenu"></div>
          <div v-if="langMenuOpen" class="portal-header__dropdown portal-header__dropdown--lang">
            <button class="portal-header__dropdown-item" :class="{ 'portal-header__dropdown-item--active': locale === 'en' }" @click="switchLang('en')">English</button>
            <button class="portal-header__dropdown-item" :class="{ 'portal-header__dropdown-item--active': locale === 'fa' }" @click="switchLang('fa')">فارسی</button>
            <button class="portal-header__dropdown-item" :class="{ 'portal-header__dropdown-item--active': locale === 'zh' }" @click="switchLang('zh')">中文</button>
          </div>
        </div>

        <!-- Theme toggle -->
        <button @click="toggleTheme" class="portal-header__btn" :aria-label="t('portal.nav.theme')">
          {{ isDark ? '☀️' : '🌙' }}
        </button>

        <!-- User menu -->
        <div class="portal-header__user-menu">
          <button class="portal-header__user-toggle" @click="toggleUserMenu">
            <span class="portal-header__user-avatar">{{ (auth.user?.username || '?')[0].toUpperCase() }}</span>
            <span class="portal-header__user-name">{{ auth.user?.username }}</span>
            <svg class="portal-header__chevron" :class="{ 'portal-header__chevron--open': userMenuOpen }" viewBox="0 0 20 20" fill="currentColor" width="14" height="14">
              <path fill-rule="evenodd" d="M5.23 7.21a.75.75 0 011.06.02L10 11.168l3.71-3.938a.75.75 0 111.08 1.04l-4.25 4.5a.75.75 0 01-1.08 0l-4.25-4.5a.75.75 0 01.02-1.06z" clip-rule="evenodd" />
            </svg>
          </button>
          <div v-if="userMenuOpen" class="portal-header__dropdown-backdrop" @click="closeUserMenu"></div>
          <div v-if="userMenuOpen" class="portal-header__dropdown">
            <div class="portal-header__dropdown-header">{{ auth.user?.username }}</div>
            <button class="portal-header__dropdown-item" @click="goToProfile">{{ t('portal.nav.profile') }}</button>
            <button class="portal-header__dropdown-item portal-header__dropdown-item--danger" @click="logout">{{ t('portal.nav.logout') }}</button>
          </div>
        </div>
      </div>
    </header>

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
.portal-shell {
  min-height: 100vh;
  background: var(--color-bg);
}
.portal-header {
  display: flex;
  align-items: center;
  gap: var(--space-4);
  padding: var(--space-3) var(--space-5);
  border-bottom: 1px solid var(--color-border);
  background: var(--color-surface);
  position: sticky;
  top: 0;
  z-index: 100;
}
.portal-header__brand {
  display: flex;
  align-items: center;
  gap: var(--space-2);
}
.portal-header__logo {
  width: 32px;
  height: 32px;
  border-radius: var(--radius-md);
  background: var(--gradient-brand);
  display: flex;
  align-items: center;
  justify-content: center;
  color: #fff;
  font-weight: 800;
  font-size: 14px;
}
.portal-header__title {
  font-weight: 700;
  font-size: var(--text-md);
}
.portal-header__actions {
  margin-left: auto;
  display: flex;
  align-items: center;
  gap: var(--space-2);
}
.portal-header__btn {
  background: none;
  border: none;
  color: var(--color-muted);
  cursor: pointer;
  font-size: var(--text-sm);
  padding: var(--space-2) var(--space-2);
  border-radius: var(--radius-md);
  min-width: 40px;
  min-height: 40px;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: var(--space-1);
  transition: background 0.15s;
}
.portal-header__btn:hover {
  background: var(--color-surface-2);
  color: var(--color-text);
}
.portal-header__lang {
  position: relative;
}
.portal-header__user-menu {
  position: relative;
}
.portal-header__user-toggle {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  background: none;
  border: none;
  cursor: pointer;
  padding: var(--space-1) var(--space-2);
  border-radius: var(--radius-md);
  min-height: 40px;
  transition: background 0.15s;
}
.portal-header__user-toggle:hover {
  background: var(--color-surface-2);
}
.portal-header__user-avatar {
  width: 28px;
  height: 28px;
  border-radius: var(--radius-full);
  background: var(--color-primary);
  color: #fff;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 12px;
  font-weight: 700;
}
.portal-header__user-name {
  font-size: var(--text-sm);
  color: var(--color-text);
  font-weight: 500;
}
.portal-header__chevron {
  transition: transform 0.15s;
  color: var(--color-muted);
}
.portal-header__chevron--open {
  transform: rotate(180deg);
}
.portal-header__dropdown-backdrop {
  position: fixed;
  inset: 0;
  z-index: 99;
}
.portal-header__dropdown {
  position: absolute;
  top: calc(100% + var(--space-2));
  right: 0;
  min-width: 160px;
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  box-shadow: 0 4px 12px rgba(0,0,0,0.1);
  z-index: 100;
  overflow: hidden;
}
.portal-header__dropdown--lang {
  min-width: 120px;
}
.portal-header__dropdown-header {
  padding: var(--space-3) var(--space-4);
  font-size: var(--text-xs);
  color: var(--color-muted);
  border-bottom: 1px solid var(--color-border);
  font-weight: 500;
}
.portal-header__dropdown-item {
  display: block;
  width: 100%;
  padding: var(--space-3) var(--space-4);
  font-size: var(--text-sm);
  color: var(--color-text);
  background: none;
  border: none;
  text-align: left;
  cursor: pointer;
  min-height: 44px;
  display: flex;
  align-items: center;
  transition: background 0.15s;
}
.portal-header__dropdown-item:hover {
  background: var(--color-surface-2);
}
.portal-header__dropdown-item--active {
  color: var(--color-primary);
  font-weight: 600;
}
.portal-header__dropdown-item--danger {
  color: var(--color-danger);
}
.portal-header__dropdown-item--danger:hover {
  background: var(--color-danger-bg, #fef2f2);
}
.portal-main {
  padding: var(--space-5);
  max-width: 720px;
  margin: 0 auto;
}
.fade-enter-active, .fade-leave-active {
  transition: opacity 0.15s ease;
}
.fade-enter-from, .fade-leave-to {
  opacity: 0;
}

/* Mobile */
@media (max-width: 640px) {
  .portal-header {
    padding: var(--space-2) var(--space-3);
  }
  .portal-header__title {
    display: none;
  }
  .portal-header__user-name {
    display: none;
  }
  .portal-main {
    padding: var(--space-3);
  }
}

/* Extra small screens */
@media (max-width: 360px) {
  .portal-header {
    padding: var(--space-1) var(--space-2);
    gap: var(--space-1);
  }
  .portal-header__actions {
    gap: var(--space-1);
  }
  .portal-header__btn {
    min-width: 36px;
    min-height: 36px;
    padding: var(--space-1);
    font-size: 12px;
  }
  .portal-header__user-toggle {
    padding: var(--space-1);
    gap: var(--space-1);
    min-height: 36px;
  }
  .portal-header__user-avatar {
    width: 24px;
    height: 24px;
    font-size: 10px;
  }
  .portal-header__logo {
    width: 28px;
    height: 28px;
    font-size: 12px;
  }
  .portal-header__chevron {
    display: none;
  }
  .portal-main {
    padding: var(--space-2);
  }
}

/* RTL support */
[dir="rtl"] .portal-header {
  flex-direction: row-reverse;
}
[dir="rtl"] .portal-header__brand {
  flex-direction: row-reverse;
}
[dir="rtl"] .portal-header__actions {
  margin-left: 0;
  margin-right: auto;
  flex-direction: row-reverse;
}
[dir="rtl"] .portal-header__user-toggle {
  flex-direction: row-reverse;
}
[dir="rtl"] .portal-header__dropdown {
  right: auto;
  left: 0;
}
[dir="rtl"] .portal-header__dropdown-item {
  text-align: right;
}
[dir="rtl"] .portal-header__dropdown-header {
  text-align: right;
}
[dir="rtl"] .portal-main {
  text-align: right;
}
</style>
