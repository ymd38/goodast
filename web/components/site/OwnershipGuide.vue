<script setup lang="ts">
import type { SiteResponse } from '~/types/goodast'

defineProps<{ verification: NonNullable<SiteResponse['verification']> }>()
</script>

<template>
  <section class="border border-hairline bg-surface-card p-6">
    <h3 class="font-display text-label font-bold uppercase tracking-label text-muted">
      ドメイン所有確認
    </h3>
    <ol v-if="verification.method === 'file'" data-testid="guide-file" class="mt-4 space-y-3 text-body-sm">
      <li>1. 次のパスにファイルを作成します：<code class="text-on-dark">{{ verification.file_path }}</code></li>
      <li>2. ファイルに次の内容を書き込みます：<code class="block bg-surface-soft p-2 text-on-dark">{{ verification.file_content }}</code></li>
      <li>3. 公開できたら「確認する」を押してください。</li>
    </ol>
    <ol v-else data-testid="guide-dns" class="mt-4 space-y-3 text-body-sm">
      <li>1. DNS に次の TXT レコードを追加します：<code class="block bg-surface-soft p-2 text-on-dark">{{ verification.dns_record }}</code></li>
      <li>2. 反映されたら「確認する」を押してください。</li>
    </ol>
  </section>
</template>
