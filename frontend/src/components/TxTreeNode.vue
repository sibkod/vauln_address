<script setup lang="ts">
import { ref, computed } from 'vue'
import TxTreeNode from './TxTreeNode.vue'

export interface TxNode {
  address: string
  tx_count: number
  amount: number
  currency: string
  status: string
  associated_hacker?: boolean
  is_program?: boolean
  children?: TxNode[]
}

const props = defineProps<{ node: TxNode; depth?: number }>()

const depth = computed(() => props.depth ?? 0)

// Корень разбивается на группы-корзины, чтобы не рендерить сотни детей разом
const TREE_GROUP_SIZE = 50
const openGroups = ref<Set<string>>(new Set())

const groups = computed(() => {
  if (depth.value !== 0 || !props.node.children?.length) return null
  const out: { key: string; label: string; min: number; max: number }[] = []
  const count = props.node.children.length
  for (let start = 0; start < count; start += TREE_GROUP_SIZE) {
    const min = start + 1
    const max = Math.min(start + TREE_GROUP_SIZE, count)
    out.push({ key: `${min}-${max}`, label: `${min}–${max}`, min, max })
  }
  return out.length > 1 ? out : null
})

function toggleGroup(key: string) {
  if (openGroups.value.has(key)) openGroups.value.delete(key)
  else openGroups.value.add(key)
  openGroups.value = new Set(openGroups.value)
}

function addrTitle(node: TxNode) {
  return node.is_program ? `${node.address} — on-chain program` : node.address
}

function statusClass(status: string) {
  switch (status) {
    case 'hacked':
    case 'hacker':
    case 'drained':
    case 'phishing':
    case 'scam':
    case 'sanctioned':
      return 'danger'
    case 'potential_hacker':
    case 'program':
    case 'vulnerable':
    case 'suspicious':
    case 'mixer':
    case 'frozen':
      return 'warn'
    case 'safe':
    case 'not_found':
    case 'exchange':
      return 'safe'
    default:
      return 'muted'
  }
}

const statusIcons: Record<string, string> = {
  hacked: '🚨',
  hacker: '💀',
  potential_hacker: '⚠️',
  vulnerable: '⚠️',
  safe: '✅',
  drained: '🏴',
  phishing: '🎣',
  scam: '🕳️',
  mixer: '🌀',
  sanctioned: '⛔',
  suspicious: '🔍',
  frozen: '🧊',
  exchange: '🏦',
  program: '⚙️',
  unknown: '❓',
  not_found: '✅'
}
</script>

<template>
  <div class="tx-node">
    <div class="tx-card" :class="statusClass(props.node.status)">
      <span class="tx-icon">{{ statusIcons[props.node.status] || '❓' }}</span>
      <span v-if="props.node.associated_hacker" class="tx-assoc" title="Transferred funds to a known hacker operator">🕸️</span>
      <span class="tx-addr" :title="addrTitle(props.node)">{{ props.node.address }}</span>
      <span class="tx-badge">{{ (props.node.status || 'unknown').replace('_', ' ') }}</span>
      <span class="tx-meta">{{ props.node.tx_count }} tx</span>
      <span class="tx-amount">{{ props.node.amount }} {{ props.node.currency }}</span>
    </div>
    <div v-if="props.node.children && props.node.children.length" class="tx-children">
      <template v-if="groups">
        <div v-for="g in groups" :key="g.key" class="tx-group">
          <button class="tx-group-toggle" @click="toggleGroup(g.key)">
            <span class="tx-group-arrow" :class="{ open: openGroups.has(g.key) }">▾</span>
            children {{ g.label }} / {{ props.node.children!.length }}
          </button>
          <template v-if="openGroups.has(g.key)">
            <TxTreeNode
              v-for="child in props.node.children!.slice(g.min - 1, g.max)"
              :key="child.address"
              :node="child"
              :depth="depth + 1" />
          </template>
        </div>
      </template>
      <template v-else>
        <TxTreeNode
          v-for="child in props.node.children"
          :key="child.address"
          :node="child"
          :depth="depth + 1" />
      </template>
    </div>
  </div>
</template>

<style scoped>
.tx-node {
  min-width: 0;
}

.tx-card {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 0.35rem;
  padding: 0.35rem 0.6rem;
  background: #151a24;
  border: 1px solid #2a3548;
  border-radius: 7px;
  font-size: 0.78rem;
  min-width: 0;
  overflow: hidden;
}

.tx-card.danger { border-color: rgba(255, 107, 107, 0.5); }
.tx-card.warn { border-color: rgba(255, 179, 71, 0.5); }
.tx-card.safe { border-color: rgba(75, 201, 160, 0.4); }

.tx-assoc {
  font-size: 0.75rem;
}

.tx-addr {
  color: #98a8ce;
  font-family: monospace;
  min-width: 0;
  flex: 1 1 100%;
  overflow-wrap: anywhere;
  word-break: break-all;
}

.tx-badge {
  padding: 0.05rem 0.4rem;
  border-radius: 5px;
  font-size: 0.62rem;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.03em;
  background: rgba(76, 90, 122, 0.3);
  color: #98a8ce;
  white-space: nowrap;
}

.tx-card.danger .tx-badge {
  background: rgba(255, 107, 107, 0.2);
  color: #ff6b6b;
}

.tx-card.warn .tx-badge {
  background: rgba(255, 179, 71, 0.2);
  color: #ffb347;
}

.tx-card.safe .tx-badge {
  background: rgba(75, 201, 160, 0.2);
  color: #4bc9a0;
}

.tx-meta {
  color: #4c5a7a;
  font-size: 0.7rem;
  white-space: nowrap;
}

.tx-amount {
  margin-left: auto;
  color: #e7ecf5;
  font-family: monospace;
  font-size: 0.74rem;
  white-space: nowrap;
}

.tx-children {
  margin-left: 1rem;
  padding-left: 0.6rem;
  border-left: 1px solid #2a3548;
  display: flex;
  flex-direction: column;
  gap: 0.3rem;
  padding-top: 0.3rem;
  min-width: 0;
}

.tx-group {
  display: flex;
  flex-direction: column;
  gap: 0.3rem;
}

.tx-group-toggle {
  display: flex;
  align-items: center;
  gap: 0.35rem;
  padding: 0.25rem 0.6rem;
  background: #151a24;
  border: 1px dashed #2a3548;
  border-radius: 7px;
  color: #4c5a7a;
  font-size: 0.72rem;
  cursor: pointer;
  font-family: inherit;
}

.tx-group-toggle:hover {
  color: #98a8ce;
  border-color: #4c5a7a;
}

.tx-group-arrow {
  transition: transform 0.15s ease;
}

.tx-group-arrow.open {
  transform: rotate(0deg);
}

.tx-group-arrow:not(.open) {
  transform: rotate(-90deg);
}
</style>
