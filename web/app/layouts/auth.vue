<script setup lang="ts">
// Layout halaman auth: fokus, tanpa sidebar, pesan brand di samping.
</script>

<template>
  <div class="auth-layout">
    <section class="auth-aside">
      <img
        src="~/assets/brand/logo.svg"
        alt="Angkut"
        height="32"
      >
      <h1 class="auth-tagline">
        Ship smarter.<br>
        Grow faster.
      </h1>
      <p class="auth-sub">
        Bandingkan kurir, kirim, lacak, dan kelola COD dalam satu tempat —
        dengan asisten cerdas yang bekerja untuk Anda.
      </p>
    </section>

    <section class="auth-main">
      <div class="auth-card card">
        <slot />
      </div>
    </section>
  </div>
</template>

<style scoped>
.auth-layout {
  min-height: 100vh;
  display: grid;
  grid-template-columns: 1fr 1fr;
}

.auth-aside {
  background: linear-gradient(160deg, var(--color-primary), #0f3bb0);
  color: var(--color-ink-inverse);
  padding: var(--space-12);
  display: flex;
  flex-direction: column;
  justify-content: center;
  gap: var(--space-4);
}

.auth-tagline {
  font-size: var(--text-3xl);
  font-weight: 700;
  letter-spacing: -0.02em;
}

.auth-sub {
  max-width: 42ch;
  color: rgba(255, 255, 255, 0.82);
  line-height: var(--leading-relaxed);
}

.auth-main {
  display: grid;
  place-items: center;
  padding: var(--space-6);
  background: var(--color-surface-subtle);
}

.auth-card {
  width: 100%;
  max-width: 420px;
  padding: var(--space-8);
}

@media (max-width: 900px) {
  .auth-layout {
    grid-template-columns: 1fr;
  }

  .auth-aside {
    display: none;
  }
}
</style>