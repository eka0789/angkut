// Composable untuk notifikasi (TASK-040): pusat notifikasi in-app, badge
// belum-dibaca, preferensi, dan template (admin). Semua endpoint tenant-scoped
// dan milik pengguna yang sedang masuk.
import type {
  AppNotification,
  NotificationChannel,
  NotificationEventKey,
  NotificationPreferenceView,
  NotificationTemplateView,
} from '~/types/api'

export function useNotifications() {
  const { apiFetch, apiFetchWithMeta } = useApi()

  /** Daftar notifikasi (default: terbaru dulu). */
  async function list(opts: { unread?: boolean, priority?: string, limit?: number, offset?: number } = {}): Promise<{ items: AppNotification[], meta: { total: number, limit: number, offset: number } }> {
    const query: Record<string, string | number | boolean | undefined> = {}
    if (opts.unread) query.unread = true
    if (opts.priority) query.priority = opts.priority
    if (opts.limit !== undefined) query.limit = opts.limit
    if (opts.offset !== undefined) query.offset = opts.offset
    const res = await apiFetchWithMeta<AppNotification[]>('/api/v1/notifications', { query })
    return {
      items: res.data,
      meta: {
        total: Number(res.meta?.total ?? res.data.length),
        limit: Number(res.meta?.limit ?? opts.limit ?? 20),
        offset: Number(res.meta?.offset ?? opts.offset ?? 0),
      },
    }
  }

  /** Jumlah notifikasi belum dibaca (badge lonceng). */
  async function unreadCount(): Promise<number> {
    const res = await apiFetch<{ unread: number }>('/api/v1/notifications/unread-count')
    return res.unread
  }

  /** Tandai satu notifikasi dibaca. */
  async function markRead(id: string): Promise<void> {
    await apiFetch(`/api/v1/notifications/${id}/read`, { method: 'POST' })
  }

  /** Tandai semua notifikasi dibaca. */
  async function markAllRead(): Promise<number> {
    const res = await apiFetch<{ marked: number }>('/api/v1/notifications/read-all', { method: 'POST' })
    return res.marked
  }

  /** Template efektif (kustom organisasi / bawaan). */
  async function templates(): Promise<NotificationTemplateView[]> {
    return apiFetch<NotificationTemplateView[]>('/api/v1/notification-templates')
  }

  /** Simpan template kustom untuk satu event. */
  async function updateTemplate(key: NotificationEventKey, title: string, body: string): Promise<void> {
    await apiFetch(`/api/v1/notification-templates/${key}`, {
      method: 'PUT',
      body: { key, title_template: title, body_template: body },
    })
  }

  /** Preferensi efektif pengguna (event × kanal). */
  async function preferences(): Promise<NotificationPreferenceView[]> {
    return apiFetch<NotificationPreferenceView[]>('/api/v1/notification-preferences')
  }

  /** Ubah satu preferensi. */
  async function updatePreference(eventKey: NotificationEventKey, channel: NotificationChannel, enabled: boolean): Promise<void> {
    await apiFetch('/api/v1/notification-preferences', {
      method: 'PUT',
      body: { event_key: eventKey, channel, enabled },
    })
  }

  return { list, unreadCount, markRead, markAllRead, templates, updateTemplate, preferences, updatePreference }
}
