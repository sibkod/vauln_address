<script setup lang="ts">
import { ref, onMounted, onUnmounted, provide } from 'vue'
import { RouterLink, RouterView } from 'vue-router'

// Network config from environment variables
const IS_MAINNET = import.meta.env.VITE_SOLANA_CLUSTER !== 'devnet'
const SOLANA_NETWORK = import.meta.env.VITE_SOLANA_CLUSTER || 'devnet'

// API base URL - uses subdomain based on environment
const API_BASE = import.meta.env.VITE_API_URL || ''

// Helper function for API calls
function apiUrl(path: string): string {
  return API_BASE + path
}

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
const phantomNetwork = ref<'mainnet-beta' | 'devnet' | ''>('')

const stats = ref({ evm: 0, btc: 0, solana: 0, sui: 0, tron: 0 })
const showWalletMenu = ref(false)

// Rate limit info
const rateLimitInfo = ref({
  limit: 0,
  remaining: 0,
  used: 0,
  reset: 0,
  source: 'ip' as 'ip' | 'balance'
})

// Purchase history
const recentPurchases = ref<any[]>([])

// Polling interval ID
let mePollInterval: number | null = null

// Only Phantom wallet for now
const walletOptions = [
  { id: 'phantom', name: 'Phantom', icon: '👻', url: 'https://phantom.app/', type: 'extension', chain: 'solana' },
]

// Provide auth state and wallet modal control to all components
provide('wallet', { connected, walletAddress, walletChain, userBalance, authToken, refreshBalance, fetchPurchaseHistory, fetchMe, openWalletModal, phantomNetwork })
provide('network', { isMainnet: IS_MAINNET, solanaNetwork: SOLANA_NETWORK })
provide('rateLimitInfo', rateLimitInfo)
provide('apiBase', API_BASE)

// Fetch comprehensive user info from /api/me endpoint
async function fetchMe() {
  try {
    const headers: Record<string, string> = {}
    if (authToken.value) {
      headers['Authorization'] = `Bearer ${authToken.value}`
    }
    const res = await fetch(apiUrl('/api/me'), { headers })
    
    if (res.ok) {
      const data = await res.json()
      
      // Parse rate limit info from response body (more reliable)
      if (data.rate_limit_limit !== undefined) rateLimitInfo.value.limit = data.rate_limit_limit
      if (data.rate_limit_remaining !== undefined) rateLimitInfo.value.remaining = data.rate_limit_remaining
      if (data.rate_limit_used !== undefined) rateLimitInfo.value.used = data.rate_limit_used
      if (data.reset_at !== undefined) rateLimitInfo.value.reset = data.reset_at
      if (data.rate_limit_source) rateLimitInfo.value.source = data.rate_limit_source as 'ip' | 'balance'
      
      // Also try headers as fallback
      if (!data.reset_at) {
        const resetHeader = res.headers.get('X-RateLimit-Reset')
        if (resetHeader) rateLimitInfo.value.reset = parseInt(resetHeader)
      }
      
      // Update balance
      if (data.balance !== undefined) {
        userBalance.value = data.balance
        // Save to localStorage for authenticated users
        if (data.is_authenticated && data.balance > 0) {
          localStorage.setItem('userBalance', data.balance.toString())
        }
      }
      
      // Update connection state if needed
      if (data.is_authenticated && data.wallet_address) {
        if (!connected.value) {
          connected.value = true
          walletAddress.value = data.wallet_address
          if (data.chain) walletChain.value = data.chain
        }
      }
      
      console.log('[Me] Updated:', {
        balance: data.balance,
        isPremium: data.is_premium,
        isAuthenticated: data.is_authenticated,
        rateLimit: rateLimitInfo.value
      })
    }
  } catch (err) {
    console.error('Failed to fetch /api/me:', err)
  }
}

// Start periodic polling for /api/me
function startMePolling(intervalMs = 30000) {
  stopMePolling()
  mePollInterval = window.setInterval(fetchMe, intervalMs)
  console.log('[Me] Started polling every', intervalMs, 'ms')
}

// Stop periodic polling
function stopMePolling() {
  if (mePollInterval !== null) {
    clearInterval(mePollInterval)
    mePollInterval = null
    console.log('[Me] Stopped polling')
  }
}

// Fetch balance from backend (works for both auth and anonymous users)
async function refreshBalance() {
  try {
    const headers: Record<string, string> = {}
    if (authToken.value) {
      headers['Authorization'] = `Bearer ${authToken.value}`
    }
    const res = await fetch(apiUrl('/api/user/balance'), { headers })
    if (res.ok) {
      const data = await res.json()
      userBalance.value = data.balance
      // For authenticated users, save to localStorage
      if (data.source === 'purchased' && authToken.value) {
        localStorage.setItem('userBalance', data.balance.toString())
      }
    }
  } catch (err) {
    console.error('Failed to fetch balance:', err)
  }
}

// Fetch purchase history
async function fetchPurchaseHistory(limit = 5) {
  if (!authToken.value) return []
  try {
    const res = await fetch(apiUrl(`/api/user/purchases?limit=${limit}`), {
      headers: { 'Authorization': `Bearer ${authToken.value}` }
    })
    if (res.ok) {
      const data = await res.json()
      recentPurchases.value = data.orders || []
      return data.orders || []
    }
  } catch (err) {
    console.error('Failed to fetch purchase history:', err)
  }
  return []
}

onMounted(async () => {
  const saved = localStorage.getItem('pwndTheme')
  if (saved === 'light') darkMode.value = false
  
  // Restore auth state
  const token = localStorage.getItem('authToken')
  const addr = localStorage.getItem('walletAddress')
  const chain = localStorage.getItem('walletChain')
  
  if (token && addr) {
    authToken.value = token
    walletAddress.value = addr
    walletChain.value = chain || 'solana'
    connected.value = true
    // Fetch purchase history for auth users
    await fetchPurchaseHistory()
  }
  
  // Always fetch balance from backend (works for auth and anonymous users)
  await refreshBalance()
  
  // Fetch user info from /api/me
  await fetchMe()
  
  // Start periodic polling for user data (every 30 seconds)
  startMePolling(30000)
  
  // Check backend availability
  try {
    const res = await fetch(apiUrl('/api/chains'), { 
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

// Stop polling when component unmounts
onUnmounted(() => {
  stopMePolling()
})

function toggleTheme() {
  darkMode.value = !darkMode.value
  document.body.classList.toggle('light', !darkMode.value)
  localStorage.setItem('pwndTheme', darkMode.value ? 'dark' : 'light')
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
    const wallet = walletOptions.find(w => w.id === walletId)
    if (!wallet) {
      connectError.value = 'Wallet not found'
      connecting.value = false
      return
    }
    
    // Solana wallets
    if (wallet.chain === 'solana') {
      const solanaProviders: Record<string, any> = {
        phantom: (window as any).solana,
        solflare: (window as any).solflare,
        slope: (window as any).slope,
        glow: (window as any).glow,
      }
      
      const provider = solanaProviders[walletId]
      if (provider?.connect) {
        await provider.connect()
        if (provider.publicKey) {
          const address = provider.publicKey.toString()
          
          // Get Phantom network - dump all properties
          if (walletId === 'phantom') {
            console.log('[Phantom] === FULL PROVIDER DUMP ===')
            console.log('[Phantom] All keys:', Object.keys(provider))
            console.log('[Phantom] network:', (provider as any).network)
            console.log('[Phantom] isMainnetBeta:', (provider as any).isMainnetBeta)
            console.log('[Phantom] session:', (provider as any).session)
            console.log('[Phantom] provider details:', JSON.stringify(Object.getOwnPropertyNames(provider)))
            
            // Try session if exists
            const session = (provider as any).session
            if (session) {
              console.log('[Phantom] session.network:', session.network)
            }
          }
          
          await authenticateSolana(provider, address)
          return
        }
      } else {
        window.open(wallet.url, '_blank')
        connectError.value = `Install ${wallet.name} wallet`
        connecting.value = false
        return
      }
    }
    
    // EVM wallets (MetaMask, Coinbase)
    if (wallet.chain === 'evm') {
      if (walletId === 'metamask') {
        const ethereum = (window as any).ethereum
        if (ethereum?.isMetaMask) {
          try {
            const accounts = await ethereum.request({ method: 'eth_requestAccounts' })
            if (accounts.length > 0) {
              await authenticateEVM(ethereum, accounts[0])
              return
            }
          } catch (err: any) {
            if (err.code === 4001) {
              connectError.value = 'User rejected connection'
            } else {
              connectError.value = err.message || 'Connection failed'
            }
            connecting.value = false
            return
          }
        }
      }
      
      if (walletId === 'coinbase') {
        const coinbase = (window as any).ethereum
        if (coinbase?.isCoinbaseWallet) {
          try {
            const accounts = await coinbase.request({ method: 'eth_requestAccounts' })
            if (accounts.length > 0) {
              await authenticateEVM(coinbase, accounts[0])
              return
            }
          } catch (err: any) {
            connectError.value = err.message || 'Connection failed'
            connecting.value = false
            return
          }
        }
      }
      
      window.open(wallet.url, '_blank')
      connectError.value = `Install ${wallet.name} wallet`
      connecting.value = false
      return
    }
    
    // Mobile wallets
    if (walletId === 'walletconnect') {
      connectError.value = 'WalletConnect - coming soon'
      connecting.value = false
      return
    }
    
    if (walletId === 'exodus') {
      window.location.href = 'exodus://solana/wc'
      connectError.value = 'Opening Exodus...'
      connecting.value = false
      return
    }
    
    if (walletId === 'ledger') {
      connectError.value = 'Ledger - coming soon'
      connecting.value = false
      return
    }
    
    window.open(wallet.url, '_blank')
    connectError.value = `Install ${wallet.name}`
  } catch (err: any) {
    connectError.value = err.message || 'Failed to connect'
  }
  
  connecting.value = false
}

// Convert Uint8Array to base64 (browser compatible)
function uint8ArrayToBase64(bytes: Uint8Array): string {
  let binary = ''
  const len = bytes.byteLength
  for (let i = 0; i < len; i++) {
    binary += String.fromCharCode(bytes[i])
  }
  return btoa(binary)
}

// Convert hex to base64 (for EVM signatures)
function hexToBase64(hex: string): string {
  // Remove 0x prefix if present
  const cleanHex = hex.startsWith('0x') ? hex.slice(2) : hex
  // Convert hex to binary
  let binary = ''
  for (let i = 0; i < cleanHex.length; i += 2) {
    binary += String.fromCharCode(parseInt(cleanHex.substr(i, 2), 16))
  }
  return btoa(binary)
}

async function authenticateSolana(provider: any, address: string) {
  try {
    const nonceRes = await fetch(apiUrl(`/api/auth/nonce?address=${address}&chain=solana`))
    const nonceData = await nonceRes.json()
    
    if (!nonceData.nonce) {
      connectError.value = 'Failed to get nonce'
      connecting.value = false
      return
    }
    
    const message = nonceData.message || nonceData.nonce
    console.log('Signing message:', message)
    
    // Phantom uses signMessage - this returns { signature: Uint8Array } or just Uint8Array
    const signResult = await provider.signMessage(new TextEncoder().encode(message))
    console.log('Sign result type:', typeof signResult, signResult?.constructor?.name)
    
    // Handle both formats: { signature: Uint8Array } or Uint8Array
    let signedMessage: Uint8Array
    if (signResult && typeof signResult === 'object' && 'signature' in signResult) {
      signedMessage = signResult.signature
    } else if (signResult instanceof Uint8Array) {
      signedMessage = signResult
    } else {
      connectError.value = 'Signing failed - unexpected response'
      connecting.value = false
      return
    }
    
    if (!signedMessage || signedMessage.length === 0) {
      connectError.value = 'Signing failed - no signature returned'
      connecting.value = false
      return
    }
    
    const signature = uint8ArrayToBase64(signedMessage)
    console.log('Signature (base64):', signature.substring(0, 20) + '...')
    
    const authRes = await fetch(apiUrl('/api/auth/login'), {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ address, chain: 'solana', Signature: signature, Message: message })
    })
    
    const authData = await authRes.json()
    console.log('Auth response:', authData)
    
    if (authData.token) {
      walletAddress.value = address
      authToken.value = authData.token
      userBalance.value = authData.user?.balance || 0
      connected.value = true
      walletChain.value = 'solana'
      
      localStorage.setItem('authToken', authData.token)
      localStorage.setItem('walletAddress', address)
      localStorage.setItem('walletChain', 'solana')
      localStorage.setItem('userBalance', String(authData.user?.balance || 0))
      
      // Fetch full user info after successful auth
      await fetchMe()
      
      closeWalletModal()
    } else {
      connectError.value = authData.error || 'Auth failed'
    }
  } catch (err: any) {
    console.error('Auth error:', err)
    connectError.value = err.message || 'Auth failed'
  }
  
  connecting.value = false
}

async function authenticateEVM(provider: any, address: string) {
  try {
    const nonceRes = await fetch(apiUrl(`/api/auth/nonce?address=${address}&chain=evm`))
    const nonceData = await nonceRes.json()
    
    if (!nonceData.nonce) {
      connectError.value = 'Failed to get nonce'
      connecting.value = false
      return
    }
    
    const message = nonceData.message || nonceData.nonce
    
    // Sign message using personal_sign
    const signature = await provider.request({
      method: 'personal_sign',
      params: [message, address]
    })
    
    const signatureBase64 = hexToBase64(signature)
    
    const authRes = await fetch(apiUrl('/api/auth/login'), {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ address, chain: 'evm', Signature: signatureBase64, Message: message })
    })
    
    const authData = await authRes.json()
    
    if (authData.token) {
      walletAddress.value = address
      authToken.value = authData.token
      userBalance.value = authData.user?.balance || 0
      connected.value = true
      walletChain.value = 'evm'
      
      localStorage.setItem('authToken', authData.token)
      localStorage.setItem('walletAddress', address)
      localStorage.setItem('walletChain', 'evm')
      localStorage.setItem('userBalance', String(authData.user?.balance || 0))
      
      // Fetch full user info after successful auth
      await fetchMe()
      
      closeWalletModal()
    } else {
      connectError.value = authData.error || 'Auth failed'
    }
  } catch (err: any) {
    if (err.code === 4001) {
      connectError.value = 'User rejected signing'
    } else {
      connectError.value = err.message || 'Auth failed'
    }
  }
  
  connecting.value = false
}

function disconnectWallet() {
  // Disconnect from wallet if possible
  if (walletChain.value === 'solana') {
    const phantom = (window as any).solana
    if (phantom?.disconnect) {
      phantom.disconnect()
    }
  } else if (walletChain.value === 'evm') {
    const ethereum = (window as any).ethereum
    // EVM wallets don't have a standard disconnect method
  }
  
  connected.value = false
  walletAddress.value = ''
  walletChain.value = ''
  userBalance.value = 0
  authToken.value = ''
  showWalletMenu.value = false
  localStorage.removeItem('authToken')
  localStorage.removeItem('walletAddress')
  localStorage.removeItem('walletChain')
  localStorage.removeItem('userBalance')
}

function toggleWalletMenu() {
  showWalletMenu.value = !showWalletMenu.value
  if (showWalletMenu.value) {
    fetchPurchaseHistory()
  }
}

function closeWalletMenu() {
  showWalletMenu.value = false
}

function getTotal() {
  return Object.values(stats.value).reduce((a, b) => a + b, 0)
}
</script>

<template>
  <!-- Backend warning -->
  <div v-if="!checkingBackend && !backendAvailable" class="backend-warning">
    <span class="warning-icon">⚠️</span>
    <span>Backend unavailable</span>
    <button @click="() => { checkingBackend = true; $router.go(0) }">Retry</button>
  </div>
        
  <!-- Navigation -->
  <nav class="nav">
    <div class="nav-brand" @click="$router.push('/')">◈ <span>pwnd</span></div>
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
      
      <!-- Balance display for all users -->
      <div v-if="userBalance > 0" class="balance-display" :class="{ clickable: connected }" @click.stop="connected ? toggleWalletMenu() : openWalletModal()">
        <span class="balance-icon">🔍</span>
        <span class="balance-count">{{ userBalance }}</span>
      </div>

      <!-- Connected state with dropdown -->
      <div v-if="connected" class="wallet-dropdown">
        <div class="wallet-connected" @click.stop="toggleWalletMenu">
          <span class="dot active"></span>
          <span class="wallet-addr">{{ formatAddress(walletAddress) }}</span>
          <span v-if="userBalance > 0" class="wallet-balance">{{ userBalance }}</span>
          <span class="dropdown-arrow">▼</span>
        </div>
        
        <!-- Dropdown menu -->
        <div v-if="showWalletMenu" class="wallet-menu" @click.stop>
          <div class="menu-header">
            <div class="menu-address">{{ walletAddress }}</div>
            <div class="menu-chain">{{ walletChain.toUpperCase() }}</div>
          </div>
          <div v-if="userBalance > 0" class="menu-balance">
            <span>Balance:</span>
            <span class="balance-value">{{ userBalance }} checks</span>
          </div>
          
          <!-- Purchase history link -->
          <RouterLink to="/purchases" class="menu-item" @click="closeWalletMenu">
            📦 Purchase History
          </RouterLink>
          
          <!-- Checks history link -->
          <RouterLink to="/checks" class="menu-item" @click="closeWalletMenu">
            🔍 My Checks
          </RouterLink>
          
          <button class="menu-item logout" @click="disconnectWallet">
            🚪 Logout
          </button>
        </div>
      </div>
      
      <!-- Not connected - click to open modal -->
      <button v-else class="connect-btn" @click.stop="openWalletModal">
        <span class="dot"></span>
        <span>Connect</span>
      </button>
    </div>
  </nav>

  <!-- Wallet Modal -->
  <Teleport to="body">
    <div v-if="showWalletModal" class="wallet-modal-overlay" @click.self="closeWalletModal">
      <div class="wallet-modal">
        <div class="modal-header">
          <h2>Connect Wallet</h2>
          <button class="modal-close" @click="closeWalletModal">×</button>
        </div>
        
        <div class="wallet-section">
          <div class="wallet-section-title">🟢 Solana</div>
          <div class="wallet-options">
            <button 
              v-for="wallet in walletOptions.filter(w => w.chain === 'solana')" 
              :key="wallet.id"
              class="wallet-option"
              @click="connectWallet(wallet.id)"
              :disabled="connecting"
            >
              <span class="wallet-icon">{{ wallet.icon }}</span>
              <span class="wallet-name">{{ wallet.name }}</span>
            </button>
          </div>
        </div>
        
        <div v-if="connectError" class="wallet-error">{{ connectError }}</div>
      </div>
    </div>
  </Teleport>

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
      <span>© 2026 <a href="https://pwnd.info" style="color:inherit;text-decoration:none;">pwnd.info</a></span>
      <span>Security intelligence for Web3</span>
      <span>v2.0</span>
    </div>
  </footer>
</template>

<style scoped>
.wallet-connected {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.45rem 1rem 0.45rem 1rem;
  background: rgba(24, 32, 48, 0.7);
  border: 1px solid rgba(75, 201, 160, 0.3);
  border-radius: 60px;
  cursor: pointer;
  transition: all 0.2s;
}
.wallet-connected:hover {
  background: rgba(40, 50, 70, 0.7);
  border-color: rgba(75, 201, 160, 0.5);
}
.wallet-addr {
  font-size: 0.8rem;
  color: #e1e8f5;
  font-weight: 500;
}
.wallet-balance {
  font-size: 0.7rem;
  color: #4bc9a0;
  font-weight: 600;
}
.dropdown-arrow {
  font-size: 0.6rem;
  color: #6b7a9e;
  margin-left: 0.25rem;
}

/* Wallet Dropdown */
.wallet-dropdown {
  position: relative;
}

.wallet-menu {
  position: absolute;
  top: calc(100% + 8px);
  right: 0;
  background: #1a1f2e;
  border: 1px solid #2a3548;
  border-radius: 12px;
  min-width: 220px;
  padding: 0.75rem;
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.4);
  z-index: 1000;
}

.menu-header {
  padding-bottom: 0.75rem;
  border-bottom: 1px solid #2a3548;
  margin-bottom: 0.75rem;
}

.menu-address {
  color: #e7ecf5;
  font-size: 0.8rem;
  font-family: monospace;
  word-break: break-all;
  margin-bottom: 0.25rem;
}

.menu-chain {
  color: #667eea;
  font-size: 0.7rem;
  font-weight: 600;
}

.menu-balance {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 0.5rem 0;
  color: #6b7a9e;
  font-size: 0.8rem;
}

.balance-value {
  color: #4bc9a0;
  font-weight: 600;
}

.menu-item {
  width: 100%;
  padding: 0.6rem 0.75rem;
  background: transparent;
  border: none;
  border-radius: 8px;
  color: #e7ecf5;
  font-size: 0.85rem;
  text-align: left;
  cursor: pointer;
  transition: all 0.2s;
  text-decoration: none;
  display: block;
  box-sizing: border-box;
}

.menu-item:hover {
  background: #252d3d;
}

.menu-item.logout {
  margin-top: 0.5rem;
  color: #ff6b6b;
  border-top: 1px solid #2a3548;
  padding-top: 0.75rem;
}

.menu-item.logout:hover {
  background: #2a1f1f;
}

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
  margin-bottom: 1rem;
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
}
.modal-close:hover { color: #e7ecf5; }
.wallet-section {
  margin-bottom: 1rem;
}
.wallet-section-title {
  font-size: 0.7rem;
  font-weight: 600;
  color: #5a6a8e;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  margin-bottom: 0.5rem;
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
}
.wallet-option:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}
.wallet-icon { font-size: 1.5rem; }
.wallet-name { font-size: 0.95rem; font-weight: 500; color: #e7ecf5; }
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

/* Purchase history in dropdown */
.menu-purchases {
  padding: 0.75rem 0;
  border-top: 1px solid #2a3548;
  margin-top: 0.5rem;
}
.purchases-title {
  font-size: 0.7rem;
  color: #6b7a9e;
  text-transform: uppercase;
  margin-bottom: 0.5rem;
  font-weight: 600;
}
.purchase-item {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 0.35rem 0;
  font-size: 0.8rem;
}
.purchase-checks {
  color: #e7ecf5;
}
.purchase-status {
  font-size: 0.65rem;
  padding: 0.15rem 0.4rem;
  border-radius: 4px;
  text-transform: uppercase;
}
.purchase-status.completed {
  background: rgba(75, 201, 160, 0.2);
  color: #4bc9a0;
}
.purchase-status.pending {
  background: rgba(255, 193, 7, 0.2);
  color: #ffc107;
}
.purchase-status.cancelled {
  background: rgba(255, 107, 107, 0.2);
  color: #ff6b6b;
}
.view-all-link {
  display: block;
  margin-top: 0.5rem;
  font-size: 0.75rem;
  color: #667eea;
  text-decoration: none;
}
.view-all-link:hover {
  text-decoration: underline;
}

/* Balance display */
.balance-display {
  display: flex;
  align-items: center;
  gap: 0.3rem;
  padding: 0.4rem 0.8rem;
  background: rgba(75, 201, 160, 0.15);
  border: 1px solid rgba(75, 201, 160, 0.3);
  border-radius: 20px;
  color: #4bc9a0;
  font-size: 0.85rem;
  font-weight: 600;
}
.balance-display.clickable {
  cursor: pointer;
  transition: all 0.2s;
}
.balance-display.clickable:hover {
  background: rgba(75, 201, 160, 0.25);
  border-color: rgba(75, 201, 160, 0.5);
}
.balance-icon {
  font-size: 0.9rem;
}
.balance-count {
  min-width: 20px;
  text-align: center;
}
</style>
