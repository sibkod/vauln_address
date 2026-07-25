<script setup lang="ts">
import { ref, onMounted, provide } from 'vue'
import { RouterLink, RouterView } from 'vue-router'

const darkMode = ref(true)
const connected = ref(false)
const walletAddress = ref('')
const walletChain = ref('evm')
const userBalance = ref(0)
const authToken = ref('')

const stats = ref({ evm: 0, btc: 0, solana: 0, sui: 0, tron: 0 })

// Provide auth state to all components
provide('wallet', { connected, walletAddress, walletChain, userBalance, authToken })

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
    walletChain.value = chain || 'evm'
    userBalance.value = parseInt(balance || '0')
    connected.value = true
  }
  
  // Fetch stats
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

function formatAddress(addr: string) {
  return `${addr.slice(0, 6)}…${addr.slice(-4)}`
}

async function connectWallet() {
  // Check for ethereum (MetaMask)
  const ethereum = (window as any).ethereum
  if (ethereum && ethereum.isMetaMask) {
    try {
      const accounts = await ethereum.request({ method: 'eth_requestAccounts' })
      if (accounts.length > 0) {
        walletAddress.value = accounts[0]
        walletChain.value = 'evm'
        connected.value = true
        
        // Get nonce
        const nonceRes = await fetch(`/api/auth/nonce?address=${walletAddress.value}&chain=evm`)
        const nonceData = await nonceRes.json()
        
        // Sign message
        const message = nonceData.nonce
        const signature = await ethereum.request({
          method: 'personal_sign',
          params: [message, walletAddress.value]
        })
        
        // Authenticate
        const authRes = await fetch('/api/auth/login', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            address: walletAddress.value,
            chain: 'evm',
            signature: signature,
            message: message
          })
        })
        
        const authData = await authRes.json()
        if (authData.token) {
          authToken.value = authData.token
          userBalance.value = authData.user?.balance || 0
          
          // Save to localStorage
          localStorage.setItem('authToken', authData.token)
          localStorage.setItem('walletAddress', walletAddress.value)
          localStorage.setItem('walletChain', 'evm')
          localStorage.setItem('userBalance', String(userBalance.value))
        }
      }
    } catch (err: any) {
      console.error('Wallet connection failed:', err)
      alert('Failed to connect wallet: ' + err.message)
    }
  } else {
    alert('Please install MetaMask to connect your wallet')
  }
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

function getTotal() {
  return Object.values(stats.value).reduce((a, b) => a + b, 0)
}
</script>

<template>
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
      <button v-if="connected" class="connect-btn" @click="disconnectWallet" :title="walletAddress">
        <span class="dot active"></span>
        <span>{{ formatAddress(walletAddress) }}</span>
        <span v-if="userBalance > 0" style="margin-left:0.5rem; color:#4bc9a0;">{{ userBalance }} checks</span>
      </button>
      <button v-else class="connect-btn" @click="connectWallet">
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
