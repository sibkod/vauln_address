<script setup lang="ts">
import { ref, inject, onMounted, onUnmounted } from 'vue'
import { useWallet, useConnection } from '@solana/wallet-adapter-react'
import { PublicKey, SystemProgram, Transaction } from '@solana/web3.js'
import { createTransferCheckedInstruction, getAssociatedTokenAddress } from '@solana/spl-token'

// Network config - MUST match App.vue
const IS_MAINNET = false
const SOLANA_NETWORK = IS_MAINNET ? 'mainnet-beta' : 'devnet'

// USDC addresses
const USDC_DEVNET = '4zMMC9srt5Ri5X14zfNUkFN5MkBYaAMDzAPBG7aajJJ'
const USDC_MAINNET = 'EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDj1'
const USDC_ADDRESS = IS_MAINNET ? USDC_MAINNET : USDC_DEVNET

// Merchant wallet for payments
const MERCHANT_WALLET_DEVNET = '7bMD8B3a3yDj7JMBQZYse7x4FqNKLNmEACSUitKxVNXJ'
const MERCHANT_WALLET_MAINNET = 'MERCHANT_MAINNET_WALLET'
const MERCHANT_WALLET = IS_MAINNET ? MERCHANT_WALLET_MAINNET : MERCHANT_WALLET_DEVNET

const wallet = useWallet()
const connection = useConnection()
const globalWallet = inject<any>('wallet')

const packages = [
  { id: 'starter', name: 'Starter', checks: 50, priceSOL: 0.01, priceUSDC: 10, popular: false },
  { id: 'pro', name: 'Pro', checks: 200, priceSOL: 0.03, priceUSDC: 35, popular: true },
  { id: 'enterprise', name: 'Enterprise', checks: 1000, priceSOL: 0.1, priceUSDC: 100, popular: false },
]

const selectedPackage = ref<typeof packages[0] | null>(null)
const paymentMethod = ref<'SOL' | 'USDC'>('SOL')
const processing = ref(false)
const error = ref('')
const success = ref('')

async function selectPackage(pkg: typeof packages[0]) {
  if (!wallet.connected) {
    error.value = 'Please connect your wallet first'
    return
  }
  selectedPackage.value = pkg
  error.value = ''
  success.value = ''
}

async function payWithSOL() {
  if (!selectedPackage.value || !wallet.publicKey || !wallet.signTransaction) {
    error.value = 'Wallet not connected'
    return
  }

  processing.value = true
  error.value = ''

  try {
    const transaction = new Transaction()
    
    const lamports = selectedPackage.value.priceSOL * 1e9
    transaction.add(
      SystemProgram.transfer({
        fromPubkey: wallet.publicKey,
        toPubkey: new PublicKey(MERCHANT_WALLET),
        lamports: Math.round(lamports)
      })
    )

    const { blockhash } = await connection.getLatestBlockhash()
    transaction.recentBlockhash = blockhash
    transaction.feePayer = wallet.publicKey

    const signed = await wallet.signTransaction(transaction)
    const signature = await connection.sendRawTransaction(signed.serialize())

    if (signature) {
      success.value = `Payment sent! TX: ${signature.slice(0, 8)}...`
      await notifyBackend(signature, 'SOL')
    }
  } catch (err: any) {
    error.value = 'Payment failed: ' + err.message
  }

  processing.value = false
}

async function payWithUSDC() {
  if (!selectedPackage.value || !wallet.publicKey || !wallet.signTransaction) {
    error.value = 'Wallet not connected'
    return
  }

  processing.value = true
  error.value = ''

  try {
    const usdcMint = new PublicKey(USDC_ADDRESS)
    const merchantWallet = new PublicKey(MERCHANT_WALLET)
    const sender = wallet.publicKey

    const senderUsdc = await getAssociatedTokenAddress(usdcMint, sender, true)
    const merchantUsdc = await getAssociatedTokenAddress(usdcMint, merchantWallet, true)

    const transaction = new Transaction()
    const amount = Math.round(selectedPackage.value.priceUSDC * 1e6)

    transaction.add(
      createTransferCheckedInstruction(
        senderUsdc,
        usdcMint,
        merchantUsdc,
        sender,
        amount,
        6
      )
    )

    const { blockhash } = await connection.getLatestBlockhash()
    transaction.recentBlockhash = blockhash
    transaction.feePayer = wallet.publicKey

    const signed = await wallet.signTransaction(transaction)
    const signature = await connection.sendRawTransaction(signed.serialize())

    if (signature) {
      success.value = `Payment sent! TX: ${signature.slice(0, 8)}...`
      await notifyBackend(signature, 'USDC')
    }
  } catch (err: any) {
    error.value = 'Payment failed: ' + err.message
  }

  processing.value = false
}

async function notifyBackend(tx: string, method: string) {
  try {
    await fetch('/api/payment/confirm', {
      method: 'POST',
      headers: { 
        'Content-Type': 'application/json',
        'Authorization': `Bearer ${globalWallet.authToken?.value || ''}`
      },
      body: JSON.stringify({
        tx,
        package: selectedPackage.value?.id,
        method,
        network: SOLANA_NETWORK
      })
    })
  } catch {}
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

  <!-- Payment Section -->
  <div v-if="selectedPackage" class="payment-section">
    <h2>Pay with {{ selectedPackage.name }}</h2>
    
    <div class="payment-methods">
      <button 
        class="method-btn"
        :class="{ active: paymentMethod === 'SOL' }"
        @click="paymentMethod = 'SOL'"
      >
        <span class="method-icon">◎</span>
        <span>Pay with SOL</span>
      </button>
      <button 
        class="method-btn"
        :class="{ active: paymentMethod === 'USDC' }"
        @click="paymentMethod = 'USDC'"
      >
        <span class="method-icon">💲</span>
        <span>Pay with USDC</span>
      </button>
    </div>

    <div class="payment-summary">
      <div class="summary-row">
        <span>Package</span>
        <span>{{ selectedPackage.name }} ({{ selectedPackage.checks }} checks)</span>
      </div>
      <div class="summary-row">
        <span>Amount</span>
        <span>{{ paymentMethod === 'SOL' ? selectedPackage.priceSOL + ' SOL' : selectedPackage.priceUSDC + ' USDC' }}</span>
      </div>
      <div class="summary-row">
        <span>Network</span>
        <span>{{ SOLANA_NETWORK }}</span>
      </div>
    </div>

    <div v-if="error" class="error-msg">{{ error }}</div>
    <div v-if="success" class="success-msg">{{ success }}</div>

    <button 
      v-if="!wallet.connected"
      class="pay-btn"
      disabled
    >
      Connect wallet to pay
    </button>
    <button 
      v-else
      class="pay-btn"
      :disabled="processing"
      @click="paymentMethod === 'SOL' ? payWithSOL() : payWithUSDC()"
    >
      {{ processing ? 'Processing...' : `Pay ${paymentMethod === 'SOL' ? selectedPackage.priceSOL + ' SOL' : selectedPackage.priceUSDC + ' USDC'}` }}
    </button>
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
