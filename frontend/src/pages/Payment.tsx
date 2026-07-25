import { useConnection, useWallet } from '@solana/wallet-adapter-react'
import { useWalletModal } from '@solana/wallet-adapter-react-ui'
import { useState, useEffect } from 'react'
import { PublicKey, Transaction, SystemProgram, LAMPORTS_PER_SOL } from '@solana/web3.js'
import '@solana/wallet-adapter-react-ui/styles.css'

const USE_DEVNET = true
const MERCHANT_ADDRESS = 'CW58CLARKr9mL4d7oRDj6FKv3cM2xT6vH3kQVZqW4xXy'

const EXPLORER = USE_DEVNET
  ? 'https://explorer.solana.com/?cluster=devnet'
  : 'https://explorer.solana.com/'

export default function Payment() {
  const { connection } = useConnection()
  const { publicKey, connected, sendTransaction } = useWallet()
  const { setVisible } = useWalletModal()

  const [merchantBalance, setMerchantBalance] = useState('Loading...')
  const [walletBalance, setWalletBalance] = useState<string | null>(null)
  const [status, setStatus] = useState('')
  const [txSig, setTxSig] = useState('')
  const [pkg, setPkg] = useState('100')

  const getBalance = async (addr: string) => {
    try {
      const lamports = await connection.getBalance(new PublicKey(addr))
      return (lamports / LAMPORTS_PER_SOL).toFixed(4)
    } catch {
      return 'Error'
    }
  }

  const refreshBalances = async () => {
    setMerchantBalance(await getBalance(MERCHANT_ADDRESS) + ' SOL')
    if (publicKey) {
      setWalletBalance(await getBalance(publicKey.toString()) + ' SOL')
    }
  }

  useEffect(() => {
    refreshBalances()
    const interval = setInterval(refreshBalances, 30000)
    return () => clearInterval(interval)
  }, [publicKey, connection])

  const handlePayment = async () => {
    if (!publicKey || !sendTransaction) return

    const amount = (parseInt(pkg) * 0.1).toFixed(4)

    try {
      setStatus('Creating transaction...')

      const tx = new Transaction().add(
        SystemProgram.transfer({
          fromPubkey: publicKey,
          toPubkey: new PublicKey(MERCHANT_ADDRESS),
          lamports: Math.floor(parseFloat(amount) * LAMPORTS_PER_SOL)
        })
      )

      const { blockhash } = await connection.getLatestBlockhash()
      tx.recentBlockhash = blockhash
      tx.feePayer = publicKey

      setStatus('Sign in wallet...')
      const sig = await sendTransaction(tx, connection)

      setStatus('Confirming...')
      await connection.confirmTransaction(sig, 'confirmed')

      setStatus('Payment confirmed!')
      setTxSig(sig)
      refreshBalances()
    } catch (err: any) {
      setStatus('Error: ' + (err.message || 'Unknown'))
    }
  }

  const packages = [
    { value: '10', label: '10 checks', price: '$1.00', sol: '0.05' },
    { value: '50', label: '50 checks', price: '$5.00', sol: '0.25' },
    { value: '100', label: '100 checks', price: '$10.00', sol: '0.5', popular: true },
    { value: '500', label: '500 checks', price: '$50.00', sol: '2.5' },
    { value: '1000', label: '1000 checks', price: '$100.00', sol: '5' },
  ]

  return (
    <div style={{ maxWidth: '1000px', margin: '0 auto' }}>
      <div className="hero" style={{ paddingBottom: '2rem' }}>
        <h1>Buy <span className="hero-gradient">Checks</span></h1>
        <p>Purchase wallet checks to verify unlimited addresses</p>
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(180px, 1fr))', gap: '1rem', marginBottom: '3rem' }}>
        {packages.map((p) => (
          <div 
            key={p.value} 
            className={`card ${p.popular ? 'popular' : ''}`}
            style={{ textAlign: 'center', cursor: 'pointer' }}
            onClick={() => setPkg(p.value)}
          >
            {p.popular && <div style={{ fontSize: '0.6875rem', color: 'var(--accent)', marginBottom: '0.5rem', fontWeight: 600 }}>POPULAR</div>}
            <div style={{ fontSize: '1.5rem', fontWeight: 700 }}>{p.label}</div>
            <div style={{ color: 'var(--text-secondary)', marginTop: '0.5rem' }}>{p.price}</div>
            <div style={{ fontSize: '0.8125rem', color: 'var(--text-muted)', marginTop: '0.25rem' }}>~{p.sol} SOL</div>
          </div>
        ))}
      </div>

      <div className="grid grid-2">
        <div>
          <div className="card">
            <div className="card-header">Your Wallet</div>
            {!connected ? (
              <button className="btn" onClick={() => setVisible(true)} style={{ width: '100%' }}>
                Connect Wallet
              </button>
            ) : (
              <>
                <div className="info-row">
                  <span className="label">Address</span>
                  <span className="value" style={{ fontSize: '0.8125rem' }}>
                    {publicKey?.toString().slice(0, 8)}...{publicKey?.toString().slice(-8)}
                  </span>
                </div>
                <div className="info-row">
                  <span className="label">Balance</span>
                  <span className="value">{walletBalance || 'Loading...'}</span>
                </div>
              </>
            )}
          </div>

          <div className="card" style={{ marginTop: '1.5rem' }}>
            <div className="card-header">Selected: {packages.find(p => p.value === pkg)?.label}</div>
            <div style={{ marginBottom: '1rem' }}>
              <div className="info-row">
                <span className="label">Amount</span>
                <span className="value">{packages.find(p => p.value === pkg)?.sol} SOL</span>
              </div>
            </div>
            <button
              className="btn btn-lg"
              onClick={handlePayment}
              disabled={!connected || !!status.includes('Sign')}
              style={{ width: '100%' }}
            >
              {!connected ? 'Connect to Buy' : status || 'Pay with SOL'}
            </button>
            {txSig && (
              <a
                href={`${EXPLORER}tx/${txSig}`}
                target="_blank"
                rel="noopener noreferrer"
                style={{ marginTop: '1rem', display: 'block', textAlign: 'center' }}
              >
                View transaction →
              </a>
            )}
          </div>
        </div>

        <div>
          <div className="card">
            <div className="card-header">Merchant Address</div>
            <div style={{ 
              background: 'var(--bg-primary)', 
              padding: '1rem', 
              borderRadius: '8px', 
              marginBottom: '1rem', 
              fontSize: '0.8125rem', 
              wordBreak: 'break-all',
              fontFamily: 'monospace'
            }}>
              {MERCHANT_ADDRESS}
            </div>
            <div className="info-row">
              <span className="label">Network</span>
              <span className="value">{USE_DEVNET ? 'Devnet' : 'Mainnet'}</span>
            </div>
            <div className="info-row">
              <span className="label">Balance</span>
              <span className="value">{merchantBalance}</span>
            </div>
          </div>

          <div className="card" style={{ marginTop: '1.5rem' }}>
            <div className="card-header">How It Works</div>
            <ol style={{ paddingLeft: '1.25rem', margin: 0, color: 'var(--text-secondary)', fontSize: '0.875rem' }}>
              <li style={{ marginBottom: '0.5rem' }}>Connect your Solana wallet</li>
              <li style={{ marginBottom: '0.5rem' }}>Select number of checks</li>
              <li style={{ marginBottom: '0.5rem' }}>Send SOL to merchant address</li>
              <li>Checks added to your account</li>
            </ol>
          </div>
        </div>
      </div>
    </div>
  )
}
