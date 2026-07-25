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
    { value: '10', label: '10 checks - $1.00', sol: '0.05' },
    { value: '50', label: '50 checks - $5.00', sol: '0.25' },
    { value: '100', label: '100 checks - $10.00', sol: '0.5' },
    { value: '500', label: '500 checks - $50.00', sol: '2.5' },
    { value: '1000', label: '1000 checks - $100.00', sol: '5' },
  ]

  return (
    <>
      <h1>// PAYMENT</h1>
      <p className="text-dim mb-3">Buy checks to verify unlimited wallets.</p>

      <div className="grid grid-2">
        <div>
          <div className="card">
            <div className="card-header">CONNECTED WALLET</div>
            {!connected ? (
              <button className="btn" onClick={() => setVisible(true)} style={{ width: '100%' }}>
                CONNECT WALLET
              </button>
            ) : (
              <>
                <div className="info-row">
                  <span className="text-dim">Address</span>
                  <span style={{ fontSize: '0.85rem' }}>
                    {publicKey?.toString().slice(0, 8)}...{publicKey?.toString().slice(-8)}
                  </span>
                </div>
                <div className="info-row">
                  <span className="text-dim">Balance</span>
                  <span>{walletBalance || 'Loading...'}</span>
                </div>
              </>
            )}
          </div>

          <div className="card">
            <div className="card-header">SELECT PACKAGE</div>
            <select value={pkg} onChange={(e) => setPkg(e.target.value)} className="mb-2">
              {packages.map((p) => (
                <option key={p.value} value={p.value}>
                  {p.label} (~{p.sol} SOL)
                </option>
              ))}
            </select>
            <button
              className="btn"
              onClick={handlePayment}
              disabled={!connected || !!status.includes('Sign')}
              style={{ width: '100%' }}
            >
              {!connected ? 'CONNECT TO BUY' : status || 'PAY WITH SOL'}
            </button>
            {txSig && (
              <a
                href={`${EXPLORER}tx/${txSig}`}
                target="_blank"
                rel="noopener noreferrer"
                style={{ color: 'var(--fg)', marginTop: '1rem', display: 'block' }}
              >
                View transaction
              </a>
            )}
          </div>
        </div>

        <div>
          <div className="card">
            <div className="card-header">MERCHANT ADDRESS</div>
            <div style={{ background: '#111', padding: '1rem', marginBottom: '1rem', fontSize: '0.85rem', wordBreak: 'break-all' }}>
              {MERCHANT_ADDRESS}
            </div>
            <div className="info-row">
              <span className="text-dim">Network</span>
              <span>{USE_DEVNET ? 'DEVNET' : 'MAINNET'}</span>
            </div>
            <div className="info-row">
              <span className="text-dim">Balance</span>
              <span>{merchantBalance}</span>
            </div>
          </div>

          <div className="card">
            <div className="card-header">HOW IT WORKS</div>
            <ol style={{ color: 'var(--fg-dim)', paddingLeft: '1.5rem', fontSize: '0.85rem' }}>
              <li style={{ marginBottom: '0.5rem' }}>Connect your Solana wallet</li>
              <li style={{ marginBottom: '0.5rem' }}>Select number of checks</li>
              <li style={{ marginBottom: '0.5rem' }}>Send SOL to merchant address</li>
              <li>Checks added to your balance</li>
            </ol>
          </div>
        </div>
      </div>

      <style>{`
        .info-row {
          display: flex;
          justify-content: space-between;
          padding: 0.5rem 0;
          border-bottom: 1px solid #111;
        }
        .info-row:last-child {
          border-bottom: none;
        }
      `}</style>
    </>
  )
}
