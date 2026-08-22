<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import ChainLogo from './ChainLogo.vue'

export interface ChainOption {
  value: string
  label: string
}

const props = defineProps<{
  modelValue: string
  options: ChainOption[]
}>()

const emit = defineEmits<{ 'update:modelValue': [value: string] }>()

const open = ref(false)
const root = ref<HTMLElement | null>(null)

const selected = computed(() =>
  props.options.find(o => o.value === props.modelValue) ?? props.options[0]
)

function toggle() {
  open.value = !open.value
}

function choose(value: string) {
  emit('update:modelValue', value)
  open.value = false
}

function onOutsideClick(e: MouseEvent) {
  if (root.value && !root.value.contains(e.target as Node)) open.value = false
}

function onEscape(e: KeyboardEvent) {
  if (e.key === 'Escape') open.value = false
}

onMounted(() => {
  document.addEventListener('click', onOutsideClick)
  document.addEventListener('keydown', onEscape)
})

onUnmounted(() => {
  document.removeEventListener('click', onOutsideClick)
  document.removeEventListener('keydown', onEscape)
})
</script>

<template>
  <div ref="root" class="chain-select" :class="{ open }">
    <button type="button" class="chain-select-trigger" @click="toggle" aria-haspopup="listbox" :aria-expanded="open">
      <ChainLogo :chain="selected?.value || ''" :size="18" />
      <span class="chain-select-label">{{ selected?.label }}</span>
      <span class="chain-select-arrow">▾</span>
    </button>
    <ul v-if="open" class="chain-select-menu" role="listbox">
      <li
        v-for="opt in options"
        :key="opt.value"
        role="option"
        :aria-selected="opt.value === modelValue"
        class="chain-select-option"
        :class="{ active: opt.value === modelValue }"
        @click="choose(opt.value)"
      >
        <ChainLogo :chain="opt.value" :size="18" />
        <span>{{ opt.label }}</span>
      </li>
    </ul>
  </div>
</template>

<style scoped>
.chain-select {
  position: relative;
  z-index: 60;
  flex-shrink: 0;
  display: flex;
  align-items: center;
  align-self: stretch;
}

.chain-select-trigger {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  height: 100%;
  padding: 0 0.7rem 0 0.8rem;
  background: transparent;
  border: none;
  border-right: 1px solid rgba(255, 255, 255, 0.05);
  color: #c8d2ea;
  font-size: 0.86rem;
  font-weight: 600;
  cursor: pointer;
  white-space: nowrap;
}

.chain-select-arrow {
  color: #4c5a7a;
  font-size: 0.6rem;
}

.chain-select-menu {
  position: absolute;
  top: calc(100% + 0.6rem);
  left: 0;
  min-width: 100%;
  margin: 0;
  padding: 0.25rem;
  list-style: none;
  background: #11182a;
  border: 1px solid #2a3548;
  border-radius: 12px;
  z-index: 50;
  box-shadow: 0 12px 32px rgba(0, 0, 0, 0.6);
}

.chain-select-option {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.45rem 0.6rem;
  border-radius: 8px;
  color: #c7d2e8;
  font-size: 0.85rem;
  white-space: nowrap;
  cursor: pointer;
  transition: background 0.15s;
}

.chain-select-option:hover {
  background: #1a2233;
}

.chain-select-option.active {
  background: rgba(102, 126, 234, 0.15);
  color: #e7ecf5;
}

@media (max-width: 720px) {
  .chain-select {
    width: 100%;
    background: rgba(16, 22, 34, 0.6);
    border: 1px solid rgba(255, 255, 255, 0.03);
    border-radius: 60px;
  }

  .chain-select-trigger {
    width: 100%;
    padding: 0.6rem 0.9rem;
    border-right: none;
  }
}
</style>
