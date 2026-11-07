<script setup lang="ts">
// Layout halaman auth: fokus, tanpa sidebar, pesan brand di samping.
</script>

<template>
  <div class="auth-layout">
    <section class="auth-aside">
      <span
        class="aside-orb o1"
        aria-hidden="true"
      />
      <span
        class="aside-orb o2"
        aria-hidden="true"
      />
      <img
        src="~/assets/brand/logo.svg"
        alt="Angkut"
        height="32"
        class="auth-logo"
      >
      <h1 class="auth-tagline">
        Ship smarter.<br>
        Grow faster.
      </h1>
      <p class="auth-sub">
        Bandingkan kurir, kirim, lacak, dan kelola COD dalam satu tempat —
        dengan asisten cerdas yang bekerja untuk Anda.
      </p>
      <ul class="auth-points">
        <li>Pelacakan realtime per resi</li>
        <li>COD terekonsiliasi sampai cair</li>
        <li>Asisten AI dari data Anda sendiri</li>
      </ul>
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
  background: var(--gradient-brand);
  color: var(--color-ink-inverse);
  padding: var(--space-12);
  display: flex;
  flex-direction: column;
  justify-content: center;
  gap: var(--space-4);
  position: relative;
  overflow: hidden;
}

/* Orba cahaya melayang di panel biru (transform-only, hormati reduced-motion) */
.aside-orb {
  position: absolute;
  border-radius: var(--radius-pill);
  filter: blur(60px);
  pointer-events: none;
  will-change: transform;
  animation: orb-float 14s ease-in-out infinite;
}
.aside-orb.o1 {
  width: 380px;
  height: 380px;
  background: rgba(255, 255, 255, 0.14);
  top: -110px;
  right: -90px;
}
.aside-orb.o2 {
  width: 280px;
  height: 280px;
  background: rgba(249, 115, 22, 0.22);
  bottom: -80px;
  left: -60px;
  animation-delay: -6s;
}

.auth-logo { animation: pop-in 500ms var(--ease-spring) both; }

/* Konten di atas orba; :not agar aturan ini tidak menimpa position: absolute orba */
.auth-aside > *:not(.aside-orb) { position: relative; z-index: 1; }

.auth-points {
  list-style: none;
  margin: var(--space-4) 0 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  font-size: var(--text-sm);
  color: rgba(255, 255, 255, 0.9);
}
.auth-points li {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  animation: fade-up 480ms var(--ease-out) both;
}
.auth-points li:nth-child(1) { animation-delay: 120ms; }
.auth-points li:nth-child(2) { animation-delay: 200ms; }
.auth-points li:nth-child(3) { animation-delay: 280ms; }
.auth-points li::before {
  content: '';
  width: 8px;
  height: 8px;
  border-radius: var(--radius-pill);
  background: var(--color-cta);
  box-shadow: 0 0 0 4px rgba(249, 115, 22, 0.25);
  flex: none;
}

.auth-card {
  width: 100%;
  max-width: 420px;
  padding: var(--space-8);
  animation: pop-in 480ms var(--ease-spring) 80ms both;
}

@media (prefers-reduced-motion: reduce) {
  .aside-orb, .auth-logo, .auth-card, .auth-points li { animation: none !important; }
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

@media (max-width: 900px) {
  .auth-layout {
    grid-template-columns: 1fr;
  }

  .auth-aside {
    display: none;
  }
}
</style>