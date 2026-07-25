<script setup lang="ts">
import { ref, inject, computed } from 'vue'
import QRCode from 'qrcode'

// Network config - MUST match App.vue
const IS_MAINNET = false
const SOLANA_NETWORK = IS_MAINNET ? 'mainnet-beta' : 'devnet'

// Merchant wallet for payments
const MERCHANT_WALLET_DEVNET = '7bMD8B3a3yDj7JMBQZYse7x4FqNKLNmEACSUitKxVNXJ'
const MERCHANT_WALLET_MAINNET = 'MERCHANT_MAINNET_WALLET'
const MERCHANT_WALLET = IS_MAINNET ? MERCHANT_WALLET_MAINNET : MERCHANT_WALLET_DEVNET

const globalWallet = inject<any>('wallet')

const packages = [
  { id: 'starter', name: 'Starter', checks: 50, priceSOL: 0.01, popular: false },
  { id: 'pro', name: 'Pro', checks: 200, priceSOL: 0.03, popular: true },
  { id: 'enterprise', name: 'Enterprise', checks: 1000, priceSOL: 0.1, popular: false },
]

const selectedPackage = ref<typeof packages[0] | null>(null)
const paymentMethod = ref<'SOL'>('SOL')
const processing = ref(false)
const error = ref('')
const success = ref('')
const txSignature = ref('')
const qrDataUrl = ref('')
const pollInterval = ref<number | null>(null)

const walletAddress = computed(() => globalWallet?.walletAddress?.value || '')
const isConnected = computed(() => globalWallet?.connected?.value || false)

function selectPackage(pkg: typeof packages[0]) {
  if (!isConnected.value) {
    error.value = 'Please connect your wallet first'
    return
  }
  selectedPackage.value = pkg
  error.value = ''
  success.value = ''
  txSignature.value = ''
  qrDataUrl.value = ''
  
  // Generate Solana Pay URL
  generateSolanaPayLink(pkg)
}

async function generateSolanaPayLink(pkg: typeof packages[0]) {
  // Create unique reference for this transaction
  const reference = generateReference()
  const amount = pkg.priceSOL.toString()
  
  // Solana Pay URL format
  // solana:<recipient>?amount=<lamports>&reference=<reference>&label=<label>&message=<message>
  const label = encodeURIComponent('WalletChecker - ' + pkg.name)
  const message = encodeURIComponent(`Payment for ${pkg.checks} checks - ${pkg.name} package`)
  
  // Amount in SOL (not lamports for display)
  const solanaPayUrl = `solana:${MERCHANT_WALLET}?amount=${amount}&reference=${reference}&label=${label}&message=${message}`
  
  // Generate QR code
  try {
    qrDataUrl.value = await QRCode.toDataURL(solanaPayUrl, {
      width: 256,
      margin: 2,
      color: { dark: '#e7ecf5', light: '#151a24' }
    })
  } catch (err) {
    console.error('QR generation error:', err)
  }
}

function generateReference(): string {
  // Generate random reference for the transaction
  const array = new Uint8Array(32)
  crypto.getRandomValues(array)
  return btoa(String.fromCharCode(...array)).replace(/\+/g, '-').replace(/\//g, '_').replace(/=/g, '')
}

function startPolling(signature: string) {
  txSignature.value = signature
  
  // Poll backend every 3 seconds to check if payment is confirmed
  pollInterval.value = window.setInterval(async () => {
    try {
      const res = await fetch(`/api/payment/status/${signature}`)
      const data = await res.json()
      
      if (data.status === 'confirmed') {
        stopPolling()
        success.value = `Payment confirmed! You now have ${data.balance} checks.`
        
        // Update wallet balance
        if (globalWallet) {
          globalWallet.userBalance.value = data.balance
          localStorage.setItem('userBalance', String(data.balance))
        }
        
        // Reset after 3 seconds
        setTimeout(() => {
          selectedPackage.value = null
          success.value = ''
        }, 3000)
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

function cancelPayment() {
  stopPolling()
  selectedPackage.value = null
  error.value = ''
  success.value = ''
  txSignature.value = ''
  qrDataUrl.value = ''
}

function openWalletLink() {
  if (selectedPackage.value) {
    const amount = selectedPackage.value.priceSOL.toString()
    const reference = generateReference()
    const solanaPayUrl = `solana:${MERCHANT_WALLET}?amount=${amount}&reference=${reference}`
    window.location.href = solanaPayUrl
  }
}
</script>

<template>
  <!-- Header -->
  <div class="pricing-header">
    <div class="badge">💎 Fair pricing</div>
    <h1>Choose Your Plan</h1>
    <div class="sub">Secure wallet checks with SOL or USDC</div>
  </div>

  <!-- Network Badge -->
  <div class="network-info">
    <span class="network-badge" :class="IS_MAINNET ? 'mainnet' : 'devnet'">
      {{ IS_MAINNET ? 'Mainnet' : 'Devnet' }} Mode
    </span>
    <span class="network-hint">
      {{ IS_MAINNET ? 'Real transactions' : 'Test transactions only' }}
    </span>
  </div>

  <!-- Packages -->
  <div class="packages-grid">
    <div 
      v-for="pkg in packages" 
      :key="pkg.id"
      class="package-card"
      :class="{ popular: pkg.popular, selected: selectedPackage?.id === pkg.id }"
      @click="selectPackage(pkg)"
    >
      <div v-if="pkg.popular" class="popular-badge">Most Popular</div>
      <div class="pkg-name">{{ pkg.name }}</div>
      <div class="pkg-checks">{{ pkg.checks }} checks</div>
      <div class="pkg-price">
        <span class="price-sol">{{ pkg.priceSOL }} SOL</span>
        <span class="price-usdc">or {{ pkg.priceUSDC }} USDC</span>
      </div>
      <div class="pkg-per-check">~${{ (pkg.priceUSDC / pkg.checks).toFixed(2) }} per check</div>
    </div>
  </div>

  <!-- Payment Section with QR -->
  <div v-if="selectedPackage" class="payment-section">
    <h2>Pay with SOL</h2>
    
    <div class="payment-summary">
      <div class="summary-row">
        <span>Package</span>
        <span>{{ selectedPackage.name }} ({{ selectedPackage.checks }} checks)</span>
      </div>
      <div class="summary-row">
        <span>Amount</span>
        <span class="amount-highlight">{{ selectedPackage.priceSOL }} SOL</span>
      </div>
      <div class="summary-row">
        <span>Network</span>
        <span>{{ IS_MAINNET ? 'Mainnet' : 'Devnet' }}</span>
      </div>
    </div>

    <!-- QR Code -->
    <div v-if="qrDataUrl" class="qr-section">
      <img :src="qrDataUrl" alt="Payment QR Code" class="qr-code" />
      <p class="qr-instruction">Scan with your Solana wallet</p>
      <p class="qr-wallet">To: {{ MERCHANT_WALLET.slice(0, 8) }}...{{ MERCHANT_WALLET.slice(-4) }}</p>
    </div>

    <!-- Direct Link Button -->
    <button class="wallet-link-btn" @click="openWalletLink">
      🔗 Pay with Solana Wallet
    </button>

    <!-- Manual Signature Entry -->
    <div class="signature-section">
      <p>Or enter transaction signature after payment:</p>
      <input 
        v-model="txSignature" 
        placeholder="Enter transaction signature (e.g., 4xJ4...)"
        class="signature-input"
      />
      <button 
        class="check-status-btn"
        @click="startPolling(txSignature)"
        :disabled="!txSignature || txSignature.length < 32"
      >
        Check Payment Status
      </button>
    </div>

    <div v-if="error" class="error-msg">{{ error }}</div>
    <div v-if="success" class="success-msg">{{ success }}</div>

    <button class="cancel-btn" @click="cancelPayment">Cancel</button>
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

.network-info {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 0.8rem;
  margin-bottom: 1.5rem;
}
.network-badge {
  padding: 0.3rem 0.8rem;
  border-radius: 20px;
  font-size: 0.75rem;
  font-weight: 600;
}
.network-badge.devnet {
  background: #1a2a1a;
  color: #4bc9a0;
  border: 1px solid #4bc9a040;
}
.network-badge.mainnet {
  background: #2a1a1a;
  color: #ff6b6b;
  border: 1px solid #ff6b6b40;
}
.network-hint {
  font-size: 0.75rem;
  color: #5a6a8e;
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
.package-card.selected {
  border-color: #4bc9a0;
  box-shadow: 0 0 20px #4bc9a020;
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

/* QR Code */
.qr-section {
  text-align: center;
  margin: 1.5rem 0;
}
.qr-code {
  border-radius: 12px;
  border: 2px solid #2a3548;
}
.qr-instruction {
  margin: 0.8rem 0 0.3rem;
  color: #98a8ce;
  font-size: 0.85rem;
}
.qr-wallet {
  margin: 0;
  color: #5a6a8e;
  font-size: 0.75rem;
  font-family: monospace;
}

/* Wallet Link */
.wallet-link-btn {
  width: 100%;
  padding: 0.9rem;
  background: linear-gradient(135deg, #667eea, #764ba2);
  border: none;
  border-radius: 10px;
  color: white;
  font-weight: 600;
  font-size: 0.9rem;
  cursor: pointer;
  transition: all 0.2s;
  margin-bottom: 1rem;
}
.wallet-link-btn:hover {
  transform: translateY(-2px);
  box-shadow: 0 4px 20px #667eea40;
}

/* Signature */
.signature-section {
  margin: 1rem 0;
  padding: 1rem;
  background: #151a24;
  border-radius: 10px;
  border: 1px solid #252d3d;
}
.signature-section p {
  margin: 0 0 0.8rem;
  color: #6b7a9e;
  font-size: 0.8rem;
}
.signature-input {
  width: 100%;
  padding: 0.7rem;
  background: #0c0f14;
  border: 1px solid #252d3d;
  border-radius: 8px;
  color: #e7ecf5;
  font-size: 0.8rem;
  font-family: monospace;
  margin-bottom: 0.8rem;
  box-sizing: border-box;
}
.signature-input:focus {
  outline: none;
  border-color: #667eea;
}
.check-status-btn {
  width: 100%;
  padding: 0.7rem;
  background: #2a3548;
  border: 1px solid #3a4568;
  border-radius: 8px;
  color: #e7ecf5;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.2s;
}
.check-status-btn:hover:not(:disabled) {
  background: #3a4568;
}
.check-status-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

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
  margin-top: 1rem;
}
.cancel-btn:hover {
  border-color: #ff6b6b40;
  color: #ff6b6b;
}

/* Amount highlight */
.amount-highlight {
  color: #4bc9a0;
  font-weight: 600;
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
</style>
