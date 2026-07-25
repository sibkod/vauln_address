<script setup lang="ts">
import { ref, computed } from 'vue'

const chain = ref('evm')
const address = ref('')
const loading = ref(false)
const result = ref<any>(null)
const freeCheckUsed = ref(false)
const alerts = ref<any[]>([])
const alertId = ref(0)

const chainIcons: Record<string, string> = { evm: '🟣', btc: '🟠', solana: '🟢', sui: '🔵', tron: '🔴' }
const chainPlaceholders: Record<string, string> = {
  evm: 'Enter EVM address (0x…)',
  btc: 'Enter Bitcoin address',
  solana: 'Enter Solana address',
  sui: 'Enter Sui address (0x…)',
  tron: 'Enter Tron address (T…)'
}

const checkCount = computed(() => freeCheckUsed.value ? '1 / 1' : '0 / 1')
const resultClass = computed(() => {
  if (!result.value) return ''
  if (result.value.status === 'hacked') return 'danger'
  if (result.value.status === 'vulnerable') return 'vulnerable'
  return 'success'
})

const exampleAddresses = [
  { chain: 'evm', addr: '0xdeadbeef1234567890abcdef1234567890abcdef' },
  { chain: 'btc', addr: '1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa' },
  { chain: 'solana', addr: '4f3v9w8x2y7z6a5b4c3d2e1f0g9h8i7j6k5l4m3n2o1p' },
]

async function check() {
  if (!address.value.trim()) {
    result.value = { status: 'error', message: 'Enter a wallet address' }
    return
  }
  if (freeCheckUsed.value) {
    result.value = { status: 'error', message: 'Limit reached. Connect wallet for unlimited checks.' }
    return
  }

  loading.value = true
  try {
    const res = await fetch('/api/check', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ address: address.value.trim(), chain: chain.value })
    })
    const data = await res.json()
    result.value = data
    freeCheckUsed.value = true
    addAlert(address.value.trim(), chain.value, data.status || 'safe')
  } catch (e) {
    result.value = { status: 'error', message: 'Request failed' }
  }
  loading.value = false
}

function addAlert(addr: string, ch: string, status: string) {
  const now = new Date()
  const timeStr = now.toLocaleTimeString('en-US', { hour: '2-digit', minute: '2-digit', second: '2-digit' })
  const users = ['0xMike', 'CryptoGuy', 'WalletWhale', 'DeFiKing']
  alerts.value.unshift({
    id: alertId.value++,
    address: `${addr.slice(0,6)}…${addr.slice(-4)}`,
    full: addr,
    chain: ch,
    status,
    time: timeStr,
    user: users[Math.floor(Math.random() * users.length)]
  })
  if (alerts.value.length > 30) alerts.value.pop()
}

function useExample(ex: { chain: string, addr: string }) {
  address.value = ex.addr
  chain.value = ex.chain
  check()
}

function setResultText() {
  if (!result.value) return ''
  if (result.value.error) return result.value.error
  if (result.value.status === 'hacked') return '🚨 COMPROMISED — DO NOT use this wallet.'
  if (result.value.status === 'vulnerable') return '⚠️ VULNERABLE — data available, not yet exploited.'
  return '✅ Safe. Not found in database.'
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

  <!-- Search info -->
  <div class="search-info">
    <span class="counter">{{ checkCount }}</span>
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

  <!-- Example hints -->
  <div class="example-hint">
    <span>💡 try:</span>
    <template v-for="ex in exampleAddresses" :key="ex.chain + ex.addr">
      <span class="example-addr" @click="useExample(ex)">{{ ex.addr.slice(0,8) }}…</span>
      <span class="chain-tag">{{ ex.chain.toUpperCase() }}</span>
    </template>
  </div>

  <!-- Result -->
  <div class="result-box" :class="resultClass">
    <div class="result-text">{{ setResultText() }}</div>
    <div class="result-sub" v-if="result && !result.error">
      Found in database · {{ chain.toUpperCase() }}
    </div>
    <div class="result-chain" v-if="result && !result.error">🔗 {{ chain.toUpperCase() }}</div>
  </div>

  <!-- Alerts -->
  <div class="alerts-section">
    <div class="alerts-header">
      <span class="alerts-title">🔴 live alerts</span>
      <span class="alerts-badge" :class="{ idle: alerts.length === 0 }">
        {{ alerts.length }} alert{{ alerts.length !== 1 ? 's' : '' }}
      </span>
    </div>
    <div class="alerts-container">
      <div v-if="alerts.length === 0" style="color:#4c5a7a; font-size:0.8rem; padding:0.6rem; text-align:center;">
        waiting for checks…
      </div>
      <div 
        v-for="alert in alerts" 
        :key="alert.id" 
        class="alert-item"
        :class="alert.status === 'hacked' ? 'danger' : alert.status === 'vulnerable' ? 'vulnerable' : 'success'"
      >
        <span class="alert-icon">{{ alert.status === 'hacked' ? '🚨' : alert.status === 'vulnerable' ? '⚠️' : '✅' }}</span>
        <span class="alert-addr">{{ alert.address }}</span>
        <span class="alert-chain">{{ alert.chain.toUpperCase() }}</span>
        <span class="alert-status" :class="alert.status">{{ alert.status.toUpperCase() }}</span>
        <span class="alert-user">{{ alert.user }}</span>
        <span class="alert-time">{{ alert.time }}</span>
      </div>
    </div>
  </div>
</template>
