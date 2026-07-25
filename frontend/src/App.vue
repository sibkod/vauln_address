<script setup lang="ts">
import { ref, onMounted, provide } from 'vue'
import { RouterLink, RouterView } from 'vue-router'
import { ConnectionProvider, WalletProvider } from '@solana/wallet-adapter-react'
import { WalletModalProvider } from '@solana/wallet-adapter-react-ui'
import { PhantomWalletAdapter, SolflareWalletAdapter, TorusWalletAdapter } from '@solana/wallet-adapter-wallets'
import { clusterApiUrl } from '@solana/web3.js'
import '@solana/wallet-adapter-react-ui/styles.css'

// Network config - change IS_MAINNET to switch networks
const IS_MAINNET = false
const SOLANA_NETWORK = IS_MAINNET ? 'mainnet-beta' : 'devnet'
const SOLANA_RPC = IS_MAINNET ? 'https://api.mainnet-beta.solana.com' : clusterApiUrl('devnet')

const wallets = [
  new PhantomWalletAdapter(),
  new SolflareWalletAdapter(),
  new TorusWalletAdapter()
]

const darkMode = ref(true)
const connected = ref(false)
const walletAddress = ref('')
const walletChain = ref('solana')
const userBalance = ref(0)
const authToken = ref('')

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
  <ConnectionProvider :endpoint="SOLANA_RPC">
    <WalletProvider :wallets="wallets" autoConnect>
      <WalletModalProvider>
        <!-- Solana wallet hook must be inside provider -->
        <WalletConnector @connected="handleWalletConnected" />
        
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
      </WalletModalProvider>
    </WalletProvider>
  </ConnectionProvider>
</template>

<script lang="ts">
import { defineComponent, watch } from 'vue'
import { useWallet } from '@solana/wallet-adapter-react'

// Separate component to use wallet hook inside provider
const WalletConnector = defineComponent({
  name: 'WalletConnector',
  emits: ['connected'],
  setup(props, { emit }) {
    const wallet = useWallet()
    
    watch(() => ({ connected: wallet.connected, publicKey: wallet.publicKey }), async ({ connected, publicKey }) => {
      if (connected && publicKey) {
        const address = publicKey.toBase58()
        
        try {
          // Get nonce from backend
          const nonceRes = await fetch(`/api/auth/nonce?address=${address}&chain=solana`)
          const nonceData = await nonceRes.json()
          
          if (!nonceData.nonce) {
            console.error('Failed to get nonce')
            return
          }
          
          // Sign message with Solana wallet
          const message = nonceData.nonce
          const encodedMessage = new TextEncoder().encode(message)
          const signedMessage = await wallet.signMessage!(encodedMessage)
          const signature = Buffer.from(signedMessage).toString('base64')
          
          // Authenticate
          const authRes = await fetch('/api/auth/login', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
              address: address,
              chain: 'solana',
              signature: signature,
              message: message
            })
          })
          
          const authData = await authRes.json()
          if (authData.token) {
            emit('connected', address, authData.token, authData.user?.balance || 0)
          }
        } catch (err) {
          console.error('Solana auth error:', err)
        }
      }
    }, { immediate: true })
    
    return { wallet }
  },
  template: `<div style="display:none"></div>`
})
</script>
