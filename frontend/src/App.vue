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
const showWalletModal = ref(false)
const connecting = ref(false)
const connectError = ref('')

const stats = ref({ evm: 0, btc: 0, solana: 0, sui: 0, tron: 0 })

const walletOptions = [
  { id: 'phantom', name: 'Phantom', icon: '👻', url: 'https://phantom.app/' },
  { id: 'solflare', name: 'Solflare', icon: '☀️', url: 'https://solflare.com/' },
  { id: 'torus', name: 'Torus', icon: '🔮', url: 'https://toruswallet.io/' },
]

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

function openWalletModal() {
  showWalletModal.value = true
  connectError.value = ''
}

function closeWalletModal() {
  showWalletModal.value = false
  connectError.value = ''
  connecting.value = false
}

async function connectWallet(walletId: string) {
  connecting.value = true
  connectError.value = ''
  
  try {
    // Check for Solana wallet providers
    if (walletId === 'phantom') {
      const phantom = (window as any).solana
      if (phantom?.isPhantom) {
        const res = await phantom.connect()
        if (res.publicKey) {
          await authenticateWithBackend(phantom, res.publicKey.toString())
          return
        }
      } else {
        // Redirect to install
        window.open('https://phantom.app/', '_blank')
        connectError.value = 'Please install Phantom wallet'
        connecting.value = false
        return
      }
    }
    
    if (walletId === 'solflare') {
      const solflare = (window as any).solflare
      if (solflare?.isSolflare) {
        await solflare.connect()
        if (solflare.publicKey) {
          await authenticateWithBackend(solflare, solflare.publicKey.toString())
          return
        }
      } else {
        window.open('https://solflare.com/', '_blank')
        connectError.value = 'Please install Solflare wallet'
        connecting.value = false
        return
      }
    }
    
    if (walletId === 'torus') {
      connectError.value = 'Torus wallet connection coming soon'
      connecting.value = false
      return
    }
    
    connectError.value = 'Wallet not found. Please install a supported wallet.'
  } catch (err: any) {
    connectError.value = err.message || 'Failed to connect wallet'
  }
  
  connecting.value = false
}

async function authenticateWithBackend(wallet: any, address: string) {
  try {
    // Get nonce from backend
    const nonceRes = await fetch(`/api/auth/nonce?address=${address}&chain=solana`)
    const nonceData = await nonceRes.json()
    
    if (!nonceData.nonce) {
      connectError.value = 'Failed to get authentication nonce'
      connecting.value = false
      return
    }
    
    // Sign message with wallet
    const message = nonceData.nonce
    const encodedMessage = new TextEncoder().encode(message)
    const signedMessage = await wallet.signMessage(encodedMessage)
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
      walletAddress.value = address
      authToken.value = authData.token
      userBalance.value = authData.user?.balance || 0
      connected.value = true
      
      localStorage.setItem('authToken', authData.token)
      localStorage.setItem('walletAddress', address)
      localStorage.setItem('walletChain', 'solana')
      localStorage.setItem('userBalance', String(authData.user?.balance || 0))
      
      closeWalletModal()
    } else {
      connectError.value = authData.error || 'Authentication failed'
    }
  } catch (err: any) {
    connectError.value = err.message || 'Authentication failed'
  }
  
  connecting.value = false
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
        <span v-if="userBalance > 0" style="margin-left:0.5rem; color:#4bc9a0;">{{ userBalance }}</span>
      </button>
      <button v-else class="connect-btn" @click="openWalletModal">
        <span class="dot"></span>
        <span>Connect</span>
      </button>
    </div>
  </nav>

  <!-- Wallet Modal -->
  <div v-if="showWalletModal" class="wallet-modal-overlay" @click.self="closeWalletModal">
    <div class="wallet-modal">
      <div class="modal-header">
        <h2>Connect Wallet</h2>
        <button class="modal-close" @click="closeWalletModal">×</button>
      </div>
      <p class="modal-desc">Select a wallet to connect</p>
      
      <div class="wallet-options">
        <button 
          v-for="wallet in walletOptions" 
          :key="wallet.id"
          class="wallet-option"
          @click="connectWallet(wallet.id)"
          :disabled="connecting"
        >
          <span class="wallet-icon">{{ wallet.icon }}</span>
          <span class="wallet-name">{{ wallet.name }}</span>
          <span v-if="connecting" class="connecting-spinner">⏳</span>
        </button>
      </div>
      
      <div v-if="connectError" class="wallet-error">{{ connectError }}</div>
      
      <p class="modal-hint">
        Don't have a wallet? 
        <a href="https://phantom.app/" target="_blank">Get Phantom</a> or 
        <a href="https://solflare.com/" target="_blank">Get Solflare</a>
      </p>
    </div>
  </div>

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

<style scoped>
/* Wallet Modal */
.wallet-modal-overlay {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background: rgba(0, 0, 0, 0.7);
  backdrop-filter: blur(4px);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
}
.wallet-modal {
  background: #1a1f2e;
  border: 1px solid #2a3548;
  border-radius: 16px;
  padding: 1.5rem;
  width: 90%;
  max-width: 380px;
}
.modal-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 0.5rem;
}
.modal-header h2 {
  font-size: 1.2rem;
  font-weight: 600;
  color: #e7ecf5;
}
.modal-close {
  background: none;
  border: none;
  color: #6b7a9e;
  font-size: 1.5rem;
  cursor: pointer;
  padding: 0;
  line-height: 1;
}
.modal-close:hover { color: #e7ecf5; }
.modal-desc {
  color: #6b7a9e;
  font-size: 0.85rem;
  margin-bottom: 1.2rem;
}
.wallet-options {
  display: flex;
  flex-direction: column;
  gap: 0.8rem;
}
.wallet-option {
  display: flex;
  align-items: center;
  gap: 1rem;
  padding: 1rem;
  background: #151a24;
  border: 1px solid #252d3d;
  border-radius: 12px;
  cursor: pointer;
  transition: all 0.2s;
}
.wallet-option:hover:not(:disabled) {
  border-color: #667eea;
  background: #1a1f2e;
}
.wallet-option:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}
.wallet-icon {
  font-size: 1.8rem;
}
.wallet-name {
  font-size: 1rem;
  font-weight: 500;
  color: #e7ecf5;
  flex: 1;
}
.connecting-spinner {
  font-size: 1.2rem;
}
.wallet-error {
  margin-top: 1rem;
  padding: 0.8rem;
  background: #2a1a1a;
  border: 1px solid #ff6b6b40;
  border-radius: 8px;
  color: #ff6b6b;
  font-size: 0.85rem;
  text-align: center;
}
.modal-hint {
  margin-top: 1.2rem;
  font-size: 0.8rem;
  color: #5a6a8e;
  text-align: center;
}
.modal-hint a {
  color: #667eea;
  text-decoration: none;
}
.modal-hint a:hover {
  text-decoration: underline;
}
</style>
