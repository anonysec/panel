<script setup lang="ts">
import { ref } from 'vue'
import { usePortalAuthStore } from '@/stores/auth'
import KButton from '@koris/ui/KButton.vue'
import KFormField from '@koris/ui/KFormField.vue'
import KInput from '@koris/ui/KInput.vue'

const auth = usePortalAuthStore()

const profileForm = ref({
  display_name: auth.user?.display_name || '',
})

const passwordForm = ref({
  current_password: '',
  new_password: '',
  confirm_password: '',
})

const notice = ref('')
const passwordError = ref('')

async function handleUpdateProfile() {
  notice.value = ''
  const success = await auth.updateProfile({
    display_name: profileForm.value.display_name,
  })
  if (success) {
    notice.value = 'Profile updated successfully.'
  }
}

async function handleChangePassword() {
  notice.value = ''
  passwordError.value = ''

  if (!passwordForm.value.current_password || !passwordForm.value.new_password) {
    passwordError.value = 'Please fill in all password fields.'
    return
  }

  if (passwordForm.value.new_password !== passwordForm.value.confirm_password) {
    passwordError.value = 'New passwords do not match.'
    return
  }

  if (passwordForm.value.new_password.length < 6) {
    passwordError.value = 'New password must be at least 6 characters.'
    return
  }

  const success = await auth.updateProfile({
    current_password: passwordForm.value.current_password,
    password: passwordForm.value.new_password,
  })

  if (success) {
    notice.value = 'Password changed successfully.'
    passwordForm.value = { current_password: '', new_password: '', confirm_password: '' }
  } else {
    passwordError.value = auth.error || 'Failed to change password.'
  }
}
</script>
<template>
  <div class="profile">
    <h1 class="profile__title">Profile Settings</h1>

    <div v-if="notice" class="profile__notice" role="status">{{ notice }}</div>

    <!-- Account Info -->
    <section class="profile__section">
      <h2 class="profile__section-title">Account Information</h2>
      <div class="profile__info">
        <div class="profile__info-item">
          <span class="profile__info-label">Username</span>
          <span class="profile__info-value">{{ auth.username }}</span>
        </div>
        <div class="profile__info-item">
          <span class="profile__info-label">Status</span>
          <span class="profile__info-value">{{ auth.status }}</span>
        </div>
        <div class="profile__info-item">
          <span class="profile__info-label">Plan</span>
          <span class="profile__info-value">{{ auth.planName }}</span>
        </div>
      </div>
    </section>

    <!-- Update Display Name -->
    <section class="profile__section">
      <h2 class="profile__section-title">Display Name</h2>
      <form class="profile__form" @submit.prevent="handleUpdateProfile">
        <KFormField label="Display Name">
          <KInput v-model="profileForm.display_name" placeholder="Your display name" />
        </KFormField>
        <KButton type="submit" variant="primary" :loading="auth.loading">
          Update Name
        </KButton>
      </form>
    </section>

    <!-- Change Password -->
    <section class="profile__section">
      <h2 class="profile__section-title">Change Password</h2>
      <form class="profile__form" @submit.prevent="handleChangePassword">
        <KFormField label="Current Password" :required="true">
          <KInput v-model="passwordForm.current_password" type="password" placeholder="Current password" autocomplete="current-password" />
        </KFormField>

        <KFormField label="New Password" :required="true">
          <KInput v-model="passwordForm.new_password" type="password" placeholder="New password" autocomplete="new-password" />
        </KFormField>

        <KFormField label="Confirm New Password" :required="true">
          <KInput v-model="passwordForm.confirm_password" type="password" placeholder="Confirm new password" autocomplete="new-password" />
        </KFormField>

        <div v-if="passwordError" class="profile__error" role="alert">
          {{ passwordError }}
        </div>

        <KButton type="submit" variant="primary" :loading="auth.loading">
          Change Password
        </KButton>
      </form>
    </section>
  </div>
</template>
<style scoped>
.profile__title {
  font-size: var(--text-2xl);
  font-weight: 700;
  margin-bottom: var(--space-6);
}
.profile__notice {
  padding: var(--space-3) var(--space-4);
  border-radius: var(--radius-md);
  background: rgba(34, 197, 94, 0.1);
  color: var(--color-success);
  font-size: var(--text-sm);
  margin-bottom: var(--space-4);
  border: 1px solid rgba(34, 197, 94, 0.2);
}
.profile__section {
  padding: var(--space-5);
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-lg);
  margin-bottom: var(--space-4);
}
.profile__section-title {
  font-size: var(--text-md);
  font-weight: 600;
  margin-bottom: var(--space-4);
}
.profile__info {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}
.profile__info-item {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: var(--space-2) 0;
  border-bottom: 1px solid var(--color-border);
}
.profile__info-item:last-child {
  border-bottom: none;
}
.profile__info-label {
  font-size: var(--text-sm);
  color: var(--color-muted);
}
.profile__info-value {
  font-size: var(--text-sm);
  font-weight: 500;
}
.profile__form {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
  max-width: 400px;
}
.profile__error {
  padding: var(--space-3);
  border-radius: var(--radius-md);
  background: rgba(239, 68, 68, 0.1);
  color: var(--color-danger);
  font-size: var(--text-sm);
  border: 1px solid rgba(239, 68, 68, 0.2);
}
</style>
