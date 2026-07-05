<script setup lang="ts">
const route = useRoute()
const siteId = String(route.params.id)
const client = useApiClient()

// SSR では相対 apiBase を解決できないため client 側でのみ取得（composables/useApi.ts 参照）
const { data, error } = await useAsyncData(
  `site-dashboard-${siteId}`,
  async () => {
    const [siteRes, dashboardRes] = await Promise.all([
      client.GET('/sites/{id}', { params: { path: { id: siteId } } }),
      client.GET('/sites/{id}/dashboard', { params: { path: { id: siteId } } }),
    ])
    const apiError = siteRes.error ?? dashboardRes.error
    if (apiError) throw new Error(toApiErrorMessage(apiError))
    return { site: siteRes.data, dashboard: dashboardRes.data }
  },
  { server: false },
)

const site = computed(() => data.value?.site)
const latest = computed(() => data.value?.dashboard?.latest ?? null)
const history = computed(() => data.value?.dashboard?.history ?? [])
</script>

<template>
  <p v-if="error" class="border border-m-red p-4 text-body-sm text-m-red">
    {{ error.message }}
  </p>
  <section v-else-if="site">
    <NuxtLink to="/" class="text-caption uppercase tracking-caption text-muted hover:text-on-dark">
      ← Sites
    </NuxtLink>
    <h1 class="mt-2 font-display text-display-sm font-bold uppercase text-on-dark">
      {{ site.name }}
    </h1>
    <p class="mt-1 text-body-sm text-muted">{{ site.base_url }}</p>
    <div class="m-stripe mt-4" />

    <!-- 上段: 状態（今どうか） -->
    <div class="mt-8 grid gap-4 lg:grid-cols-[minmax(0,20rem)_1fr]">
      <DashboardScoreCard :latest="latest" />
      <DashboardSeverityCountCards v-if="latest?.counts" :counts="latest.counts" />
    </div>

    <!-- 中段: 遷移（どう変わったか）/ 下段: 内訳（なぜそのスコアか） -->
    <div class="mt-8 flex flex-col gap-8">
      <DashboardScoreTrendChart :history="history" />
      <DashboardSeverityStackChart :history="history" />
    </div>
  </section>
  <p v-else class="text-body-sm text-muted">読み込み中…</p>
</template>
