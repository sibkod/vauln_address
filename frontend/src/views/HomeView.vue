<script setup lang="ts">
import { ref, inject, onMounted, computed, onUnmounted } from 'vue'
import ChainSelect from '../components/ChainSelect.vue'

const chain = ref('evm')
const address = ref('')
const loading = ref(false)
const result = ref<any>(null)
const chains = ref<any[]>([])
const recentChecks = ref<any[]>([])
const alertId = ref(0)

// API base URL
const apiBase = inject<string>('apiBase', '')

// Rate limit info from App.vue
const rateLimitInfo = inject<any>('rateLimitInfo')

// Get wallet auth from App.vue
const wallet = inject<any>('wallet')
const isConnected = computed(() => wallet?.connected?.value || false)
const userBalance = computed(() => wallet?.userBalance?.value || 0)

// Helper for API URLs
function apiUrl(path: string): string {
  return apiBase + path
}

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

const chainOptions = [
  { value: 'evm', label: 'EVM' },
  { value: 'btc', label: 'BTC' },
  { value: 'solana', label: 'Solana' },
  { value: 'sui', label: 'Sui' },
  { value: 'tron', label: 'Tron' },
]
const chainPlaceholders: Record<string, string> = {
  evm: 'Enter EVM address (0x…)',
  btc: 'Enter Bitcoin address',
  solana: 'Enter Solana address',
  sui: 'Enter Sui address (0x…)',
  tron: 'Enter Tron address (T…)'
}

onMounted(async () => {
  try {
    const res = await fetch(apiUrl('/api/chains'))
    if (res.ok) {
      const data = await res.json()
      chains.value = data.chains || []
    }
  } catch {}

  try {
    const res = await fetch(apiUrl('/api/recent'))
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
    const res = await fetch(apiUrl('/api/check'), {
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
      const data = await res.json()
      // Show details from response
      result.value = { status: 'error', message: data.details || data.error || 'No checks remaining' }
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

// Wallet status catalog — mirror of the backend /api/statuses data.
// Icons summarize wallet risk in the check result and the recent-checks list.
const statusMeta: Record<string, { icon: string; label: string; desc: string; cls: string }> = {
  hacked: { icon: '🚨', label: 'COMPROMISED', desc: 'DO NOT use this wallet', cls: 'danger' },
  compromised: { icon: '🚨', label: 'COMPROMISED', desc: 'DO NOT use this wallet', cls: 'danger' },
  hacker: { icon: '💀', label: 'HACKER', desc: 'never send assets here', cls: 'danger' },
  drained: { icon: '🏴', label: 'DRAINED', desc: 'wallet emptied after compromise', cls: 'danger' },
  phishing: { icon: '🎣', label: 'PHISHING', desc: 'collects funds from phishing victims', cls: 'danger' },
  scam: { icon: '🕳️', label: 'SCAM', desc: 'fraudulent scheme address', cls: 'danger' },
  sanctioned: { icon: '⛔', label: 'SANCTIONED', desc: 'on a sanctions list', cls: 'danger' },
  vulnerable: { icon: '⚠️', label: 'VULNERABLE', desc: 'data exposed, not yet exploited', cls: 'vulnerable' },
  suspicious: { icon: '🔍', label: 'SUSPICIOUS', desc: 'drainer-like activity, unconfirmed', cls: 'vulnerable' },
  mixer: { icon: '🌀', label: 'MIXER', desc: 'launders funds, compliance risk', cls: 'vulnerable' },
  frozen: { icon: '🧊', label: 'FROZEN', desc: 'assets frozen by issuer or court', cls: 'vulnerable' },
  exchange: { icon: '🏦', label: 'EXCHANGE', desc: 'verified service deposit address', cls: 'success' },
  safe: { icon: '✅', label: 'SAFE', desc: 'not found in database', cls: 'success' },
  not_found: { icon: '✅', label: 'SAFE', desc: 'not found in database', cls: 'success' },
  unknown: { icon: '❓', label: 'UNKNOWN', desc: 'no verdict yet — linked to a known hacker', cls: 'vulnerable' }
}

const legendItems = ['hacked', 'hacker', 'drained', 'phishing', 'vulnerable', 'suspicious', 'exchange', 'safe']
  .map(st => statusMeta[st])

function metaFor(status: string | undefined) {
  return statusMeta[status || ''] || { icon: '❓', label: (status || 'unknown').toUpperCase(), desc: '', cls: '' }
}

function setResultText() {
  if (!result.value) return ''
  if (result.value.message) return result.value.message
  if (result.value.error) return result.value.error
  const m = metaFor(result.value.status)
  return `${m.icon} ${m.label}${m.desc ? ` — ${m.desc}.` : ''}`
}

function getResultClass() {
  if (!result.value) return ''
  return metaFor(result.value.status).cls || 'success'
}
</script>

<template>
  <!-- Legend -->
  <div class="status-legend">
    <div class="legend-item" v-for="l in legendItems" :key="l.label">
      <span class="dot" :class="l.cls"></span>
      <div><span class="label">{{ l.icon }} {{ l.label }}</span><span class="desc">{{ l.desc }}</span></div>
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
    <ChainSelect v-model="chain" :options="chainOptions" />
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
    <RouterLink
      v-if="result && !result.message && !result.error && result.found && address"
      :to="{ path: '/report', query: { address: address.trim(), chain } }"
      class="detail-link"
    >
      📄 Full report →
    </RouterLink>
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
        :class="metaFor(check.status).cls"
      >
        <span class="alert-icon">{{ metaFor(check.status).icon }}</span>
        <span class="alert-addr">{{ check.address }}</span>
        <span class="alert-chain">{{ (check.chain || chain).toUpperCase() }}</span>
        <span class="alert-status" :class="check.status">{{ (check.status || 'safe').toUpperCase() }}</span>
        <span class="alert-time">{{ check.time }}</span>
      </div>
    </div>
  </div>
</template>
