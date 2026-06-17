<script setup lang="ts">
import { ref } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { useI18n } from '@koris/composables/useI18n'
import KFormField from '@koris/ui/KFormField.vue'
import KInput from '@koris/ui/KInput.vue'
import KButton from '@koris/ui/KButton.vue'

const { t } = useI18n()
const router = useRouter()
const route = useRoute()
const store = useAuthStore()

const username = ref('')
const password = ref('')
const errors = ref<{ username?: string; password?: string }>({})

function validate(): boolean {
  errors.value = {}
  if (!username.value.trim()) {
    errors.value.username = t('login.username_required')
  }
  if (!password.value) {
    errors.value.password = t('login.password_required')
  }
  return Object.keys(errors.value).length === 0
}

async function handleLogin() {
  if (!validate()) return

  const success = await store.login(username.value, password.value)
  if (success) {
    const redirect = route.query.redirect as string
    router.replace(redirect || '/dashboard')
  }
}
</script>

<template>
  <div class="login-page">
    <!-- Left: Branding Hero -->
    <aside class="login-hero">
      <div class="login-hero__content">
        <div class="login-hero__logo">
          <span class="login-hero__logo-icon">&#9670;</span>
          <span class="login-hero__logo-text">KorisPanel</span>
        </div>
        <h1 class="login-hero__title">{{ t('login.hero_title') }}</h1>
        <p class="login-hero__desc">
          {{ t('login.hero_desc') }}
        </p>
      </div>
      <div class="login-hero__gradient" />
    </aside>

    <!-- Right: Login Form -->
    <main class="login-form-wrapper">
      <form class="login-form" @submit.prevent="handleLogin">
        <h2 class="login-form__title">{{ t('login.sign_in') }}</h2>
        <p class="login-form__subtitle text-muted">{{ t('login.sign_in_subtitle') }}</p>

        <div class="login-form__fields">
          <KFormField name="username" :label="t('login.username')" :error="errors.username">
            <template #default="{ fieldId, describedBy }">
              <KInput
                :id="fieldId"
                v-model="username"
                autocomplete="username"
                placeholder="admin"
                :aria-describedby="describedBy"
              />
            </template>
          </KFormField>

          <KFormField name="password" :label="t('login.password')" :error="errors.password">
            <template #default="{ fieldId, describedBy }">
              <KInput
                :id="fieldId"
                v-model="password"
                type="password"
                autocomplete="current-password"
                :placeholder="t('login.enter_password')"
                :aria-describedby="describedBy"
              />
            </template>
          </KFormField>
        </div>

        <!-- Error Message -->
        <div v-if="store.error" class="login-form__error" role="alert">
          {{ t(store.error) || store.error }}
        </div>

        <KButton
          type="submit"
          variant="primary"
          :loading="store.loading"
          full-width
        >
          {{ t('login.sign_in_btn') }}
        </KButton>
      </form>
    </main>
  </div>
</template>

<style scoped>
.login-page {
  display: grid;
  grid-template-columns: 1fr 1fr;
  min-height: 100vh;
}

/* Hero Section */
.login-hero {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: var(--space-8);
  background: var(--color-bg);
  overflow: hidden;
}

.login-hero__content {
  position: relative;
  z-index: 1;
  max-width: 400px;
}

.login-hero__logo {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  margin-bottom: var(--space-6);
}

.login-hero__logo-icon {
  font-size: var(--text-2xl);
  color: var(--color-primary);
}

.login-hero__logo-text {
  font-size: var(--text-lg);
  font-weight: var(--font-bold);
  color: var(--color-text);
}

.login-hero__title {
  font-size: 2.5rem;
  font-weight: var(--font-bold);
  line-height: 1.2;
  color: var(--color-text);
  margin: 0 0 var(--space-4);
}

.login-hero__desc {
  font-size: var(--text-base);
  color: var(--color-muted);
  line-height: 1.6;
}

.login-hero__gradient {
  position: absolute;
  inset: 0;
  background: radial-gradient(ellipse at 30% 50%, rgba(37, 99, 235, 0.08) 0%, transparent 70%);
  pointer-events: none;
}

/* Form Section */
.login-form-wrapper {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: var(--space-8);
  background: var(--color-surface);
}

.login-form {
  width: 100%;
  max-width: 380px;
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}

.login-form__title {
  margin: 0;
  font-size: var(--text-2xl);
  font-weight: var(--font-bold);
}

.login-form__subtitle {
  margin: 0;
  font-size: var(--text-sm);
}

.login-form__fields {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  margin: var(--space-2) 0;
}

.login-form__error {
  padding: var(--space-2) var(--space-3);
  background: rgba(239, 68, 68, 0.1);
  border: 1px solid rgba(239, 68, 68, 0.3);
  border-radius: var(--radius-md);
  color: var(--color-danger);
  font-size: var(--text-sm);
}

.text-muted { color: var(--color-muted); }

@media (max-width: 768px) {
  .login-page { grid-template-columns: 1fr; }
  .login-hero { display: none; }
}
</style>
