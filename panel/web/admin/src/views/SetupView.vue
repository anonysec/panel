<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import KFormField from '@koris/ui/KFormField.vue'
import KInput from '@koris/ui/KInput.vue'
import KButton from '@koris/ui/KButton.vue'

const router = useRouter()
const store = useAuthStore()

const username = ref('')
const password = ref('')
const confirmPassword = ref('')
const setupKey = ref('')
const errors = ref<{ username?: string; password?: string; confirm?: string }>({})

function validate(): boolean {
  errors.value = {}
  if (!username.value.trim()) {
    errors.value.username = 'Username is required'
  } else if (username.value.length < 3) {
    errors.value.username = 'Username must be at least 3 characters'
  }
  if (!password.value) {
    errors.value.password = 'Password is required'
  } else if (password.value.length < 6) {
    errors.value.password = 'Password must be at least 6 characters'
  }
  if (password.value !== confirmPassword.value) {
    errors.value.confirm = 'Passwords do not match'
  }
  return Object.keys(errors.value).length === 0
}

async function handleSetup() {
  if (!validate()) return

  const params: { username: string; password: string; setup_key?: string } = {
    username: username.value,
    password: password.value,
  }
  if (store.setupKeyRequired && setupKey.value) {
    params.setup_key = setupKey.value
  }

  const success = await store.setup(params)
  if (success) {
    router.replace('/dashboard')
  }
}
</script>

<template>
  <div class="setup-page">
    <!-- Left: Branding Hero -->
    <aside class="setup-hero">
      <div class="setup-hero__content">
        <div class="setup-hero__logo">
          <span class="setup-hero__logo-icon">◆</span>
          <span class="setup-hero__logo-text">KorisPanel</span>
        </div>
        <h1 class="setup-hero__title">Welcome to<br>KorisPanel</h1>
        <p class="setup-hero__desc">
          Set up your admin account to start managing your VPN infrastructure.
        </p>
      </div>
      <div class="setup-hero__gradient" />
    </aside>

    <!-- Right: Setup Form -->
    <main class="setup-form-wrapper">
      <form class="setup-form" @submit.prevent="handleSetup">
        <h2 class="setup-form__title">Initial Setup</h2>
        <p class="setup-form__subtitle text-muted">Create the owner account to get started.</p>

        <div class="setup-form__fields">
          <!-- Setup Key (if required) -->
          <KFormField v-if="store.setupKeyRequired" name="setup-key" label="Setup Key" hint="Provided during installation">
            <template #default="{ fieldId, describedBy }">
              <KInput
                :id="fieldId"
                v-model="setupKey"
                placeholder="Enter setup key"
                :aria-describedby="describedBy"
              />
            </template>
          </KFormField>

          <KFormField name="setup-username" label="Username" :error="errors.username" required>
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

          <KFormField name="setup-password" label="Password" :error="errors.password" required>
            <template #default="{ fieldId, describedBy }">
              <KInput
                :id="fieldId"
                v-model="password"
                type="password"
                autocomplete="new-password"
                placeholder="Min 6 characters"
                :aria-describedby="describedBy"
              />
            </template>
          </KFormField>

          <KFormField name="setup-confirm" label="Confirm Password" :error="errors.confirm" required>
            <template #default="{ fieldId, describedBy }">
              <KInput
                :id="fieldId"
                v-model="confirmPassword"
                type="password"
                autocomplete="new-password"
                placeholder="Repeat password"
                :aria-describedby="describedBy"
              />
            </template>
          </KFormField>
        </div>

        <!-- Error Message -->
        <div v-if="store.error" class="setup-form__error" role="alert">
          {{ store.error }}
        </div>

        <KButton
          type="submit"
          variant="primary"
          :loading="store.loading"
          full-width
        >
          Create Account
        </KButton>
      </form>
    </main>
  </div>
</template>

<style scoped>
.setup-page {
  display: grid;
  grid-template-columns: 1fr 1fr;
  min-height: 100vh;
}

/* Hero Section */
.setup-hero {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: var(--space-8);
  background: var(--color-bg);
  overflow: hidden;
}

.setup-hero__content {
  position: relative;
  z-index: 1;
  max-width: 400px;
}

.setup-hero__logo {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  margin-bottom: var(--space-6);
}

.setup-hero__logo-icon {
  font-size: var(--text-2xl);
  color: var(--color-accent);
}

.setup-hero__logo-text {
  font-size: var(--text-lg);
  font-weight: var(--font-bold);
  color: var(--color-text);
}

.setup-hero__title {
  font-size: 2.5rem;
  font-weight: var(--font-bold);
  line-height: 1.2;
  color: var(--color-text);
  margin: 0 0 var(--space-4);
}

.setup-hero__desc {
  font-size: var(--text-base);
  color: var(--color-muted);
  line-height: 1.6;
}

.setup-hero__gradient {
  position: absolute;
  inset: 0;
  background: radial-gradient(ellipse at 30% 50%, rgba(34, 211, 238, 0.08) 0%, transparent 70%);
  pointer-events: none;
}

/* Form Section */
.setup-form-wrapper {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: var(--space-8);
  background: var(--color-surface);
}

.setup-form {
  width: 100%;
  max-width: 380px;
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}

.setup-form__title {
  margin: 0;
  font-size: var(--text-2xl);
  font-weight: var(--font-bold);
}

.setup-form__subtitle {
  margin: 0;
  font-size: var(--text-sm);
}

.setup-form__fields {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  margin: var(--space-2) 0;
}

.setup-form__error {
  padding: var(--space-2) var(--space-3);
  background: rgba(239, 68, 68, 0.1);
  border: 1px solid rgba(239, 68, 68, 0.3);
  border-radius: var(--radius-md);
  color: var(--color-danger);
  font-size: var(--text-sm);
}

.text-muted { color: var(--color-muted); }

@media (max-width: 768px) {
  .setup-page { grid-template-columns: 1fr; }
  .setup-hero { display: none; }
}
</style>
