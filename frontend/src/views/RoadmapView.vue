<script setup lang="ts">
const roadmap = {
  sections: [
    {
      title: "✓ Done",
      items: [
        { text: "Phantom wallet authorization" },
        { text: "Wallet checks functionality" },
        { text: "Basic database with 100,000+ records" }
      ]
    },
    {
      title: "◐ In Progress",
      items: [
        { text: "Adding more wallet options" }
      ]
    },
    {
      title: "○ Future Plans",
      items: [
        { text: "Solana network transaction analysis" },
        { text: "Multi-chain transaction analysis" },
        { text: "Hacked wallet detection" },
        { text: "Malicious smart contract identification" },
        { text: "Stolen funds and hacker wallet tracking" }
      ]
    }
  ]
}
</script>

<template>
  <div class="roadmap-container">
    <div class="roadmap-header">
      <h1>Roadmap</h1>
      <p class="roadmap-sub">What's coming next</p>
    </div>
    
    <div v-if="loading" class="loading">
      <div class="spinner"></div>
    </div>

    <div v-else-if="roadmap" class="roadmap-grid">
      <div 
        v-for="(section, idx) in roadmap.sections" 
        :key="idx"
        class="roadmap-section"
        :class="{ 'in-progress': section.title.includes('Progress') }"
      >
        <h2 class="section-title">{{ section.title }}</h2>
        <ul class="checklist">
          <li v-for="(item, i) in section.items" :key="i">{{ item.text }}</li>
        </ul>
      </div>
    </div>
  </div>
</template>

<style scoped>
.roadmap-container {
  max-width: 1200px;
  margin: 0 auto;
  padding: 2rem 1rem;
}

.roadmap-header {
  text-align: center;
  margin-bottom: 3rem;
}

.roadmap-header h1 {
  font-size: 2.5rem;
  font-weight: 700;
  background: linear-gradient(135deg, #00e5ff 0%, #7b5cff 100%);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  margin-bottom: 0.5rem;
}

.roadmap-sub {
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
  border: 3px solid rgba(0, 229, 255, 0.1);
  border-top-color: #667eea;
  border-radius: 50%;
  margin: 0 auto;
  animation: spin 1s linear infinite;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

.roadmap-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
  gap: 1.5rem;
}

.roadmap-section {
  background: rgba(13, 18, 32, 0.88);
  border: 1px solid rgba(0, 229, 255, 0.1);
  border-radius: 12px;
  padding: 1.5rem;
}

.roadmap-section.in-progress {
  border-color: #667eea40;
}

.section-title {
  color: #e7ecf5;
  font-size: 1.1rem;
  margin-bottom: 1rem;
  padding-bottom: 0.75rem;
  border-bottom: 1px solid rgba(0, 229, 255, 0.1);
}

.checklist {
  list-style: none;
  padding: 0;
  margin: 0;
}

.checklist li {
  color: #98a8ce;
  padding: 0.5rem 0;
  padding-left: 1.5rem;
  position: relative;
  font-size: 0.95rem;
  border-bottom: 1px solid rgba(42, 53, 72, 0.5);
}

.checklist li:last-child {
  border-bottom: none;
}

.checklist li::before {
  content: '○';
  position: absolute;
  left: 0;
  color: #4a5568;
}

.in-progress .checklist li::before {
  content: '◐';
  color: #667eea;
}

.roadmap-section:first-child .checklist li::before {
  content: '✓';
  color: #4bc9a0;
}

.contribute-section {
  margin-top: 3rem;
  text-align: center;
  padding: 2rem;
  background: linear-gradient(135deg, rgba(13, 18, 32, 0.88) 0%, #1a2233 100%);
  border: 1px solid rgba(0, 229, 255, 0.1);
  border-radius: 16px;
}

.contribute-section h3 {
  color: #e7ecf5;
  font-size: 1.3rem;
  margin-bottom: 0.5rem;
}

.contribute-section p {
  color: #6b7a9e;
  margin-bottom: 1.5rem;
}

.btn-primary {
  display: inline-block;
  padding: 0.75rem 1.5rem;
  background: linear-gradient(135deg, #00e5ff, #7b5cff);
  color: white;
  text-decoration: none;
  border-radius: 8px;
  font-weight: 500;
  transition: transform 0.2s, box-shadow 0.2s;
}

.btn-primary:hover {
  transform: translateY(-2px);
  box-shadow: 0 4px 12px rgba(102, 126, 234, 0.3);
}
</style>
