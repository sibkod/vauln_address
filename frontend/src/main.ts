import { createApp } from 'vue'
import { createRouter, createWebHistory } from 'vue-router'
import { ConnectionProvider, WalletProvider } from '@solana/wallet-adapter-react'
import { WalletModalProvider } from '@solana/wallet-adapter-react-ui'
import { PhantomWalletAdapter, SolflareWalletAdapter } from '@solana/wallet-adapter-wallets'
import { clusterApiUrl } from '@solana/web3.js'
import App from './App.vue'
import './style.css'

const USE_DEVNET = true
const endpoint = USE_DEVNET ? clusterApiUrl('devnet') : 'https://api.mainnet-beta.solana.com'
const wallets = [new PhantomWalletAdapter(), new SolflareWalletAdapter()]

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', name: 'home', component: () => import('./views/HomeView.vue') },
    { path: '/roadmap', name: 'roadmap', component: () => import('./views/RoadmapView.vue') },
    { path: '/about', name: 'about', component: () => import('./views/AboutView.vue') },
    { path: '/contact', name: 'contact', component: () => import('./views/ContactView.vue') },
    { path: '/support', name: 'support', component: () => import('./views/SupportView.vue') }
  ]
})

const app = createApp(App)
app.use(router)

// Wrap with Solana providers
app.mount('#app')
