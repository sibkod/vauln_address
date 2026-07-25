<script setup lang="ts">
import { ref, onMounted, provide } from 'vue'
import { RouterLink, RouterView } from 'vue-router'

// Network config - change IS_MAINNET to switch networks
const IS_MAINNET = false
const SOLANA_NETWORK = IS_MAINNET ? 'mainnet-beta' : 'devnet'

const darkMode = ref(true)
const connected = ref(false)
const walletAddress = ref('')
const walletChain = ref('solana')
const userBalance = ref(0)
const authToken = ref('')
const backendAvailable = ref(true)
const checkingBackend = ref(true)

const stats = ref({ evm: 0, btc: 0, solana: 0, sui: 0, tron: 0 })

// Provide auth state to all components
provide('wallet', { connected, walletAddress, walletChain, userBalance, authToken })
provide('network', { isMainnet: IS_MAINNET, solanaNetwork: SOLANA_NETWORK })

onMounted(async () => {
  const saved = localStorage.getItem('walletCheckerTheme')
  if (saved === 'light') darkMode.value = false
  
  // Restore auth state
  const token = localStorage.getItem('authToken')
  const addr = localStorage.getItem('walletAddress')
  const chain = localStorage.getItem('walletChain')
  const balance = localStorage.getItem('userBalance')
  
  if (token && addr) {
    authToken.value = token
    walletAddress.value = addr
    walletChain.value = chain || 'solana'
    userBalance.value = parseInt(balance || '0')
    connected.value = true
  }
  
  // Check backend availability
  try {
    const res = await fetch('/api/chains', { 
      signal: AbortSignal.timeout(5000)
    })
    if (res.ok) {
      const data = await res.json()
      stats.value = data.counts || stats.value
      backendAvailable.value = true
    } else {
      backendAvailable.value = false
    }
  } catch {
    backendAvailable.value = false
  }
  checkingBackend.value = false
})

function toggleTheme() {
  darkMode.value = !darkMode.value
  document.body.classList.toggle('light', !darkMode.value)
  localStorage.setItem('walletCheckerTheme', darkMode.value ? 'dark' : 'light')
}

function formatAddress(addr: string) {
  if (!addr) return ''
  return `${addr.slice(0, 4)}…${addr.slice(-4)}`
}

function disconnectWallet() {
  connected.value = false
  walletAddress.value = ''
  userBalance.value = 0
  authToken.value = ''
  localStorage.removeItem('authToken')
  localStorage.removeItem('walletAddress')
  localStorage.removeItem('walletChain')
  localStorage.removeItem('userBalance')
}

function handleWalletConnected(addr: string, token: string, balance: number) {
  walletAddress.value = addr
  authToken.value = token
  userBalance.value = balance
  connected.value = true
  
  localStorage.setItem('authToken', token)
  localStorage.setItem('walletAddress', addr)
  localStorage.setItem('walletChain', 'solana')
  localStorage.setItem('userBalance', String(balance))
}

function getTotal() {
  return Object.values(stats.value).reduce((a, b) => a + b, 0)
}
</script>

<template>
  <!-- Backend unavailable warning -->
  <div v-if="!checkingBackend && !backendAvailable" class="backend-warning">
    <span class="warning-icon">⚠️</span>
    <span>Backend unavailable. Some features may not work.</span>
    <button @click="() => { checkingBackend = true; backendAvailable = true; $router.go(0) }">Retry</button>
  </div>
        
  <!-- Navigation -->
  <nav class="nav">
    <div class="nav-brand" @click="$router.push('/')">◈ <span>Wallet</span>Checker</div>
    <div class="nav-center">
      <RouterLink to="/" class="nav-link">Home</RouterLink>
      <RouterLink to="/pricing" class="nav-link">Pricing</RouterLink>
      <RouterLink to="/roadmap" class="nav-link">Roadmap</RouterLink>
      <RouterLink to="/about" class="nav-link">About</RouterLink>
      <RouterLink to="/contact" class="nav-link">Contact</RouterLink>
      <RouterLink to="/support" class="nav-link">Support</RouterLink>
    </div>
    <div class="nav-right">
      <span class="network-badge">{{ IS_MAINNET ? 'Mainnet' : 'Devnet' }}</span>
      <button class="theme-toggle" @click="toggleTheme">{{ darkMode ? '◐' : '◑' }}</button>
      <button v-if="connected" class="connect-btn" @click="disconnectWallet" :title="walletAddress">
        <span class="dot active"></span>
        <span>{{ formatAddress(walletAddress) }}</span>
        <span v-if="userBalance > 0" style="margin-left:0.5rem; color:#4bc9a0;">{{ userBalance }} checks</span>
      </button>
      <button v-else class="connect-btn" @click="$router.push('/pricing')">
        <span class="dot"></span>
        <span>Connect</span>
      </button>
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
</template>
