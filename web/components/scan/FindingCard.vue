<script setup lang="ts">
import type { Finding } from '~/types/goodast'

defineProps<{ finding: Finding }>()
</script>

<template>
  <article data-testid="finding" class="border border-hairline bg-surface-card p-6">
    <div class="flex items-center justify-between gap-4">
      <p class="font-display text-title-md font-bold text-on-dark">{{ finding.title }}</p>
      <ScanSeverityBadge :severity="finding.severity" />
    </div>
    <p class="mt-2 break-all text-body-sm">{{ finding.url }}</p>
    <p v-if="finding.cwe" data-testid="finding-cwe" class="mt-2 text-caption uppercase tracking-caption text-muted">{{ finding.cwe }}</p>
    <p v-if="finding.remediation" data-testid="finding-remediation" class="mt-3 text-body-sm">
      <span class="text-label font-bold uppercase tracking-label text-muted">修正方法：</span>{{ finding.remediation }}
    </p>
  </article>
</template>
