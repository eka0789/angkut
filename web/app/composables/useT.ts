// useT — helper terjemahan ringan. Menerjemahkan kunci dari kamus Indonesia;
// bila kunci tidak ada, mengembalikan kunci apa adanya (gagal terlihat, bukan
// gagal senyap) sehingga mudah ditemukan saat pengembangan.
import { id, type MessageKey } from '~/utils/i18n'

export function useT() {
  function t(key: MessageKey | string): string {
    const dict: Record<string, string> = id
    return dict[key] ?? String(key)
  }
  return { t }
}