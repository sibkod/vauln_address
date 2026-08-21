<script setup lang="ts">
import { ref, computed, inject, onMounted } from 'vue'

const apiBase = inject<string>('apiBase', '')
const wallet = inject<any>('wallet')

const chains = ['solana', 'evm', 'btc', 'sui', 'tron']

const txSignature = ref('')
const chain = ref('solana')
const siteUrl = ref('')
const description = ref('')

const captchaId = ref('')
const captchaImage = ref('')
const captchaAnswer = ref('')
const captchaLoading = ref(false)

const submitting = ref(false)
const error = ref('')
const errorCode = ref('')
const successId = ref<number | null>(null)
const telegramSent = ref(false)

const isConnected = computed(() => wallet?.connected?.value || false)

async function loadCaptcha() {
  captchaLoading.value = true
  captchaAnswer.value = ''
  try {
    const res = await fetch(`${apiBase}/api/captcha`)
    if (!res.ok) throw new Error('captcha request failed')
    const data = await res.json()
    captchaId.value = data.captcha_id
    captchaImage.value = data.image
  } catch {
    error.value = 'Failed to load captcha. Is the backend running?'
  } finally {
    captchaLoading.value = false
  }
}

const canSubmit = computed(() =>
  txSignature.value.trim().length >= 40 &&
  captchaAnswer.value.trim().length >= 4 &&
  !submitting.value
)

async function submit() {
  error.value = ''
  errorCode.value = ''
  submitting.value = true

  const headers: Record<string, string> = { 'Content-Type': 'application/json' }
  if (wallet?.authToken?.value) {
    headers['Authorization'] = `Bearer ${wallet.authToken.value}`
  }

  try {
    const res = await fetch(`${apiBase}/api/drainer-reports`, {
      method: 'POST',
      headers,
      body: JSON.stringify({
        tx_signature: txSignature.value.trim(),
        chain: chain.value,
        site_url: siteUrl.value.trim(),
        description: description.value.trim(),
        captcha_id: captchaId.value,
        captcha_answer: captchaAnswer.value.trim()
      })
    })
    const data = await res.json()
    if (!res.ok) {
      errorCode.value = data.code || 'ERROR'
      error.value = data.details || data.error || 'Submission failed'
      if (errorCode.value === 'CAPTCHA_INVALID') loadCaptcha()
      return
    }
    successId.value = data.id
    telegramSent.value = !!data.telegram_sent
  } catch {
    error.value = 'Request failed. Is the backend running?'
  } finally {
    submitting.value = false
  }
}

function resetForm() {
  txSignature.value = ''
  siteUrl.value = ''
  description.value = ''
  successId.value = null
  telegramSent.value = false
  loadCaptcha()
}

onMounted(loadCaptcha)
</script>

<template>
  <div class="drainer-report-page">
    <div class="page-header">
      <h1>Report a Drainer</h1>
      <p class="subtitle">
        Were your funds stolen by a drainer? Submit the theft transaction and any details —
        our analysts will review it and add the attackers to the threat database.
      </p>
    </div>

    <div v-if="successId !== null" class="success-box">
      <div class="success-icon">✅</div>
      <h2>Report #{{ successId }} submitted</h2>
      <p>
        Thank you! The report has been saved and
        <span v-if="telegramSent">forwarded to our analysts on Telegram.</span>
        <span v-else>queued for analysis.</span>
      </p>
      <button class="btn-secondary" @click="resetForm">Submit another report</button>
    </div>

    <form v-else class="report-form" @submit.prevent="submit">
      <label class="field">
        <span class="field-label">Theft transaction signature / hash <em>*</em></span>
        <span class="field-hint">The transaction in which coins were stolen from your wallet</span>
        <input
          v-model="txSignature"
          type="text"
          placeholder="e.g. 5Kd4… (Solana signature) or 0x… (EVM hash)"
          maxlength="120"
          required
        />
      </label>

      <label class="field">
        <span class="field-label">Chain</span>
        <select v-model="chain">
          <option v-for="c in chains" :key="c" :value="c">{{ c.toUpperCase() }}</option>
        </select>
      </label>

      <label class="field">
        <span class="field-label">Scam website</span>
        <span class="field-hint">The phishing site where you signed the transaction (if known)</span>
        <input
          v-model="siteUrl"
          type="text"
          placeholder="https://scam-site.example"
          maxlength="300"
        />
      </label>

      <label class="field">
        <span class="field-label">Additional information</span>
        <textarea
          v-model="description"
          rows="4"
          maxlength="2000"
          placeholder="What happened? Which tokens were stolen? Any other details that help the investigation."
        ></textarea>
      </label>

      <div class="field captcha-field">
        <span class="field-label">Captcha <em>*</em></span>
        <div class="captcha-row">
          <div class="captcha-image" :class="{ loading: captchaLoading }">
            <img v-if="captchaImage" :src="captchaImage" alt="captcha" />
            <span v-else>…</span>
          </div>
          <button type="button" class="captcha-refresh" @click="loadCaptcha" title="New captcha">⟳</button>
          <input
            v-model="captchaAnswer"
            type="text"
            class="captcha-input"
            placeholder="Code from image"
            maxlength="6"
            autocomplete="off"
            required
          />
        </div>
      </div>

      <div v-if="error" class="form-error">{{ error }}</div>

      <button type="submit" class="submit-btn" :disabled="!canSubmit">
        {{ submitting ? 'Submitting…' : '🚨 Submit report' }}
      </button>

      <p class="privacy-note">
        Reports are sent to our analysts{{ isConnected ? '' : ' anonymously' }}.
        {{ isConnected ? '' : 'Connect a wallet to link the report to your account.' }}
      </p>
    </form>
  </div>
</template>

<style scoped>
.drainer-report-page {
  max-width: 640px;
  margin: 0 auto;
  padding: 2rem 1rem;
}

.page-header {
  text-align: center;
  margin-bottom: 1.75rem;
}

.page-header h1 {
  font-size: 2rem;
  font-weight: 700;
  background: linear-gradient(135deg, #ff6b6b 0%, #c0392b 100%);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  margin-bottom: 0.5rem;
}

.subtitle {
  color: #6b7a9e;
  font-size: 0.95rem;
  line-height: 1.5;
}

.report-form {
  background: #1a1f2e;
  border: 1px solid #2a3548;
  border-radius: 12px;
  padding: 1.5rem;
  display: flex;
  flex-direction: column;
  gap: 1.1rem;
}

.field {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
}

.field-label {
  color: #e7ecf5;
  font-size: 0.9rem;
  font-weight: 600;
}

.field-label em {
  color: #ff6b6b;
  font-style: normal;
}

.field-hint {
  color: #4c5a7a;
  font-size: 0.78rem;
}

.field input,
.field select,
.field textarea {
  background: #0c111b;
  border: 1px solid #2a3548;
  border-radius: 8px;
  color: #e7ecf5;
  padding: 0.65rem 0.8rem;
  font-size: 0.9rem;
  font-family: inherit;
  transition: border-color 0.2s;
}

.field input:focus,
.field select:focus,
.field textarea:focus {
  outline: none;
  border-color: #667eea;
}

.field input {
  font-family: monospace;
}

.field textarea {
  resize: vertical;
  font-family: inherit;
}

.captcha-row {
  display: flex;
  align-items: center;
  gap: 0.6rem;
}

.captcha-image {
  width: 220px;
  height: 64px;
  border-radius: 8px;
  border: 1px solid #2a3548;
  overflow: hidden;
  display: flex;
  align-items: center;
  justify-content: center;
  background: #0c111b;
  flex-shrink: 0;
}

.captcha-image.loading { opacity: 0.4; }
.captcha-image img { display: block; }

.captcha-refresh {
  background: #1a2233;
  border: 1px solid #2a3548;
  color: #c7d2e8;
  border-radius: 8px;
  width: 40px;
  height: 40px;
  font-size: 1.1rem;
  cursor: pointer;
  flex-shrink: 0;
  transition: all 0.2s;
}

.captcha-refresh:hover {
  border-color: #667eea;
  color: #e7ecf5;
}

.captcha-input {
  flex: 1;
  min-width: 0;
  letter-spacing: 0.2em;
  text-transform: uppercase;
}

.form-error {
  background: rgba(255, 107, 107, 0.1);
  border: 1px solid rgba(255, 107, 107, 0.35);
  color: #ff6b6b;
  border-radius: 8px;
  padding: 0.6rem 0.8rem;
  font-size: 0.85rem;
}

.submit-btn {
  background: linear-gradient(135deg, #c0392b, #ff6b6b);
  border: none;
  color: #fff;
  border-radius: 10px;
  padding: 0.8rem 1.6rem;
  font-size: 1rem;
  font-weight: 600;
  cursor: pointer;
  transition: all 0.2s;
}

.submit-btn:hover:not(:disabled) {
  filter: brightness(1.1);
}

.submit-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.privacy-note {
  color: #4c5a7a;
  font-size: 0.78rem;
  text-align: center;
  margin: 0;
}

.success-box {
  background: #1a1f2e;
  border: 1px solid rgba(75, 201, 160, 0.4);
  border-radius: 12px;
  padding: 2.5rem 2rem;
  text-align: center;
}

.success-icon {
  font-size: 2.5rem;
  margin-bottom: 0.75rem;
}

.success-box h2 {
  color: #e7ecf5;
  margin: 0 0 0.5rem;
}

.success-box p {
  color: #98a8ce;
  margin: 0 0 1.25rem;
}

.btn-secondary {
  background: #1a2233;
  border: 1px solid #2a3548;
  color: #c7d2e8;
  border-radius: 8px;
  padding: 0.6rem 1.2rem;
  font-size: 0.9rem;
  cursor: pointer;
  transition: all 0.2s;
}

.btn-secondary:hover {
  border-color: #667eea;
  color: #e7ecf5;
}
</style>
