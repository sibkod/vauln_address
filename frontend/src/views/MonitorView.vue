<script setup lang="ts">
import { ref, inject, onMounted, onUnmounted } from 'vue'
import ChainLogo from '../components/ChainLogo.vue'
import { getChainMeta } from '../chains'

interface Finding {
  id: number
  chain: string
  signature: string
  slot: number
  verdict: string
  indicators: string[]
  victim_address?: string
  hacker_address?: string
  amount_sol: number
  programs?: string[]
  exposed_addresses?: string[]
  source: string
  created_at: string
}

interface OperationType {
  key: string
  label: string
  icon: string
  from: string
  to: string
  danger: boolean
}

// Derives a human-readable operation type from the scanner verdict and
// indicator codes (mirrors scanIndicatorMeta on the backend).
function operationType(f: Finding): OperationType {
  const ind = f.indicators || []
  if (ind.includes('P1_ACCOUNT_TAKEOVER')) {
    return { key: 'takeover', label: 'Account takeover', icon: '🔑', from: 'Victim', to: 'Hacker', danger: true }
  }
  if (ind.includes('F2_REPEAT_DOWNSTREAM')) {
    return { key: 'distribution', label: 'Hacker moving funds', icon: '🔀', from: 'Operator', to: 'Recipient', danger: true }
  }
  if (ind.includes('F1_DOWNSTREAM_TRANSFER')) {
    return { key: 'downstream', label: 'Downstream transfer', icon: '↪️', from: 'Operator', to: 'Recipient', danger: false }
  }
  if (f.verdict === 'DRAINER') {
    return { key: 'drained', label: 'Wallet drained', icon: '💀', from: 'Victim', to: 'Hacker', danger: true }
  }
  return { key: 'suspicious', label: 'Suspicious activity', icon: '⚠️', from: 'Wallet', to: 'Counterparty', danger: false }
}

const apiBase = inject<string>('apiBase', '')

const findings = ref<Finding[]>([])
const loading = ref(true)
const live = ref(true)
const lastError = ref('')

const POLL_INTERVAL = 4000
const FEED_SIZE = 10
let pollTimer: number | null = null

async function fetchLatest() {
  const res = await fetch(`${apiBase}/api/monitor/findings`)
  if (!res.ok) throw new Error('findings request failed')
  const data = await res.json()
  findings.value = (data.findings || []).slice(0, FEED_SIZE)
}

async function pollNew() {
  if (!live.value) return
  try {
    const maxId = findings.value.length ? findings.value[0].id : 0
    const res = await fetch(`${apiBase}/api/monitor/findings?after_id=${maxId}`)
    if (res.ok) {
      const data = await res.json()
      const fresh: Finding[] = data.findings || []
      if (fresh.length) {
        // fresh rows come ascending; newest first in the feed
        findings.value = [...fresh.reverse(), ...findings.value].slice(0, FEED_SIZE)
      }
    }
    lastError.value = ''
  } catch {
    lastError.value = 'Connection lost — retrying…'
  }
}

function toggleLive() {
  live.value = !live.value
}

function shortAddr(addr?: string): string {
  if (!addr) return '—'
  return addr.length <= 16 ? addr : `${addr.slice(0, 6)}…${addr.slice(-6)}`
}

// Flow-trace findings (F1/F2) store no victim: the operator wallet is the
// first exposed source address.
function fromAddr(f: Finding): string {
  return f.victim_address || f.exposed_addresses?.[0] || ''
}

// Token/NFT transfers carry an ERC20:<symbol> / ERC721:<symbol>
// meta-indicator (set by the live-block scanners); without it the amount
// is in the chain-native coin.
function tokenSymbol(f: Finding): string {
  const ind = (f.indicators || []).find(i => i.startsWith('ERC20:') || i.startsWith('ERC721:'))
  return ind ? ind.slice(ind.indexOf(':') + 1) : ''
}

function displaySymbol(f: Finding): string {
  return tokenSymbol(f) || getChainMeta(f.chain).symbol
}

function formatAmount(f: Finding): string {
  const a = f.amount_sol
  if (tokenSymbol(f)) {
    // token amounts: no fixed chain decimals — trim to something readable
    if (a >= 1000) return Math.round(a).toLocaleString('en-US')
    return String(parseFloat(a.toFixed(6)))
  }
  return a.toFixed(getChainMeta(f.chain).decimals)
}

function txUrl(f: Finding): string {
  return getChainMeta(f.chain).txUrl(f.signature)
}

function accountUrl(f: Finding, addr: string): string {
  return getChainMeta(f.chain).addrUrl(addr)
}

function timeAgo(ts: string): string {
  const diff = Date.now() - new Date(ts).getTime()
  if (diff < 60000) return `${Math.max(1, Math.floor(diff / 1000))}s ago`
  if (diff < 3600000) return `${Math.floor(diff / 60000)}m ago`
  if (diff < 86400000) return `${Math.floor(diff / 3600000)}h ago`
  return new Date(ts).toLocaleString()
}

onMounted(async () => {
  try {
    await fetchLatest()
    lastError.value = ''
  } catch {
    lastError.value = 'Failed to load monitoring data. Is the backend running?'
  } finally {
    loading.value = false
  }
  pollTimer = window.setInterval(pollNew, POLL_INTERVAL)
})

onUnmounted(() => {
  if (pollTimer) clearInterval(pollTimer)
})
</script>

<template>
  <div class="monitor-page">
    <div class="monitor-header">
      <h1>Live Drainer Monitor</h1>
      <p class="subtitle">Real-time detections from our multi-chain drainer scanner</p>
      <div class="live-controls">
        <span class="live-badge" :class="{ paused: !live }">
          <span class="pulse-dot"></span>
          {{ live ? 'LIVE' : 'PAUSED' }}
        </span>
        <button class="live-toggle" @click="toggleLive">
          {{ live ? '⏸ Pause' : '▶ Resume' }}
        </button>
        <span v-if="lastError" class="feed-error">{{ lastError }}</span>
      </div>
    </div>

    <div v-if="loading" class="loading">
      <div class="spinner"></div>
      <p>Connecting to the scanner feed…</p>
    </div>

    <div v-else-if="!findings.length" class="empty-state">
      <div class="empty-icon">🛰️</div>
      <h2>No detections yet</h2>
      <p>The scanner is watching the chains. New drainer detections will appear here in real time.</p>
    </div>

    <div v-else class="feed">
      <div v-for="f in findings" :key="f.id" class="finding-card" :class="operationType(f).danger ? 'drainer' : 'suspicious'">
        <div class="finding-top">
          <span class="chain-badge">
            <ChainLogo :chain="f.chain" :size="16" />
            {{ getChainMeta(f.chain).name }}
          </span>
          <span class="op-badge" :class="operationType(f).danger ? 'drainer' : 'suspicious'">
            {{ operationType(f).icon }} {{ operationType(f).label }}
          </span>
          <span v-if="f.amount_sol > 0" class="amount">
            -{{ formatAmount(f) }} {{ displaySymbol(f) }}
          </span>
          <span class="time">{{ timeAgo(f.created_at) }}</span>
        </div>

        <div class="parties">
          <div class="party">
            <span class="party-label">{{ operationType(f).from }}</span>
            <a v-if="fromAddr(f)" :href="accountUrl(f, fromAddr(f))" target="_blank" rel="noopener" class="addr victim">
              {{ shortAddr(fromAddr(f)) }}
            </a>
            <span v-else class="addr none">—</span>
          </div>
          <span class="party-arrow">→</span>
          <div class="party">
            <span class="party-label">{{ operationType(f).to }}</span>
            <a v-if="f.hacker_address" :href="accountUrl(f, f.hacker_address)" target="_blank" rel="noopener" class="addr hacker">
              {{ shortAddr(f.hacker_address) }}
            </a>
            <span v-else class="addr none">—</span>
          </div>
        </div>

        <div class="finding-footer">
          <a :href="txUrl(f)" target="_blank" rel="noopener" class="tx-link">
            {{ shortAddr(f.signature) }} ↗
          </a>
          <span v-if="f.slot" class="slot">{{ getChainMeta(f.chain).blockLabel }} {{ f.slot }}</span>
          <span class="source">{{ f.source }}</span>
        </div>
      </div>

    </div>
  </div>
</template>

<style scoped>
.monitor-page {
  max-width: 1000px;
  margin: 0 auto;
  padding: 2rem 1rem;
}

.monitor-header {
  text-align: center;
  margin-bottom: 1.5rem;
}

.monitor-header h1 {
  font-size: 2rem;
  font-weight: 700;
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  margin-bottom: 0.35rem;
}

.subtitle {
  color: #6b7a9e;
  font-size: 0.95rem;
  margin: 0 0 1rem;
}

.live-controls {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 0.75rem;
  flex-wrap: wrap;
}

.live-badge {
  display: inline-flex;
  align-items: center;
  gap: 0.45rem;
  padding: 0.3rem 0.9rem;
  border-radius: 30px;
  background: rgba(231, 76, 60, 0.12);
  border: 1px solid rgba(231, 76, 60, 0.4);
  color: #ff6b6b;
  font-size: 0.75rem;
  font-weight: 700;
  letter-spacing: 0.08em;
}

.live-badge.paused {
  background: rgba(138, 148, 176, 0.12);
  border-color: rgba(138, 148, 176, 0.4);
  color: #8a94b0;
}

.pulse-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: #ff6b6b;
  animation: pulse 1.4s ease-in-out infinite;
}

.live-badge.paused .pulse-dot {
  background: #8a94b0;
  animation: none;
}

@keyframes pulse {
  0%, 100% { opacity: 1; transform: scale(1); }
  50% { opacity: 0.35; transform: scale(0.8); }
}

.live-toggle {
  background: #1a2233;
  border: 1px solid #2a3548;
  color: #c7d2e8;
  border-radius: 8px;
  padding: 0.35rem 0.9rem;
  font-size: 0.8rem;
  cursor: pointer;
  transition: all 0.2s;
}

.live-toggle:hover {
  border-color: #667eea;
  color: #e7ecf5;
}

.feed-error {
  color: #ffb347;
  font-size: 0.8rem;
}

.loading {
  text-align: center;
  padding: 3rem;
  color: #6b7a9e;
}

.spinner {
  width: 40px;
  height: 40px;
  border: 3px solid #2a3548;
  border-top-color: #667eea;
  border-radius: 50%;
  margin: 0 auto 1rem;
  animation: spin 1s linear infinite;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

.empty-state {
  text-align: center;
  padding: 3rem 2rem;
  background: #1a1f2e;
  border: 1px solid #2a3548;
  border-radius: 12px;
}

.empty-state h2 { color: #e7ecf5; margin: 0.5rem 0; }
.empty-state p { color: #6b7a9e; margin: 0; }
.empty-icon { font-size: 2.5rem; }

.feed {
  display: flex;
  flex-direction: column;
  gap: 0.85rem;
}

.finding-card {
  background: #1a1f2e;
  border: 1px solid #2a3548;
  border-left-width: 4px;
  border-radius: 12px;
  padding: 0.9rem 1.1rem;
}

.finding-card.drainer { border-left-color: #ff6b6b; }
.finding-card.suspicious { border-left-color: #ffb347; }

.finding-top {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  margin-bottom: 0.6rem;
}

.chain-badge {
  display: inline-flex;
  align-items: center;
  gap: 0.35rem;
  font-size: 0.72rem;
  font-weight: 600;
  color: #8fa1c4;
  background: rgba(138, 148, 176, 0.08);
  border: 1px solid #2a3548;
  padding: 0.2rem 0.55rem;
  border-radius: 6px;
}

.op-badge {
  font-size: 0.72rem;
  font-weight: 700;
  padding: 0.2rem 0.6rem;
  border-radius: 6px;
}

.op-badge.drainer {
  background: rgba(255, 107, 107, 0.12);
  color: #ff6b6b;
}

.op-badge.suspicious {
  background: rgba(255, 179, 71, 0.12);
  color: #ffb347;
}

.amount {
  color: #ff6b6b;
  font-weight: 700;
  font-size: 0.85rem;
  font-family: monospace;
}

.time {
  margin-left: auto;
  color: #4c5a7a;
  font-size: 0.75rem;
}

.parties {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  margin-bottom: 0.6rem;
  flex-wrap: wrap;
}

.party {
  display: flex;
  align-items: center;
  gap: 0.45rem;
}

.party-label {
  color: #6b7a9e;
  font-size: 0.72rem;
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.party-arrow { color: #4c5a7a; }

.addr {
  font-family: monospace;
  font-size: 0.85rem;
  text-decoration: none;
}

.addr.victim { color: #ffb347; }
.addr.hacker { color: #ff6b6b; }
.addr.none { color: #4c5a7a; }
.addr:hover { text-decoration: underline; }

.finding-footer {
  display: flex;
  align-items: center;
  gap: 0.9rem;
  font-size: 0.75rem;
}

.tx-link {
  color: #7ea2ff;
  font-family: monospace;
  text-decoration: none;
}

.tx-link:hover { text-decoration: underline; }

.slot, .source {
  color: #4c5a7a;
}
</style>
