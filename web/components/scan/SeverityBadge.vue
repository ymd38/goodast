<script setup lang="ts">
// 生成型 Finding.severity は optional のため受け口も optional（未設定は未知扱いで muted）
const props = defineProps<{ severity?: string }>()
const cls = computed(() => severityTextClass(props.severity ?? ''))
</script>

<template>
  <span data-testid="severity-badge" :class="['text-caption font-bold uppercase tracking-caption', cls]">
    {{ severity }}
  </span>
</template>
