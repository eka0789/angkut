// Struktur navigasi sidebar. Data, bukan logika — memudahkan penambahan item
// saat modul diimplementasikan tanpa menyentuh komponen layout.
import type { MessageKey } from './i18n'

export interface NavItem {
  label: MessageKey
  to: string
  icon: string
}

export interface NavSection {
  title: MessageKey
  items: NavItem[]
}

export const navSections: NavSection[] = [
  {
    title: 'nav.dashboard',
    items: [
      { label: 'nav.dashboard', to: '/dashboard', icon: '▦' },
      { label: 'nav.commandCenter', to: '/command-center', icon: '◎' },
    ],
  },
  {
    title: 'nav.operations',
    items: [
      { label: 'nav.shipments', to: '/shipments', icon: '▤' },
      { label: 'nav.tracking', to: '/tracking', icon: '⌖' },
      { label: 'nav.pickup', to: '/shipments/pickup', icon: '⏱' },
      { label: 'nav.orders', to: '/orders', icon: '⛃' },
      { label: 'nav.inventory', to: '/inventory', icon: '☰' },
      { label: 'nav.warehouses', to: '/warehouses', icon: '⌂' },
      { label: 'nav.customers', to: '/customers', icon: '☺' },
    ],
  },
  {
    title: 'nav.money',
    items: [
      { label: 'nav.cod', to: '/cod', icon: '₨' },
      { label: 'nav.settlements', to: '/cod/settlements', icon: '✓' },
      { label: 'nav.analytics', to: '/analytics', icon: '◔' },
    ],
  },
  {
    title: 'nav.intelligence',
    items: [
      { label: 'nav.aiAssistant', to: '/ai', icon: '✦' },
      { label: 'nav.settings', to: '/settings/organization', icon: '⚙' },
      { label: 'nav.addresses', to: '/settings/addresses', icon: '⌖' },
    ],
  },
]

/** Ambil judul section yang cocok dengan sebuah path (untuk topbar). */
export function sectionTitleForPath(path: string): MessageKey | null {
  for (const section of navSections) {
    for (const item of section.items) {
      const match = item.to === '/dashboard' ? path === item.to : path.startsWith(item.to)
      if (match) return item.label
    }
  }
  return null
}

/**
 * Prefix rute yang memang direncanakan tetapi belum diimplementasikan.
 * Dipakai catch-all untuk memutuskan antara halaman "sedang dikembangkan"
 * (200) dan 404 sungguhan.
 */
export const plannedModulePrefixes = [
  '/command-center',
  '/tracking',
  '/inventory',
  '/warehouses',
  '/customers',
  '/cod',
  '/analytics',
  '/ai',
  '/returns',
  '/couriers',
  '/stores',
  '/settings',
  '/admin',
  '/features',
  '/pricing',
  '/docs',
  '/kalkulator',
] as const

/** True bila path berada di bawah salah satu prefix modul terencana. */
export function isPlannedModulePath(path: string): boolean {
  return plannedModulePrefixes.some(
    (prefix) => path === prefix || path.startsWith(`${prefix}/`),
  )
}