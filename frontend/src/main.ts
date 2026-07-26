import { createApp } from 'vue'
import { createRouter, createWebHistory } from 'vue-router'
import App from './App.vue'
import './style.css'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', name: 'home', component: () => import('./views/HomeView.vue') },
    { path: '/pricing', name: 'pricing', component: () => import('./views/PricingView.vue') },
    { path: '/roadmap', name: 'roadmap', component: () => import('./views/RoadmapView.vue') },
    { path: '/about', name: 'about', component: () => import('./views/AboutView.vue') },
    { path: '/contact', name: 'contact', component: () => import('./views/ContactView.vue') },
    { path: '/support', name: 'support', component: () => import('./views/SupportView.vue') },
    { path: '/purchases', name: 'purchases', component: () => import('./views/PurchasesView.vue') },
    { path: '/checks', name: 'checks', component: () => import('./views/ChecksView.vue') },
    { path: '/backend-error', name: 'backend-error', component: () => import('./views/BackendErrorView.vue') },
    { path: '/:pathMatch(.*)*', name: 'not-found', component: () => import('./views/NotFoundView.vue') }
  ]
})

createApp(App).use(router).mount('#app')
