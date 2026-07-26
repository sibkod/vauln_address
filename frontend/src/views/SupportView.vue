<script setup lang="ts">
import { ref, onMounted } from 'vue'

interface Wallet {
  name: string
  symbol: string
  icon: string
  address: string
  color: string
}

interface DonationsData {
  title: string
  subtitle: string
  wallets: Wallet[]
}

const donations = ref<DonationsData | null>(null)
const loading = ref(true)
const copiedWallet = ref('')

onMounted(async () => {
  try {
    const res = await fetch('/data/donations.json')
    donations.value = await res.json()
  } catch (err) {
    console.error('Failed to load donations:', err)
  } finally {
    loading.value = false
  }
})

function copyAddress(address: string) {
  navigator.clipboard.writeText(address)
  copiedWallet.value = address
  setTimeout(() => {
    copiedWallet.value = ''
  }, 2000)
}
</script>

<template>
  <div class="support-container">
    <div class="support-header">
      <h1>Support Us</h1>
      <p v-if="donations" class="subtitle">{{ donations.subtitle }}</p>
    </div>

    <div v-if="loading" class="loading">
      <div class="spinner"></div>
    </div>

    <div v-else-if="donations" class="donations-section">
      <div class="wallets-grid">
        <div 
          v-for="wallet in donations.wallets" 
          :key="wallet.symbol"
          class="wallet-card"
        >
          <div class="wallet-header">
            <span class="wallet-icon" :style="{ background: wallet.color + '20', color: wallet.color }">
              {{ wallet.icon }}
            </span>
            <div class="wallet-info">
              <h3>{{ wallet.name }}</h3>
              <span class="wallet-symbol">{{ wallet.symbol }}</span>
            </div>
          </div>
          
          <div class="wallet-address" @click="copyAddress(wallet.address)">
            <code>{{ wallet.address }}</code>
            <button class="copy-btn">
              {{ copiedWallet === wallet.address ? '✓' : '📋' }}
            </button>
          </div>
        </div>
      </div>
    </div>

    <div class="support-note">
      <p>💡 All donations are greatly appreciated and help us maintain and improve pwnd.</p>
    </div>
  </div>
</template>

<style scoped>
.support-container {
  max-width: 800px;
  margin: 0 auto;
  padding: 2rem 1rem;
}

.support-header {
  text-align: center;
  margin-bottom: 3rem;
}

.support-header h1 {
  font-size: 2.5rem;
  font-weight: 700;
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  margin-bottom: 0.5rem;
}

.subtitle {
  color: #6b7a9e;
  font-size: 1rem;
  max-width: 500px;
  margin: 0 auto;
}

.loading {
  text-align: center;
  padding: 3rem;
}

.spinner {
  width: 40px;
  height: 40px;
  border: 3px solid #2a3548;
  border-top-color: #667eea;
  border-radius: 50%;
  margin: 0 auto;
  animation: spin 1s linear infinite;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

.donations-section {
  margin-bottom: 2rem;
}

.wallets-grid {
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.wallet-card {
  background: #1a1f2e;
  border: 1px solid #2a3548;
  border-radius: 12px;
  padding: 1.25rem;
  transition: all 0.2s ease;
}

.wallet-card:hover {
  border-color: #3a4560;
}

.wallet-header {
  display: flex;
  align-items: center;
  gap: 1rem;
  margin-bottom: 1rem;
}

.wallet-icon {
  font-size: 1.8rem;
  width: 50px;
  height: 50px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 12px;
}

.wallet-info h3 {
  color: #e7ecf5;
  font-size: 1.1rem;
  margin: 0 0 0.2rem 0;
}

.wallet-symbol {
  color: #6b7a9e;
  font-size: 0.85rem;
}

.wallet-address {
  display: flex;
  align-items: center;
  justify-content: space-between;
  background: #151a24;
  border-radius: 8px;
  padding: 0.75rem 1rem;
  cursor: pointer;
  transition: all 0.2s ease;
}

.wallet-address:hover {
  background: #1a2030;
}

.wallet-address code {
  font-family: 'Monaco', 'Menlo', monospace;
  font-size: 0.75rem;
  color: #98a8ce;
  word-break: break-all;
}

.copy-btn {
  flex-shrink: 0;
  margin-left: 0.5rem;
  background: none;
  border: none;
  font-size: 1rem;
  cursor: pointer;
  padding: 0.25rem;
  opacity: 0.7;
  transition: opacity 0.2s;
}

.copy-btn:hover {
  opacity: 1;
}

.support-note {
  margin-top: 2rem;
  padding: 1rem;
  background: #151a24;
  border-radius: 8px;
  text-align: center;
}

.support-note p {
  color: #6b7a9e;
  font-size: 0.85rem;
  margin: 0;
}
</style>
