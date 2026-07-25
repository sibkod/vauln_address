export default function About() {
  return (
    <div style={{ maxWidth: '800px', margin: '0 auto' }}>
      <h1 style={{ marginBottom: '2rem' }}>About</h1>

      <div className="card mb-2">
        <div className="card-header">What We Do</div>
        <p className="mb-0">We check if wallet addresses have been compromised by searching through:</p>
        <ul style={{ paddingLeft: '1.5rem', marginTop: '0.5rem', color: 'var(--text-secondary)' }}>
          <li>Public data breaches</li>
          <li>GitHub commits with exposed keys</li>
          <li>Pastebin and similar dumps</li>
          <li>Social media posts</li>
        </ul>
      </div>

      <div className="card mb-2">
        <div className="card-header">How It Works</div>
        <p className="mb-0">
          When you check a wallet, we compare the address against our database of known compromised wallets.
          If found, we show you the status and potential exposure sources.
        </p>
      </div>

      <div className="card mb-2">
        <div className="card-header">Privacy</div>
        <p className="mb-0">
          We do <strong>NOT</strong> store seed phrases or private keys.
          Our database only contains addresses that have been publicly leaked,
          linked to their source and discovery date.
        </p>
      </div>

      <div className="card">
        <div className="card-header">Support</div>
        <p className="mb-0">
          For questions or issues, contact us via the support channel.
        </p>
      </div>
    </div>
  )
}
