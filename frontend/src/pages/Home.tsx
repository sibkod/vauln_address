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
      <div className="hero">
        <h1>
          <span className="hero-gradient">Check if your wallet</span>
          <br />
          has been compromised
        </h1>
        <p>
          Protect your assets by checking if your wallet address has been exposed in data leaks or security breaches.
        </p>
      </div>

      <div className="card" style={{ maxWidth: '700px', margin: '0 auto' }}>
        <form onSubmit={handleCheck}>
          <div style={{ display: 'flex', gap: '1rem', marginBottom: '1rem' }}>
            <select 
              value={chain} 
              onChange={(e) => setChain(e.target.value)}
              style={{ width: '140px', flexShrink: 0 }}
            >
              <option value="evm">EVM</option>
              <option value="btc">Bitcoin</option>
              <option value="solana">Solana</option>
              <option value="sui">Sui</option>
              <option value="tron">Tron</option>
            </select>
            <input
              type="text"
              value={address}
              onChange={(e) => setAddress(e.target.value)}
              placeholder="Enter wallet address..."
              style={{ flex: 1, marginBottom: 0 }}
            />
          </div>
          <button type="submit" className="btn btn-lg" disabled={loading} style={{ width: '100%' }}>
            {loading ? 'Checking...' : 'Check Wallet'}
          </button>
        </form>

        {result && (
          <div style={{ marginTop: '1.5rem', paddingTop: '1.5rem', borderTop: '1px solid var(--border)' }}>
            {result.error ? (
              <p className="text-error">{result.error}</p>
            ) : (
              <>
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '1rem' }}>
                  <span style={{ fontSize: '0.8125rem', color: 'var(--text-muted)' }}>{result.address}</span>
                  <span className={`status status-${result.status === 'safe' ? 'safe' : 'hacked'}`}>
                    {result.status === 'safe' ? 'Safe' : 'Compromised'}
                  </span>
                </div>
                <div style={{ fontSize: '0.875rem', color: 'var(--text-secondary)' }}>
                  <div style={{ display: 'flex', justifyContent: 'space-between', padding: '0.5rem 0' }}>
                    <span>Chain</span>
                    <span>{result.chain?.toUpperCase()}</span>
                  </div>
                  <div style={{ display: 'flex', justifyContent: 'space-between', padding: '0.5rem 0', borderTop: '1px solid var(--border)' }}>
                    <span>Seed/Key Found</span>
                    <span className={result.has_seed || result.has_pk ? 'text-error' : 'text-success'}>
                      {result.has_seed || result.has_pk ? 'Yes' : 'No'}
                    </span>
                  </div>
                </div>
              </>
            )}
          </div>
        )}
      </div>

      <div className="grid grid-3" style={{ marginTop: '4rem' }}>
        <div className="card feature-card">
          <div className="feature-icon">🔐</div>
          <h3>Seed Check</h3>
          <p>Detects if private keys or seed phrases were leaked online</p>
        </div>
        <div className="card feature-card">
          <div className="feature-icon">🌐</div>
          <h3>Multichain</h3>
          <p>EVM, Bitcoin, Solana, Sui, Tron supported</p>
        </div>
        <div className="card feature-card">
          <div className="feature-icon">🔒</div>
          <h3>No Storage</h3>
          <p>We never store your wallet seed phrases</p>
        </div>
      </div>
    </>
  )
}
