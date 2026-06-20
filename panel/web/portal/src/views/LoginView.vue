<script setup lang="ts">
import { ref, computed } from 'vue'
import { useRouter } from 'vue-router'
import { usePortalAuthStore } from '@/stores/auth'
import KButton from '@koris/ui/KButton.vue'
import KFormField from '@koris/ui/KFormField.vue'
import KInput from '@koris/ui/KInput.vue'

const router = useRouter()
const auth = usePortalAuthStore()

const form = ref({
  username: '',
  password: '',
  totp_code: '',
})

const formError = ref('')
const showTotp = computed(() => auth.totpRequired)

async function handleLogin() {
  formError.value = ''

  if (!form.value.username || !form.value.password) {
    formError.value = 'Please enter username and password'
    return
  }

  const success = await auth.login({
    username: form.value.username,
    password: form.value.password,
    totp_code: form.value.totp_code || undefined,
  })

  if (success) {
    await router.replace({ name: 'portal-home' })
  } else if (!auth.totpRequired) {
    formError.value = auth.error || 'Invalid credentials'
  }
}
</script>
<template>
  <div class="login-page">
    <div class="login-hero">
      <div class="login-hero__brand">
        <span class="login-hero__logo">K</span>
        <div>
          <div class="login-hero__title">KorisPanel</div>
          <div class="login-hero__subtitle">Client Portal</div>
        </div>
      </div>
      <h1 class="login-hero__heading">Your VPN<br>Dashboard</h1>
      <p class="login-hero__desc">Monitor your account, download connection profiles, manage subscription, and get support.</p>
      <div class="login-hero__chips">
        <span>OpenVPN</span>
        <span>L2TP/IPSec</span>
        <span>IKEv2</span>
      </div>
    </div>

    <div class="login-card">
      <h2 class="login-card__title">Welcome back</h2>
      <p class="login-card__subtitle">Sign in with your account credentials</p>

      <form class="login-form" @submit.prevent="handleLogin">
        <KFormField label="Username">
          <KInput
            v-model="form.username"
            placeholder="Your username"
            autocomplete="username"
          />
        </KFormField>

        <KFormField label="Password">
          <KInput
            v-model="form.password"
            type="password"
            placeholder="Your password"
            autocomplete="current-password"
          />
        </KFormField>

        <KFormField v-if="showTotp" label="TOTP Code" :required="true">
          <KInput
            v-model="form.totp_code"
            placeholder="6-digit code"
            autocomplete="one-time-code"
            maxlength="6"
          />
        </KFormField>

        <div v-if="formError" class="login-error" role="alert">
          {{ formError }}
        </div>

        <KButton
          type="submit"
          variant="primary"
          size="lg"
          :loading="auth.loading"
          :full-width="true"
        >
          {{ showTotp ? 'Verify Code' : 'Sign In' }}
        </KButton>
      </form>
    </div>
  </div>
</template>
<style scoped>
.login-page {
  min-height: 100vh;
  display: grid;
  grid-template-columns: 1fr 1fr;
  background: var(--color-bg);
}
.login-hero {
  display: flex;
  flex-direction: column;
  justify-content: center;
  padding: var(--space-12);
  background: var(--color-surface);
  border-right: 1px solid var(--color-border);
}
.login-hero__brand {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  margin-bottom: var(--space-8);
}
.login-hero__logo {
  width: 40px;
  height: 40px;
  border-radius: var(--radius-md);
  background: var(--gradient-brand);
  display: flex;
  align-items: center;
  justify-content: center;
  color: #fff;
  font-weight: 800;
  font-size: 16px;
}
.login-hero__title { font-weight: 700; font-size: var(--text-lg); }
.login-hero__subtitle { font-size: var(--text-xs); color: var(--color-muted); }
.login-hero__heading {
  font-size: 2.5rem;
  font-weight: 800;
  line-height: 1.2;
  margin-bottom: var(--space-4);
}
.login-hero__desc {
  color: var(--color-muted);
  font-size: var(--text-sm);
  max-width: 400px;
  margin-bottom: var(--space-6);
}
.login-hero__chips {
  display: flex;
  gap: var(--space-2);
}
.login-hero__chips span {
  padding: var(--space-1) var(--space-3);
  border-radius: var(--radius-full);
  background: var(--color-surface-2);
  font-size: var(--text-xs);
  color: var(--color-muted);
}
.login-card {
  display: flex;
  flex-direction: column;
  justify-content: center;
  padding: var(--space-12);
  max-width: 420px;
  margin: 0 auto;
  width: 100%;
}
.login-card__title {
  font-size: var(--text-xl);
  font-weight: 700;
  margin-bottom: var(--space-2);
}
.login-card__subtitle {
  color: var(--color-muted);
  font-size: var(--text-sm);
  margin-bottom: var(--space-8);
}
.login-form {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}
.login-error {
  padding: var(--space-3);
  border-radius: var(--radius-md);
  background: rgba(239, 68, 68, 0.1);
  color: var(--color-danger);
  font-size: var(--text-sm);
  border: 1px solid rgba(239, 68, 68, 0.2);
}
@media (max-width: 768px) {
  .login-page { grid-template-columns: 1fr; }
  .login-hero { display: none; }
}
</style>
