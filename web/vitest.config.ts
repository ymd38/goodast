import { defineVitestConfig } from '@nuxt/test-utils/config'

export default defineVitestConfig({
  test: {
    environment: 'nuxt',
    coverage: {
      provider: 'istanbul',
      include: [
        'components/**/*.vue',
        'composables/**/*.ts',
        'layouts/**/*.vue',
        'pages/**/*.vue',
        'utils/**/*.ts',
      ],
      exclude: ['**/*.d.ts'],
      thresholds: { statements: 100, branches: 100, functions: 100 },
    },
  },
})
