<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'

const router = useRouter()
const checking = ref(false)

async function retry() {
  checking.value = true
  try {
    const res = await fetch('/api/chains', { 
      signal: AbortSignal.timeout(5000)
    })
    if (res.ok) {
      router.push('/')
    }
  } catch {}
  checking.value = false
}

function goHome() {
  router.push('/')
}
</script>

<template>
  <div class="error-page">
    <div class="error-icon">⚠️</div>
    <h1>Backend Unavailable</h1>
    <p class="error-message">Cannot connect to the server</p>
    <p class="error-hint">
      The backend server is not responding. Please check if the server is running
      or try again in a few moments.
    </p>
    <div class="error-actions">
      <button class="back-btn primary" @click="retry" :disabled="checking">
        {{ checking ? 'Checking...' : '🔄 Retry Connection' }}
      </button>
      <button class="back-btn" @click="goHome">
        ← Go to Home
      </button>
    </div>
    <div class="troubleshooting">
      <h3>Troubleshooting:</h3>
      <ul>
        <li>Make sure the backend server is running</li>
        <li>Check if the server URL is correct</li>
        <li>Verify your internet connection</li>
      </ul>
    </div>
  </div>
</template>

<style scoped>
.error-page {
  text-align: center;
  padding: 4rem 1rem;
}
.error-icon {
  font-size: 4rem;
  margin-bottom: 1rem;
}
.error-page h1 {
  font-size: 2rem;
  font-weight: 700;
  color: #ff6b6b;
  margin: 0 0 1rem;
}
.error-message {
  font-size: 1.3rem;
  color: #6b7a9e;
  margin: 0 0 0.5rem;
}
.error-hint {
  color: #4c5a7a;
  font-size: 0.9rem;
  margin-bottom: 2rem;
  max-width: 400px;
  margin-left: auto;
  margin-right: auto;
}
.error-actions {
  display: flex;
  gap: 1rem;
  justify-content: center;
  flex-wrap: wrap;
  margin-bottom: 2rem;
}
.back-btn {
  padding: 0.8rem 1.5rem;
  background: #1a2030;
  border: 1px solid #252d3d;
  border-radius: 10px;
  color: #98a8ce;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.2s;
}
.back-btn:hover {
  border-color: #4bc9a050;
}
.back-btn.primary {
  background: linear-gradient(135deg, #667eea, #764ba2);
  border: none;
  color: white;
}
.back-btn.primary:hover {
  transform: translateY(-2px);
  box-shadow: 0 4px 20px #667eea40;
}
.back-btn:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}
.troubleshooting {
  background: #151a24;
  border-radius: 12px;
  padding: 1.2rem;
  text-align: left;
  max-width: 400px;
  margin: 0 auto;
}
.troubleshooting h3 {
  font-size: 0.9rem;
  color: #98a8ce;
  margin: 0 0 0.8rem;
}
.troubleshooting ul {
  margin: 0;
  padding-left: 1.2rem;
  color: #5a6a8e;
  font-size: 0.8rem;
}
.troubleshooting li {
  margin: 0.3rem 0;
}
</style>
