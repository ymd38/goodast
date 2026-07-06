<script setup lang="ts">
defineProps<{ submitting: boolean; error: string | null }>()
const emit = defineEmits<{ submit: [{ name: string; base_url: string; verify_method: string }] }>()

const name = ref('')
const baseUrl = ref('')
const verifyMethod = ref('file')

function onSubmit() {
  emit('submit', { name: name.value, base_url: baseUrl.value, verify_method: verifyMethod.value })
}
</script>

<template>
  <form class="space-y-6" @submit.prevent="onSubmit">
    <div>
      <label class="block text-label font-bold uppercase tracking-label text-muted" for="name">サイト名</label>
      <input
id="name" v-model="name" data-testid="field-name" required
        class="mt-2 w-full border border-hairline bg-surface-card p-3 text-body-md text-on-dark" >
    </div>
    <div>
      <label class="block text-label font-bold uppercase tracking-label text-muted" for="base-url">ベース URL</label>
      <input
id="base-url" v-model="baseUrl" data-testid="field-base-url" type="url" required
        class="mt-2 w-full border border-hairline bg-surface-card p-3 text-body-md text-on-dark" >
    </div>
    <div>
      <label class="block text-label font-bold uppercase tracking-label text-muted" for="method">所有確認方式</label>
      <select
id="method" v-model="verifyMethod" data-testid="field-method"
        class="mt-2 w-full border border-hairline bg-surface-card p-3 text-body-md text-on-dark">
        <option value="file">ファイル設置</option>
        <option value="dns">DNS TXT</option>
      </select>
    </div>
    <p v-if="error" class="border border-m-red p-4 text-body-sm text-m-red">{{ error }}</p>
    <button
type="submit" data-testid="submit" :disabled="submitting"
      class="bg-on-dark px-6 py-3 font-display text-label font-bold uppercase tracking-label text-canvas disabled:opacity-50">
      {{ submitting ? '登録中…' : 'サイトを登録' }}
    </button>
  </form>
</template>
