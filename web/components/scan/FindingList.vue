<script setup lang="ts">
import type { Finding } from '~/types/goodast'

const props = defineProps<{ findings: Finding[] }>()
const sorted = computed(() => sortFindingsBySeverity(props.findings))
</script>

<template>
  <div v-if="sorted.length" class="space-y-4">
    <FindingCard v-for="f in sorted" :key="f.id" :finding="f" />
  </div>
  <p v-else class="border border-hairline bg-surface-soft p-6 text-body-sm text-muted">
    検出はありませんでした。
  </p>
</template>
