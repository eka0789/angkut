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
      { label: 'nav.dashboard', to: '/dashboard', icon: 'grid' },
      { label: 'nav.commandCenter', to: '/command-center', icon: 'activity' },
    ],
  },
  {
    title: 'nav.operations',
    items: [
      { label: 'nav.shipments', to: '/shipments', icon: 'package' },
      { label: 'nav.tracking', to: '/tracking', icon: 'map-pin' },
      { label: 'nav.pickup', to: '/shipments/pickup', icon: 'clock' },
      { label: 'nav.orders', to: '/orders', icon: 'clipboard' },
      { label: 'nav.inventory', to: '/inventory', icon: 'layers' },
      { label: 'nav.warehouses', to: '/warehouses', icon: 'home' },
      { label: 'nav.customers', to: '/customers', icon: 'users' },
    ],
  },
  {
    title: 'nav.money',
    items: [
      { label: 'nav.money', to: '/finance', icon: 'banknote' },
      { label: 'nav.analytics', to: '/analytics', icon: 'bar-chart' },
    ],
  },
  {
    title: 'nav.intelligence',
    items: [
      { label: 'nav.aiAssistant', to: '/ai', icon: 'sparkles' },
      { label: 'nav.settings', to: '/settings/organization', icon: 'settings' },
      { label: 'nav.addresses', to: '/settings/addresses', icon: 'map-pin' },
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
  '/finance',
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