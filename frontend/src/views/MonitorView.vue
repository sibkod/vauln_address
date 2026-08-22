<script setup lang="ts">
import { ref, inject, onMounted, onUnmounted } from 'vue'

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
  source: string
  created_at: string
}

interface Stats {
  total_findings: number
  drainer_count: number
  suspicious_count: number
  victim_count: number
  hacker_count: number
  stolen_sol: number
}

const apiBase = inject<string>('apiBase', '')

const findings = ref<Finding[]>([])
const stats = ref<Stats | null>(null)
const loading = ref(true)
const loadingMore = ref(false)
const hasMore = ref(true)
const live = ref(true)
const lastError = ref('')

const POLL_INTERVAL = 4000
const PAGE_SIZE = 50
let pollTimer: number | null = null

async function fetchStats() {
  try {
    const res = await fetch(`${apiBase}/api/monitor/stats`)
    if (res.ok) stats.value = await res.json()
  } catch { /* keep old stats */ }
}

async function fetchLatest() {
  const res = await fetch(`${apiBase}/api/monitor/findings?limit=${PAGE_SIZE}`)
  if (!res.ok) throw new Error('findings request failed')
  const data = await res.json()
  findings.value = data.findings || []
  hasMore.value = findings.value.length >= PAGE_SIZE
}

async function pollNew() {
  if (!live.value) return
  try {
    const maxId = findings.value.length ? findings.value[0].id : 0
    const res = await fetch(`${apiBase}/api/monitor/findings?after_id=${maxId}&limit=100`)
    if (res.ok) {
      const data = await res.json()
      const fresh: Finding[] = data.findings || []
      if (fresh.length) {
        // fresh rows come ascending; newest first in the feed
        findings.value = [...fresh.reverse(), ...findings.value].slice(0, 500)
      }
    }
    lastError.value = ''
    fetchStats()
  } catch {
    lastError.value = 'Connection lost — retrying…'
  }
}

async function loadMore() {
  if (loadingMore.value || !findings.value.length || !hasMore.value) return
  loadingMore.value = true
  try {
    const minId = findings.value[findings.value.length - 1].id
    const res = await fetch(`${apiBase}/api/monitor/findings?before_id=${minId}&limit=${PAGE_SIZE}`)
    if (res.ok) {
      const data = await res.json()
      const older: Finding[] = data.findings || []
      if (older.length < PAGE_SIZE) hasMore.value = false
      const seen = new Set(findings.value.map(f => f.id))
      findings.value = [...findings.value, ...older.filter(f => !seen.has(f.id))]
    }
  } catch { /* retry on next click */ } finally {
    loadingMore.value = false
  }
}

function toggleLive() {
  live.value = !live.value
}

function shortAddr(addr?: string): string {
  if (!addr) return '—'
  return addr.length <= 16 ? addr : `${addr.slice(0, 6)}…${addr.slice(-6)}`
}

function solscanTx(sig: string): string {
  return `https://solscan.io/tx/${sig}`
}

function solscanAccount(addr: string): string {
  return `https://solscan.io/account/${addr}`
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
    await Promise.all([fetchLatest(), fetchStats()])
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
      <p class="subtitle">Real-time detections from our Solana drainer scanner</p>
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

    <div v-if="stats" class="stats-grid">
      <div class="stat-card">
        <div class="stat-value">{{ stats.total_findings }}</div>
        <div class="stat-label">Findings</div>
      </div>
      <div class="stat-card danger">
        <div class="stat-value">{{ stats.drainer_count }}</div>
        <div class="stat-label">Drainer TXs</div>
      </div>
      <div class="stat-card warn">
        <div class="stat-value">{{ stats.suspicious_count }}</div>
        <div class="stat-label">Suspicious</div>
      </div>
      <div class="stat-card victim">
        <div class="stat-value">{{ stats.victim_count }}</div>
        <div class="stat-label">Victims</div>
      </div>
      <div class="stat-card hacker">
        <div class="stat-value">{{ stats.hacker_count }}</div>
        <div class="stat-label">Hackers</div>
      </div>
      <div class="stat-card sol">
        <div class="stat-value">{{ stats.stolen_sol.toFixed(2) }} ◎</div>
        <div class="stat-label">SOL swept</div>
      </div>
    </div>

    <div v-if="loading" class="loading">
      <div class="spinner"></div>
      <p>Connecting to the scanner feed…</p>
    </div>

    <div v-else-if="!findings.length" class="empty-state">
      <div class="empty-icon">🛰️</div>
      <h2>No detections yet</h2>
      <p>The scanner is watching the chain. New drainer detections will appear here in real time.</p>
    </div>

    <div v-else class="feed">
      <div v-for="f in findings" :key="f.id" class="finding-card" :class="f.verdict === 'DRAINER' ? 'drainer' : 'suspicious'">
        <div class="finding-top">
          <span class="verdict-badge" :class="f.verdict === 'DRAINER' ? 'drainer' : 'suspicious'">
            {{ f.verdict === 'DRAINER' ? '💀 DRAINER' : '⚠️ SUSPICIOUS' }}
          </span>
          <span v-if="f.amount_sol > 0" class="amount">-{{ f.amount_sol.toFixed(4) }} SOL</span>
          <span class="time">{{ timeAgo(f.created_at) }}</span>
        </div>

        <div class="parties">
          <div class="party">
            <span class="party-label">Victim</span>
            <a v-if="f.victim_address" :href="solscanAccount(f.victim_address)" target="_blank" rel="noopener" class="addr victim">
              {{ shortAddr(f.victim_address) }}
            </a>
            <span v-else class="addr none">—</span>
          </div>
          <span class="party-arrow">→</span>
          <div class="party">
            <span class="party-label">Hacker</span>
            <a v-if="f.hacker_address" :href="solscanAccount(f.hacker_address)" target="_blank" rel="noopener" class="addr hacker">
              {{ shortAddr(f.hacker_address) }}
            </a>
            <span v-else class="addr none">—</span>
          </div>
        </div>

        <div class="indicator-chips">
          <span v-for="ind in f.indicators" :key="ind" class="chip">{{ ind }}</span>
        </div>

        <div class="finding-footer">
          <a :href="solscanTx(f.signature)" target="_blank" rel="noopener" class="tx-link">
            {{ shortAddr(f.signature) }} ↗
          </a>
          <span class="slot">slot {{ f.slot }}</span>
          <span class="source">{{ f.source }}</span>
        </div>
      </div>

      <div v-if="hasMore" class="load-more">
        <button class="load-more-btn" :disabled="loadingMore" @click="loadMore">
          {{ loadingMore ? 'Loading…' : 'Load older events' }}
        </button>
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

.stats-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(140px, 1fr));
  gap: 0.75rem;
  margin-bottom: 1.5rem;
}

.stat-card {
  background: #1a1f2e;
  border: 1px solid #2a3548;
  border-radius: 12px;
  padding: 0.9rem;
  text-align: center;
}

.stat-card.danger { border-color: rgba(255, 107, 107, 0.35); }
.stat-card.warn { border-color: rgba(255, 179, 71, 0.35); }
.stat-card.victim { border-color: rgba(255, 179, 71, 0.35); }
.stat-card.hacker { border-color: rgba(255, 107, 107, 0.35); }
.stat-card.sol { border-color: rgba(102, 126, 234, 0.4); }

.stat-value {
  font-size: 1.4rem;
  font-weight: 700;
  color: #e7ecf5;
}

.stat-card.danger .stat-value, .stat-card.hacker .stat-value { color: #ff6b6b; }
.stat-card.warn .stat-value, .stat-card.victim .stat-value { color: #ffb347; }
.stat-card.sol .stat-value { color: #7ea2ff; }

.stat-label {
  color: #6b7a9e;
  font-size: 0.72rem;
  text-transform: uppercase;
  letter-spacing: 0.06em;
  margin-top: 0.2rem;
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

.load-more {
  display: flex;
  justify-content: center;
  padding-top: 0.5rem;
}

.load-more-btn {
  padding: 0.6rem 1.4rem;
  border-radius: 8px;
  border: 1px solid #2a3548;
  background: #1a1f2e;
  color: #98a8ce;
  font-size: 0.85rem;
  cursor: pointer;
}

.load-more-btn:hover:not(:disabled) { border-color: #3d4c6e; color: #e7ecf5; }
.load-more-btn:disabled { opacity: 0.6; cursor: default; }

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

.verdict-badge {
  font-size: 0.72rem;
  font-weight: 700;
  padding: 0.2rem 0.6rem;
  border-radius: 6px;
}

.verdict-badge.drainer {
  background: rgba(255, 107, 107, 0.12);
  color: #ff6b6b;
}

.verdict-badge.suspicious {
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

.indicator-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 0.35rem;
  margin-bottom: 0.6rem;
}

.chip {
  background: #12182a;
  border: 1px solid #2a3548;
  color: #8fa3d0;
  border-radius: 6px;
  padding: 0.15rem 0.5rem;
  font-size: 0.68rem;
  font-family: monospace;
}

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
