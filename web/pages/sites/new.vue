<script setup lang="ts">
import type { SiteResponse } from '~/types/goodast'

const client = useApiClient()
const submitting = ref(false)
const registerError = ref<string | null>(null)
const site = ref<SiteResponse | null>(null)
const verifying = ref(false)
const verifyError = ref<string | null>(null)
const verifiedNow = ref(false)

async function register(payload: { name: string; base_url: string; verify_method: string }) {
  submitting.value = true
  registerError.value = null
  const { data, error } = await client.POST('/sites', { body: payload })
  submitting.value = false
  if (error || !data) {
    registerError.value = toApiErrorMessage(error)
    return
  }
  site.value = data
}

async function verify() {
  if (!site.value?.id) return
  verifying.value = true
  verifyError.value = null
  const { data, error } = await client.POST('/sites/{id}/verify', { params: { path: { id: site.value.id } } })
  verifying.value = false
  if (error || !data) {
    verifyError.value = toApiErrorMessage(error)
    return
  }
  site.value = data
  verifiedNow.value = true
}

const needsGuide = computed(() => site.value && !site.value.ownership_verified && site.value.verification)
</script>

<template>
  <section class="mx-auto max-w-xl">
    <h1 class="font-display text-display-sm font-bold uppercase text-on-dark">サイトを登録</h1>

    <SiteRegisterForm v-if="!site" class="mt-8" :submitting="submitting" :error="registerError" @submit="register" />

    <div v-else class="mt-8 space-y-6">
      <p v-if="site.ownership_verified" class="border border-success p-4 text-body-sm text-success">
        {{ verifiedNow ? '所有確認が完了しました。' : '登録が完了しました（このサイトは所有確認が不要です）。' }}
      </p>
      <template v-if="needsGuide">
        <SiteOwnershipGuide :verification="site.verification!" />
        <p v-if="verifyError" class="border border-m-red p-4 text-body-sm text-m-red">{{ verifyError }}</p>
        <button
data-testid="verify" :disabled="verifying"
          class="bg-on-dark px-6 py-3 font-display text-label font-bold uppercase tracking-label text-canvas disabled:opacity-50"
          @click="verify">
          {{ verifying ? '確認中…' : '確認する' }}
        </button>
      </template>
      <NuxtLink
v-if="site.ownership_verified" :to="`/sites/${site.id}`" data-testid="go-site"
        class="inline-block border border-on-dark px-6 py-3 font-display text-label font-bold uppercase tracking-label text-on-dark">
        サイトを開く
      </NuxtLink>
    </div>
  </section>
</template>
