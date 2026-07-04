import tailwindcss from '@tailwindcss/vite'

export default defineNuxtConfig({
  compatibilityDate: '2026-07-04',
  modules: ['@nuxt/eslint', '@nuxt/test-utils/module'],
  css: ['~/assets/css/main.css'],
  vite: { plugins: [tailwindcss()] },
  typescript: { strict: true },
  runtimeConfig: {
    public: {
      // API のベースパス。開発時は下の devProxy が :8080 の Gin API へ中継する
      apiBase: '/api',
    },
  },
  nitro: {
    devProxy: {
      '/api': { target: 'http://localhost:8080', changeOrigin: true },
    },
  },
  app: {
    head: { title: 'goodast', htmlAttrs: { lang: 'ja' } },
  },
})
