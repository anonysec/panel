<template>
  <textarea
    :id="id"
    :value="modelValue"
    :placeholder="placeholder"
    :disabled="disabled"
    :rows="rows"
    :aria-describedby="ariaDescribedby"
    :aria-disabled="disabled"
    class="k-textarea"
    :class="{ 'k-textarea--disabled': disabled }"
    :style="{ minHeight: `${Number(rows) * 1.5 + 1}em` }"
    @input="onInput"
  ></textarea>
</template>

<script setup lang="ts">
export interface KTextareaProps {
  modelValue?: string
  placeholder?: string
  disabled?: boolean
  rows?: number | string
  id?: string
  ariaDescribedby?: string
}

withDefaults(defineProps<KTextareaProps>(), {
  modelValue: '',
  placeholder: '',
  disabled: false,
  rows: 3,
})

const emit = defineEmits<{
  (e: 'update:modelValue', value: string): void
}>()

function onInput(event: Event) {
  const target = event.target as HTMLTextAreaElement
  emit('update:modelValue', target.value)
}
</script>

<style scoped>
.k-textarea {
  display: block;
  width: 100%;
  padding: var(--space-2) var(--space-3);
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  color: var(--color-text);
  font-family: var(--font-family);
  font-size: var(--text-base);
  line-height: var(--leading-normal);
  outline: none;
  resize: vertical;
  transition:
    border-color var(--duration-normal) var(--ease-default),
    box-shadow var(--duration-normal) var(--ease-default);
}

.k-textarea::placeholder {
  color: var(--color-muted);
}

.k-textarea:focus-visible {
  border-color: var(--color-primary);
  box-shadow: 0 0 0 2px rgba(37, 99, 235, 0.25);
}

.k-textarea--disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
</style>
