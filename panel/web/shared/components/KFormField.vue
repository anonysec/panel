<template>
  <div class="k-form-field">
    <label :for="fieldId" class="k-form-field__label">
      {{ label }}
      <span v-if="required" class="k-form-field__required" aria-hidden="true">*</span>
    </label>

    <slot
      :field-id="fieldId"
      :error-id="errorId"
      :hint-id="hintId"
      :described-by="describedBy"
    />

    <p
      v-if="error"
      :id="errorId"
      class="k-form-field__error"
      aria-live="polite"
    >
      {{ error }}
    </p>

    <p
      v-else-if="hint"
      :id="hintId"
      class="k-form-field__hint"
    >
      {{ hint }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { KFormFieldProps } from '@koris/types/components'

const props = defineProps<KFormFieldProps>()

let fieldCounter = 0
const autoId = `field-auto-${++fieldCounter}`

const fieldId = computed(() => props.name ? `field-${props.name}` : autoId)
const errorId = computed(() => `${fieldId.value}-error`)
const hintId = computed(() => `${fieldId.value}-hint`)

const describedBy = computed(() => {
  if (props.error) return errorId.value
  if (props.hint) return hintId.value
  return undefined
})
</script>

<style scoped>
.k-form-field {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.k-form-field__label {
  font-size: 13px;
  font-weight: 500;
  color: var(--color-text);
}

.k-form-field__required {
  color: var(--color-danger);
  margin-left: 2px;
}

.k-form-field__error {
  font-size: 12px;
  color: var(--color-danger);
  margin: 0;
}

.k-form-field__hint {
  font-size: 12px;
  color: var(--color-muted);
  margin: 0;
}
</style>
