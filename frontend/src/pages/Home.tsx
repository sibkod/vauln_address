import { useState } from 'react'

export default function Home() {
  const [address, setAddress] = useState('')
  const [chain, setChain] = useState('evm')
  const [result, setResult] = useState<any>(null)
  const [loading, setLoading] = useState(false)

  const handleCheck = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!address) return

    setLoading(true)
    try {
      const res = await fetch('/api/check', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ address, chain })
      })
      const data = await res.json()
      setResult(data)
    } catch (err) {
      setResult({ error: 'Request failed' })
    }
    setLoading(false)
  }

  return (
    <>
      <h1>// WALLET CHECKER</h1>
      <p className="text-dim mb-3">Check if your wallet has been compromised.</p>

      <div className="card">
        <div className="card-header">CHECK WALLET</div>
        <form onSubmit={handleCheck} style={{ display: 'flex', gap: '0.5rem', marginBottom: '1rem' }}>
          <select value={chain} onChange={(e) => setChain(e.target.value)} style={{ width: '120px' }}>
            <option value="evm">EVM</option>
            <option value="btc">BTC</option>
            <option value="solana">SOL</option>
            <option value="sui">SUI</option>
            <option value="tron">TRX</option>
          </select>
          <input
            type="text"
            value={address}
            onChange={(e) => setAddress(e.target.value)}
            placeholder="Enter wallet address..."
            style={{ flex: 1, marginBottom: 0 }}
          />
          <button type="submit" className="btn" disabled={loading}>
            {loading ? 'CHECKING...' : 'CHECK'}
          </button>
        </form>

        {result && (
          <div>
            {result.error ? (
              <p className="text-error">ERROR: {result.error}</p>
            ) : (
              <>
                <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: '1rem' }}>
                  <span style={{ fontSize: '0.85rem', wordBreak: 'break-all' }}>{result.address}</span>
                  <span className={`status status-${result.status === 'safe' ? 'safe' : 'hacked'}`}>
                    {result.status?.toUpperCase()}
                  </span>
                </div>
                <div style={{ fontSize: '0.85rem' }}>
                  <div>Chain: {result.chain}</div>
                  <div>Seed/Key Found: {result.has_seed || result.has_pk ? 'YES' : 'NO'}</div>
                </div>
              </>
            )}
          </div>
        )}
      </div>

      <div className="grid grid-3">
        <div className="card">
          <h3>// SEED CHECK</h3>
          <p style={{ fontSize: '0.85rem' }}>Detects if private keys or seed phrases were leaked online</p>
        </div>
        <div className="card">
          <h3>// MULTICHAIN</h3>
          <p style={{ fontSize: '0.85rem' }}>EVM, Bitcoin, Solana, Sui, Tron supported</p>
        </div>
        <div className="card">
          <h3>// NO STORAGE</h3>
          <p style={{ fontSize: '0.85rem' }}>We never store your wallet seed phrases</p>
        </div>
      </div>
    </>
  )
}
