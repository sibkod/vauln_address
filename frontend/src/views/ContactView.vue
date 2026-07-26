<script setup lang="ts">
import { ref, onMounted } from 'vue'

interface Channel {
  name: string
  icon: string
  url: string
  description: string
}

const channels = ref<Channel[]>([])
const loading = ref(true)

onMounted(async () => {
  try {
    const res = await fetch('/data/contacts.json')
    const data = await res.json()
    channels.value = data.channels || []
  } catch (err) {
    console.error('Failed to load contacts:', err)
  } finally {
    loading.value = false
  }
})
</script>

<template>
  <div class="contacts-container">
    <div class="contacts-header">
      <h1>Connect With Us</h1>
      <p class="subtitle">Join our community and stay updated</p>
    </div>

    <div v-if="loading" class="loading">
      <div class="spinner"></div>
    </div>

    <div v-else class="channels-grid">
      <a 
        v-for="channel in channels" 
        :key="channel.name"
        :href="channel.url"
        target="_blank"
        rel="noopener noreferrer"
        class="channel-card"
      >
        <span class="channel-icon">{{ channel.icon }}</span>
        <div class="channel-info">
          <h3>{{ channel.name }}</h3>
          <span class="channel-desc">{{ channel.description }}</span>
        </div>
        <span class="channel-arrow">→</span>
      </a>
    </div>

    <div class="contact-note">
      <p>For partnership inquiries or security-related reports, please reach out via Telegram or Discord.</p>
    </div>
  </div>
</template>

<style scoped>
.contacts-container {
  max-width: 800px;
  margin: 0 auto;
  padding: 2rem 1rem;
}

.contacts-header {
  text-align: center;
  margin-bottom: 3rem;
}

.contacts-header h1 {
  font-size: 2.5rem;
  font-weight: 700;
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  margin-bottom: 0.5rem;
}

.subtitle {
  color: #6b7a9e;
  font-size: 1.1rem;
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

.channels-grid {
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.channel-card {
  display: flex;
  align-items: center;
  gap: 1rem;
  padding: 1.25rem 1.5rem;
  background: #1a1f2e;
  border: 1px solid #2a3548;
  border-radius: 12px;
  text-decoration: none;
  transition: all 0.2s ease;
}

.channel-card:hover {
  border-color: #667eea;
  transform: translateX(4px);
  box-shadow: 0 4px 20px rgba(102, 126, 234, 0.15);
}

.channel-icon {
  font-size: 2rem;
  width: 50px;
  height: 50px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: #151a24;
  border-radius: 12px;
}

.channel-info {
  flex: 1;
}

.channel-info h3 {
  color: #e7ecf5;
  font-size: 1.1rem;
  margin: 0 0 0.25rem 0;
}

.channel-desc {
  color: #6b7a9e;
  font-size: 0.85rem;
}

.channel-arrow {
  color: #4a5568;
  font-size: 1.2rem;
  transition: all 0.2s ease;
}

.channel-card:hover .channel-arrow {
  color: #667eea;
  transform: translateX(4px);
}

.contact-note {
  margin-top: 2rem;
  padding: 1rem;
  background: #151a24;
  border-radius: 8px;
  text-align: center;
}

.contact-note p {
  color: #6b7a9e;
  font-size: 0.85rem;
  margin: 0;
}
</style>
