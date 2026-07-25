<script setup lang="ts">
import { ref, onMounted, inject } from 'vue'

interface Order {
  order_uuid: string
  wallet_address: string
  chain: string
  checks_count: number
  total_usd: number
  currency: string
  token_amount: number
  payment_address: string
  status: string
  tx_hash: string
  created_at: string
  completed_at: string | null
}

const wallet = inject<any>('wallet')
const orders = ref<Order[]>([])
const loading = ref(true)
const error = ref('')

async function fetchPurchases() {
  if (!wallet?.authToken.value) {
    error.value = 'Please connect your wallet to view purchase history'
    loading.value = false
    return
  }

  try {
    const res = await fetch('/api/user/purchases?limit=100', {
      headers: { 'Authorization': `Bearer ${wallet.authToken.value}` }
    })
    
    if (!res.ok) {
      throw new Error('Failed to fetch purchases')
    }
    
    const data = await res.json()
    orders.value = data.orders || []
  } catch (err: any) {
    error.value = err.message || 'Failed to load purchase history'
  } finally {
    loading.value = false
  }
}

function formatDate(dateStr: string) {
  const date = new Date(dateStr)
  return date.toLocaleDateString() + ' ' + date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
}

function formatAmount(order: Order) {
  if (order.currency === 'solana' && order.token_amount) {
    return `${order.token_amount.toFixed(4)} SOL`
  }
  return `$${order.total_usd.toFixed(2)}`
}

onMounted(() => {
  fetchPurchases()
})
</script>

<template>
  <div class="purchases-page">
    <div class="page-header">
      <h1>Purchase History</h1>
      <p class="subtitle">Your wallet: {{ wallet?.walletAddress.value }}</p>
    </div>

    <div v-if="loading" class="loading">
      <div class="spinner"></div>
      <p>Loading purchase history...</p>
    </div>

    <div v-else-if="error" class="error-message">
      <p>{{ error }}</p>
    </div>

    <div v-else-if="orders.length === 0" class="empty-state">
      <div class="empty-icon">📦</div>
      <h2>No purchases yet</h2>
      <p>Purchase checks to start checking wallet addresses for vulnerabilities.</p>
      <RouterLink to="/pricing" class="btn-primary">View Pricing</RouterLink>
    </div>

    <div v-else class="orders-list">
      <table class="orders-table">
        <thead>
          <tr>
            <th>Date</th>
            <th>Checks</th>
            <th>Amount</th>
            <th>Status</th>
            <th>Transaction</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="order in orders" :key="order.order_uuid">
            <td class="date-cell">{{ formatDate(order.created_at) }}</td>
            <td class="checks-cell">{{ order.checks_count }} checks</td>
            <td class="amount-cell">{{ formatAmount(order) }}</td>
            <td>
              <span :class="['status-badge', order.status]">
                {{ order.status }}
              </span>
            </td>
            <td class="tx-cell">
              <a v-if="order.tx_hash" :href="'https://solscan.io/tx/' + order.tx_hash" target="_blank" class="tx-link">
                {{ order.tx_hash.slice(0, 8) }}...{{ order.tx_hash.slice(-6) }}
              </a>
              <span v-else class="no-tx">—</span>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<style scoped>
.purchases-page {
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

.orders-table {
  width: 100%;
  border-collapse: collapse;
  background: #1a1f2e;
  border-radius: 12px;
  overflow: hidden;
}

.orders-table th,
.orders-table td {
  padding: 1rem;
  text-align: left;
}

.orders-table th {
  background: #151a24;
  color: #6b7a9e;
  font-weight: 600;
  font-size: 0.85rem;
  text-transform: uppercase;
}

.orders-table td {
  border-bottom: 1px solid #2a3548;
  color: #e7ecf5;
}

.orders-table tr:last-child td {
  border-bottom: none;
}

.date-cell {
  font-size: 0.85rem;
  white-space: nowrap;
}

.checks-cell {
  font-weight: 500;
}

.amount-cell {
  color: #4bc9a0;
  font-weight: 500;
}

.status-badge {
  display: inline-block;
  padding: 0.25rem 0.6rem;
  border-radius: 6px;
  font-size: 0.75rem;
  font-weight: 600;
  text-transform: uppercase;
}

.status-badge.completed {
  background: rgba(75, 201, 160, 0.2);
  color: #4bc9a0;
}

.status-badge.pending {
  background: rgba(255, 193, 7, 0.2);
  color: #ffc107;
}

.status-badge.cancelled {
  background: rgba(255, 107, 107, 0.2);
  color: #ff6b6b;
}

.tx-link {
  color: #667eea;
  text-decoration: none;
  font-size: 0.85rem;
  font-family: monospace;
}

.tx-link:hover {
  text-decoration: underline;
}

.no-tx {
  color: #4a5568;
}
</style>
