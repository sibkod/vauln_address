<script setup lang="ts">
import { computed } from 'vue'

const props = withDefaults(defineProps<{ chain: string; size?: number }>(), { size: 20 })

const chain = computed(() => (props.chain || '').toLowerCase())
</script>

<template>
  <!-- Solana -->
  <svg
    v-if="chain === 'solana'"
    :width="size" :height="size" viewBox="0 0 397.7 311.7"
    role="img" aria-label="Solana"
  >
    <defs>
      <linearGradient id="solana-grad" x1="360" y1="351" x2="141" y2="-70" gradientUnits="userSpaceOnUse">
        <stop offset="0" stop-color="#00FFA3" />
        <stop offset="1" stop-color="#DC1FFF" />
      </linearGradient>
    </defs>
    <path fill="url(#solana-grad)" d="M64.6 237.9c2.4-2.4 5.7-3.8 9.2-3.8h317.4c5.8 0 8.7 7 4.6 11.1l-62.7 62.7c-2.4 2.4-5.7 3.8-9.2 3.8H6.5c-5.8 0-8.7-7-4.6-11.1l62.7-62.7z" />
    <path fill="url(#solana-grad)" d="M64.6 3.8C67.1 1.4 70.4 0 73.8 0h317.4c5.8 0 8.7 7 4.6 11.1l-62.7 62.7c-2.4 2.4-5.7 3.8-9.2 3.8H6.5c-5.8 0-8.7-7-4.6-11.1L64.6 3.8z" />
    <path fill="url(#solana-grad)" d="M333.1 120.1c-2.4-2.4-5.7-3.8-9.2-3.8H6.5c-5.8 0-8.7 7-4.6 11.1l62.7 62.7c2.4 2.4 5.7 3.8 9.2 3.8h317.4c5.8 0 8.7-7 4.6-11.1l-62.7-62.7z" />
  </svg>

  <!-- Ethereum / EVM -->
  <svg
    v-else-if="chain === 'evm'"
    :width="size" :height="size" viewBox="0 0 32 32"
    role="img" aria-label="Ethereum"
  >
    <circle cx="16" cy="16" r="16" fill="#627EEA" />
    <path fill="#fff" fill-opacity=".6" d="M16.5 4v8.87l7.5 3.35z" />
    <path fill="#fff" d="M16.5 4L9 16.22l7.5-3.35z" />
    <path fill="#fff" fill-opacity=".6" d="M16.5 21.97v6.03L24 17.62z" />
    <path fill="#fff" d="M16.5 28v-6.03L9 17.62z" />
    <path fill="#fff" fill-opacity=".2" d="M16.5 20.57l7.5-4.35-7.5-3.35z" />
    <path fill="#fff" fill-opacity=".6" d="M9 16.22l7.5 4.35v-7.7z" />
  </svg>

  <!-- Bitcoin -->
  <svg
    v-else-if="chain === 'btc'"
    :width="size" :height="size" viewBox="0 0 32 32"
    role="img" aria-label="Bitcoin"
  >
    <circle cx="16" cy="16" r="16" fill="#F7931A" />
    <text x="16" y="21.5" text-anchor="middle" font-size="16" font-weight="700" fill="#fff" font-family="Arial, sans-serif">₿</text>
  </svg>

  <!-- Sui -->
  <svg
    v-else-if="chain === 'sui'"
    :width="size" :height="size" viewBox="0 0 32 32"
    role="img" aria-label="Sui"
  >
    <circle cx="16" cy="16" r="16" fill="#4DA2FF" />
    <path fill="#fff" d="M16 6c-4.2 4.9-6.3 8.3-6.3 11.2a6.3 6.3 0 0 0 12.6 0C22.3 14.3 20.2 10.9 16 6z" />
  </svg>

  <!-- Tron -->
  <svg
    v-else-if="chain === 'tron'"
    :width="size" :height="size" viewBox="0 0 32 32"
    role="img" aria-label="Tron"
  >
    <circle cx="16" cy="16" r="16" fill="#EB0029" />
    <path fill="#fff" d="M10.5 9.5L22 16l-11.5 6.5V9.5z" />
  </svg>

  <!-- Fallback: first letter on a neutral disc -->
  <svg
    v-else
    :width="size" :height="size" viewBox="0 0 32 32"
    role="img" :aria-label="chain || 'chain'"
  >
    <circle cx="16" cy="16" r="16" fill="#2a3548" />
    <text x="16" y="21" text-anchor="middle" font-size="14" font-weight="700" fill="#c7d2e8" font-family="Arial, sans-serif">
      {{ (chain || '?').charAt(0).toUpperCase() }}
    </text>
  </svg>
</template>
