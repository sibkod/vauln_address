import { BrowserRouter, Routes, Route, Link, useLocation } from 'react-router-dom'
import { useState, useEffect } from 'react'
import { WalletProviderComponent } from './components/WalletProvider'
import { WalletMultiButton } from '@solana/wallet-adapter-react-ui'
import Home from './pages/Home'
import Payment from './pages/Payment'
import About from './pages/About'
import Roadmap from './pages/Roadmap'

function ConnectButton() {
  const [mounted, setMounted] = useState(false)
  useEffect(() => { setMounted(true) }, [])
  if (!mounted) return null
  return <WalletMultiButton className="btn" />
}

function Navbar() {
  const location = useLocation()
  return (
    <nav>
      <Link to="/" className="brand">WALLET_CHECKER</Link>
      <div className="nav-links">
        <Link to="/" className={`nav-link ${location.pathname === '/' ? 'active' : ''}`}>HOME</Link>
        <Link to="/roadmap" className={`nav-link ${location.pathname === '/roadmap' ? 'active' : ''}`}>ROADMAP</Link>
        <Link to="/about" className={`nav-link ${location.pathname === '/about' ? 'active' : ''}`}>ABOUT</Link>
        <Link to="/payment" className={`nav-link ${location.pathname === '/payment' ? 'active' : ''}`}>BUY</Link>
      </div>
      <WalletProviderComponent>
        <ConnectButton />
      </WalletProviderComponent>
    </nav>
  )
}

function Footer() {
  return (
    <footer>
      WALLET_CHECKER v1.0 // WITHOUT SEED PHRASES STORED
    </footer>
  )
}

export default function App() {
  return (
    <BrowserRouter>
      <Navbar />
      <main>
        <Routes>
          <Route path="/" element={<Home />} />
          <Route path="/roadmap" element={<Roadmap />} />
          <Route path="/about" element={<About />} />
          <Route path="/payment" element={<Payment />} />
        </Routes>
      </main>
      <Footer />
    </BrowserRouter>
  )
}
