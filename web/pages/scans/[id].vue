<script setup lang="ts">
import type { Finding, LatestState } from '~/types/goodast'

const route = useRoute()
const scanId = route.params.id as string
const client = useApiClient()
const { state, done, start } = useScanPolling(scanId)

const findings = ref<Finding[]>([])
const findingsError = ref<string | null>(null)

const isDone = computed(() => done.value && state.value?.status === 'done')

// done になったら明細を取得する。テストが mount 前に done=true をセットするケースがあるため
// immediate: true で初回描画時にも判定する（非 immediate では発火しないケースがある）
watch(
  isDone,
  async (v) => {
    if (!v) return
    const { data, error } = await client.GET('/scans/{id}/findings', {
      params: { path: { id: scanId } },
    })
    if (error || !data) {
      findingsError.value = toApiErrorMessage(error)
      return
    }
    findings.value = data.findings ?? []
  },
  { immediate: true },
)

// summary → ScoreCard 用 LatestState 形へ寄せる（delta/date は無し）
const latest = computed<LatestState | null>(() => {
  const s = state.value?.summary
  if (!s) return null
  return { scan_id: scanId, score: s.score, band: s.band, label: s.label, counts: s.counts }
})

onMounted(start)
</script>

<template>
  <section class="mx-auto max-w-3xl">
    <h1 class="font-display text-display-sm font-bold uppercase text-on-dark">スキャン結果</h1>

    <ScanProgress v-if="!isDone" class="mt-8" :status="state?.status ?? 'queued'" />

    <template v-else>
      <DashboardScoreCard class="mt-8" :latest="latest" />
      <p v-if="findingsError" class="mt-6 border border-m-red p-4 text-body-sm text-m-red">
        {{ findingsError }}
      </p>
      <ScanFindingList class="mt-6" :findings="findings" />
      <div class="mt-10 border-t border-hairline pt-6 text-body-sm text-muted">
        より詳しい対策が必要ですか？
        <a href="#" data-testid="expert" class="text-on-dark underline">専門家への相談</a>
        をご検討ください。
      </div>
    </template>
  </section>
</template>
