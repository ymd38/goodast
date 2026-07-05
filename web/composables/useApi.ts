import createClient from 'openapi-fetch'
import type { paths } from '~/types/api'

// swagger.yaml 生成型で全パス・パラメータ・レスポンスが型付けされたクライアント。
// SSR では相対 baseUrl を解決できないため、データ取得は useAsyncData の
// { server: false } と組み合わせてクライアント側でのみ実行する
export function useApiClient() {
  const { apiBase } = useRuntimeConfig().public
  return createClient<paths>({ baseUrl: apiBase })
}
