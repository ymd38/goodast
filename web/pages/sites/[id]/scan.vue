<script setup lang="ts">
import { DEFAULT_PRESET, type ScanPresetValue } from '~/utils/scan-preset'

const route = useRoute()
const siteId = String(route.params.id)
const client = useApiClient()

const preset = ref<ScanPresetValue>(DEFAULT_PRESET)
const starting = ref(false)
const startError = ref<string | null>(null)

async function start() {
  // :disabled="starting" が連打を防ぐ。開始後は starting を保持したまま遷移し、二重開始を防ぐ。
  starting.value = true
  startError.value = null
  const { data, error, response } = await client.POST('/scans', { body: { site_id: siteId, preset: preset.value } })
  // scan_id は生成型上 optional。欠落 202 は契約違反として /scans/undefined へ遷移させずエラー表示に落とす
  if (error || !data?.scan_id) {
    starting.value = false
    // 409 = 同一サイトで実行中のスキャンが既にある（backend の同時実行拒否）
    startError.value = response?.status === 409
      ? 'このサイトは既にスキャンを実行中です。完了までお待ちください。'
      : toApiErrorMessage(error)
    return
  }
  // 成功時は starting を保持したまま結果ページへ遷移する（遷移中のボタン再活性化＝二重開始を防ぐ）
  await navigateTo(`/scans/${data.scan_id}`)
}
</script>

<template>
  <section class="mx-auto max-w-3xl">
    <h1 class="font-display text-display-sm font-bold uppercase text-on-dark">スキャン設定</h1>

    <h2 class="mt-8 text-label font-bold uppercase tracking-label text-muted">プリセット</h2>
    <ScanPresetPicker v-model="preset" class="mt-4" />

    <div class="mt-8 border border-hairline bg-surface-soft p-6">
      <h2 class="text-label font-bold uppercase tracking-label text-muted">安全設定</h2>
      <p class="mt-2 text-body-sm">
        <code>logout</code> / <code>signout</code> / <code>delete</code> などの危険パスは
        <span class="text-success">自動で除外</span>されます。破壊的なテンプレートも既定で無効です。
      </p>
    </div>

    <p v-if="startError" class="mt-6 border border-m-red p-4 text-body-sm text-m-red">{{ startError }}</p>
    <button
      data-testid="start-scan"
      :disabled="starting"
      class="mt-8 bg-on-dark px-6 py-3 font-display text-label font-bold uppercase tracking-label text-canvas disabled:opacity-50"
      @click="start"
    >
      {{ starting ? '開始中…' : 'スキャンを開始' }}
    </button>
  </section>
</template>
