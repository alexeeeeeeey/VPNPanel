import { readonly, ref } from 'vue'

type Notice = { id: number; text: string; error: boolean }
const notices = ref<Notice[]>([])
let sequence = 0
const timers = new Map<number, ReturnType<typeof setTimeout>>()

export function dismissNotice(id: number) {
  const timer = timers.get(id)
  if (timer) clearTimeout(timer)
  timers.delete(id)
  notices.value = notices.value.filter(notice => notice.id !== id)
}

export function notify(text: string, error = false) {
  if (!text || notices.value.some(notice => notice.text === text && notice.error === error)) return
  if (notices.value.length >= 4) dismissNotice(notices.value[0].id)
  const id = ++sequence
  notices.value.push({ id, text, error })
  timers.set(id, setTimeout(() => dismissNotice(id), 3000))
}

export const activeNotices = readonly(notices)
