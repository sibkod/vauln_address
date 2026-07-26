<script setup lang="ts">
import { ref, inject, onMounted, computed, onUnmounted } from 'vue'

const chain = ref('evm')
const address = ref('')
const loading = ref(false)
const result = ref<any>(null)
const chains = ref<any[]>([])
const recentChecks = ref<any[]>([])
const alertId = ref(0)

// Rate limit info from App.vue
const rateLimitInfo = inject<any>('rateLimitInfo')

// Get wallet auth from App.vue
const wallet = inject<any>('wallet')
const isConnected = computed(() => wallet?.connected?.value || false)
const userBalance = computed(() => wallet?.userBalance?.value || 0)

// Calculate display balance and status
const displayBalance = computed(() => {
  if (isConnected.value) {
    // Use purchased balance, or IP-based free checks if purchased is 0
    const purchased = userBalance.value || 0
    if (purchased > 0) {
      return purchased
    }
    // Fall back to IP-based free checks
    return rateLimitInfo?.value?.remaining || 0
  }
  return rateLimitInfo?.value?.remaining || 0
})

const isExhausted = computed(() => displayBalance.value <= 0)

// Countdown timer
const countdown = ref('')
let countdownInterval: number | null = null

function updateCountdown() {
  const reset = rateLimitInfo?.value?.reset
  if (!reset) {
    countdown.value = ''
    return
  }
  
  const now = Date.now()
  const resetMs = reset * 1000
  const diff = resetMs - now
  
  if (diff <= 0) {
    countdown.value = ''
    // Refresh to get new balance
    if (rateLimitInfo) {
      rateLimitInfo.value.remaining = rateLimitInfo.value.limit
    }
    return
  }
  
  const hours = Math.floor(diff / (1000 * 60 * 60))
  const minutes = Math.floor((diff % (1000 * 60 * 60)) / (1000 * 60))
  const seconds = Math.floor((diff % (1000 * 60)) / 1000)
  
  if (hours > 0) {
    countdown.value = `${hours}h ${minutes.toString().padStart(2, '0')}m`
  } else if (minutes > 0) {
    countdown.value = `${minutes}m ${seconds.toString().padStart(2, '0')}s`
  } else {
    countdown.value = `${seconds}s`
  }
}

onMounted(() => {
  updateCountdown()
  countdownInterval = window.setInterval(updateCountdown, 1000)
})

onUnmounted(() => {
  if (countdownInterval) {
    clearInterval(countdownInterval)
  }
})

const chainIcons: Record<string, string> = { evm: '🟣', btc: '🟠', solana: '🟢', sui: '🔵', tron: '🔴' }
const chainPlaceholders: Record<string, string> = {
  evm: 'Enter EVM address (0x…)',
  btc: 'Enter Bitcoin address',
  solana: 'Enter Solana address',
  sui: 'Enter Sui address (0x…)',
  tron: 'Enter Tron address (T…)'
}

onMounted(async () => {
  try {
    const res = await fetch('/api/chains')
    if (res.ok) {
      const data = await res.json()
      chains.value = data.chains || []
    }
  } catch {}

  try {
    const res = await fetch('/api/recent')
    if (res.ok) {
      const data = await res.json()
      recentChecks.value = (data.checks || []).map((c: any) => ({
        ...c,
        address: c.address ? `${c.address.slice(0,6)}…${c.address.slice(-4)}` : 'unknown',
        time: new Date(c.checked_at).toLocaleTimeString()
      }))
    }
  } catch {}
})

async function check() {
  if (!address.value.trim()) {
    result.value = { status: 'error', message: 'Enter a wallet address' }
    return
  }

  loading.value = true
  result.value = null
  
  const headers: Record<string, string> = { 'Content-Type': 'application/json' }
  if (wallet?.authToken?.value) {
    headers['Authorization'] = `Bearer ${wallet.authToken.value}`
  }
  
  try {
    const res = await fetch('/api/check', {
      method: 'POST',
      headers,
      body: JSON.stringify({ address: address.value.trim(), chain: chain.value })
    })
    
    // Always update rate limit info from headers (even on error)
    if (rateLimitInfo) {
      const remaining = res.headers.get('X-RateLimit-Remaining')
      const used = res.headers.get('X-RateLimit-Used')
      const reset = res.headers.get('X-RateLimit-Reset')
      const limit = res.headers.get('X-RateLimit-Limit')
      if (remaining !== null) rateLimitInfo.value.remaining = parseInt(remaining)
      if (used !== null) rateLimitInfo.value.used = parseInt(used)
      if (reset !== null) rateLimitInfo.value.reset = parseInt(reset)
      if (limit !== null) rateLimitInfo.value.limit = parseInt(limit)
    }
    
    if (res.status === 429) {
      result.value = { status: 'error', message: '⏳ Too many requests. Try again later.' }
      loading.value = false
      return
    }
    
    if (res.status === 402) {
      const data = await res.json()
      result.value = { status: 'error', message: data.error + (data.details ? ` (${data.details})` : '') }
      loading.value = false
      return
    }
    
    const data = await res.json()
    
    if (data.error) {
      result.value = { status: 'error', message: data.error + (data.details ? ` (${data.details})` : '') }
      loading.value = false
      return
    }
    
    result.value = data
    
    // Update balance from response
    if (data.balance_left !== undefined && wallet) {
      wallet.userBalance.value = data.balance_left
      localStorage.setItem('userBalance', String(data.balance_left))
    }
    
    recentChecks.value.unshift({
      id: alertId.value++,
      address: `${address.value.slice(0,6)}…${address.value.slice(-4)}`,
      chain: chain.value,
      status: data.status === 'not_found' ? 'safe' : data.status,
      time: new Date().toLocaleTimeString()
    })
    if (recentChecks.value.length > 30) recentChecks.value.pop()
  } catch (e) {
    result.value = { status: 'error', message: 'Request failed. Is the backend running?' }
  }
  loading.value = false
}

function setResultText() {
  if (!result.value) return ''
  if (result.value.message) return result.value.message
  if (result.value.error) return result.value.error
  if (result.value.status === 'hacked' || result.value.status === 'compromised') return '🚨 COMPROMISED — DO NOT use this wallet.'
  if (result.value.status === 'vulnerable') return '⚠️ VULNERABLE — data available, not yet exploited.'
  if (result.value.status === 'not_found' || result.value.status === 'safe') return '✅ Safe. Not found in database.'
  return `Status: ${result.value.status}`
}

function getResultClass() {
  if (!result.value) return ''
  if (result.value.status === 'hacked' || result.value.status === 'compromised') return 'danger'
  if (result.value.status === 'vulnerable') return 'vulnerable'
  return 'success'
}
</script>

<template>
  <!-- Legend -->
  <div class="status-legend">
    <div class="legend-item">
      <span class="dot hacked"></span>
      <div><span class="label">Hacked</span><span class="desc">Compromised — DO NOT use</span></div>
    </div>
    <div class="legend-item">
      <span class="dot vulnerable"></span>
      <div><span class="label">Vulnerable</span><span class="desc">Data available, not yet exploited</span></div>
    </div>
    <div class="legend-item">
      <span class="dot safe"></span>
      <div><span class="label">Safe</span><span class="desc">Not found in database</span></div>
    </div>
  </div>

  <!-- Balance info -->
  <div class="free-tier-info" :class="{ exhausted: isExhausted }">
    <span v-if="isExhausted && countdown">No checks. Reset in {{ countdown }}</span>
    <span v-else-if="isExhausted">No checks available</span>
    <span v-else-if="isConnected">{{ displayBalance }} checks</span>
    <span v-else>{{ displayBalance }} free checks</span>
    <RouterLink v-if="!isConnected" to="/pricing" class="upgrade-link">Connect wallet →</RouterLink>
    <RouterLink v-else-if="isExhausted" to="/pricing" class="upgrade-link">Buy more →</RouterLink>
    <span v-else class="upgrade-link">Buy more →</span>
  </div>

  <!-- Logo -->
  <div class="logo-area">
    <div class="badge">⚡ multi‑chain security</div>
    <h1>pwnd</h1>
    <div class="sub">EVM · BTC · Solana · Sui · Tron</div>
  </div>

  <!-- Search box -->
  <div class="search-box">
    <div class="chain-selector-wrapper">
      <span class="chain-icon">{{ chainIcons[chain] }}</span>
      <select v-model="chain" class="chain-selector">
        <option value="evm">EVM</option>
        <option value="btc">BTC</option>
        <option value="solana">Solana</option>
        <option value="sui">Sui</option>
        <option value="tron">Tron</option>
      </select>
      <span class="chain-arrow">▾</span>
    </div>
    <input 
      v-model="address" 
      type="text" 
      :placeholder="chainPlaceholders[chain]"
      @keydown.enter="check"
    />
    <button class="check-btn" @click="check" :disabled="loading">
      {{ loading ? 'Checking…' : 'Check' }}
    </button>
  </div>

  <!-- Result -->
  <div class="result-box" :class="result && result.message ? '' : getResultClass()">
    <div class="result-text">{{ setResultText() || 'Enter address and click Check' }}</div>
    <div class="result-sub" v-if="result && !result.message && !result.error && result.status !== 'not_found'">
      Found in database · {{ chain.toUpperCase() }}
    </div>
    <div class="result-chain" v-if="result && !result.message && !result.error">🔗 {{ chain.toUpperCase() }}</div>
  </div>

  <!-- Recent Checks -->
  <div class="alerts-section">
    <div class="alerts-header">
      <span class="alerts-title">🔴 recent checks</span>
      <span class="alerts-badge" :class="{ idle: recentChecks.length === 0 }">
        {{ recentChecks.length }} checked
      </span>
    </div>
    <div class="alerts-container">
      <div v-if="recentChecks.length === 0" style="color:#4c5a7a; font-size:0.8rem; padding:0.6rem; text-align:center;">
        No checks yet. Try one above!
      </div>
      <div 
        v-for="check in recentChecks" 
        :key="check.id || check.address + check.time"
        class="alert-item"
        :class="check.status === 'hacked' || check.status === 'compromised' ? 'danger' : check.status === 'vulnerable' ? 'vulnerable' : 'success'"
      >
        <span class="alert-icon">{{ check.status === 'hacked' || check.status === 'compromised' ? '🚨' : check.status === 'vulnerable' ? '⚠️' : '✅' }}</span>
        <span class="alert-addr">{{ check.address }}</span>
        <span class="alert-chain">{{ (check.chain || chain).toUpperCase() }}</span>
        <span class="alert-status" :class="check.status">{{ (check.status || 'safe').toUpperCase() }}</span>
        <span class="alert-time">{{ check.time }}</span>
      </div>
    </div>
  </div>
</template>
