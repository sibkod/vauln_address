export default function Roadmap() {
  return (
    <>
      <h1>// ROADMAP</h1>
      <p className="text-dim mb-3">Development timeline and future plans.</p>

      <div className="card">
        <div className="card-header">DONE</div>
        <ul style={{ color: 'var(--success)', paddingLeft: '1.5rem' }}>
          <li>EVM wallet checking</li>
          <li>Bitcoin address validation</li>
          <li>Solana address support</li>
          <li>Sui blockchain integration</li>
          <li>Tron network support</li>
        </ul>
      </div>

      <div className="card">
        <div className="card-header">IN PROGRESS</div>
        <ul style={{ color: 'var(--warning)', paddingLeft: '1.5rem' }}>
          <li>Solana payment integration</li>
          <li>API key management</li>
          <li>User dashboard</li>
        </ul>
      </div>

      <div className="card">
        <div className="card-header">PLANNED</div>
        <ul style={{ color: 'var(--fg-dim)', paddingLeft: '1.5rem' }}>
          <li>Batch wallet checking</li>
          <li>Real-time monitoring</li>
          <li>Telegram bot alerts</li>
          <li>Portfolio tracking</li>
          <li>Mobile app</li>
          <li>Browser extension</li>
        </ul>
      </div>
    </>
  )
}
