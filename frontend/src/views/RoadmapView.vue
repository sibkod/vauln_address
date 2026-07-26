<script setup lang="ts">
import { ref, onMounted } from 'vue'

interface RoadmapItem {
  text: string
}

interface Section {
  title: string
  items: RoadmapItem[]
}

interface RoadmapData {
  sections: Section[]
}

const roadmap = ref<RoadmapData | null>(null)
const loading = ref(true)

onMounted(async () => {
  try {
    const res = await fetch('/data/roadmap.json')
    roadmap.value = await res.json()
  } catch (err) {
    console.error('Failed to load roadmap:', err)
  } finally {
    loading.value = false
  }
})
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

    <div class="contribute-section">
      <h3>🤝 Contribute to Security</h3>
      <p>Help us build a safer Web3 by reporting new threats.</p>
      <RouterLink to="/contact" class="btn-primary">Report a Threat</RouterLink>
    </div>
  </div>
</template>

<style scoped>
.roadmap-container {
  max-width: 900px;
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
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
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
  border: 3px solid #2a3548;
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
  background: #1a1f2e;
  border: 1px solid #2a3548;
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
  border-bottom: 1px solid #2a3548;
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
  background: linear-gradient(135deg, #1a1f2e 0%, #1a2233 100%);
  border: 1px solid #2a3548;
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
  background: linear-gradient(135deg, #667eea, #764ba2);
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
