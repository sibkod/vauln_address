<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'

const router = useRouter()
const message = ref('Page not found')
const showBack = ref(true)

function goHome() {
  router.push('/')
}
</script>

<template>
  <div class="error-page">
    <div class="error-icon">🔍</div>
    <h1>404</h1>
    <p class="error-message">{{ message }}</p>
    <p class="error-hint">The page you're looking for doesn't exist or you don't have access.</p>
    <button v-if="showBack" class="back-btn" @click="goHome">
      ← Back to Home
    </button>
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
  font-size: 6rem;
  font-weight: 700;
  color: #2a3555;
  margin: 0;
  line-height: 1;
}
.error-message {
  font-size: 1.3rem;
  color: #6b7a9e;
  margin: 1rem 0;
}
.error-hint {
  color: #4c5a7a;
  font-size: 0.9rem;
  margin-bottom: 2rem;
}
.back-btn {
  padding: 0.8rem 2rem;
  background: linear-gradient(135deg, #00e5ff, #7b5cff);
  border: none;
  border-radius: 10px;
  color: white;
  font-weight: 600;
  cursor: pointer;
  transition: all 0.2s;
}
.back-btn:hover {
  transform: translateY(-2px);
  box-shadow: 0 4px 20px #667eea40;
}
</style>
