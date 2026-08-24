<script setup lang="ts">
const channels = [
  { name: "Discord", icon: "💬", url: "https://discord.gg/PEFtkRcSe", description: "Join our Discord community" },
  { name: "Telegram", icon: "✈️", url: "https://t.me/pwndinfo", description: "Follow us on Telegram" },
  // { name: "Bitcointalk", icon: "📋", url: "#", description: "Bitcointalk thread (coming soon)" },
  // { name: "Reddit", icon: "🅡", url: "#", description: "Reddit community (coming soon)" }
]
</script>

<template>
  <div class="contacts-container">
    <div class="contacts-header">
      <h1>Connect With Us</h1>
      <p class="subtitle">Join our community and stay updated</p>
    </div>

    <div class="channels-grid">
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
      <p>For partnership inquiries or security-related reports, please reach out via Discord or Telegram.</p>
    </div>
  </div>
</template>

<style scoped>
.contacts-container {
  max-width: 1100px;
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
  background: linear-gradient(135deg, #00e5ff 0%, #7b5cff 100%);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  margin-bottom: 0.5rem;
}

.subtitle {
  color: #6b7a9e;
  font-size: 1.1rem;
}

.channels-grid {
  display: grid;
  grid-template-columns: 1fr;
  gap: 1rem;
}
@media (min-width: 900px) {
  .channels-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
}

.channel-card {
  display: flex;
  align-items: center;
  gap: 1rem;
  padding: 1.25rem 1.5rem;
  background: rgba(13, 18, 32, 0.88);
  border: 1px solid rgba(0, 229, 255, 0.1);
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
  background: rgba(10, 15, 28, 0.8);
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
  background: rgba(10, 15, 28, 0.8);
  border-radius: 8px;
  text-align: center;
}

.contact-note p {
  color: #6b7a9e;
  font-size: 0.85rem;
  margin: 0;
}
</style>
