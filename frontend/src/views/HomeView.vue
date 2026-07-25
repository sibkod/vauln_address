<script setup lang="ts">
import { ref, inject, onMounted } from 'vue'

const chain = ref('evm')
const address = ref('')
const loading = ref(false)
const result = ref<any>(null)
const chains = ref<any[]>([])
const recentChecks = ref<any[]>([])
const alertId = ref(0)

// Get wallet auth from App.vue
const wallet = inject<any>('wallet')

const chainIcons: Record<string, string> = { evm: '🟣', btc: '🟠', solana: '🟢', sui: '🔵', tron: '🔴' }
const chainPlaceholders: Record<string, string> = {
  evm: 'Enter EVM address (0x…)',
  btc: 'Enter Bitcoin address',
  solana: 'Enter Solana address',
  sui: 'Enter Sui address (0x…)',
  tron: 'Enter Tron address (T…)'
}

onMounted(async () => {
  // Fetch supported chains
  try {
    const res = await fetch('/api/chains')
    if (res.ok) {
      const data = await res.json()
      chains.value = data.chains || []
    }
  } catch {}

  // Fetch recent checks
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
    
    if (res.status === 429) {
      const data = await res.json()
      result.value = { status: 'error', message: `⏳ Rate limit: ${data.details || 'Try again later'}` }
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
    
    // Update balance if returned
    if (data.balance_left !== undefined && wallet) {
      wallet.userBalance.value = data.balance_left
      localStorage.setItem('userBalance', String(data.balance_left))
    }
    
    // Add to recent checks only AFTER successful API response
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

function useExample(ex: { chain: string, addr: string }) {
  address.value = ex.addr
  chain.value = ex.chain
  check()
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

  <!-- Logo -->
  <div class="logo-area">
    <div class="badge">⚡ multi‑chain security</div>
    <h1>Wallet Checker</h1>
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
