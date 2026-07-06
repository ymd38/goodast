<script setup lang="ts">
const client = useApiClient()

// SSR では相対 apiBase を解決できないため client 側でのみ取得（composables/useApi.ts 参照）
const { data: sites, error } = await useAsyncData(
  'sites',
  async () => {
    const { data, error: apiError } = await client.GET('/sites')
    if (apiError) throw new Error(toApiErrorMessage(apiError))
    return data ?? []
  },
  { server: false, default: () => [] },
)
</script>

<template>
  <section>
    <div class="flex items-center justify-between">
      <h1 class="font-display text-display-sm font-bold uppercase text-on-dark">Sites</h1>
      <NuxtLink
        to="/sites/new"
        data-testid="register-link"
        class="border border-on-dark px-4 py-2 font-display text-label font-bold uppercase tracking-label text-on-dark"
      >
        サイトを登録
      </NuxtLink>
    </div>
    <p v-if="error" class="mt-6 border border-m-red p-4 text-body-sm text-m-red">
      {{ error.message }}
    </p>
    <p v-else-if="sites.length === 0" class="mt-6 text-body-sm text-muted">
      サイトが未登録です。右上の『サイトを登録』から追加できます。
    </p>
    <ul v-else class="mt-6 grid gap-4 md:grid-cols-2 lg:grid-cols-3">
      <li v-for="site in sites" :key="site.id">
        <NuxtLink
          :to="`/sites/${site.id}`"
          class="block border border-hairline bg-surface-card p-6 transition-colors hover:border-on-dark"
        >
          <p class="font-display text-title-lg font-bold text-on-dark">{{ site.name }}</p>
          <p class="mt-1 text-body-sm">{{ site.base_url }}</p>
          <p
            class="mt-3 text-caption uppercase tracking-caption"
            :class="site.ownership_verified ? 'text-success' : 'text-warning'"
            data-testid="ownership-badge"
          >
            {{ site.ownership_verified ? '所有確認済み' : '所有未確認' }}
          </p>
        </NuxtLink>
      </li>
    </ul>
  </section>
</template>
