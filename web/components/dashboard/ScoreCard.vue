<script setup lang="ts">
import type { LatestState } from '~/types/goodast'

const props = defineProps<{ latest: LatestState | null }>()

const band = computed(() => scoreBandStyle(props.latest?.band))
const delta = computed(() => formatDelta(props.latest?.delta))
</script>

<template>
  <section class="bg-surface-card p-6">
    <h2 class="font-display text-label font-bold uppercase tracking-label text-muted">
      Goodast Security Score
    </h2>
    <template v-if="latest">
      <p class="mt-2 flex items-baseline gap-3">
        <span
          data-testid="score-value"
          class="font-display text-display-lg font-bold"
          :class="[band.text, { 'animate-pulse': band.emphasis }]"
        >
          {{ latest.score }}
        </span>
        <span v-if="delta" class="text-title-md text-body-strong">{{ delta }}</span>
      </p>
      <p class="mt-1 text-body-sm">
        {{ latest.label }}
        <span v-if="latest.date" data-testid="score-date" class="ml-2 text-caption text-muted">
          {{ latest.date }}
        </span>
      </p>
    </template>
    <p v-else class="mt-2 text-body-sm text-muted">スキャン未実行</p>
  </section>
</template>
