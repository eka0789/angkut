<script setup lang="ts">
// AppCountUp — angka statistik yang naik dari 0 ke nilai target saat masuk
// viewport (IntersectionObserver + requestAnimationFrame). Angka awal yang
// dirender adalah nilai final (aman SSR/hidrasi); animasi hanya berjalan di
// klien setelah mount dan dilewati bila pengguna memilih reduced-motion.
const props = withDefaults(defineProps<{
  value: number
  /** 'idr' menambahkan awalan Rp dan pemisah ribuan id-ID. */
  currency?: boolean
  /** Jumlah desimal (mis. 2.3 hari). */
  decimals?: number
  duration?: number
}>(), {
  currency: false,
  decimals: 0,
  duration: 900,
})

const el = ref<HTMLElement | null>(null)
const shown = ref(props.value)
let started = false

function format(v: number): string {
  const fixed = v.toFixed(props.decimals)
  const n = props.decimals > 0 ? Number(fixed) : Math.round(v)
  const body = n.toLocaleString('id-ID', {
    minimumFractionDigits: props.decimals,
    maximumFractionDigits: props.decimals,
  })
  return props.currency ? `Rp ${body}` : body
}

function animate() {
  if (typeof window === 'undefined') return
  if (window.matchMedia('(prefers-reduced-motion: reduce)').matches) {
    shown.value = props.value
    return
  }
  const target = props.value
  const t0 = performance.now()
  const dur = props.duration
  const easeOut = (t: number): number => 1 - Math.pow(1 - t, 4)
  const tick = (now: number): void => {
    const p = Math.min(1, (now - t0) / dur)
    shown.value = target * easeOut(p)
    if (p < 1) requestAnimationFrame(tick)
    else shown.value = target
  }
  requestAnimationFrame(tick)
}

function run() {
  if (started) return
  started = true
  // Mulai sedikit setelah mount agar frame pertama stabil.
  requestAnimationFrame(() => animate())
}

onMounted(() => {
  if (props.value === 0) return
  if (typeof IntersectionObserver === 'undefined') {
    run()
    return
  }
  const io = new IntersectionObserver((entries) => {
    if (entries.some(e => e.isIntersecting)) {
      io.disconnect()
      run()
    }
  }, { threshold: 0.3 })
  if (el.value) io.observe(el.value)
})

watch(() => props.value, (v) => {
  if (!started || v === shown.value) return
  shown.value = v
})
</script>

<template>
  <span
    ref="el"
    class="countup num-tabular"
  >{{ format(shown) }}</span>
</template>

<style scoped>
.countup {
  font-variant-numeric: tabular-nums;
}
</style>
