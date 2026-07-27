<script setup lang="ts">
import { ref, inject, computed, onMounted } from 'vue'
import { 
  Connection, 
  Transaction, 
  PublicKey, 
  SystemProgram, 
  LAMPORTS_PER_SOL 
} from '@solana/web3.js'

interface Package {
  id: string
  name: string
  checks: number
  price_usd: number
  price_sol: number
  discount_percent: number
  discount_label: string
  popular: boolean
}

interface PackagesResponse {
  packages: Package[]
  payment_address: string
  price_per_check: number
  network: string
}

// Network config - from environment variables
// Set VITE_SOLANA_RPC in .env to switch between devnet/mainnet
const RPC_URL = import.meta.env.VITE_SOLANA_RPC || 'https://api.devnet.solana.com'
const SOLANA_CLUSTER = import.meta.env.VITE_SOLANA_CLUSTER || 'devnet'

// API base URL
const apiBase = inject<string>('apiBase', '')
const globalWallet = inject<any>('wallet')

function apiUrl(path: string): string {
  return apiBase + path
}

// Create connection
const connection = new Connection(RPC_URL, 'confirmed')

// Packages loaded from backend
const packages = ref<Package[]>([])
const paymentAddress = ref('')
const loadingPackages = ref(true)
const packagesError = ref('')

const selectedPackage = ref<Package | null>(null)

// Payment modal state
const showPaymentModal = ref(false)
const paymentStatus = ref<'waiting' | 'processing' | 'success' | 'error'>('waiting')
const paymentMessage = ref('')
const txSignature = ref('')
const pollInterval = ref<number | null>(null)

const walletAddress = computed(() => globalWallet?.walletAddress?.value || '')
const isConnected = computed(() => globalWallet?.connected?.value || false)
const walletChain = computed(() => globalWallet?.walletChain?.value || '')
const phantomNetwork = computed(() => {
  const network = globalWallet?.phantomNetwork?.value || ''
  console.log('[PricingView] phantomNetwork:', network, 'SOLANA_CLUSTER:', SOLANA_CLUSTER)
  return network
})
const isCorrectNetwork = computed(() => {
  if (walletChain.value !== 'solana') return false
  
  // Mainnet check - must be mainnet-beta
  if (SOLANA_CLUSTER === 'mainnet') {
    return phantomNetwork.value === 'mainnet-beta'
  }
  
  // Devnet/testnet check - if NOT mainnet-beta, it's dev/test network
  return phantomNetwork.value !== 'mainnet-beta'
})

// Switch Phantom to correct network
async function switchToCorrectNetwork() {
  const phantom = (window as any).solana
  if (!phantom) {
    paymentStatus.value = 'error'
    paymentMessage.value = 'Phantom wallet not found'
    return
  }
  
  const targetLabel = SOLANA_CLUSTER === 'mainnet' ? 'Mainnet' : 'Devnet'
  
  // Show instruction
  paymentStatus.value = 'error'
  paymentMessage.value = `Please switch Phantom to ${targetLabel} and click the button again.`
  
  // Reconnect to refresh network detection
  try {
    await phantom.disconnect()
    await phantom.connect()
    // Network will be re-detected in App.vue
  } catch (err: any) {
    console.log('Reconnection:', err.message)
  }
}

// Fetch packages from backend
async function fetchPackages() {
  loadingPackages.value = true
  packagesError.value = ''
  try {
    const res = await fetch(apiUrl('/api/packages'))
    if (!res.ok) throw new Error('Failed to load packages')
    
    const data: PackagesResponse = await res.json()
    packages.value = data.packages
    paymentAddress.value = data.payment_address
  } catch (err: any) {
    packagesError.value = err.message || 'Failed to load pricing packages'
    console.error('Failed to fetch packages:', err)
  } finally {
    loadingPackages.value = false
  }
}

function selectPackage(pkg: Package) {
  if (!isConnected.value) {
    // Open wallet connection modal
    if (globalWallet?.openWalletModal) {
      globalWallet.openWalletModal()
    }
    return
  }
  
  // Check if wallet is on correct network
  if (walletChain.value === 'solana' && !isCorrectNetwork.value) {
    const expectedLabel = SOLANA_CLUSTER === 'mainnet' ? 'Mainnet' : 'Devnet'
    const currentLabel = phantomNetwork.value === 'mainnet-beta' ? 'Mainnet' : (phantomNetwork.value || 'Unknown')
    paymentStatus.value = 'error'
    paymentMessage.value = `Wrong network! Please switch Phantom to ${expectedLabel} (currently on ${currentLabel})`
    showPaymentModal.value = true
    return
  }
  
  selectedPackage.value = pkg
  payWithSolana()
}

onMounted(() => {
  fetchPackages()
})

// Pay directly with Solana wallet
async function payWithSolana() {
  if (!selectedPackage.value) return
  
  // Check if Phantom is installed
  const phantom = (window as any).solana
  if (!phantom) {
    paymentStatus.value = 'error'
    paymentMessage.value = 'Please install Phantom wallet: https://phantom.app/'
    showPaymentModal.value = true
    return
  }
  
  // Try to connect if not connected
  if (!phantom.isConnected || !phantom.publicKey) {
    try {
      await phantom.connect()
    } catch (err) {
      paymentStatus.value = 'error'
      paymentMessage.value = 'Please connect your Phantom wallet first'
      showPaymentModal.value = true
      return
    }
  }
  
  // Check if we have publicKey now
  if (!phantom.publicKey) {
    paymentStatus.value = 'error'
    paymentMessage.value = 'Please connect your Phantom wallet first'
    showPaymentModal.value = true
    return
  }
  
  // Check if user is on Solana chain
  if (walletChain.value !== 'solana') {
    paymentStatus.value = 'error'
    paymentMessage.value = 'Please connect a Solana wallet to pay with SOL'
    showPaymentModal.value = true
    return
  }
  
  // Open payment modal
  showPaymentModal.value = true
  paymentStatus.value = 'waiting'
  paymentMessage.value = 'Creating order...'
  txSignature.value = ''
  
  try {
    const authToken = localStorage.getItem('authToken')
    if (!authToken) {
      paymentStatus.value = 'error'
      paymentMessage.value = 'Please log in first'
      return
    }
    
    paymentStatus.value = 'processing'
    
    const walletAddr = phantom.publicKey.toString()
    
    // Create order first (requires auth)
    const orderRes = await fetch(apiUrl('/api/orders'), {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'Authorization': `Bearer ${authToken}`
      },
      body: JSON.stringify({
        checks: selectedPackage.value.checks,
        chain: 'solana',
        wallet_address: walletAddr
      })
    })
    
    // Parse response
    let orderData
    const text = await orderRes.text()
    try {
      orderData = JSON.parse(text)
    } catch {
      console.error('Non-JSON response:', text.substring(0, 200))
      throw new Error('Server error: ' + text.substring(0, 100))
    }
    
    if (!orderRes.ok) {
      throw new Error(orderData.error || 'Failed to create order')
    }
    
    // Show actual amount from backend (may differ from displayed price due to SOL price changes)
    const solAmount = parseFloat(orderData.amount)
    paymentMessage.value = `Send ${solAmount.toFixed(4)} SOL to complete your purchase`
    
    const senderPublicKey = phantom.publicKey
    const recipientPublicKey = new PublicKey(orderData.payment_address || paymentAddress.value)
    const lamports = Math.round(solAmount * LAMPORTS_PER_SOL)
    
    // Create transaction
    const transaction = new Transaction()
    transaction.add(
      SystemProgram.transfer({
        fromPubkey: senderPublicKey,
        toPubkey: recipientPublicKey,
        lamports: lamports,
      })
    )
    
    transaction.recentBlockhash = (await connection.getLatestBlockhash()).blockhash
    transaction.feePayer = senderPublicKey
    
    // Sign and send using Phantom
    paymentStatus.value = 'waiting'
    paymentMessage.value = `Sign the transaction in your wallet (${solAmount.toFixed(4)} SOL)...`
    const { signature } = await phantom.signAndSendTransaction(transaction)
    
    console.log('Transaction signature:', signature)
    txSignature.value = signature
    paymentMessage.value = 'Transaction sent! Waiting for confirmation...'
    
    // Poll for confirmation
    startPolling(signature)
    
  } catch (err: any) {
    console.error('Payment error:', err)
    paymentStatus.value = 'error'
    if (err.message?.includes('User rejected') || err.message?.includes('rejected') || err.message?.includes('cancelled')) {
      paymentMessage.value = 'Transaction cancelled'
    } else {
      paymentMessage.value = err.message || 'Payment failed'
    }
  }
}

function startPolling(signature: string) {
  const authToken = localStorage.getItem('authToken')
  pollInterval.value = window.setInterval(async () => {
    try {
      const res = await fetch(apiUrl(`/api/payment/status/${signature}`), {
        method: 'POST',
        headers: {
          'Authorization': authToken ? `Bearer ${authToken}` : ''
        }
      })
      const data = await res.json()
      
      if (data.status === 'confirmed') {
        stopPolling()
        paymentStatus.value = 'success'
        paymentMessage.value = `🎉 Success! ${selectedPackage.value?.checks} checks added to your balance.`
        
        if (globalWallet) {
          globalWallet.userBalance.value = data.balance || 0
          localStorage.setItem('userBalance', String(data.balance || 0))
          if (globalWallet.fetchMe) {
            globalWallet.fetchMe()
          }
        }
      } else if (data.status === 'failed') {
        stopPolling()
        paymentStatus.value = 'error'
        paymentMessage.value = 'Transaction failed on blockchain'
      }
    } catch (err) {
      console.error('Poll error:', err)
    }
  }, 3000)
}

function stopPolling() {
  if (pollInterval.value) {
    clearInterval(pollInterval.value)
    pollInterval.value = null
  }
}

function closePaymentModal() {
  stopPolling()
  showPaymentModal.value = false
  selectedPackage.value = null
  paymentStatus.value = 'waiting'
  paymentMessage.value = ''
  txSignature.value = ''
}

function closeIfAllowed() {
  // Only allow close by clicking overlay on success or error
  if (paymentStatus.value !== 'waiting' && paymentStatus.value !== 'processing') {
    closePaymentModal()
  }
}
</script>

<template>
  <!-- Loading state -->
  <div v-if="loadingPackages" class="loading-state">
    <div class="spinner"></div>
    <p>Loading pricing packages...</p>
  </div>

  <!-- Error state -->
  <div v-else-if="packagesError" class="error-state">
    <p>{{ packagesError }}</p>
    <button @click="fetchPackages" class="retry-btn">Retry</button>
  </div>

  <!-- Main content -->
  <template v-else>
    <!-- Header -->
    <div class="pricing-header">
      <div class="badge">💎 Fair pricing</div>
      <h1>Choose Your Plan</h1>
      <div class="sub">Secure wallet checks with SOL</div>
    </div>

    <!-- Wallet connection prompt -->
    <div v-if="!isConnected" class="wallet-prompt">
      <p>👻 Connect your wallet to purchase checks</p>
      <button class="connect-wallet-btn" @click="globalWallet?.openWalletModal?.()">Connect Wallet</button>
    </div>

    <!-- Wrong network warning -->
    <div v-else-if="walletChain === 'solana' && !isCorrectNetwork" class="network-warning">
      <p>⚠️ Wrong network! Please switch Phantom to {{ SOLANA_CLUSTER === 'mainnet' ? 'Mainnet' : 'Devnet' }}</p>
      <button class="network-switch-btn" @click="switchToCorrectNetwork">
        🔄 Switch Network
      </button>
    </div>

    <!-- Packages -->
    <div class="packages-grid">
      <div 
        v-for="pkg in packages" 
        :key="pkg.id"
        class="package-card"
        :class="{ popular: pkg.popular }"
        @click="selectPackage(pkg)"
      >
        <div v-if="pkg.popular" class="popular-badge">Most Popular</div>
        <div v-if="pkg.discount_label" class="discount-badge">{{ pkg.discount_label }}</div>
        <div class="pkg-name">{{ pkg.name }}</div>
        <div class="pkg-checks">{{ pkg.checks }} checks</div>
        <div class="pkg-price">
          <span class="price-sol">{{ pkg.price_sol }} SOL</span>
          <span class="price-usdc">or ${{ pkg.price_usd.toFixed(2) }}</span>
        </div>
        <div class="pkg-per-check">~${{ (pkg.price_usd / pkg.checks).toFixed(2) }} per check</div>
      </div>
    </div>

    <!-- Features -->
    <div class="features-section">
      <h2>What's Included</h2>
      <div class="features-grid">
        <div class="feature">
          <span class="feat-icon">🔍</span>
          <div>
            <div class="feat-title">Multi-chain Coverage</div>
            <div class="feat-desc">EVM, Bitcoin, Solana, Sui, Tron</div>
          </div>
        </div>
        <div class="feature">
          <span class="feat-icon">⚡</span>
          <div>
            <div class="feat-title">Instant Results</div>
            <div class="feat-desc">Real-time database lookup</div>
          </div>
        </div>
        <div class="feature">
          <span class="feat-icon">🛡️</span>
          <div>
            <div class="feat-title">Private & Secure</div>
            <div class="feat-desc">No data collection</div>
          </div>
        </div>
        <div class="feature">
          <span class="feat-icon">💎</span>
          <div>
            <div class="feat-title">Fresh Database</div>
            <div class="feat-desc">Daily updated threat intel</div>
          </div>
        </div>
      </div>
    </div>
  </template>

  <!-- Payment Modal -->
  <Teleport to="body">
    <div v-if="showPaymentModal" class="payment-modal-overlay" :class="{ 'no-close': paymentStatus === 'waiting' || paymentStatus === 'processing' }" @click.self="closeIfAllowed">
      <div class="payment-modal">
        <button v-if="paymentStatus === 'success' || paymentStatus === 'error'" class="modal-close" @click="closePaymentModal">×</button>
        
        <div class="modal-icon">
          <template v-if="paymentStatus === 'success'">🎉</template>
          <template v-else-if="paymentStatus === 'error'">❌</template>
          <template v-else>⏳</template>
        </div>
        
        <h2 class="modal-title">
          <template v-if="paymentStatus === 'success'">Payment Complete!</template>
          <template v-else-if="paymentStatus === 'error'">Payment Failed</template>
          <template v-else>Processing Payment</template>
        </h2>
        
        <div class="modal-message">{{ paymentMessage }}</div>
        
        <div v-if="selectedPackage && paymentStatus !== 'error' && paymentStatus !== 'success'" class="modal-details">
          <div class="detail-row">
            <span>Package</span>
            <span>{{ selectedPackage.name }}</span>
          </div>
          <div class="detail-row">
            <span>Checks</span>
            <span>{{ selectedPackage.checks }}</span>
          </div>
          <div class="detail-row">
            <span>Amount</span>
            <span>{{ selectedPackage.price_sol }} SOL</span>
          </div>
        </div>
        
        <div v-if="txSignature && paymentStatus !== 'error'" class="modal-tx">
          <span>TX:</span>
          <a :href="`https://explorer.solana.com/tx/${txSignature}?cluster=${SOLANA_CLUSTER}`" target="_blank">
            {{ txSignature.slice(0, 8) }}...{{ txSignature.slice(-8) }}
          </a>
        </div>
        
        <div v-if="paymentStatus === 'error' && walletChain === 'solana' && !isCorrectNetwork" class="switch-network-section">
          <button class="switch-network-btn" @click="switchToCorrectNetwork">
            🔄 Switch Network
          </button>
        </div>

        <button v-if="paymentStatus === 'success' || paymentStatus === 'error'" class="modal-btn" @click="closePaymentModal">
          <template v-if="paymentStatus === 'success'">Done</template>
          <template v-else>Close</template>
        </button>
      </div>
    </div>
  </Teleport>
</template>

<style scoped>
.pricing-header {
  text-align: center;
  margin-bottom: 2rem;
}
.pricing-header h1 {
  font-size: 2.2rem;
  font-weight: 700;
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  margin: 0.5rem 0;
}
.pricing-header .sub {
  color: #6b7a9e;
  font-size: 0.95rem;
}

.wallet-prompt {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 1rem;
  padding: 1rem 1.5rem;
  background: rgba(102, 126, 234, 0.1);
  border: 1px solid rgba(102, 126, 234, 0.3);
  border-radius: 12px;
  margin-bottom: 1.5rem;
  flex-wrap: wrap;
}
.wallet-prompt p {
  margin: 0;
  color: #98a8ce;
  font-size: 0.95rem;
}
.connect-wallet-btn {
  padding: 0.6rem 1.2rem;
  background: linear-gradient(135deg, #667eea, #764ba2);
  border: none;
  border-radius: 8px;
  color: white;
  font-weight: 600;
  font-size: 0.85rem;
  cursor: pointer;
  transition: all 0.2s;
}
.connect-wallet-btn:hover {
  transform: translateY(-1px);
  box-shadow: 0 4px 15px rgba(102, 126, 234, 0.4);
}

.network-warning {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 1rem;
  padding: 1rem 1.5rem;
  background: rgba(255, 193, 7, 0.1);
  border: 1px solid rgba(255, 193, 7, 0.3);
  border-radius: 12px;
  margin-bottom: 1.5rem;
  flex-wrap: wrap;
}
.network-warning p {
  margin: 0;
  color: #ffc107;
  font-size: 0.95rem;
  font-weight: 500;
}
.network-switch-btn {
  padding: 0.5rem 1rem;
  background: linear-gradient(135deg, #4bc9a0, #2ed573);
  border: none;
  border-radius: 8px;
  color: #0c0f14;
  font-weight: 600;
  font-size: 0.8rem;
  cursor: pointer;
  transition: all 0.2s;
}
.network-switch-btn:hover {
  transform: translateY(-1px);
  box-shadow: 0 4px 15px rgba(75, 201, 160, 0.4);
}

.packages-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: 1.2rem;
  margin-bottom: 2rem;
}
.package-card {
  background: #151a24;
  border: 1px solid #252d3d;
  border-radius: 16px;
  padding: 1.5rem;
  cursor: pointer;
  transition: all 0.2s;
  position: relative;
}
.package-card:hover {
  border-color: #4bc9a050;
  transform: translateY(-2px);
}
.package-card.popular {
  border-color: #667eea;
  background: linear-gradient(180deg, #1a1f2e 0%, #151a24 100%);
}
.popular-badge {
  position: absolute;
  top: -10px;
  left: 50%;
  transform: translateX(-50%);
  background: linear-gradient(135deg, #667eea, #764ba2);
  color: white;
  padding: 0.2rem 0.8rem;
  border-radius: 10px;
  font-size: 0.65rem;
  font-weight: 600;
}
.pkg-name {
  font-size: 1.2rem;
  font-weight: 600;
  color: #e7ecf5;
  margin-bottom: 0.3rem;
}
.pkg-checks {
  font-size: 1.8rem;
  font-weight: 700;
  color: #c8d2ea;
  margin-bottom: 0.8rem;
}
.pkg-price {
  display: flex;
  flex-direction: column;
  gap: 0.2rem;
  margin-bottom: 0.5rem;
}
.price-sol {
  font-size: 1rem;
  font-weight: 600;
  color: #4bc9a0;
}
.price-usdc {
  font-size: 0.8rem;
  color: #6b7a9e;
}
.pkg-per-check {
  font-size: 0.7rem;
  color: #5a6a8e;
}

.payment-section {
  background: #151a24;
  border: 1px solid #252d3d;
  border-radius: 16px;
  padding: 1.5rem;
  margin-bottom: 2rem;
}
.payment-section h2 {
  font-size: 1.2rem;
  margin-bottom: 1rem;
}
.payment-methods {
  display: flex;
  gap: 1rem;
  margin-bottom: 1.2rem;
}
.method-btn {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 0.5rem;
  padding: 0.8rem;
  background: #1a2030;
  border: 1px solid #252d3d;
  border-radius: 10px;
  color: #98a8ce;
  cursor: pointer;
  transition: all 0.2s;
}
.method-btn:hover {
  border-color: #4bc9a050;
}
.method-btn.active {
  border-color: #4bc9a0;
  background: #1a2a2020;
  color: #4bc9a0;
}
.method-icon {
  font-size: 1.2rem;
}
.payment-summary {
  background: #0c0f14;
  border-radius: 10px;
  padding: 1rem;
  margin-bottom: 1rem;
}
.summary-row {
  display: flex;
  justify-content: space-between;
  padding: 0.4rem 0;
  border-bottom: 1px solid #1a2030;
  font-size: 0.85rem;
}
.summary-row:last-child { border: none; }
.summary-row span:first-child { color: #6b7a9e; }
.summary-row span:last-child { color: #c8d2ea; font-weight: 500; }
.pay-btn {
  width: 100%;
  padding: 1rem;
  background: linear-gradient(135deg, #667eea, #764ba2);
  border: none;
  border-radius: 10px;
  color: white;
  font-weight: 600;
  font-size: 1rem;
  cursor: pointer;
  transition: all 0.2s;
}
.pay-btn:hover:not(:disabled) {
  transform: translateY(-1px);
  box-shadow: 0 4px 20px #667eea40;
}
.pay-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.error-msg {
  padding: 0.8rem;
  background: #2a1a1a;
  border: 1px solid #ff6b6b40;
  border-radius: 8px;
  color: #ff6b6b;
  font-size: 0.85rem;
  margin-bottom: 1rem;
}
.success-msg {
  padding: 0.8rem;
  background: #1a2a1a;
  border: 1px solid #4bc9a040;
  border-radius: 8px;
  color: #4bc9a0;
  font-size: 0.85rem;
  margin-bottom: 1rem;
}
.connect-wallet-prompt {
  text-align: center;
  padding: 1rem;
  color: #6b7a9e;
}
.connect-wallet-prompt p {
  margin-bottom: 1rem;
}
.wallet-info {
  font-size: 0.8rem;
  color: #4bc9a0;
  margin-bottom: 0.5rem;
  word-break: break-all;
}

/* Payment Section */
.payment-section {
  background: #151a24;
  border: 1px solid #252d3d;
  border-radius: 16px;
  padding: 1.5rem;
  margin-bottom: 1.5rem;
}
.payment-section h2 {
  text-align: center;
  margin-bottom: 1.2rem;
  font-size: 1.3rem;
  color: #e7ecf5;
}
.payment-summary {
  background: #0c0f14;
  border-radius: 10px;
  padding: 1rem;
  margin-bottom: 1.2rem;
}
.summary-row {
  display: flex;
  justify-content: space-between;
  padding: 0.4rem 0;
  border-bottom: 1px solid #1a2030;
}
.summary-row:last-child { border-bottom: none; }
.summary-row span:first-child { color: #6b7a9e; font-size: 0.85rem; }
.amount-highlight { color: #4bc9a0; font-weight: 600; font-size: 1.1rem; }
.address-small { font-size: 0.8rem; font-family: monospace; color: #98a8ce; }

/* Pay Button */
.pay-btn {
  width: 100%;
  padding: 1rem;
  background: linear-gradient(135deg, #667eea, #764ba2);
  border: none;
  border-radius: 10px;
  color: white;
  font-weight: 600;
  font-size: 1rem;
  cursor: pointer;
  transition: all 0.2s;
}
.pay-btn:hover:not(:disabled) {
  transform: translateY(-2px);
  box-shadow: 0 4px 20px #667eea40;
}
.pay-btn:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

/* Wallet Info Box */
.wallet-info-box {
  text-align: center;
  margin: 0.8rem 0;
  padding: 0.5rem;
  background: #1a2a1a;
  border-radius: 8px;
  font-size: 0.85rem;
  color: #4bc9a0;
}

/* TX Info */
.tx-info {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-top: 1rem;
  padding: 0.8rem;
  background: #151a24;
  border-radius: 8px;
  font-size: 0.8rem;
  color: #98a8ce;
}
.view-link {
  color: #667eea;
  text-decoration: none;
}
.view-link:hover { text-decoration: underline; }

/* Cancel Button */
.cancel-btn {
  width: 100%;
  padding: 0.7rem;
  background: transparent;
  border: 1px solid #2a3548;
  border-radius: 8px;
  color: #6b7a9e;
  font-size: 0.85rem;
  cursor: pointer;
  margin-top: 0.8rem;
}
.cancel-btn:hover {
  border-color: #ff6b6b40;
  color: #ff6b6b;
}

.features-section {
  margin-top: 2rem;
}
.features-section h2 {
  font-size: 1.3rem;
  margin-bottom: 1rem;
  text-align: center;
}
.features-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: 1rem;
}
.feature {
  display: flex;
  align-items: flex-start;
  gap: 0.8rem;
  padding: 1rem;
  background: #151a24;
  border-radius: 12px;
}
.feat-icon {
  font-size: 1.5rem;
}
.feat-title {
  font-weight: 600;
  font-size: 0.9rem;
  margin-bottom: 0.2rem;
}
.feat-desc {
  font-size: 0.75rem;
  color: #6b7a9e;
}

/* Payment Modal Styles */
.payment-modal-overlay {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background: rgba(0, 0, 0, 0.8);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
}

.payment-modal-overlay.no-close {
  cursor: not-allowed;
}

.payment-modal {
  background: #1a1f2e;
  border: 1px solid #2a3548;
  border-radius: 20px;
  padding: 2rem;
  width: 90%;
  max-width: 400px;
  text-align: center;
  position: relative;
}

.modal-close {
  position: absolute;
  top: 1rem;
  right: 1rem;
  background: none;
  border: none;
  color: #6b7a9e;
  font-size: 1.5rem;
  cursor: pointer;
}

.modal-close:hover {
  color: #e7ecf5;
}

.modal-icon {
  font-size: 4rem;
  margin-bottom: 1rem;
}

.modal-title {
  font-size: 1.5rem;
  font-weight: 600;
  color: #e7ecf5;
  margin-bottom: 1rem;
}

.modal-message {
  color: #98a8ce;
  font-size: 0.95rem;
  margin-bottom: 1.5rem;
  line-height: 1.5;
}

.modal-details {
  background: #151a24;
  border-radius: 12px;
  padding: 1rem;
  margin-bottom: 1.5rem;
}

.detail-row {
  display: flex;
  justify-content: space-between;
  padding: 0.5rem 0;
  border-bottom: 1px solid #2a3548;
}

.detail-row:last-child {
  border-bottom: none;
}

.detail-row span:first-child {
  color: #6b7a9e;
  font-size: 0.85rem;
}

.detail-row span:last-child {
  color: #e7ecf5;
  font-weight: 500;
}

.modal-tx {
  background: #151a24;
  border-radius: 8px;
  padding: 0.75rem;
  margin-bottom: 1.5rem;
  font-size: 0.85rem;
}

.modal-tx a {
  color: #667eea;
  text-decoration: none;
  font-family: monospace;
}

.modal-tx a:hover {
  text-decoration: underline;
}

.switch-network-section {
  margin-bottom: 1rem;
}

.switch-network-btn {
  width: 100%;
  padding: 0.8rem;
  background: linear-gradient(135deg, #4bc9a0, #2ed573);
  border: none;
  border-radius: 10px;
  color: #0c0f14;
  font-weight: 600;
  font-size: 0.9rem;
  cursor: pointer;
  transition: all 0.2s;
}

.switch-network-btn:hover {
  transform: translateY(-2px);
  box-shadow: 0 4px 15px rgba(75, 201, 160, 0.4);
}

.modal-btn {
  width: 100%;
  padding: 1rem;
  background: linear-gradient(135deg, #667eea, #764ba2);
  border: none;
  border-radius: 10px;
  color: white;
  font-weight: 600;
  font-size: 1rem;
  cursor: pointer;
  transition: all 0.2s;
}

.modal-btn:hover {
  transform: translateY(-2px);
  box-shadow: 0 4px 20px #667eea40;
}
</style>
