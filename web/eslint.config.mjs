// @ts-check
import withNuxt from './.nuxt/eslint.config.mjs'

export default withNuxt({
  // openapi-typescript 生成物（手編集禁止）は lint 対象外
  ignores: ['types/api.d.ts'],
})
