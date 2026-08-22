<script setup lang="ts">
import { ref, computed, inject, onMounted, onUnmounted } from 'vue'
import { useRoute } from 'vue-router'
import TxTreeNode, { type TxNode } from '../components/TxTreeNode.vue'

interface LeakInfo {
  key_type: string
  source: string
  discovered_at: string
}

interface StatusEvidence {
  code: string
  title: string
  description: string
  tx_signature?: string
  counterparty?: string
  amount_sol?: number
  detected_at?: string
}

interface Report {
  address: string
  chain: string
  found: boolean
  status: string
  reason?: string
  details: string
  source?: string
  has_pk: boolean
  has_seed: boolean
  leaks?: LeakInfo[]
  evidence?: StatusEvidence[]
  transactions?: TxNode
  expires_at?: string
  public?: boolean
  created_at: string
}

const route = useRoute()
const apiBase = inject<string>('apiBase', '')
const wallet = inject<any>('wallet')

// Shared mode: opened via /report/<uuid>; owner mode: /report?address&chain
const shareId = computed(() => String(route.params.id || ''))
const address = computed(() => shareId.value ? (report.value?.address || '') : String(route.query.address || ''))
const chain = computed(() => shareId.value ? (report.value?.chain || '') : String(route.query.chain || ''))

const loading = ref(true)
const report = ref<Report | null>(null)
const error = ref('')
const errorCode = ref('')

const isConnected = computed(() => wallet?.connected?.value || false)

// Countdown for anonymous expiry notice
const expiresIn = ref('')
let timer: number | null = null

function updateCountdown() {
  if (!report.value?.expires_at) return
  const diff = new Date(report.value.expires_at).getTime() - Date.now()
  if (diff <= 0) {
    expiresIn.value = 'expired'
    return
  }
  const h = Math.floor(diff / 3600000)
  const m = Math.floor((diff % 3600000) / 60000)
  expiresIn.value = h > 0 ? `${h}h ${m}m` : `${m}m`
}

async function fetchReport() {
  loading.value = true
  error.value = ''
  errorCode.value = ''

  const headers: Record<string, string> = {}
  if (wallet?.authToken?.value) {
    headers['Authorization'] = `Bearer ${wallet.authToken.value}`
  }

  try {
    const url = shareId.value
      ? `${apiBase}/api/report/shared/${encodeURIComponent(shareId.value)}`
      : `${apiBase}/api/report?address=${encodeURIComponent(address.value)}&chain=${encodeURIComponent(chain.value)}`
    const res = await fetch(url, { headers })
    const data = await res.json()
    if (!res.ok) {
      errorCode.value = data.code || 'ERROR'
      error.value = data.details || data.error || 'Failed to load report'
      report.value = null
      return
    }
    report.value = data
    updateCountdown()
    if (timer) clearInterval(timer)
    if (data.expires_at) {
      timer = window.setInterval(updateCountdown, 30000)
    }
  } catch (e) {
    error.value = 'Request failed. Is the backend running?'
    errorCode.value = 'NETWORK'
  } finally {
    loading.value = false
  }
}

// ---- Public sharing (authenticated users only) ----

const sharing = ref(false)
const shareError = ref('')
const shareUrl = ref('')
const copied = ref(false)

const canMakePublic = computed(() =>
  isConnected.value && !shareId.value && report.value && !report.value.public && !shareUrl.value
)

const publicUrl = computed(() =>
  shareUrl.value || (report.value?.public ? window.location.href : '')
)

const shareText = computed(() =>
  `Wallet security report (${chain.value.toUpperCase()}): ${address.value} — status ${(report.value?.status || '').toUpperCase()}`
)

async function makePublic() {
  if (!wallet?.authToken?.value) return
  sharing.value = true
  shareError.value = ''
  try {
    const res = await fetch(`${apiBase}/api/report/share`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${wallet.authToken.value}`
      },
      body: JSON.stringify({ address: address.value, chain: chain.value })
    })
    const data = await res.json()
    if (!res.ok) {
      shareError.value = data.details || data.error || 'Failed to make the report public'
      return
    }
    shareUrl.value = window.location.origin + data.share_url
    // The report page now lives at the public /report/<uuid> URL
    window.history.replaceState(null, '', data.share_url)
  } catch {
    shareError.value = 'Request failed. Is the backend running?'
  } finally {
    sharing.value = false
  }
}

function openShare(network: 'x' | 'telegram') {
  const url = encodeURIComponent(publicUrl.value)
  const text = encodeURIComponent(shareText.value)
  const target = network === 'x'
    ? `https://twitter.com/intent/tweet?text=${text}&url=${url}`
    : `https://t.me/share/url?url=${url}&text=${text}`
  window.open(target, '_blank', 'noopener,width=600,height=450')
}

async function copyLink() {
  try {
    await navigator.clipboard.writeText(publicUrl.value)
    copied.value = true
    setTimeout(() => { copied.value = false }, 2000)
  } catch {
    shareError.value = 'Copy failed — select the link manually'
  }
}

function connectWallet() {
  wallet?.openWalletModal?.()
}

onMounted(fetchReport)
onUnmounted(() => { if (timer) clearInterval(timer) })

const statusMeta: Record<string, { icon: string; label: string; cls: string }> = {
  hacked: { icon: '🚨', label: 'HACKED', cls: 'danger' },
  hacker: { icon: '💀', label: 'HACKER', cls: 'danger' },
  vulnerable: { icon: '⚠️', label: 'VULNERABLE', cls: 'warn' },
  drained: { icon: '🏴', label: 'DRAINED', cls: 'danger' },
  phishing: { icon: '🎣', label: 'PHISHING', cls: 'danger' },
  scam: { icon: '🕳️', label: 'SCAM', cls: 'danger' },
  mixer: { icon: '🌀', label: 'MIXER', cls: 'warn' },
  sanctioned: { icon: '⛔', label: 'SANCTIONED', cls: 'danger' },
  suspicious: { icon: '🔍', label: 'SUSPICIOUS', cls: 'warn' },
  frozen: { icon: '🧊', label: 'FROZEN', cls: 'warn' },
  exchange: { icon: '🏦', label: 'EXCHANGE', cls: 'safe' },
  safe: { icon: '✅', label: 'SAFE', cls: 'safe' },
  not_found: { icon: '✅', label: 'SAFE', cls: 'safe' }
}

const meta = computed(() => statusMeta[report.value?.status || ''] || { icon: '❓', label: (report.value?.status || 'unknown').toUpperCase(), cls: '' })

function exposureText(r: Report): string[] {
  const out: string[] = []
  if (r.has_pk) out.push('private key')
  if (r.has_seed) out.push('mnemonic (seed) phrase')
  for (const leak of r.leaks || []) {
    if (leak.key_type === 'private_key' && !r.has_pk) out.push('private key')
    if (leak.key_type === 'seed' && !r.has_seed) out.push('mnemonic (seed) phrase')
  }
  return out
}

// ---- Evidence chain ----

const evidenceMeta: Record<string, { icon: string; cls: string }> = {
  registry: { icon: '📋', cls: 'registry' },
  key_leak: { icon: '🔑', cls: 'leak' },
  P1_ACCOUNT_TAKEOVER: { icon: '🎯', cls: 'scanner' },
  P2_FULL_BALANCE_SWEEP: { icon: '🧹', cls: 'scanner' },
  P3_UNKNOWN_PROGRAM: { icon: '❓', cls: 'scanner' },
  P4_CONTROL_ACCOUNT: { icon: '🕹️', cls: 'scanner' },
  P5_KNOWN_DRAINER_PROGRAM: { icon: '💀', cls: 'scanner' },
  P6_TOKEN_SWEEP: { icon: '🪙', cls: 'scanner' }
}

function evidenceIcon(code: string): string {
  return evidenceMeta[code]?.icon || '🔎'
}

function evidenceCls(code: string): string {
  return evidenceMeta[code]?.cls || 'scanner'
}

function shortAddr(addr: string): string {
  return addr.length <= 16 ? addr : `${addr.slice(0, 6)}…${addr.slice(-6)}`
}

function solscanTx(sig: string): string {
  return `https://solscan.io/tx/${sig}`
}

function evidenceDate(e: StatusEvidence): string {
  if (!e.detected_at) return ''
  const d = new Date(e.detected_at)
  return isNaN(d.getTime()) ? '' : d.toLocaleDateString()
}
</script>

<template>
  <div class="report-page">
    <div class="report-header">
      <h1>Security Report</h1>
      <div class="report-target" v-if="report || !shareId">
        <span class="chain-badge">{{ chain.toUpperCase() }}</span>
        <span class="full-address">{{ address }}</span>
      </div>
      <span v-if="report && report.public" class="public-badge">🌐 Public report</span>
      <RouterLink v-if="report && report.expires_at" to="/pricing" class="keep-link">
        ⏳ Anonymous report expires in {{ expiresIn }} — connect a wallet to keep it
      </RouterLink>
    </div>

    <div v-if="loading" class="loading">
      <div class="spinner"></div>
      <p>Loading report…</p>
    </div>

    <div v-else-if="error" class="error-box">
      <div class="empty-icon">⛔</div>
      <h2 v-if="errorCode === 'REPORT_EXPIRED'">Report expired</h2>
      <h2 v-else-if="errorCode === 'NOT_FOUND'">Not found</h2>
      <h2 v-else>Report unavailable</h2>
      <p>{{ error }}</p>
      <RouterLink to="/" class="btn-primary">Check address</RouterLink>
    </div>

    <div v-else-if="report" class="report-body">
      <!-- Status banner -->
      <div class="status-banner" :class="meta.cls">
        <span class="status-icon">{{ meta.icon }}</span>
        <div>
          <div class="status-label">{{ meta.label }}</div>
          <div class="status-sub">Found in database · {{ chain.toUpperCase() }}</div>
        </div>
      </div>

      <!-- Public sharing -->
      <div v-if="publicUrl" class="section share-section">
        <h2>Public report</h2>
        <p class="share-hint">Anyone with this link can view the report</p>
        <div class="share-link-row">
          <input class="share-link" readonly :value="publicUrl" @focus="($event.target as HTMLInputElement).select()" />
          <button class="share-btn" @click="copyLink">{{ copied ? '✓ Copied' : 'Copy' }}</button>
        </div>
        <div class="share-socials">
          <button class="share-btn social x" @click="openShare('x')">𝕏 Share on X</button>
          <button class="share-btn social tg" @click="openShare('telegram')">✈ Share on Telegram</button>
        </div>
      </div>
      <div v-else-if="canMakePublic" class="section share-section">
        <button class="share-make" :disabled="sharing" @click="makePublic">
          {{ sharing ? 'Making public…' : '🔗 Make public & share' }}
        </button>
        <p v-if="shareError" class="share-error">{{ shareError }}</p>
      </div>
      <div v-else-if="!isConnected && !shareId" class="section share-section">
        <p class="share-hint">
          🔒 Anonymous reports can't be shared.
          <button class="link-btn" @click="connectWallet">Connect a wallet</button>
          to make this report public and share it on social networks.
        </p>
      </div>

      <!-- Reason & details -->
      <div class="section">
        <h2>Why it was flagged</h2>
        <p v-if="report.reason" class="reason">Reason: {{ report.reason }}<span v-if="report.source"> (source: {{ report.source }})</span></p>
        <p class="details">{{ report.details }}</p>
        <div v-if="exposureText(report).length" class="exposure">
          Exposed data: {{ exposureText(report).join(' and ') }} — publicly available
        </div>
      </div>

      <!-- Evidence chain -->
      <div v-if="report.evidence && report.evidence.length" class="section">
        <h2>Evidence chain</h2>
        <p class="tree-hint">Why this wallet has the {{ meta.label }} status — step by step</p>
        <ol class="evidence-chain">
          <li v-for="(e, i) in report.evidence" :key="i" class="evidence-item" :class="evidenceCls(e.code)">
            <span class="evidence-icon">{{ evidenceIcon(e.code) }}</span>
            <div class="evidence-body">
              <div class="evidence-title">
                {{ e.title }}
                <span v-if="evidenceDate(e)" class="evidence-date">{{ evidenceDate(e) }}</span>
              </div>
              <div class="evidence-desc">{{ e.description }}</div>
              <div class="evidence-meta">
                <a v-if="e.tx_signature" :href="solscanTx(e.tx_signature)" target="_blank" rel="noopener" class="evidence-link">
                  tx {{ shortAddr(e.tx_signature) }} ↗
                </a>
                <span v-if="e.counterparty" class="evidence-counterparty">with {{ shortAddr(e.counterparty) }}</span>
                <span v-if="e.amount_sol" class="evidence-amount">-{{ e.amount_sol.toFixed(4) }} SOL</span>
              </div>
            </div>
          </li>
        </ol>
      </div>

      <!-- Leaks -->
      <div v-if="report.leaks && report.leaks.length" class="section">
        <h2>Leaky records</h2>
        <table class="leaks-table">
          <thead>
            <tr><th>Type</th><th>Source</th><th>Discovered</th></tr>
          </thead>
          <tbody>
            <tr v-for="leak in report.leaks" :key="leak.key_type + leak.discovered_at">
              <td>{{ leak.key_type === 'seed' ? 'mnemonic phrase' : 'private key' }}</td>
              <td>{{ leak.source || 'unknown' }}</td>
              <td>{{ new Date(leak.discovered_at).toLocaleDateString() }}</td>
            </tr>
          </tbody>
        </table>
      </div>

      <!-- Transaction tree -->
      <div v-if="report.transactions" class="section">
        <h2>Outgoing transactions</h2>
        <p class="tree-hint">Where the funds went and the status of each wallet</p>
        <TxTreeNode :node="report.transactions" />
      </div>

      <div class="section footer-note">
        Report created {{ new Date(report.created_at).toLocaleString() }}
      </div>
    </div>
  </div>
</template>

<style scoped>
.report-page {
  max-width: 900px;
  margin: 0 auto;
  padding: 2rem 1rem;
}

.report-header {
  text-align: center;
  margin-bottom: 2rem;
}

.report-header h1 {
  font-size: 1.8rem;
  color: #e7ecf5;
  margin-bottom: 0.75rem;
}

.report-target {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 0.6rem;
  flex-wrap: wrap;
}

.chain-badge {
  padding: 0.2rem 0.6rem;
  background: #667eea;
  color: white;
  border-radius: 6px;
  font-size: 0.75rem;
  font-weight: 700;
}

.full-address {
  color: #98a8ce;
  font-family: monospace;
  font-size: 0.85rem;
  word-break: break-all;
}

.keep-link {
  display: inline-block;
  margin-top: 0.75rem;
  color: #ffb347;
  font-size: 0.8rem;
  text-decoration: none;
}

.public-badge {
  display: inline-block;
  margin-top: 0.75rem;
  padding: 0.25rem 0.9rem;
  background: rgba(39, 174, 96, 0.12);
  border: 1px solid rgba(39, 174, 96, 0.35);
  color: #2ecc71;
  border-radius: 30px;
  font-size: 0.8rem;
}

.share-section {
  text-align: center;
}

.share-hint {
  color: #8a94b0;
  font-size: 0.85rem;
  margin: 0.25rem 0 0.75rem;
}

.share-link-row {
  display: flex;
  gap: 0.5rem;
}

.share-link {
  flex: 1;
  min-width: 0;
  background: #0c111b;
  border: 1px solid #2a3548;
  border-radius: 8px;
  color: #7ea2ff;
  padding: 0.55rem 0.75rem;
  font-size: 0.8rem;
  font-family: monospace;
}

.share-btn {
  background: #1a2233;
  border: 1px solid #2a3548;
  color: #c7d2e8;
  border-radius: 8px;
  padding: 0.55rem 1rem;
  font-size: 0.85rem;
  cursor: pointer;
  transition: all 0.2s;
  white-space: nowrap;
}

.share-btn:hover {
  border-color: #667eea;
  color: #e7ecf5;
}

.share-socials {
  display: flex;
  gap: 0.6rem;
  justify-content: center;
  margin-top: 0.75rem;
}

.share-btn.social.tg:hover {
  border-color: #229ed9;
  color: #229ed9;
}

.share-make {
  background: linear-gradient(135deg, #3b5fcf, #667eea);
  border: none;
  color: #fff;
  border-radius: 10px;
  padding: 0.7rem 1.6rem;
  font-size: 0.95rem;
  cursor: pointer;
  transition: all 0.2s;
}

.share-make:hover:not(:disabled) {
  filter: brightness(1.1);
}

.share-make:disabled {
  opacity: 0.6;
  cursor: wait;
}

.share-error {
  color: #e74c3c;
  font-size: 0.8rem;
  margin-top: 0.5rem;
}

.link-btn {
  background: none;
  border: none;
  color: #7ea2ff;
  font-size: inherit;
  cursor: pointer;
  text-decoration: underline;
  padding: 0;
}

.link-btn:hover {
  color: #a8bfff;
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

.error-box {
  text-align: center;
  padding: 3rem 2rem;
  background: #1a1f2e;
  border: 1px solid #2a3548;
  border-radius: 12px;
}

.error-box h2 {
  color: #e7ecf5;
  margin-bottom: 0.5rem;
}

.error-box p {
  color: #6b7a9e;
  margin-bottom: 1.25rem;
}

.empty-icon {
  font-size: 2.5rem;
  margin-bottom: 0.75rem;
}

.btn-primary {
  display: inline-block;
  padding: 0.7rem 1.4rem;
  background: #667eea;
  color: white;
  text-decoration: none;
  border-radius: 8px;
  font-weight: 500;
}

.status-banner {
  display: flex;
  align-items: center;
  gap: 1rem;
  padding: 1.25rem;
  border-radius: 12px;
  margin-bottom: 1.5rem;
  background: #1a1f2e;
  border: 1px solid #2a3548;
}

.status-banner.danger {
  background: rgba(255, 107, 107, 0.08);
  border-color: rgba(255, 107, 107, 0.4);
}

.status-banner.warn {
  background: rgba(255, 179, 71, 0.08);
  border-color: rgba(255, 179, 71, 0.4);
}

.status-banner.safe {
  background: rgba(75, 201, 160, 0.08);
  border-color: rgba(75, 201, 160, 0.4);
}

.status-icon {
  font-size: 2rem;
}

.status-label {
  font-size: 1.25rem;
  font-weight: 700;
  color: #e7ecf5;
}

.status-sub {
  color: #6b7a9e;
  font-size: 0.8rem;
}

.section {
  background: #1a1f2e;
  border: 1px solid #2a3548;
  border-radius: 12px;
  padding: 1.25rem;
  margin-bottom: 1.25rem;
}

.section h2 {
  font-size: 1rem;
  color: #e7ecf5;
  margin: 0 0 0.75rem;
}

.reason {
  color: #ffb347;
  font-size: 0.9rem;
  margin: 0 0 0.5rem;
}

.details {
  color: #98a8ce;
  font-size: 0.9rem;
  line-height: 1.5;
  margin: 0;
}

.exposure {
  margin-top: 0.75rem;
  padding: 0.6rem 0.75rem;
  background: rgba(255, 107, 107, 0.1);
  border: 1px solid rgba(255, 107, 107, 0.3);
  border-radius: 8px;
  color: #ff6b6b;
  font-size: 0.85rem;
}

.leaks-table {
  width: 100%;
  border-collapse: collapse;
}

.leaks-table th,
.leaks-table td {
  padding: 0.6rem;
  text-align: left;
  border-bottom: 1px solid #2a3548;
  color: #98a8ce;
  font-size: 0.85rem;
}

.leaks-table th {
  color: #6b7a9e;
  text-transform: uppercase;
  font-size: 0.75rem;
}

.tree-hint {
  color: #6b7a9e;
  font-size: 0.8rem;
  margin: 0 0 0.75rem;
}

.evidence-chain {
  list-style: none;
  margin: 0;
  padding: 0;
  position: relative;
}

.evidence-chain::before {
  content: '';
  position: absolute;
  left: 15px;
  top: 8px;
  bottom: 8px;
  width: 2px;
  background: #2a3548;
}

.evidence-item {
  position: relative;
  display: flex;
  gap: 0.85rem;
  padding: 0.65rem 0;
}

.evidence-icon {
  position: relative;
  z-index: 1;
  width: 32px;
  height: 32px;
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  background: #12182a;
  border: 1px solid #2a3548;
  border-radius: 50%;
  font-size: 0.95rem;
}

.evidence-item.leak .evidence-icon {
  border-color: rgba(255, 107, 107, 0.5);
}

.evidence-item.scanner .evidence-icon {
  border-color: rgba(255, 179, 71, 0.5);
}

.evidence-item.registry .evidence-icon {
  border-color: rgba(102, 126, 234, 0.5);
}

.evidence-body {
  min-width: 0;
}

.evidence-title {
  color: #e7ecf5;
  font-size: 0.9rem;
  font-weight: 600;
}

.evidence-date {
  color: #4c5a7a;
  font-size: 0.75rem;
  font-weight: 400;
  margin-left: 0.5rem;
}

.evidence-desc {
  color: #98a8ce;
  font-size: 0.82rem;
  line-height: 1.45;
  margin-top: 0.15rem;
}

.evidence-meta {
  display: flex;
  align-items: center;
  gap: 0.85rem;
  margin-top: 0.3rem;
  font-size: 0.75rem;
  flex-wrap: wrap;
}

.evidence-link {
  color: #7ea2ff;
  font-family: monospace;
  text-decoration: none;
}

.evidence-link:hover {
  text-decoration: underline;
}

.evidence-counterparty {
  color: #6b7a9e;
  font-family: monospace;
}

.evidence-amount {
  color: #ff6b6b;
  font-family: monospace;
  font-weight: 700;
}

.footer-note {
  color: #4c5a7a;
  font-size: 0.75rem;
  padding: 0.75rem;
}
</style>
