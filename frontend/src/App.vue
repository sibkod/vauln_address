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
  // Browser Extensions
  { id: 'phantom', name: 'Phantom', icon: '👻', url: 'https://phantom.app/', type: 'extension' },
  { id: 'solflare', name: 'Solflare', icon: '☀️', url: 'https://solflare.com/', type: 'extension' },
  { id: 'slope', name: 'Slope', icon: '🛡️', url: 'https://slope.finance/', type: 'extension' },
  { id: 'glow', name: 'Glow', icon: '✨', url: 'https://glow.app/', type: 'extension' },
  { id: 'coinbase', name: 'Coinbase Wallet', icon: '💰', url: 'https://www.coinbase.com/wallet', type: 'extension' },
  { id: 'backpack', name: 'Backpack', icon: '🎒', url: 'https://backpack.app/', type: 'extension' },
  // Mobile & Hardware
  { id: 'walletconnect', name: 'WalletConnect', icon: '🔗', url: 'https://walletconnect.com/', type: 'mobile' },
  { id: 'exodus', name: 'Exodus', icon: '🚀', url: 'https://exodus.com/', type: 'mobile' },
  { id: 'ledger', name: 'Ledger Live', icon: '📱', url: 'https://www.ledger.com/ledger-live', type: 'hardware' },
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
    const providers: Record<string, any> = {
      phantom: (window as any).solana,
      solflare: (window as any).solflare,
      slope: (window as any).slope,
      glow: (window as any).glow,
    }
    
    // Direct connection for browser extensions
    if (providers[walletId]) {
      const provider = providers[walletId]
      if (provider?.isConnected || provider?.publicKey) {
        // Already connected
        await authenticateWithBackend(provider, provider.publicKey.toString())
        return
      }
      if (provider?.connect) {
        await provider.connect()
        if (provider.publicKey) {
          await authenticateWithBackend(provider, provider.publicKey.toString())
          return
        }
      }
    }
    
    // Mobile wallets - need WalletConnect or deep link
    if (walletId === 'walletconnect') {
      connectError.value = 'WalletConnect integration coming soon'
      connecting.value = false
      return
    }
    
    if (walletId === 'exodus') {
      // Exodus deep link
      window.location.href = 'exodus://solana/wc'
      connectError.value = 'Opening Exodus wallet...'
      connecting.value = false
      return
    }
    
    if (walletId === 'ledger') {
      connectError.value = 'Ledger connection via WalletConnect coming soon'
      connecting.value = false
      return
    }
    
    // If we get here, wallet not installed - redirect to install
    const wallet = walletOptions.find(w => w.id === walletId)
    if (wallet) {
      window.open(wallet.url, '_blank')
      connectError.value = `Please install ${wallet.name} wallet`
    } else {
      connectError.value = 'Wallet not found. Please install a supported wallet.'
    }
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
      <p class="modal-desc">Choose your preferred wallet</p>
      
      <!-- Extension Wallets -->
      <div class="wallet-section">
        <div class="wallet-section-title">Browser Extensions</div>
        <div class="wallet-options">
          <button 
            v-for="wallet in walletOptions.filter(w => w.type === 'extension')" 
            :key="wallet.id"
            class="wallet-option"
            @click="connectWallet(wallet.id)"
            :disabled="connecting"
          >
            <span class="wallet-icon">{{ wallet.icon }}</span>
            <span class="wallet-name">{{ wallet.name }}</span>
            <span class="wallet-badge extension">Extension</span>
          </button>
        </div>
      </div>
      
      <!-- Mobile Wallets -->
      <div class="wallet-section">
        <div class="wallet-section-title">Mobile & Hardware</div>
        <div class="wallet-options">
          <button 
            v-for="wallet in walletOptions.filter(w => w.type !== 'extension')" 
            :key="wallet.id"
            class="wallet-option"
            @click="connectWallet(wallet.id)"
            :disabled="connecting"
          >
            <span class="wallet-icon">{{ wallet.icon }}</span>
            <span class="wallet-name">{{ wallet.name }}</span>
            <span class="wallet-badge mobile">{{ wallet.type }}</span>
          </button>
        </div>
      </div>
      
      <div v-if="connectError" class="wallet-error">{{ connectError }}</div>
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
  max-width: 420px;
  max-height: 90vh;
  overflow-y: auto;
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
.wallet-section {
  margin-bottom: 1.2rem;
}
.wallet-section-title {
  font-size: 0.7rem;
  font-weight: 600;
  color: #5a6a8e;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  margin-bottom: 0.6rem;
}
.wallet-options {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}
.wallet-option {
  display: flex;
  align-items: center;
  gap: 0.8rem;
  padding: 0.8rem 1rem;
  background: #151a24;
  border: 1px solid #252d3d;
  border-radius: 10px;
  cursor: pointer;
  transition: all 0.2s;
}
.wallet-option:hover:not(:disabled) {
  border-color: #667eea;
  background: #1a1f2e;
  transform: translateX(4px);
}
.wallet-option:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}
.wallet-icon {
  font-size: 1.5rem;
  width: 32px;
  text-align: center;
}
.wallet-name {
  font-size: 0.95rem;
  font-weight: 500;
  color: #e7ecf5;
  flex: 1;
}
.wallet-badge {
  font-size: 0.6rem;
  padding: 0.15rem 0.5rem;
  border-radius: 4px;
  font-weight: 600;
  text-transform: uppercase;
}
.wallet-badge.extension {
  background: #667eea20;
  color: #667eea;
}
.wallet-badge.mobile {
  background: #4bc9a020;
  color: #4bc9a0;
}
.wallet-badge.hardware {
  background: #ffa50220;
  color: #ffa502;
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
</style>
