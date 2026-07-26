<template>
  <div class="roadmap-container">
    <div class="roadmap-header">
      <h1>Roadmap</h1>
      <p class="roadmap-sub">Building comprehensive blockchain security intelligence</p>
    </div>
    
    <div class="roadmap-timeline">
      <!-- Phase 1: Completed -->
      <div class="roadmap-phase completed">
        <div class="phase-marker">
          <span class="phase-icon">✓</span>
          <span class="phase-number">Phase 1</span>
        </div>
        <div class="phase-content">
          <h3>Foundation & Core Features</h3>
          <p class="phase-description">Basic wallet vulnerability checking with rate limiting and authentication</p>
          <ul>
            <li>✓ Multi-chain support: EVM, BTC, Solana, Sui, Tron</li>
            <li>✓ Rate limiting with IP-based free checks</li>
            <li>✓ Phantom wallet integration (Solana)</li>
            <li>✓ User authentication via Web3 signatures</li>
            <li>✓ Purchased check balance system</li>
            <li>✓ Rate limit headers (X-RateLimit-*)</li>
            <li>✓ /api/me endpoint for user info polling</li>
            <li>✓ Purchase history with pagination</li>
            <li>✓ Admin API for wallet database management</li>
          </ul>
          <span class="phase-date">Completed · Q1 2026</span>
        </div>
      </div>

      <!-- Phase 2: In Progress -->
      <div class="roadmap-phase in-progress">
        <div class="phase-marker">
          <span class="phase-icon">⟳</span>
          <span class="phase-number">Phase 2</span>
        </div>
        <div class="phase-content">
          <h3>Extended Wallet Support</h3>
          <p class="phase-description">Adding more wallet options and improving user experience</p>
          <ul>
            <li class="in-progress-item">⏳ Integrate MetaMask wallet (EVM chains)</li>
            <li class="in-progress-item">⏳ Integrate WalletConnect for mobile wallets</li>
            <li class="in-progress-item">⏳ Integrate Backpack wallet (Solana)</li>
            <li class="in-progress-item">⏳ Integrate Coinbase Wallet</li>
            <li class="planned">○ Multi-wallet session management</li>
            <li class="planned">○ Unified balance across connected wallets</li>
          </ul>
          <span class="phase-date">In progress · Q2 2026</span>
          <div class="progress-bar"><div class="progress-fill" style="width:25%;"></div></div>
        </div>
      </div>

      <!-- Phase 3: Planned -->
      <div class="roadmap-phase planned">
        <div class="phase-marker">
          <span class="phase-icon">◈</span>
          <span class="phase-number">Phase 3</span>
        </div>
        <div class="phase-content">
          <h3>Hacker Address Tracking</h3>
          <p class="phase-description">Identifying and tracking hacker wallets to trace stolen funds</p>
          <ul>
            <li class="planned">○ Hacker wallet database with attribution</li>
            <li class="planned">○ Track fund flows from compromised wallets</li>
            <li class="planned">○ Cross-chain transaction analysis</li>
            <li class="planned">○ Visualize fund movement chains</li>
            <li class="planned">○ Link multiple hacker addresses (clustering)</li>
            <li class="planned">○ Real-time alerts for new hacker activity</li>
            <li class="planned">○ Integration with law enforcement databases</li>
          </ul>
          <div class="sub-details">
            <h4>Technical Implementation:</h4>
            <ul>
              <li>Graph-based transaction analysis</li>
              <li>Pattern recognition for mixer/tumbler usage</li>
              <li>CEX deposit address identification</li>
              <li>Automatic wallet labeling via on-chain heuristics</li>
            </ul>
          </div>
          <span class="phase-date">Planned · Q3 2026</span>
        </div>
      </div>

      <!-- Phase 4: Future -->
      <div class="roadmap-phase future">
        <div class="phase-marker">
          <span class="phase-icon">◇</span>
          <span class="phase-number">Phase 4</span>
        </div>
        <div class="phase-content">
          <h3>Malicious Contract Detection</h3>
          <p class="phase-description">Identifying honeypots, drainers, and malicious smart contracts</p>
          <ul>
            <li class="future">☆ Automated honeypot contract detection</li>
            <li class="future">☆ Drainer-as-a-Service (DaaS) identification</li>
            <li class="future">☆ Flash loan attack pattern recognition</li>
            <li class="future">☆ Front-run bot detection</li>
            <li class="future">☆ Rug-pull indicator system</li>
            <li class="future">☆ Contract owner privilege analysis</li>
            <li class="future">☆ Token approval scanner</li>
          </ul>
          <div class="sub-details">
            <h4>Analysis Methods:</h4>
            <ul>
              <li>Static analysis of contract bytecode</li>
              <li>Simulation of contract interactions</li>
              <li>Ownership and proxy pattern detection</li>
              <li>Transaction sequence analysis</li>
              <li>Community-reported threat intelligence</li>
            </ul>
          </div>
          <span class="phase-date">Future · Q4 2026</span>
        </div>
      </div>

      <!-- Phase 5: Vision -->
      <div class="roadmap-phase future vision">
        <div class="phase-marker">
          <span class="phase-icon">◉</span>
          <span class="phase-number">Phase 5</span>
        </div>
        <div class="phase-content">
          <h3>AI-Powered Security</h3>
          <p class="phase-description">Advanced threat detection using machine learning</p>
          <ul>
            <li class="future">☆ ML-based risk scoring for any address</li>
            <li class="future">☆ Behavioral analysis of wallet activity</li>
            <li class="future">☆ Predictive breach detection</li>
            <li class="future">☆ Natural language threat reports</li>
            <li class="future">☆ Anomaly detection in fund movements</li>
            <li class="future">☆ Automated security audits</li>
          </ul>
          <span class="phase-date">Future · 2027</span>
        </div>
      </div>
    </div>

    <div class="contribute-section">
      <h3>🤝 Contribute to Security</h3>
      <p>Help us build a safer Web3 by reporting new threats and vulnerabilities.</p>
      <div class="contribute-cta">
        <RouterLink to="/contact" class="btn-primary">Report a Threat</RouterLink>
        <RouterLink to="/about" class="btn-secondary">Learn More</RouterLink>
      </div>
    </div>
  </div>
</template>

<style scoped>
.roadmap-container {
  max-width: 900px;
  margin: 0 auto;
  padding: 2rem 1rem;
}

.roadmap-header {
  text-align: center;
  margin-bottom: 3rem;
}

.roadmap-header h1 {
  font-size: 2.5rem;
  font-weight: 700;
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  margin-bottom: 0.5rem;
}

.roadmap-sub {
  color: #6b7a9e;
  font-size: 1.1rem;
}

.roadmap-timeline {
  position: relative;
}

.roadmap-phase {
  display: flex;
  gap: 1.5rem;
  margin-bottom: 2.5rem;
  position: relative;
}

.roadmap-phase::before {
  content: '';
  position: absolute;
  left: 24px;
  top: 60px;
  bottom: -20px;
  width: 2px;
  background: #2a3548;
}

.roadmap-phase:last-child::before {
  display: none;
}

.phase-marker {
  flex-shrink: 0;
  width: 50px;
  height: 50px;
  border-radius: 50%;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  z-index: 1;
}

.completed .phase-marker {
  background: linear-gradient(135deg, #4bc9a0, #38a169);
  color: white;
}

.in-progress .phase-marker {
  background: linear-gradient(135deg, #667eea, #764ba2);
  color: white;
  animation: pulse 2s infinite;
}

.planned .phase-marker {
  background: #2a3548;
  color: #6b7a9e;
  border: 2px dashed #4a5568;
}

.future .phase-marker {
  background: #1a1f2e;
  color: #4a5568;
  border: 2px solid #2a3548;
}

@keyframes pulse {
  0%, 100% { box-shadow: 0 0 0 0 rgba(102, 126, 234, 0.4); }
  50% { box-shadow: 0 0 0 10px rgba(102, 126, 234, 0); }
}

.phase-icon {
  font-size: 1.2rem;
}

.phase-number {
  font-size: 0.6rem;
  font-weight: 600;
}

.phase-content {
  flex: 1;
  background: #1a1f2e;
  border: 1px solid #2a3548;
  border-radius: 12px;
  padding: 1.5rem;
}

.completed .phase-content {
  border-color: #4bc9a040;
}

.in-progress .phase-content {
  border-color: #667eea40;
}

.phase-content h3 {
  color: #e7ecf5;
  font-size: 1.3rem;
  margin-bottom: 0.5rem;
}

.phase-description {
  color: #6b7a9e;
  font-size: 0.9rem;
  margin-bottom: 1rem;
}

.phase-content ul {
  list-style: none;
  padding: 0;
  margin: 0;
}

.phase-content li {
  color: #98a8ce;
  padding: 0.4rem 0;
  padding-left: 1.5rem;
  position: relative;
  font-size: 0.95rem;
}

.phase-content li::before {
  content: '•';
  position: absolute;
  left: 0;
  color: #4bc9a0;
}

.completed li::before {
  color: #4bc9a0;
}

.in-progress-item::before {
  content: '⏳';
  color: #667eea;
}

.planned::before {
  content: '○';
  color: #6b7a9e;
}

.future::before {
  content: '☆';
  color: #4a5568;
}

.phase-date {
  display: inline-block;
  margin-top: 1rem;
  padding: 0.3rem 0.8rem;
  background: #151a24;
  border-radius: 20px;
  color: #6b7a9e;
  font-size: 0.8rem;
  font-weight: 500;
}

.progress-bar {
  margin-top: 1rem;
  height: 4px;
  background: #2a3548;
  border-radius: 2px;
  overflow: hidden;
}

.progress-fill {
  height: 100%;
  background: linear-gradient(90deg, #667eea, #764ba2);
  border-radius: 2px;
  transition: width 0.3s ease;
}

.sub-details {
  margin-top: 1.5rem;
  padding-top: 1rem;
  border-top: 1px solid #2a3548;
}

.sub-details h4 {
  color: #98a8ce;
  font-size: 0.9rem;
  margin-bottom: 0.5rem;
}

.sub-details ul {
  font-size: 0.85rem;
  color: #6b7a9e;
}

.sub-details li::before {
  content: '→';
  color: #4a5568;
}

.contribute-section {
  margin-top: 4rem;
  text-align: center;
  padding: 2rem;
  background: linear-gradient(135deg, #1a1f2e 0%, #1a2233 100%);
  border: 1px solid #2a3548;
  border-radius: 16px;
}

.contribute-section h3 {
  color: #e7ecf5;
  font-size: 1.5rem;
  margin-bottom: 0.5rem;
}

.contribute-section p {
  color: #6b7a9e;
  margin-bottom: 1.5rem;
}

.contribute-cta {
  display: flex;
  gap: 1rem;
  justify-content: center;
}

.btn-primary {
  padding: 0.75rem 1.5rem;
  background: linear-gradient(135deg, #667eea, #764ba2);
  color: white;
  text-decoration: none;
  border-radius: 8px;
  font-weight: 500;
  transition: transform 0.2s, box-shadow 0.2s;
}

.btn-primary:hover {
  transform: translateY(-2px);
  box-shadow: 0 4px 12px rgba(102, 126, 234, 0.3);
}

.btn-secondary {
  padding: 0.75rem 1.5rem;
  background: transparent;
  color: #98a8ce;
  text-decoration: none;
  border-radius: 8px;
  font-weight: 500;
  border: 1px solid #2a3548;
  transition: all 0.2s;
}

.btn-secondary:hover {
  border-color: #667eea;
  color: #667eea;
}
</style>
