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
    v-else-if="chain === 'evm' || chain === 'ethereum'"
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

  <!-- BNB Chain -->
  <svg
    v-else-if="chain === 'bnb'"
    :width="size" :height="size" viewBox="0 0 32 32"
    role="img" aria-label="BNB Chain"
  >
    <circle cx="16" cy="16" r="16" fill="#F0B90B" />
    <path fill="#fff" d="M16 7l3 3-3 3-3-3 3-3zM10 13l3 3-3 3-3-3 3-3zM22 13l3 3-3 3-3-3 3-3zM16 13l3 3-3 3-3-3 3-3zM16 19l3 3-3 3-3-3 3-3z" />
  </svg>

  <!-- Base -->
  <svg
    v-else-if="chain === 'base'"
    :width="size" :height="size" viewBox="0 0 32 32"
    role="img" aria-label="Base"
  >
    <circle cx="16" cy="16" r="16" fill="#0052FF" />
    <path fill="#fff" d="M16 26a10 10 0 1 1 9.94-11.1H18.4v2.2H26A10 10 0 0 1 16 26z" />
  </svg>

  <!-- Linea -->
  <svg
    v-else-if="chain === 'linea'"
    :width="size" :height="size" viewBox="0 0 32 32"
    role="img" aria-label="Linea"
  >
    <circle cx="16" cy="16" r="16" fill="#121212" />
    <path fill="#fff" d="M11 8v12.5h10.5V23H11.5L11 22.5V8z" />
    <path fill="#fff" d="M22.5 8v4h-3.5V8z" />
  </svg>

  <!-- Arbitrum -->
  <svg
    v-else-if="chain === 'arbitrum'"
    :width="size" :height="size" viewBox="0 0 32 32"
    role="img" aria-label="Arbitrum"
  >
    <circle cx="16" cy="16" r="16" fill="#28A0F0" />
    <path fill="#fff" d="M16 7l7 16h-3.4L16 13.4 12.4 23H9l7-16z" />
    <path fill="#fff" d="M14.2 19h3.6v2.2h-3.6z" />
  </svg>

  <!-- Polygon -->
  <svg
    v-else-if="chain === 'polygon'"
    :width="size" :height="size" viewBox="0 0 32 32"
    role="img" aria-label="Polygon"
  >
    <circle cx="16" cy="16" r="16" fill="#8247E5" />
    <path fill="#fff" d="M21.5 13.4c-.6-.35-1.3-.35-1.9 0l-2.6 1.5-1.9 1.1-2.6 1.5c-.6.35-1.3.35-1.9 0l-2-1.15c-.55-.35-.9-1-.9-1.65v-2.3c0-.65.35-1.3.9-1.65l2-1.15c.6-.35 1.3-.35 1.9 0l2 1.15c.55.35.9 1 .9 1.65v1.5l2.6-1.5v-2.3c0-.65-.35-1.3-.9-1.65l-4.5-2.6c-.6-.35-1.3-.35-1.9 0l-4.5 2.6c-.55.35-.9 1-.9 1.65v5.2c0 .65.35 1.3.9 1.65l4.5 2.6c.6.35 1.3.35 1.9 0l2.6-1.5 1.9-1.1 2.6-1.5c.6-.35 1.3-.35 1.9 0l2 1.15c.55.35.9 1 .9 1.65v2.3c0 .65-.35 1.3-.9 1.65l-2 1.15c-.6.35-1.3.35-1.9 0l-2-1.15c-.55-.35-.9-1-.9-1.65v-1.5l-2.6 1.5v2.3c0 .65.35 1.3.9 1.65l4.5 2.6c.6.35 1.3.35 1.9 0l4.5-2.6c.55-.35.9-1 .9-1.65v-5.2c0-.65-.35-1.3-.9-1.65l-4.5-2.6z" />
  </svg>

  <!-- Optimism -->
  <svg
    v-else-if="chain === 'optimism'"
    :width="size" :height="size" viewBox="0 0 32 32"
    role="img" aria-label="Optimism"
  >
    <circle cx="16" cy="16" r="16" fill="#FF0420" />
    <path fill="#fff" d="M16 8.5a7.5 7.5 0 1 0 0 15 7.5 7.5 0 0 0 0-15zm0 11.6a4.1 4.1 0 1 1 0-8.2 4.1 4.1 0 0 1 0 8.2z" />
  </svg>

  <!-- Avalanche -->
  <svg
    v-else-if="chain === 'avalanche'"
    :width="size" :height="size" viewBox="0 0 32 32"
    role="img" aria-label="Avalanche"
  >
    <circle cx="16" cy="16" r="16" fill="#E84142" />
    <path fill="#fff" d="M11.5 23h-3l8-14h3l3.5 6.3h-3L18 11.8l-4.5 7.9h3l-2 3.3h-3zM21 23h3l-3-5.2-1.5 2.6L21 23z" />
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

  <!-- Tron (official mark) -->
  <svg
    v-else-if="chain === 'tron'"
    :width="size" :height="size" viewBox="0 0 64 64"
    role="img" aria-label="Tron"
  >
    <circle cx="32" cy="32" r="32" fill="#EB0029" />
    <path fill="#fff" transform="translate(12.5 12.5) scale(0.61)" d="M61.55,19.28c-3-2.77-7.15-7-10.53-10l-.2-.14a3.82,3.82,0,0,0-1.11-.62l0,0C41.56,7,3.63-.09,2.89,0a1.4,1.4,0,0,0-.58.22L2.12.37a2.23,2.23,0,0,0-.52.84l-.05.13v.71l0,.11C5.82,14.05,22.68,53,26,62.14c.2.62.58,1.8,1.29,1.86h.16c.38,0,2-2.14,2-2.14S58.41,26.74,61.34,23a9.46,9.46,0,0,0,1-1.48A2.41,2.41,0,0,0,61.55,19.28ZM36.88,23.37,49.24,13.12l7.25,6.68Zm-4.8-.67L10.8,5.26l34.43,6.35ZM34,27.27l21.78-3.51-24.9,30ZM7.91,7,30.3,26,27.06,53.78Z" />
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
