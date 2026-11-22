<script setup lang="ts">
import contour0 from '~/assets/images/auth/contour-0.svg'
import contour1 from '~/assets/images/auth/contour-1.svg'
import contour2 from '~/assets/images/auth/contour-2.svg'
import contour3 from '~/assets/images/auth/contour-3.svg'
import contour4 from '~/assets/images/auth/contour-4.svg'
import contour5 from '~/assets/images/auth/contour-5.svg'
import contour6 from '~/assets/images/auth/contour-6.svg'
import contour7 from '~/assets/images/auth/contour-7.svg'
import contour8 from '~/assets/images/auth/contour-8.svg'
import contour9 from '~/assets/images/auth/contour-9.svg'
import heroGlow from '~/assets/images/auth/hero-glow.svg'

/**
 * Brand hero of the authentication layout (Figma 6:17). Static marketing
 * content: the "ideal window" card is an illustrative example, not live data.
 */

// Glow (Figma 6:18): 560 px frame at (420, -240) plus a 32.14% blur bleed on
// each side, which is why the exported asset is 920 px.
const GLOW = { size: 920, left: 240, top: -420 }
const glowStyle = {
  width: `${GLOW.size}px`,
  height: `${GLOW.size}px`,
  left: `${GLOW.left}px`,
  top: `${GLOW.top}px`,
}

// Contour lines are concentric circles (Figma 6:19–6:28) centred at (860, 820)
// in the 780×928 hero frame, growing 140 px in diameter each step.
const CONTOUR_CENTER = { x: 860, y: 820 }
const contours = [
  contour0, contour1, contour2, contour3, contour4,
  contour5, contour6, contour7, contour8, contour9,
].map((src, index) => {
  const size = 600 + index * 140
  return {
    src,
    style: {
      width: `${size}px`,
      height: `${size}px`,
      left: `${CONTOUR_CENTER.x - size / 2}px`,
      top: `${CONTOUR_CENTER.y - size / 2}px`,
    },
  }
})
</script>

<template>
  <section class="auth-hero" aria-label="Agentes Agro">
    <div class="auth-hero__decor" aria-hidden="true">
      <img :src="heroGlow" alt="" class="auth-hero__glow" :style="glowStyle">
      <img
        v-for="(contour, index) in contours"
        :key="index"
        :src="contour.src"
        alt=""
        class="auth-hero__contour"
        :style="contour.style"
      >
    </div>

    <AppLogo class="auth-hero__logo" />

    <div class="auth-hero__message">
      <p class="auth-hero__pill text-label-meta">
        <AppIcon name="sparkles" :size="14" />
        Planejador com agente de IA
      </p>
      <h2 class="auth-hero__headline text-hero">Pulverize<br>na hora certa.</h2>
      <p class="auth-hero__lead text-body-xl">
        Clima, Delta T, vento e chuva cruzados com os seus talhões. Você decide; o agente
        mostra a melhor janela.
      </p>
    </div>

    <div class="auth-hero__card" aria-hidden="true">
      <div class="auth-hero__card-header">
        <div class="auth-hero__card-heading">
          <p class="auth-hero__card-eyebrow text-overline">Janela ideal · hoje · talhão 3</p>
          <p class="text-metric-lg">06:00 – 09:30</p>
        </div>
        <span class="auth-hero__card-status text-label-meta-strong">
          <AppIcon name="check" :size="14" />
          Apta
        </span>
      </div>
      <div class="auth-hero__metrics">
        <div class="auth-hero__metric">
          <AppIcon name="thermometer" :size="18" class="auth-hero__metric-icon" />
          <span class="auth-hero__metric-label text-caption">Delta T</span>
          <span class="text-metric">4,2 °C</span>
        </div>
        <div class="auth-hero__metric">
          <AppIcon name="wind" :size="18" class="auth-hero__metric-icon" />
          <span class="auth-hero__metric-label text-caption">Vento</span>
          <span class="text-metric">6 km/h</span>
        </div>
        <div class="auth-hero__metric">
          <AppIcon name="rain" :size="18" class="auth-hero__metric-icon" />
          <span class="auth-hero__metric-label text-caption">Chuva 4h</span>
          <span class="text-metric">0 mm</span>
        </div>
      </div>
    </div>
  </section>
</template>

<style scoped>
.auth-hero {
  position: relative;
  display: flex;
  flex-direction: column;
  min-height: calc(100vh - 2 * var(--space-16));
  padding: var(--auth-hero-padding-top) var(--auth-hero-padding-x) var(--space-64);
  overflow: hidden;
  border-radius: var(--radius-2xl);
  background: var(--color-green-900);
  color: var(--color-text-inverse);
}

.auth-hero__decor {
  position: absolute;
  inset: 0;
  pointer-events: none;
}

.auth-hero__glow,
.auth-hero__contour {
  position: absolute;
  max-width: none;
}

.auth-hero__logo,
.auth-hero__message,
.auth-hero__card {
  position: relative;
}

.auth-hero__message {
  display: flex;
  flex-direction: column;
  gap: var(--space-22);
  margin-top: var(--auth-hero-message-offset);
}

.auth-hero__pill {
  display: inline-flex;
  align-self: flex-start;
  gap: var(--space-8);
  align-items: center;
  padding: var(--space-6) var(--space-12);
  border-radius: var(--radius-pill);
  background: var(--color-sprout-pill);
  color: var(--color-sprout-400);
}

.auth-hero__lead {
  max-width: var(--auth-hero-lead-width);
  color: var(--color-text-inverse-lead);
}

.auth-hero__card {
  display: flex;
  flex-direction: column;
  gap: var(--space-18);
  width: min(var(--auth-hero-card-width), 100%);
  margin-top: auto;
  padding: var(--space-24);
  border: var(--border-width-hairline) solid var(--color-border-inverse-glass);
  border-radius: var(--radius-xl);
  background: var(--color-surface-inverse-glass);
  backdrop-filter: blur(var(--blur-glass));
}

.auth-hero__card-header {
  display: flex;
  gap: var(--space-12);
  align-items: center;
}

.auth-hero__card-heading {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: var(--space-4);
}

.auth-hero__card-eyebrow {
  color: var(--color-sprout-400);
}

.auth-hero__card-status {
  display: inline-flex;
  gap: var(--space-6);
  align-items: center;
  padding: var(--space-6) var(--space-12);
  border-radius: var(--radius-pill);
  background: var(--color-status-suitable);
}

.auth-hero__metrics {
  display: flex;
  gap: var(--space-12);
}

.auth-hero__metric {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: var(--space-6);
  min-width: 0;
  padding: var(--space-14);
  border-radius: var(--radius-tile-lg);
  background: var(--color-surface-inverse-soft);
}

.auth-hero__metric-icon {
  color: var(--color-sprout-400);
}

.auth-hero__metric-label {
  color: var(--color-text-inverse-caption);
}
</style>
