<script setup lang="ts">
import Chart from 'chart.js/auto'
import type { ChartConfiguration } from 'chart.js'
import { toRaw } from 'vue'

const props = defineProps<{ config: ChartConfiguration }>()

const canvasRef = ref<HTMLCanvasElement>()

// Chart インスタンスはリアクティブにしない（Chart.js 内部状態を Vue が proxy 化すると壊れる）
let chart: Chart

onMounted(() => {
  chart = new Chart(canvasRef.value!, toRaw(props.config))
})

watch(
  () => props.config,
  (config) => {
    // options は静的（palette は mount 時に解決済み）。動的にする場合は chart.options も更新すること
    chart.data = toRaw(config).data
    chart.update()
  },
)

onUnmounted(() => {
  chart.destroy()
})
</script>

<template>
  <canvas ref="canvasRef" />
</template>
