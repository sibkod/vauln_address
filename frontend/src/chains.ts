export interface ChainMeta {
  name: string
  symbol: string
  decimals: number
  txUrl: (sig: string) => string
  addrUrl: (addr: string) => string
  blockLabel: string
}

export const chainMeta: Record<string, ChainMeta> = {
  solana: {
    name: 'Solana',
    symbol: 'SOL',
    decimals: 4,
    txUrl: (sig) => `https://solscan.io/tx/${sig}`,
    addrUrl: (addr) => `https://solscan.io/account/${addr}`,
    blockLabel: 'slot',
  },
  evm: {
    name: 'Ethereum',
    symbol: 'ETH',
    decimals: 4,
    txUrl: (sig) => `https://etherscan.io/tx/${sig}`,
    addrUrl: (addr) => `https://etherscan.io/address/${addr}`,
    blockLabel: 'block',
  },
  ethereum: {
    name: 'Ethereum',
    symbol: 'ETH',
    decimals: 4,
    txUrl: (sig) => `https://etherscan.io/tx/${sig}`,
    addrUrl: (addr) => `https://etherscan.io/address/${addr}`,
    blockLabel: 'block',
  },
  bnb: {
    name: 'BNB Chain',
    symbol: 'BNB',
    decimals: 4,
    txUrl: (sig) => `https://bscscan.com/tx/${sig}`,
    addrUrl: (addr) => `https://bscscan.com/address/${addr}`,
    blockLabel: 'block',
  },
  base: {
    name: 'Base',
    symbol: 'ETH',
    decimals: 4,
    txUrl: (sig) => `https://basescan.org/tx/${sig}`,
    addrUrl: (addr) => `https://basescan.org/address/${addr}`,
    blockLabel: 'block',
  },
  linea: {
    name: 'Linea',
    symbol: 'ETH',
    decimals: 4,
    txUrl: (sig) => `https://lineascan.build/tx/${sig}`,
    addrUrl: (addr) => `https://lineascan.build/address/${addr}`,
    blockLabel: 'block',
  },
  arbitrum: {
    name: 'Arbitrum',
    symbol: 'ETH',
    decimals: 4,
    txUrl: (sig) => `https://arbiscan.io/tx/${sig}`,
    addrUrl: (addr) => `https://arbiscan.io/address/${addr}`,
    blockLabel: 'block',
  },
  polygon: {
    name: 'Polygon',
    symbol: 'POL',
    decimals: 4,
    txUrl: (sig) => `https://polygonscan.com/tx/${sig}`,
    addrUrl: (addr) => `https://polygonscan.com/address/${addr}`,
    blockLabel: 'block',
  },
  optimism: {
    name: 'Optimism',
    symbol: 'ETH',
    decimals: 4,
    txUrl: (sig) => `https://optimistic.etherscan.io/tx/${sig}`,
    addrUrl: (addr) => `https://optimistic.etherscan.io/address/${addr}`,
    blockLabel: 'block',
  },
  avalanche: {
    name: 'Avalanche',
    symbol: 'AVAX',
    decimals: 4,
    txUrl: (sig) => `https://snowtrace.io/tx/${sig}`,
    addrUrl: (addr) => `https://snowtrace.io/address/${addr}`,
    blockLabel: 'block',
  },
  btc: {
    name: 'Bitcoin',
    symbol: 'BTC',
    decimals: 8,
    txUrl: (sig) => `https://mempool.space/tx/${sig}`,
    addrUrl: (addr) => `https://mempool.space/address/${addr}`,
    blockLabel: 'block',
  },
  sui: {
    name: 'Sui',
    symbol: 'SUI',
    decimals: 4,
    txUrl: (sig) => `https://suiscan.xyz/mainnet/tx/${sig}`,
    addrUrl: (addr) => `https://suiscan.xyz/mainnet/account/${addr}`,
    blockLabel: 'checkpoint',
  },
  tron: {
    name: 'Tron',
    symbol: 'TRX',
    decimals: 2,
    txUrl: (sig) => `https://tronscan.org/#/transaction/${sig}`,
    addrUrl: (addr) => `https://tronscan.org/#/address/${addr}`,
    blockLabel: 'block',
  },
}

export function getChainMeta(chain?: string): ChainMeta {
  const key = (chain || '').toLowerCase()
  const meta = chainMeta[key]
  if (meta) return meta
  // Unknown chain: show its own name/symbol, fall back to solana explorer.
  const name = key ? key.charAt(0).toUpperCase() + key.slice(1) : 'Solana'
  return { ...chainMeta.solana, name, symbol: key.toUpperCase() || 'SOL' }
}
