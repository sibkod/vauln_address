<script setup lang="ts">
import TxTreeNode from './TxTreeNode.vue'

export interface TxNode {
  address: string
  tx_count: number
  amount: number
  currency: string
  status: string
  children?: TxNode[]
}

const props = defineProps<{ node: TxNode }>()

function short(addr: string) {
  if (addr.length <= 14) return addr
  return `${addr.slice(0, 8)}…${addr.slice(-6)}`
}

function statusClass(status: string) {
  switch (status) {
    case 'hacked':
    case 'hacker':
      return 'danger'
    case 'potential_hacker':
    case 'vulnerable':
      return 'warn'
    case 'safe':
    case 'not_found':
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
  unknown: '❓',
  not_found: '✅'
}
</script>

<template>
  <div class="tx-node">
    <div class="tx-card" :class="statusClass(props.node.status)">
      <span class="tx-icon">{{ statusIcons[props.node.status] || '❓' }}</span>
      <span class="tx-addr" :title="props.node.address">{{ short(props.node.address) }}</span>
      <span class="tx-badge">{{ (props.node.status || 'unknown').replace('_', ' ') }}</span>
      <span class="tx-meta">{{ props.node.tx_count }} tx</span>
      <span class="tx-amount">{{ props.node.amount }} {{ props.node.currency }}</span>
    </div>
    <div v-if="props.node.children && props.node.children.length" class="tx-children">
      <TxTreeNode v-for="child in props.node.children" :key="child.address" :node="child" />
    </div>
  </div>
</template>

<style scoped>
.tx-card {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 0.5rem;
  padding: 0.5rem 0.75rem;
  background: #151a24;
  border: 1px solid #2a3548;
  border-radius: 8px;
  font-size: 0.82rem;
}

.tx-card.danger { border-color: rgba(255, 107, 107, 0.5); }
.tx-card.warn { border-color: rgba(255, 179, 71, 0.5); }
.tx-card.safe { border-color: rgba(75, 201, 160, 0.4); }

.tx-addr {
  color: #98a8ce;
  font-family: monospace;
}

.tx-badge {
  padding: 0.1rem 0.45rem;
  border-radius: 5px;
  font-size: 0.68rem;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.03em;
  background: rgba(76, 90, 122, 0.3);
  color: #98a8ce;
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
  font-size: 0.75rem;
}

.tx-amount {
  margin-left: auto;
  color: #e7ecf5;
  font-family: monospace;
  font-size: 0.78rem;
}

.tx-children {
  margin-left: 1.25rem;
  padding-left: 0.75rem;
  border-left: 1px solid #2a3548;
  display: flex;
  flex-direction: column;
  gap: 0.4rem;
  padding-top: 0.4rem;
}
</style>
