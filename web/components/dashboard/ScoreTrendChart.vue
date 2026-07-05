<script setup lang="ts">
import type { HistoryEntry } from '~/types/goodast'

const props = defineProps<{ history: HistoryEntry[] }>()

const enough = computed(() => hasTrendData(props.history))
// config は <ClientOnly> 配下でのみ評価される（resolveChartPalette は client 専用）
const config = computed(() => buildScoreTrendConfig(props.history, resolveChartPalette()))
</script>

<template>
  <section class="bg-surface-card p-6">
    <h2 class="font-display text-label font-bold uppercase tracking-label text-muted">
      スコア推移
    </h2>
    <div v-if="enough" class="mt-4 h-64">
      <ClientOnly>
        <DashboardChartCanvas :config="config" />
      </ClientOnly>
    </div>
    <p v-else class="mt-4 text-body-sm text-muted">
      データ不足 — 2回以上のスキャンで推移が表示されます
    </p>
  </section>
</template>
