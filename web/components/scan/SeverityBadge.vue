<script setup lang="ts">
const props = defineProps<{ severity: string }>()
const cls = computed(() => severityTextClass(props.severity))
</script>

<template>
  <span data-testid="severity-badge" :class="['text-caption font-bold uppercase tracking-caption', cls]">
    {{ severity }}
  </span>
</template>
