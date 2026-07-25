<script setup lang="ts">
import { ref, onMounted, provide } from 'vue'
import { RouterLink, RouterView } from 'vue-router'
import { ConnectionProvider, WalletProvider } from '@solana/wallet-adapter-react'
import { WalletModalProvider } from '@solana/wallet-adapter-react-ui'
import { PhantomWalletAdapter, SolflareWalletAdapter } from '@solana/wallet-adapter-wallets'
import { clusterApiUrl } from '@solana/web3.js'
import '@solana/wallet-adapter-react-ui/styles.css'

const USE_DEVNET = true
const endpoint = USE_DEVNET ? clusterApiUrl('devnet') : 'https://api.mainnet-beta.solana.com'
const wallets = [new PhantomWalletAdapter(), new SolflareWalletAdapter()]

const darkMode = ref(true)
const stats = ref({ evm: 0, btc: 0, solana: 0, sui: 0, tron: 0 })

onMounted(async () => {
  const saved = localStorage.getItem('walletCheckerTheme')
  if (saved === 'light') darkMode.value = false
  
  try {
    const res = await fetch('/api/chains')
    if (res.ok) {
      const data = await res.json()
      stats.value = data.counts || stats.value
    }
  } catch {}
})

function toggleTheme() {
  darkMode.value = !darkMode.value
  document.body.classList.toggle('light', !darkMode.value)
  localStorage.setItem('walletCheckerTheme', darkMode.value ? 'dark' : 'light')
}

function getTotal() {
  return Object.values(stats.value).reduce((a, b) => a + b, 0)
}
</script>

<template>
  <ConnectionProvider :endpoint="endpoint">
    <WalletProvider :wallets="wallets" autoConnect>
      <WalletModalProvider>
        <!-- Toast -->
        <div class="toast"></div>

        <!-- Navigation -->
        <nav class="nav">
          <div class="nav-brand" @click="$router.push('/')">◈ <span>Wallet</span>Checker</div>
          <div class="nav-center">
            <RouterLink to="/" class="nav-link">Home</RouterLink>
            <RouterLink to="/roadmap" class="nav-link">Roadmap</RouterLink>
            <RouterLink to="/about" class="nav-link">About</RouterLink>
            <RouterLink to="/contact" class="nav-link">Contact</RouterLink>
            <RouterLink to="/support" class="nav-link">Support</RouterLink>
          </div>
          <div class="nav-right">
            <button class="theme-toggle" @click="toggleTheme">{{ darkMode ? '◐' : '◑' }}</button>
            <WalletMultiButton class="connect-btn" />
          </div>
        </nav>

        <!-- Main content -->
        <div class="main-content">
          <RouterView />
        </div>

        <!-- Footer -->
        <footer class="footer">
          <div class="footer-stats">
            <span class="stat-item"><span class="chain-label">EVM</span> <span class="num">{{ stats.evm }}</span></span>
            <span class="stat-item"><span class="chain-label">BTC</span> <span class="num">{{ stats.btc }}</span></span>
            <span class="stat-item"><span class="chain-label">Solana</span> <span class="num">{{ stats.solana }}</span></span>
            <span class="stat-item"><span class="chain-label">Sui</span> <span class="num">{{ stats.sui }}</span></span>
            <span class="stat-item"><span class="chain-label">Tron</span> <span class="num">{{ stats.tron }}</span></span>
            <span class="stat-item" style="font-weight:500;color:#98a8ce;">total <span class="num">{{ getTotal() }}</span></span>
          </div>
          <div class="footer-meta">
            <span>© 2026 Wallet Checker</span>
            <span>Security intelligence for Web3</span>
            <span>v2.0</span>
          </div>
        </footer>
      </WalletModalProvider>
    </WalletProvider>
  </ConnectionProvider>
</template>
