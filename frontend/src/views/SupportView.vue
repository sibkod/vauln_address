<script setup lang="ts">
</script>

<template>
  <div class="support-container">
    <div class="support-header">
      <h1>Support</h1>
    </div>

    <div class="coming-soon">
      <div class="coming-soon-icon">🚀</div>
      <h2>Coming Soon</h2>
      <p>Support options will be available soon.</p>
    </div>
  </div>
</template>

<style scoped>
.support-container {
  max-width: 1100px;
  margin: 0 auto;
  padding: 2rem 1rem;
  text-align: center;
}

.support-header {
  margin-bottom: 3rem;
}

.support-header h1 {
  font-size: 2.5rem;
  font-weight: 700;
  background: linear-gradient(135deg, #00e5ff 0%, #7b5cff 100%);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
}

.coming-soon {
  background: rgba(13, 18, 32, 0.88);
  border: 1px solid rgba(0, 229, 255, 0.1);
  border-radius: 16px;
  padding: 4rem 2rem;
}

.coming-soon-icon {
  font-size: 4rem;
  margin-bottom: 1rem;
}

.coming-soon h2 {
  color: #e7ecf5;
  font-size: 1.5rem;
  margin-bottom: 0.5rem;
}

.coming-soon p {
  color: #6b7a9e;
  font-size: 1rem;
}
</style>
