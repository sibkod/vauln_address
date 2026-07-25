export default function About() {
  return (
    <>
      <h1>// ABOUT</h1>
      <p className="text-dim mb-3">Wallet security checker service.</p>

      <div className="card mb-2">
        <div className="card-header">WHAT WE DO</div>
        <p>We check if wallet addresses have been compromised by searching through:</p>
        <ul style={{ color: 'var(--fg-dim)', paddingLeft: '1.5rem', marginTop: '0.5rem' }}>
          <li>Public data breaches</li>
          <li>GitHub commits with exposed keys</li>
          <li>Pastebin and similar dumps</li>
          <li>Social media posts</li>
        </ul>
      </div>

      <div className="card mb-2">
        <div className="card-header">HOW IT WORKS</div>
        <p className="mb-0">
          When you check a wallet, we compare the address against our database of known compromised wallets.
          If found, we show you the status and potential exposure sources.
        </p>
      </div>

      <div className="card mb-2">
        <div className="card-header">PRIVACY</div>
        <p className="mb-0">
          We do <strong>NOT</strong> store seed phrases or private keys.
          Our database only contains addresses that have been publicly leaked,
          linked to their source and discovery date.
        </p>
      </div>

      <div className="card">
        <div className="card-header">SUPPORT</div>
        <p className="mb-0">
          For questions or issues, contact us via the support channel.
        </p>
      </div>
    </>
  )
}
