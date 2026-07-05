<template>
  <div class="min-h-screen bg-canvas font-body text-body">
    <header class="border-b border-hairline">
      <div class="mx-auto flex h-16 w-full max-w-page items-center px-6">
        <NuxtLink
          to="/"
          class="font-display text-title-lg font-bold uppercase tracking-label text-on-dark"
        >
          Goodast
        </NuxtLink>
      </div>
      <div class="m-stripe" />
    </header>
    <main class="mx-auto w-full max-w-page px-6 py-10">
      <slot />
    </main>
  </div>
</template>
