<template>
  <span
    class="k-avatar"
    :style="avatarStyles"
    :aria-label="`Avatar for ${name}`"
    role="img"
  >
    <img
      v-if="src && !imgFailed"
      :src="src"
      :alt="name"
      class="k-avatar__image"
      @error="handleImageError"
    />
    <span v-else-if="emoji" class="k-avatar__emoji">{{ emoji }}</span>
    <span v-else class="k-avatar__initials">{{ initials }}</span>
  </span>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'

export interface KAvatarProps {
  name: string
  size?: number | 'sm' | 'md' | 'lg'
  src?: string
  emoji?: string
}

const props = withDefaults(defineProps<KAvatarProps>(), {
  size: 32,
})

const imgFailed = ref(false)

const resolvedSize = computed(() => {
  if (typeof props.size === 'number') return props.size
  switch (props.size) {
    case 'sm': return 24
    case 'md': return 32
    case 'lg': return 48
    default: return 32
  }
})

const initials = computed(() => {
  return props.name.slice(0, 2).toUpperCase()
})

function hashString(str: string): number {
  let hash = 0
  for (let i = 0; i < str.length; i++) {
    const char = str.charCodeAt(i)
    hash = ((hash << 5) - hash) + char
    hash = hash & hash // Convert to 32bit integer
  }
  return Math.abs(hash)
}

const gradientBackground = computed(() => {
  const hash = hashString(props.name)
  const hue1 = hash % 360
  const hue2 = (hue1 + 45) % 360
  return `linear-gradient(135deg, hsl(${hue1}, 60%, 45%), hsl(${hue2}, 70%, 55%))`
})

const avatarStyles = computed(() => ({
  width: `${resolvedSize.value}px`,
  height: `${resolvedSize.value}px`,
  fontSize: props.emoji
    ? `${Math.round(resolvedSize.value * 0.55)}px`
    : `${Math.round(resolvedSize.value * 0.38)}px`,
  background: (!props.src || imgFailed.value) ? gradientBackground.value : 'transparent',
}))

function handleImageError() {
  imgFailed.value = true
}
</script>

<style scoped>
.k-avatar {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border-radius: 50%;
  overflow: hidden;
  flex-shrink: 0;
  user-select: none;
}

.k-avatar__image {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.k-avatar__initials {
  color: #fff;
  font-family: var(--font-family);
  font-weight: var(--font-semibold);
  line-height: 1;
  letter-spacing: var(--tracking-wide);
}

.k-avatar__emoji {
  line-height: 1;
  text-align: center;
}
</style>
