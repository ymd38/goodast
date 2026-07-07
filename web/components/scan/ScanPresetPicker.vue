<script setup lang="ts">
import { SCAN_PRESETS, type ScanPresetValue } from '~/utils/scan-preset'

defineProps<{ modelValue: ScanPresetValue }>()
const emit = defineEmits<{ 'update:modelValue': [ScanPresetValue] }>()
</script>

<template>
  <div class="grid gap-4 md:grid-cols-3">
    <button
      v-for="p in SCAN_PRESETS"
      :key="p.value"
      type="button"
      data-preset-card
      :data-testid="`preset-${p.value}`"
      :class="[
        'border p-6 text-left transition-colors',
        modelValue === p.value ? 'border-on-dark bg-surface-card' : 'border-hairline bg-surface-soft',
      ]"
      @click="emit('update:modelValue', p.value)"
    >
      <p class="font-display text-title-md font-bold text-on-dark">{{ p.label }}</p>
      <p class="mt-2 text-body-sm">{{ p.description }}</p>
      <p class="mt-3 text-caption uppercase tracking-caption text-muted">{{ p.estimate }}</p>
    </button>
  </div>
</template>
