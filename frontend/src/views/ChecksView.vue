<script setup lang="ts">
import { ref, onMounted, inject } from 'vue'

interface RecentCheck {
  id: number
  address: string
  chain: string
  status: string
  checked_at: string
}

const apiBase = inject<string>('apiBase', '')
const wallet = inject<any>('wallet')
const checks = ref<RecentCheck[]>([])
const loading = ref(true)
const error = ref('')

function apiUrl(path: string): string {
  return apiBase + path
}

// Pagination
const currentOffset = ref(0)
const perPage = ref(10)
const total = ref(0)

const totalPages = ref(1)

async function fetchChecks(offset: number = 0) {
  if (!wallet?.authToken.value) {
    error.value = 'Please connect your wallet to view check history'
    loading.value = false
    return
  }

  loading.value = true
  error.value = ''

  try {
    const res = await fetch(apiUrl(`/api/checks?limit=${perPage.value}&offset=${offset}`), {
      headers: { 'Authorization': `Bearer ${wallet.authToken.value}` }
    })

    if (res.status === 401) {
      wallet?.handleUnauthorized?.(res)
      throw new Error('Session expired. Please reconnect your wallet')
    }
    if (!res.ok) {
      throw new Error('Failed to fetch checks')
    }
    
    const data = await res.json()
    checks.value = data.checks || []
    total.value = data.total || 0
    currentOffset.value = data.offset || offset
    totalPages.value = Math.ceil(total.value / perPage.value) || 1
  } catch (err: any) {
    error.value = err.message || 'Failed to load check history'
  } finally {
    loading.value = false
  }
}

function formatDate(dateStr: string) {
  const date = new Date(dateStr)
  return date.toLocaleDateString() + ' ' + date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
}

function formatAddress(addr: string) {
  if (!addr) return ''
  return `${addr.slice(0, 6)}...${addr.slice(-4)}`
}

function getStatusClass(status: string) {
  switch (status) {
    case 'vulnerable':
    case 'danger':
      return 'danger'
    case 'warning':
      return 'warning'
    case 'safe':
    case 'not_found':
      return 'safe'
    default:
      return ''
  }
}

function getStatusIcon(status: string) {
  switch (status) {
    case 'vulnerable':
    case 'danger':
      return '🚨'
    case 'warning':
      return '⚠️'
    case 'safe':
    case 'not_found':
      return '✅'
    default:
      return '❓'
  }
}

function goToPage(page: number) {
  const offset = (page - 1) * perPage.value
  fetchChecks(offset)
}

onMounted(() => {
  fetchChecks()
})
</script>

<template>
  <div class="checks-page">
    <div class="page-header">
      <h1>My Checks</h1>
      <p class="subtitle">Check history for: {{ wallet?.walletAddress.value }}</p>
    </div>

    <div v-if="loading" class="loading">
      <div class="spinner"></div>
      <p>Loading check history...</p>
    </div>

    <div v-else-if="error" class="error-message">
      <p>{{ error }}</p>
      <button @click="fetchChecks(currentOffset)" class="retry-btn">Retry</button>
    </div>

    <div v-else-if="checks.length === 0" class="empty-state">
      <div class="empty-icon">🔍</div>
      <h2>No checks yet</h2>
      <p>Start checking wallet addresses to see your history here.</p>
      <RouterLink to="/" class="btn-primary">Check Address</RouterLink>
    </div>

    <div v-else class="checks-list">
      <div class="checks-count">
        Showing {{ checks.length }} of {{ total }} checks
      </div>
      
      <table class="checks-table">
        <thead>
          <tr>
            <th>Date</th>
            <th>Address</th>
            <th>Chain</th>
            <th>Status</th>
            <th>Report</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="check in checks" :key="check.id">
            <td class="date-cell">{{ formatDate(check.checked_at) }}</td>
            <td class="address-cell">
              <a :href="`https://solscan.io/account/${check.address}`" target="_blank" class="address-link">
                {{ formatAddress(check.address) }}
              </a>
            </td>
            <td class="chain-cell">{{ check.chain.toUpperCase() }}</td>
            <td>
              <span :class="['status-badge', getStatusClass(check.status)]">
                {{ getStatusIcon(check.status) }} {{ check.status }}
              </span>
            </td>
            <td>
              <RouterLink
                :to="{ path: '/report', query: { address: check.address, chain: check.chain } }"
                class="report-btn"
                title="Open report"
              >📄</RouterLink>
            </td>
          </tr>
        </tbody>
      </table>

      <!-- Pagination -->
      <div v-if="totalPages > 1" class="pagination">
        <button 
          class="page-btn" 
          :disabled="currentOffset === 0" 
          @click="fetchChecks(currentOffset - perPage)"
        >
          ← Prev
        </button>
        
        <div class="page-numbers">
          <button 
            v-for="page in totalPages" 
            :key="page"
            class="page-num"
            :class="{ active: (currentOffset / perPage) + 1 === page }"
            @click="goToPage(page)"
          >
            {{ page }}
          </button>
        </div>
        
        <button 
          class="page-btn" 
          :disabled="currentOffset + perPage >= total" 
          @click="fetchChecks(currentOffset + perPage)"
        >
          Next →
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.checks-page {
  max-width: 900px;
  margin: 0 auto;
  padding: 2rem 1rem;
}

.page-header {
  text-align: center;
  margin-bottom: 2rem;
}

.page-header h1 {
  font-size: 2rem;
  color: #e7ecf5;
  margin-bottom: 0.5rem;
}

.subtitle {
  color: #6b7a9e;
  font-size: 0.9rem;
  word-break: break-all;
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

.error-message {
  text-align: center;
  padding: 2rem;
  background: #2a1a1a;
  border: 1px solid #ff6b6b40;
  border-radius: 12px;
  color: #ff6b6b;
}

.empty-state {
  text-align: center;
  padding: 4rem 2rem;
  background: #1a1f2e;
  border: 1px solid #2a3548;
  border-radius: 12px;
}

.empty-icon {
  font-size: 3rem;
  margin-bottom: 1rem;
}

.empty-state h2 {
  color: #e7ecf5;
  margin-bottom: 0.5rem;
}

.empty-state p {
  color: #6b7a9e;
  margin-bottom: 1.5rem;
}

.btn-primary {
  display: inline-block;
  padding: 0.75rem 1.5rem;
  background: #667eea;
  color: white;
  text-decoration: none;
  border-radius: 8px;
  font-weight: 500;
  transition: background 0.2s;
}

.btn-primary:hover {
  background: #5568d3;
}

.checks-table {
  width: 100%;
  border-collapse: collapse;
  background: #1a1f2e;
  border-radius: 12px;
  overflow: hidden;
}

.checks-table th,
.checks-table td {
  padding: 1rem;
  text-align: left;
}

.checks-table th {
  background: #151a24;
  color: #6b7a9e;
  font-weight: 600;
  font-size: 0.85rem;
  text-transform: uppercase;
}

.checks-table td {
  border-bottom: 1px solid #2a3548;
  color: #e7ecf5;
}

.checks-table tr:last-child td {
  border-bottom: none;
}

.date-cell {
  font-size: 0.85rem;
  white-space: nowrap;
  color: #6b7a9e;
}

.address-link {
  color: #667eea;
  text-decoration: none;
  font-family: monospace;
}

.address-link:hover {
  text-decoration: underline;
}

.chain-cell {
  font-weight: 500;
  text-transform: uppercase;
  font-size: 0.85rem;
}

.status-badge {
  display: inline-block;
  padding: 0.25rem 0.6rem;
  border-radius: 6px;
  font-size: 0.75rem;
  font-weight: 600;
  text-transform: uppercase;
}

.status-badge.danger {
  background: rgba(255, 107, 107, 0.2);
  color: #ff6b6b;
}

.status-badge.warning {
  background: rgba(255, 193, 7, 0.2);
  color: #ffc107;
}

.status-badge.safe {
  background: rgba(75, 201, 160, 0.2);
  color: #4bc9a0;
}

.checks-count {
  font-size: 0.85rem;
  color: #6b7a9e;
  margin-bottom: 1rem;
  text-align: right;
}

.pagination {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 0.5rem;
  margin-top: 1.5rem;
  padding-top: 1rem;
  border-top: 1px solid #2a3548;
}

.page-btn {
  padding: 0.5rem 1rem;
  background: #151a24;
  border: 1px solid #2a3548;
  border-radius: 8px;
  color: #98a8ce;
  cursor: pointer;
  transition: all 0.2s;
}

.page-btn:hover:not(:disabled) {
  border-color: #667eea;
  color: #667eea;
}

.page-btn:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

.page-numbers {
  display: flex;
  gap: 0.3rem;
}

.page-num {
  min-width: 36px;
  height: 36px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: #151a24;
  border: 1px solid #2a3548;
  border-radius: 6px;
  color: #98a8ce;
  cursor: pointer;
  transition: all 0.2s;
}

.page-num:hover {
  border-color: #667eea;
  color: #667eea;
}

.page-num.active {
  background: #667eea;
  border-color: #667eea;
  color: white;
}

.retry-btn {
  margin-top: 1rem;
  padding: 0.5rem 1rem;
  background: #667eea;
  border: none;
  border-radius: 6px;
  color: white;
  cursor: pointer;
}

.retry-btn:hover {
  background: #5568d3;
}

.report-btn {
  display: inline-flex;
  padding: 0.3rem 0.5rem;
  background: #151a24;
  border: 1px solid #2a3548;
  border-radius: 6px;
  text-decoration: none;
  font-size: 0.85rem;
  transition: all 0.2s;
}

.report-btn:hover {
  border-color: #667eea;
}
</style>
