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
  return chainMeta[(chain || '').toLowerCase()] ?? chainMeta.solana
}
