export default function Roadmap() {
  return (
    <div style={{ maxWidth: '800px', margin: '0 auto' }}>
      <h1 style={{ marginBottom: '2rem' }}>Roadmap</h1>

      <div className="card mb-2">
        <div className="card-header">Completed</div>
        <ul style={{ paddingLeft: '1.5rem', margin: 0, color: 'var(--text-secondary)' }}>
          <li>EVM wallet checking</li>
          <li>Bitcoin address validation</li>
          <li>Solana address support</li>
          <li>Sui blockchain integration</li>
          <li>Tron network support</li>
        </ul>
      </div>

      <div className="card mb-2">
        <div className="card-header">In Progress</div>
        <ul style={{ paddingLeft: '1.5rem', margin: 0, color: 'var(--text-secondary)' }}>
          <li>Solana payment integration</li>
          <li>API key management</li>
          <li>User dashboard</li>
        </ul>
      </div>

      <div className="card">
        <div className="card-header">Planned</div>
        <ul style={{ paddingLeft: '1.5rem', margin: 0, color: 'var(--text-secondary)' }}>
          <li>Batch wallet checking</li>
          <li>Real-time monitoring</li>
          <li>Telegram bot alerts</li>
          <li>Portfolio tracking</li>
          <li>Mobile app</li>
          <li>Browser extension</li>
        </ul>
      </div>
    </div>
  )
}
