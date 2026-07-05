const FALLBACK_MESSAGE = 'APIとの通信に失敗しました。時間をおいて再度お試しください。'

// API エラー（openapi-fetch の error = ErrorResponse）を表示用メッセージへ変換する。
// エラーハンドリングの集約点（.claude/rules/frontend.md「API 連携」）
export function toApiErrorMessage(error: unknown): string {
  if (typeof error === 'object' && error !== null && 'error' in error) {
    const message = (error as { error: unknown }).error
    if (typeof message === 'string' && message !== '') return message
  }
  return FALLBACK_MESSAGE
}
