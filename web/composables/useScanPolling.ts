import type { ScanState } from '~/types/goodast'

const DEFAULT_INTERVAL_MS = 2500

// スキャン進捗ポーリングの状態機械。GET /scans/:id を繰り返し、
// status が done/failed になったら停止する（.claude/rules/frontend.md「API 連携」）
export function useScanPolling(scanId: string, opts: { intervalMs?: number } = {}) {
  const intervalMs = opts.intervalMs ?? DEFAULT_INTERVAL_MS
  const client = useApiClient()
  const state = ref<ScanState | null>(null)
  const error = ref<string | null>(null)
  const done = ref(false)
  let timer: ReturnType<typeof setTimeout> | null = null
  let stopped = false

  function stop() {
    stopped = true
    if (timer) {
      clearTimeout(timer)
      timer = null
    }
  }

  async function tick() {
    const { data, error: apiError } = await client.GET('/scans/{id}', {
      params: { path: { id: scanId } },
    })
    // await 中に stop（unmount 等）された場合は状態を触らず再スケジュールもしない
    if (stopped) return
    if (apiError || !data) {
      error.value = toApiErrorMessage(apiError)
      done.value = true
      stop()
      return
    }
    state.value = data
    if (data.status === 'done' || data.status === 'failed') {
      done.value = true
      stop()
      return
    }
    timer = setTimeout(tick, intervalMs)
  }

  function start() {
    stop() // 再入時に既存のポーリングチェーンを解除してから再開する（timer リーク防止）
    stopped = false
    done.value = false
    error.value = null
    void tick()
  }

  // コンポーネント外（テスト等）から呼ばれる場合は onUnmounted 登録を skip する
  // （インスタンス外での呼び出しは Vue の警告になるため）
  if (getCurrentInstance()) {
    onUnmounted(stop)
  }

  return { state, error, done, start, stop }
}
