import { coverageConfigDefaults } from 'vitest/config'
import { defineVitestConfig } from '@nuxt/test-utils/config'

export default defineVitestConfig({
  test: {
    environment: 'nuxt',
    coverage: {
      provider: 'istanbul',
      include: ['**/*.{ts,vue}'],
      // 新規ソースディレクトリを黙ってゲート外にしないため exclude 方式で運用する。
      // app.vue は DI 配線相当（backend の cmd/main.go と同扱い）、types/ は型のみ、設定ファイルは対象外
      exclude: [
        ...coverageConfigDefaults.exclude,
        '**/*.config.ts',
        'app.vue',
        'types/**',
        '.nuxt/**',
        '.output/**',
      ],
      thresholds: { statements: 100, branches: 100, functions: 100 },
    },
  },
})
