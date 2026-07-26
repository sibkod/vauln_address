<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'

interface RoadmapItem {
  text: string
  done?: boolean
  status?: string
}

interface Phase {
  id: number
  title: string
  description: string
  status: string
  quarter: string
  progress?: number
  items: RoadmapItem[]
  technical?: string[]
}

interface RoadmapData {
  phases: Phase[]
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

function getStatusClass(status: string): string {
  switch (status) {
    case 'completed': return 'completed'
    case 'in_progress': return 'in-progress'
    case 'planned': return 'planned'
    default: return 'future'
  }
}

function getItemIcon(item: RoadmapItem): string {
  if (item.done === true) return '✓'
  if (item.status === 'in_progress') return '⏳'
  if (item.status === 'planned') return '○'
  return '☆'
}

function getItemClass(item: RoadmapItem): string {
  if (item.done === true) return 'done'
  if (item.status === 'in_progress') return 'in-progress'
  if (item.status === 'planned') return 'planned'
  return 'future'
}
</script>

<template>
  <div class="roadmap-container">
    <div class="roadmap-header">
      <h1>Roadmap</h1>
      <p class="roadmap-sub">Building comprehensive blockchain security intelligence</p>
    </div>
    
    <div v-if="loading" class="loading">
      <div class="spinner"></div>
    </div>

    <div v-else-if="roadmap" class="roadmap-timeline">
      <div 
        v-for="phase in roadmap.phases" 
        :key="phase.id"
        :class="['roadmap-phase', getStatusClass(phase.status)]"
      >
        <div class="phase-marker">
          <span class="phase-icon">
            {{ phase.status === 'completed' ? '✓' : phase.status === 'in_progress' ? '⟳' : phase.id }}
          </span>
          <span class="phase-number">Phase {{ phase.id }}</span>
        </div>
        <div class="phase-content">
          <h3>{{ phase.title }}</h3>
          <p class="phase-description">{{ phase.description }}</p>
          <ul>
            <li 
              v-for="(item, idx) in phase.items" 
              :key="idx"
              :class="getItemClass(item)"
            >
              {{ getItemIcon(item) }} {{ item.text }}
            </li>
          </ul>
          <div v-if="phase.technical" class="sub-details">
            <h4>Technical Implementation:</h4>
            <ul>
              <li v-for="(tech, idx) in phase.technical" :key="idx">→ {{ tech }}</li>
            </ul>
          </div>
          <span class="phase-date">
            {{ phase.status === 'completed' ? 'Completed' : phase.status === 'in_progress' ? 'In progress' : 'Planned' }} · {{ phase.quarter }}
          </span>
          <div v-if="phase.status === 'in_progress' && phase.progress" class="progress-bar">
            <div class="progress-fill" :style="{ width: phase.progress + '%' }"></div>
          </div>
        </div>
      </div>
    </div>

    <div class="contribute-section">
      <h3>🤝 Contribute to Security</h3>
      <p>Help us build a safer Web3 by reporting new threats and vulnerabilities.</p>
      <div class="contribute-cta">
        <RouterLink to="/contact" class="btn-primary">Report a Threat</RouterLink>
        <RouterLink to="/support" class="btn-secondary">Support Us</RouterLink>
      </div>
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

.roadmap-timeline {
  position: relative;
}

.roadmap-phase {
  display: flex;
  gap: 1.5rem;
  margin-bottom: 2.5rem;
  position: relative;
}

.roadmap-phase::before {
  content: '';
  position: absolute;
  left: 24px;
  top: 60px;
  bottom: -20px;
  width: 2px;
  background: #2a3548;
}

.roadmap-phase:last-child::before {
  display: none;
}

.phase-marker {
  flex-shrink: 0;
  width: 50px;
  height: 50px;
  border-radius: 50%;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  z-index: 1;
}

.completed .phase-marker {
  background: linear-gradient(135deg, #4bc9a0, #38a169);
  color: white;
}

.in-progress .phase-marker {
  background: linear-gradient(135deg, #667eea, #764ba2);
  color: white;
  animation: pulse 2s infinite;
}

.planned .phase-marker {
  background: #2a3548;
  color: #6b7a9e;
  border: 2px dashed #4a5568;
}

.future .phase-marker {
  background: #1a1f2e;
  color: #4a5568;
  border: 2px solid #2a3548;
}

@keyframes pulse {
  0%, 100% { box-shadow: 0 0 0 0 rgba(102, 126, 234, 0.4); }
  50% { box-shadow: 0 0 0 10px rgba(102, 126, 234, 0); }
}

.phase-icon {
  font-size: 1.2rem;
}

.phase-number {
  font-size: 0.6rem;
  font-weight: 600;
}

.phase-content {
  flex: 1;
  background: #1a1f2e;
  border: 1px solid #2a3548;
  border-radius: 12px;
  padding: 1.5rem;
}

.completed .phase-content {
  border-color: #4bc9a040;
}

.in-progress .phase-content {
  border-color: #667eea40;
}

.phase-content h3 {
  color: #e7ecf5;
  font-size: 1.3rem;
  margin-bottom: 0.5rem;
}

.phase-description {
  color: #6b7a9e;
  font-size: 0.9rem;
  margin-bottom: 1rem;
}

.phase-content ul {
  list-style: none;
  padding: 0;
  margin: 0;
}

.phase-content li {
  color: #98a8ce;
  padding: 0.4rem 0;
  padding-left: 1.5rem;
  position: relative;
  font-size: 0.95rem;
}

.phase-content li::before {
  position: absolute;
  left: 0;
  width: 1rem;
}

.done::before {
  content: '✓';
  color: #4bc9a0;
}

.in-progress::before {
  content: '⏳';
  color: #667eea;
}

.planned::before {
  content: '○';
  color: #6b7a9e;
}

.future::before {
  content: '☆';
  color: #4a5568;
}

.phase-date {
  display: inline-block;
  margin-top: 1rem;
  padding: 0.3rem 0.8rem;
  background: #151a24;
  border-radius: 20px;
  color: #6b7a9e;
  font-size: 0.8rem;
  font-weight: 500;
}

.progress-bar {
  margin-top: 1rem;
  height: 4px;
  background: #2a3548;
  border-radius: 2px;
  overflow: hidden;
}

.progress-fill {
  height: 100%;
  background: linear-gradient(90deg, #667eea, #764ba2);
  border-radius: 2px;
  transition: width 0.3s ease;
}

.sub-details {
  margin-top: 1.5rem;
  padding-top: 1rem;
  border-top: 1px solid #2a3548;
}

.sub-details h4 {
  color: #98a8ce;
  font-size: 0.9rem;
  margin-bottom: 0.5rem;
}

.sub-details ul {
  font-size: 0.85rem;
  color: #6b7a9e;
}

.sub-details li::before {
  content: none;
}

.sub-details li {
  padding-left: 1rem;
}

.contribute-section {
  margin-top: 4rem;
  text-align: center;
  padding: 2rem;
  background: linear-gradient(135deg, #1a1f2e 0%, #1a2233 100%);
  border: 1px solid #2a3548;
  border-radius: 16px;
}

.contribute-section h3 {
  color: #e7ecf5;
  font-size: 1.5rem;
  margin-bottom: 0.5rem;
}

.contribute-section p {
  color: #6b7a9e;
  margin-bottom: 1.5rem;
}

.contribute-cta {
  display: flex;
  gap: 1rem;
  justify-content: center;
}

.btn-primary {
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

.btn-secondary {
  padding: 0.75rem 1.5rem;
  background: transparent;
  color: #98a8ce;
  text-decoration: none;
  border-radius: 8px;
  font-weight: 500;
  border: 1px solid #2a3548;
  transition: all 0.2s;
}

.btn-secondary:hover {
  border-color: #667eea;
  color: #667eea;
}
</style>
