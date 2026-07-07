<script setup lang="ts">
const props = defineProps<{ status: string }>()
const label = computed(() => (props.status === 'queued' ? '待機中…' : 'スキャン実行中…'))
</script>

<template>
  <div v-if="status === 'failed'" data-testid="scan-failed" class="border border-m-red p-6 text-body-sm text-m-red">
    スキャンに失敗しました。時間をおいて再度お試しください。
  </div>
  <div v-else class="border border-hairline bg-surface-soft p-6">
    <p class="animate-pulse font-display text-title-md font-bold text-on-dark">{{ label }}</p>
    <p class="mt-2 text-caption uppercase tracking-caption text-muted">queued → running → done</p>
  </div>
</template>
