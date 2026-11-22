<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { ChevronRight, Fingerprint, Network, Smartphone, X } from 'lucide-vue-next'
import Notice from './Notice.vue'

const props = defineProps<{ user: string; name: string; limit: number }>()
type Device = { hwid: string; user_agent: string; os: string; os_version: string; model: string; app_version: string; locale: string; ip: string; first_seen_at: number; last_seen_at: number; request_count: number }
type Activity = {
  total_main: number; total_limited: number; quota_limited: number; last_seen_at: number; subscription_last_seen_at: number; traffic_known: boolean; period: string
  devices: Device[]; without_hwid: Device[]
  ips: { node_id: string; credential: string; ip: string; last_seen_at: number }[]
}
const data = ref<Activity>()
const error = ref('')
type Detail = 'ips' | 'devices' | 'without_hwid'
const activeDetail = ref<Detail | null>(null)
const dialog = ref<HTMLDialogElement>()
const dialogId = `user-activity-${props.user}`
const groups = computed(() => [
  { key: 'ips' as const, title: 'IP-адреса', icon: Network, count: data.value?.ips.length ?? 0 },
  { key: 'devices' as const, title: 'Устройства подписки', icon: Smartphone, count: data.value?.devices.length ?? 0 },
  { key: 'without_hwid' as const, title: 'Без HWID', icon: Fingerprint, count: data.value?.without_hwid.length ?? 0 },
])
const selectedGroup = computed(() => groups.value.find(group => group.key === activeDetail.value))
const devices = computed(() => activeDetail.value === 'devices' ? data.value?.devices ?? [] : data.value?.without_hwid ?? [])
async function openDetails(detail: Detail) {
  activeDetail.value = detail
  await nextTick()
  if (dialog.value && !dialog.value.open) dialog.value.showModal()
}
function dismissBackdrop(event: MouseEvent) {
  if (event.target !== dialog.value) return
  const bounds = dialog.value.getBoundingClientRect()
  if (event.clientX < bounds.left || event.clientX > bounds.right || event.clientY < bounds.top || event.clientY > bounds.bottom) dialog.value.close()
}
let timer: ReturnType<typeof setTimeout>
let stopped = false
const date = (value: number) => value ? new Intl.DateTimeFormat('ru-RU', { dateStyle: 'short', timeStyle: 'short', hourCycle: 'h23' }).format(value * 1000) : '—'
function bytes(value: number) {
  const units = ['Б', 'КиБ', 'МиБ', 'ГиБ', 'ТиБ']
  const index = Math.min(4, Math.max(0, Math.floor(Math.log2(value || 1) / 10)))
  return new Intl.NumberFormat('ru-RU', { maximumFractionDigits: 2 }).format(value / 1024 ** index) + ' ' + units[index]
}
async function load() {
  try {
    const response = await fetch(new URL('api/users/' + encodeURIComponent(props.user) + '/activity', document.baseURI), { signal: AbortSignal.timeout(10000) })
    const result = await response.json()
    if (!response.ok) throw new Error(result.error)
    data.value = result
    error.value = ''
  } catch (cause) { error.value = cause instanceof Error ? cause.message : 'Статистика недоступна' }
  if (!stopped) timer = setTimeout(load, 15000)
}
onMounted(load)
onBeforeUnmount(() => { stopped = true; clearTimeout(timer) })
</script>

<template>
  <section class="user-activity">
    <Notice :text="error" error />
    <template v-if="data">
      <div class="activity-stats">
        <div><span>Всего MAIN</span><strong>{{ data.traffic_known ? bytes(data.total_main) : '—' }}</strong></div>
        <div><span>Всего LIMITED</span><strong>{{ data.traffic_known ? bytes(data.total_limited) : '—' }}</strong></div>
        <div><span>Всего трафика</span><strong>{{ data.traffic_known ? bytes(data.total_main + data.total_limited) : '—' }}</strong></div>
        <div><span>LIMITED · {{ data.period }}</span><strong>{{ data.traffic_known ? bytes(data.quota_limited) : '—' }} / {{ limit ? bytes(limit) : 'Без лимита' }}</strong></div>
        <div><span>Последняя активность Xray</span><strong>{{ date(data.last_seen_at) }}</strong></div>
        <div><span>Последний запрос подписки</span><strong>{{ date(data.subscription_last_seen_at) }}</strong></div>
      </div>
      <div class="activity-detail-buttons">
        <button v-for="group in groups" :key="group.key" type="button" class="secondary activity-detail-button" aria-haspopup="dialog" :aria-controls="dialogId" @click="openDetails(group.key)">
          <component :is="group.icon" :size="16" /><span>{{ group.title }}</span><span class="activity-detail-count">{{ group.count }}</span><ChevronRight :size="15" />
        </button>
      </div>
      <Teleport to="body">
        <dialog :id="dialogId" ref="dialog" class="activity-dialog" :aria-labelledby="dialogId + '-title'" @click="dismissBackdrop" @close="activeDetail = null">
          <header class="activity-dialog-header">
            <div><p>{{ name }}</p><h2 :id="dialogId + '-title'">{{ selectedGroup?.title }} <span class="activity-detail-count">{{ selectedGroup?.count }}</span></h2></div>
            <button type="button" class="icon-button" aria-label="Закрыть" autofocus @click="dialog?.close()"><X :size="19" /></button>
          </header>
          <div class="activity-dialog-content">
            <template v-if="activeDetail === 'ips'">
              <p v-if="!data.ips.length" class="activity-detail-empty">Нет подключений</p>
              <div v-else class="activity-table-wrap">
                <table class="activity-table">
                  <thead><tr><th>IP</th><th>Учётка</th><th>Узел</th><th>Последняя активность</th></tr></thead>
                  <tbody><tr v-for="item in data.ips" :key="item.node_id + item.credential + item.ip"><td><code>{{ item.ip }}</code></td><td>{{ item.credential.toUpperCase() }}</td><td><code :title="item.node_id">{{ item.node_id.slice(0, 8) }}</code></td><td>{{ date(item.last_seen_at) }}</td></tr></tbody>
                </table>
              </div>
            </template>
            <template v-else>
              <p v-if="!devices.length" class="activity-detail-empty">Нет обращений</p>
              <div v-else class="device-grid">
                <article v-for="device in devices" :key="device.hwid || device.user_agent" class="device-card">
                  <div class="section-header"><strong>{{ device.model || device.os || device.user_agent || 'Неизвестный клиент' }}</strong><span class="subtle">{{ device.request_count }} запросов</span></div>
                  <dl>
                    <div v-if="activeDetail === 'devices'"><dt>HWID</dt><dd><code>{{ device.hwid }}</code></dd></div>
                    <div><dt>Клиент</dt><dd>{{ device.user_agent || '—' }}</dd></div>
                    <div v-if="device.os || device.os_version"><dt>Система</dt><dd>{{ [device.os, device.os_version].filter(Boolean).join(' ') }}</dd></div>
                    <div v-if="device.app_version"><dt>Версия приложения</dt><dd>{{ device.app_version }}</dd></div>
                    <div v-if="device.locale"><dt>Язык</dt><dd>{{ device.locale }}</dd></div>
                    <div><dt>Последний IP</dt><dd>{{ device.ip || '—' }}</dd></div>
                    <div><dt>Первый запрос</dt><dd>{{ date(device.first_seen_at) }}</dd></div>
                    <div><dt>Последний запрос</dt><dd>{{ date(device.last_seen_at) }}</dd></div>
                  </dl>
                </article>
              </div>
            </template>
          </div>
        </dialog>
      </Teleport>
    </template>
  </section>
</template>
