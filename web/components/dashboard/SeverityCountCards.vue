<script setup lang="ts">
import type { SeverityCounts } from '~/types/goodast'

const props = defineProps<{ counts: SeverityCounts }>()

const items = computed(() => [
  { label: 'Critical', value: props.counts.critical ?? 0 },
  { label: 'High', value: props.counts.high ?? 0 },
  { label: 'Medium', value: props.counts.medium ?? 0 },
  { label: 'Low', value: props.counts.low ?? 0 },
  { label: 'Info', value: props.counts.info ?? 0 },
])
</script>

<template>
  <div class="grid grid-cols-2 gap-4 md:grid-cols-5">
    <div
      v-for="item in items"
      :key="item.label"
      data-testid="severity-cell"
      class="bg-surface-soft p-6"
    >
      <p class="font-display text-display-sm font-bold text-on-dark">{{ item.value }}</p>
      <p class="mt-1 text-label font-bold uppercase tracking-label text-muted">{{ item.label }}</p>
    </div>
  </div>
</template>
