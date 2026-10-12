// useT — helper terjemahan ringan. Menerjemahkan kunci dari kamus Indonesia;
// bila kunci tidak ada, mengembalikan kunci apa adanya (gagal terlihat, bukan
// gagal senyap) sehingga mudah ditemukan saat pengembangan.
//
// Interpolasi: `t('settings.invited', { email })` mengganti placeholder
// `{email}` pada nilai kamus.
import { id, type MessageKey } from '~/utils/i18n'

export function useT() {
  function t(key: MessageKey | string, params?: Record<string, string | number>): string {
    const dict: Record<string, string> = id
    const raw = dict[key] ?? String(key)
    if (!params) return raw
    return raw.replace(/\{(\w+)\}/g, (match, name: string) =>
      name in params ? String(params[name]) : match)
  }
  return { t }
}
