<script setup lang="ts">
import { ref, computed } from 'vue'

const props = defineProps<{
  title: string
  hint?: string
  collapsed?: boolean
}>()

const open = ref(!props.collapsed)

const hintText = computed(() => props.hint || (open.value ? 'Click to collapse' : 'Click to expand'))
</script>

<template>
  <div class="collapse-section">
    <button class="collapse-toggle" @click="open = !open">
      <h2 class="collapse-title">{{ title }}</h2>
      <span class="collapse-hint">{{ hintText }}</span>
      <span class="collapse-arrow" :class="{ open }">▾</span>
    </button>
    <div v-show="open" class="collapse-body">
      <slot />
    </div>
  </div>
</template>

<style scoped>
.collapse-section {
  margin-bottom: 1.5rem;
}

.collapse-toggle {
  display: flex;
  align-items: center;
  gap: 0.6rem;
  width: 100%;
  text-align: left;
  background: none;
  border: none;
  padding: 0;
  margin-bottom: 0.5rem;
  cursor: pointer;
  color: inherit;
}

.collapse-title {
  font-size: 1.1rem;
  color: #e7ecf5;
  margin: 0;
}

.collapse-hint {
  color: #4c5a7a;
  font-size: 0.72rem;
  white-space: nowrap;
}

.collapse-arrow {
  color: #98a8ce;
  font-size: 0.8rem;
  transition: transform 0.15s ease;
}

.collapse-arrow.open {
  transform: rotate(0deg);
}

.collapse-arrow:not(.open) {
  transform: rotate(-90deg);
}

.collapse-body {
  min-width: 0;
}
</style>
