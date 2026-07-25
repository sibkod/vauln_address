<script setup lang="ts">
import { ref } from 'vue'

const name = ref('')
const email = ref('')
const type = ref('')
const message = ref('')
const submitted = ref(false)

async function submit() {
  if (!name.value || !email.value || !type.value || !message.value) return
  
  try {
    await fetch('/api/contact', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        name: name.value,
        email: email.value,
        type: type.value,
        message: message.value
      })
    })
    submitted.value = true
    setTimeout(() => {
      name.value = ''
      email.value = ''
      type.value = ''
      message.value = ''
      submitted.value = false
    }, 3000)
  } catch {}
}
</script>

<template>
  <div class="content-card">
    <h2>Contact</h2>
    <p class="sub">Report a hacked wallet or suggest data.</p>
    
    <form @submit.prevent="submit">
      <div class="form-group">
        <label for="name">Name</label>
        <input v-model="name" type="text" id="name" placeholder="Your name" required />
      </div>
      <div class="form-group">
        <label for="email">Email</label>
        <input v-model="email" type="email" id="email" placeholder="you@email.com" required />
      </div>
      <div class="form-group">
        <label for="type">Type</label>
        <select v-model="type" id="type" required>
          <option value="">Select…</option>
          <option value="suggest">Suggest hacked wallet</option>
          <option value="report">Report issue</option>
          <option value="feedback">Feedback</option>
          <option value="other">Other</option>
        </select>
      </div>
      <div class="form-group">
        <label for="message">Message</label>
        <textarea v-model="message" id="message" placeholder="Describe the wallet or suggestion…" required></textarea>
      </div>
      <button type="submit" class="submit-btn">
        {{ submitted ? '✅ Sent!' : 'Send Message' }}
      </button>
    </form>
    
    <div style="margin-top:1.5rem; padding-top:1.5rem; border-top:1px solid rgba(255,255,255,0.04); display:flex; flex-direction:column; gap:0.4rem; color:#6a7ba0; font-size:0.8rem;">
      <div style="display:flex; align-items:center; gap:0.8rem;">
        <span>🐦</span><span>@WalletChecker</span>
      </div>
      <div style="display:flex; align-items:center; gap:0.8rem;">
        <span>📧</span><span>security@walletchecker.io</span>
      </div>
      <div style="display:flex; align-items:center; gap:0.8rem;">
        <span>💬</span><span>t.me/WalletChecker</span>
      </div>
    </div>
  </div>
</template>
