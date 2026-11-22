<script setup lang="ts">
import Notice from './Notice.vue'
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { Maximize2, Minimize2, PanelRightClose, PanelRightOpen, Activity, ArrowDownToLine,  Server, Check, ChevronRight, CircleUserRound, FileJson, Plus,  RefreshCw, Save, Upload } from 'lucide-vue-next'
import { useRoute } from 'vue-router'
import Subscriptions from './Subscriptions.vue'
import Servers from './Servers.vue'
import NodeMetrics from './NodeMetrics.vue'
import JsonEditor from './JsonEditor.vue'
import PanelVariables from './PanelVariables.vue'
import AssignmentBar from './AssignmentBar.vue'
import ConfigLibrary from './ConfigLibrary.vue'
import ExpiryField from './ExpiryField.vue'
import UserFields from './UserFields.vue'
import UserActivity from './UserActivity.vue'

type User = { id: string; name: string; uuid: string; limited_uuid: string; enabled: boolean; sub_id: string; expires_at: number; created_at: number; limited_bytes: number; limited_used_bytes?: number | null; total_main: number; total_limited: number; quota_limited: number; last_seen_at: number; subscription_last_seen_at: number; device_count: number; unknown_client_count: number }
type Template = { id: string; name: string; kind: 'client' | 'server'; body: string }
const users = ref<User[]>([]), templates = ref<Template[]>([])
const configUsers = ref<User[]>([])
const nodes = ref(['local'])
const inventoryItems = ref<{ id: string; name: string; url: string; managed: boolean }[]>([])
const selfNode = ref({ id: '', name: 'Этот узел', url: '' })
const selectedNode = ref(localStorage.getItem('vpnpanel.node') || 'local')
const inventoryError = ref('')
const nodeStates = ref<Record<string, { online: boolean; status?: NodeStatus; history: MetricSample[] }>>({})
const now = ref(Date.now())
const openUsers = ref(new Set<string>())
function userToggle(id: string, event: Event) {
  if ((event.target as HTMLDetailsElement).open) openUsers.value.add(id)
  else openUsers.value.delete(id)
}
const isExpired = (user: User) => user.expires_at > 0 && user.expires_at * 1000 <= now.value
const userSearch = ref('')
const userSort = ref<'name' | 'days'>('name')
const sortDescending = ref(false)
const nameOrder = new Intl.Collator('ru', { numeric: true, sensitivity: 'base' })
const visibleUsers = computed(() => {
  const query = userSearch.value.trim().toLocaleLowerCase('ru')
  return users.value.filter(user => !query || user.name.toLocaleLowerCase('ru').includes(query) || user.sub_id.toLowerCase().includes(query)).sort((a, b) => {
    if (userSort.value === 'days') {
      // No-expiry users stay at the end in either direction.
      if (!a.expires_at && b.expires_at) return 1
      if (a.expires_at && !b.expires_at) return -1
      const difference = a.expires_at - b.expires_at
      if (difference) return sortDescending.value ? -difference : difference
    }
    const difference = nameOrder.compare(a.name, b.name) || a.id.localeCompare(b.id)
    return sortDescending.value ? -difference : difference
  })
})
function timeLeft(user: User) {
  if (!user.expires_at) return 'Без срока'
  const days = Math.max(0, Math.ceil((user.expires_at * 1000 - now.value) / 86_400_000))
  return days + ' дн.'
}
const userDate = new Intl.DateTimeFormat('ru-RU', { dateStyle: 'short', timeStyle: 'short', hourCycle: 'h23' })
const trafficSize = (bytes: number) => new Intl.NumberFormat('ru-RU', { maximumSignificantDigits: 3 }).format(bytes / 1024 ** 3) + ' ГиБ'
const route = useRoute()
const tab = computed(() => route.name as 'overview' | 'configs' | 'users' | 'subscriptions' | 'servers')
type NodeStatus = { node_id: string; node_name: string; hostname: string; interfaces: { name: string; addresses: string[] }[]; disk_free: number; disk_total: number; sampled_at: number; rx: number | null; tx: number | null; cores: number; version: string; uptime: number; cpu: number | null; memory: number | null; memory_total: number }
const node = computed(() => nodeStates.value[selectedNode.value]?.status)
const expanded = ref(false)
const showVariables = ref(true)
const configSideTab = ref<'variables' | 'configs'>('variables')
const online = computed(() => nodeStates.value[selectedNode.value]?.online)
const checkingUpdate = ref(false)
type UpdateStatus = { current: string; latest: string; available: boolean; installable: boolean; updating: boolean; error: string; last_checked: number }
const updates = ref<Record<string, UpdateStatus>>({})
const selectedUpdate = computed(() => updates.value[selectedNode.value])
async function checkUpdate() {
  checkingUpdate.value = true
  error.value = ''
  notice.value = ''
  try {
    const update = await api<UpdateStatus>('/update', 'GET', undefined, selectedNode.value)
    updates.value[selectedNode.value] = update
    notice.value = update.available ? 'Доступна версия ' + update.latest : update.latest ? 'Установлена актуальная версия' : 'Опубликованных релизов пока нет'
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : 'Не удалось проверить обновления'
  } finally {
    checkingUpdate.value = false
  }
}
async function installUpdate() {
  const target = selectedNode.value
  try {
    error.value = ''
    updates.value[target] = await api<UpdateStatus>('/update', 'POST', undefined, target)
    if (updates.value[target].error) throw new Error(updates.value[target].error)
    notice.value = 'Обновление запущено. Узел перезапустится после загрузки новой версии.'
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : 'Не удалось запустить обновление'
  }
}
type MetricSample = Pick<NodeStatus, 'sampled_at' | 'cpu' | 'memory' | 'rx' | 'tx'>
const samplesHistory = computed(() => nodeStates.value[selectedNode.value]?.history ?? [])
let metricsTimer: ReturnType<typeof setTimeout>
let disposed = false
async function pollStatus() {
  now.value = Date.now()
  await Promise.all(nodes.value.map(async id => {
    try {
      const { history, ...status } = await api<NodeStatus & { history: MetricSample[] }>('/status', 'GET', undefined, id)
      const previousVersion = nodeStates.value[id]?.status?.version
      nodeStates.value[id] = { online: true, status, history: history ?? [] }
      if (id === 'local' && previousVersion && previousVersion !== status.version) {
        window.location.reload()
        return
      }
      try {
        const state = await api<UpdateStatus>('/update-status', 'GET', undefined, id)
        if (id === selectedNode.value && state.error && state.error !== updates.value[id]?.error) {
          error.value = state.error
          notice.value = ''
        }
        updates.value[id] = state
      } catch { /* Older nodes may not expose installation status yet. */ }
    } catch {
      nodeStates.value[id] = { ...nodeStates.value[id], online: false, history: nodeStates.value[id]?.history ?? [] }
    }
  }))
  if (!disposed && !busy.value) {
    try {
      await refreshInventory()
      const state = await api<{ users: User[]; templates: Template[] }>('/state')
      const clean = !hasEdits.value
      templates.value = state.templates
      if (clean && templateID.value) {
        const updated = state.templates.find(template => template.id === templateID.value)
        if (updated) { name.value = updated.name; body.value = updated.body; kind.value = updated.kind }
      }
      users.value = state.users
    } catch { /* Keep the last received state while the panel is unreachable. */ }
  }
  if (!disposed) metricsTimer = setTimeout(pollStatus, 4000)
}
const uptime = computed(() => {
  const seconds = node.value?.uptime
  if (seconds == null) return '—'
  return seconds < 60 ? seconds + ' с' : Math.floor(seconds / 3600) + ' ч ' + Math.floor(seconds % 3600 / 60) + ' мин'
})
const kind = ref<'client' | 'server'>('server'), templateID = ref(''), name = ref('Серверный конфиг')
const userID = ref(''), userName = ref(''), output = ref(''), error = ref(''), notice = ref(''), busy = ref(false)
const newSubID = ref('')
const newExpiry = ref('')
const expirySeconds = (value: string) => value ? Math.floor(new Date(value).getTime() / 1000) : 0
async function saveUser(u: User, changes: { name?: string; sub_id?: string; expires_at?: number; reset_main?: boolean; reset_limited?: boolean; limited_bytes?: number }) {
  await run(async () => {
    await api('/users/' + u.id, 'PATCH', changes)
    await refresh()
    notice.value = 'Пользователь обновлён'
  })
}
const samples = {
  client: JSON.stringify({ outbounds: [
    { tag: 'main', protocol: 'vless', settings: { vnext: [{ address: 'vpn.example.com', port: 443, users: [{ id: '&UUID', encryption: 'none' }] }] } },
    { tag: 'limited', protocol: 'vless', settings: { vnext: [{ address: 'limited.example.com', port: 443, users: [{ id: '&UUID_LIMITED', encryption: 'none' }] }] } },
  ] }, null, 2),
  server: JSON.stringify({ inbounds: [
    { tag: 'main', listen: '127.0.0.1', port: 10001, protocol: 'vless', settings: { clients: '&CLIENTS_MAIN', decryption: 'none' } },
    { tag: 'limited', listen: '127.0.0.1', port: 10002, protocol: 'vless', settings: { clients: '&CLIENTS_LIMITED', decryption: 'none' } },
  ], outbounds: [{ protocol: 'freedom', tag: 'direct' }] }, null, 2),
}
const body = ref(samples.server)
const activeUsers = computed(() => configUsers.value.filter(u => u.enabled && !isExpired(u)))
const selectedUser = computed(() => activeUsers.value.find(u => u.id === userID.value))
const dirty = computed(() => {
  const saved = templates.value.find(t => t.id === templateID.value)
  return !saved || saved.body !== body.value || saved.name !== name.value || saved.kind !== kind.value
})
const hasEdits = computed(() => templateID.value ? dirty.value : body.value !== samples[kind.value] || name.value !== (kind.value === 'client' ? 'Клиентский конфиг' : 'Серверный конфиг'))
async function api<T>(path: string, method = 'GET', data?: unknown, target = 'local'): Promise<T> {
  const prefix = target === 'local' ? '/api' : '/api/nodes/' + encodeURIComponent(target)
  const response = await fetch(new URL(prefix.slice(1) + path, document.baseURI), { method, signal: AbortSignal.timeout(15000), headers: { 'Content-Type': 'application/json', 'X-Panel-Request': '1' }, body: data === undefined ? undefined : JSON.stringify(data) })
  const result = await response.json()
  if (!response.ok) throw new Error(result.error || 'Ошибка запроса')
  return result
}
async function run(action: () => Promise<void>) {
  busy.value = true; error.value = ''; notice.value = ''
  try { await action() } catch (e) { error.value = e instanceof Error ? e.message : String(e) }
  finally { busy.value = false }
}
async function refresh() {
  const state = await api<{ users: User[]; templates: Template[] }>('/state')
  users.value = state.users
  if (selectedNode.value === 'local') setConfigState(state)
}
function setConfigState(state: { users: User[]; templates: Template[] }) {
  configUsers.value = state.users
  templates.value = state.templates
  if (!activeUsers.value.some(u => u.id === userID.value)) userID.value = activeUsers.value[0]?.id || ''
}
async function refreshConfigs(id: string) {
  const library = await api<{ users: User[]; templates: Template[] }>('/state')
  templates.value = library.templates
  users.value = library.users
  const state = id === 'local' ? library : await api<{ users: User[]; templates: Template[] }>('/state', 'GET', undefined, id)
  configUsers.value = state.users
  if (!activeUsers.value.some(u => u.id === userID.value)) userID.value = activeUsers.value[0]?.id || ''
}
async function selectNode(id: string, force = false) {
  if (busy.value) return
  if (!force && id === selectedNode.value) return
  selectedNode.value = id
  localStorage.setItem('vpnpanel.node', id)
  configUsers.value = []
  userID.value = ''
  output.value = ''
  await run(async () => {
    await refreshConfigs(id)
    if (force && !templateID.value) {
      const first = templates.value.find(template => template.kind === 'server') ?? templates.value[0]
      if (first) {
        templateID.value = first.id; kind.value = first.kind; name.value = first.name; body.value = first.body
      }
    }
  })
}
function newTemplate(next: 'client' | 'server') {
  if (hasEdits.value && !window.confirm('Отбросить несохранённые изменения?')) return
  kind.value = next; templateID.value = ''; name.value = next === 'client' ? 'Клиентский конфиг' : 'Серверный конфиг'; body.value = samples[next]
}
function load(t: Template) {
  if (hasEdits.value && !window.confirm('Отбросить несохранённые изменения?')) return
  templateID.value = t.id; kind.value = t.kind; name.value = t.name; body.value = t.body
}
async function save() { await run(async () => {
  const id = selectedNode.value
  const t = await api<Template>('/templates', 'POST', { id: templateID.value, name: name.value, kind: kind.value, body: body.value })
  templateID.value = t.id; await refreshConfigs(id); notice.value = 'Шаблон сохранён'
}) }
async function render() { await run(async () => {
  const result = await api<unknown>('/render', 'POST', { kind: kind.value, body: body.value, user_id: userID.value }, selectedNode.value)
  output.value = JSON.stringify(result, null, 2)
}) }
async function addUser() { await run(async () => {
  const u = await api<User>('/users', 'POST', { name: userName.value, sub_id: newSubID.value, expires_at: expirySeconds(newExpiry.value) })
  userName.value = ''; newSubID.value = ''; newExpiry.value = ''
  await refresh()
  if (selectedNode.value === 'local') userID.value = u.id
  notice.value = 'Пользователь добавлен'
}) }
async function toggle(u: User) { await run(async () => {
  await api('/users/' + u.id, 'PATCH', { enabled: !u.enabled }); output.value = ''; await refresh()
}) }
async function resetUserUUIDs(u: User) {
  if (busy.value || !window.confirm('Сбросить оба UUID пользователя «' + u.name + '»? MAIN и LIMITED будут заменены новыми ключами.')) return
  await run(async () => {
    await api('/users/' + u.id, 'PATCH', { reset_main: true, reset_limited: true })
    output.value = ''
    await refresh()
    notice.value = 'Оба UUID сброшены'
  })
}
async function deleteUser(u: User) {
  if (busy.value || !window.confirm('Удалить пользователя «' + u.name + '»? Его ссылка подписки перестанет работать.')) return
  await run(async () => {
    await api('/users/' + u.id, 'DELETE')
    output.value = ''
    await refresh()
    notice.value = 'Пользователь удалён'
  })
}
function download() {
  const url = URL.createObjectURL(new Blob([output.value + '\n'], { type: 'application/json' }))
  const a = document.createElement('a'); a.href = url; a.download = kind.value === 'client' ? 'client.json' : 'server.json'; a.click(); URL.revokeObjectURL(url)
}
watch([body, kind, userID], () => { output.value = ''; notice.value = ''; error.value = '' })
async function importConfig(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file || (hasEdits.value && !window.confirm('Отбросить несохранённые изменения?'))) return
  await run(async () => {
    if (file.size > 2 * 1024 * 1024) throw new Error('Файл больше 2 МиБ')
    const text = await file.text()
    templateID.value = ''
    name.value = file.name.replace(/\.json$/i, '').slice(0, 120)
    body.value = text
  })
}
async function refreshInventory() {
  try {
    const inventory = await api<{ nodes: string[]; items: typeof inventoryItems.value; self: typeof selfNode.value; error: string }>('/nodes')
    nodes.value = inventory.nodes
    inventoryItems.value = inventory.items ?? []
    selfNode.value = inventory.self
    inventoryError.value = inventory.error
  } catch {
    inventoryError.value = 'Не удалось загрузить список серверов'
  }
}
async function inventoryChanged() {
  await refreshInventory()
  if (!nodes.value.includes(selectedNode.value)) await selectNode('local')
}
onMounted(async () => {
  await refreshInventory()
  if (disposed) return
  void pollStatus()
  const id = nodes.value.includes(selectedNode.value) ? selectedNode.value : 'local'
  await selectNode(id, true)
  if (id !== 'local') {
    try { await refresh() } catch { /* The local API is reported in the selector. */ }
  }
})
onBeforeUnmount(() => { disposed = true; clearTimeout(metricsTimer) })
</script>

<template>

      <div class="content">
        <header class="page-header">
          <h1>{{ { overview: 'Обзор', configs: 'Конфиги', users: 'Пользователи', subscriptions: 'Подписки', servers: 'Серверы' }[tab] }}</h1>
          <button v-if="tab === 'overview'" class="secondary" :disabled="!online || checkingUpdate || selectedUpdate?.updating" @click="checkUpdate">
            <RefreshCw :size="16" /> {{ checkingUpdate ? 'Проверяем…' : 'Проверить обновления' }}
          </button>
          <button v-if="tab === 'overview' && (selectedUpdate?.available || selectedUpdate?.updating)" class="secondary"
            :disabled="!online || !selectedUpdate?.installable || selectedUpdate?.updating"
            :title="selectedUpdate?.installable ? '' : 'Автоматическая установка доступна на Linux'" @click="installUpdate">
            <ArrowDownToLine :size="16" /> {{ selectedUpdate?.updating ? 'Обновляем…' : 'Обновить до ' + selectedUpdate?.latest }}
          </button>
        </header>
        <section v-if="tab === 'overview' || tab === 'configs'" class="panel card node-picker" aria-label="Выбор сервера">
          <h2>Серверы</h2>

          <div class="node-options">
            <button v-for="id in nodes" :key="id" type="button" class="node-option"
              :aria-pressed="id === selectedNode" :disabled="busy || checkingUpdate" @click="selectNode(id)">
              <Server :size="18" />
              <span class="node-label">
                <strong>{{ id === 'local' ? selfNode.name : inventoryItems.find(item => item.id === id)?.name || id }}</strong>
                <small>{{ nodeStates[id]?.status?.hostname || '—' }}</small>
              </span>
              <span class="node-mini" :class="{ 'is-online': nodeStates[id]?.online, 'is-offline': nodeStates[id]?.online === false }">
                <i />{{ nodeStates[id]?.online == null ? 'Проверка' : nodeStates[id]?.online ? 'В сети' : 'Нет связи' }}
              </span>
            </button>
          </div>
        </section>
        <Notice :text="inventoryError" error />
        <Notice :text="error" error /><Notice :text="notice" />
        <Subscriptions v-if="tab === 'subscriptions'" :users="users.filter(u => !isExpired(u))" />
        <template v-if="tab === 'servers'">

          <Servers :servers="inventoryItems" :self="selfNode" :states="nodeStates" @changed="inventoryChanged" />
        </template>
        <template v-if="tab === 'overview'">
          <section class="panel card node-summary">
            <div class="section-header">
              <h2><Server :size="19" /> {{ node?.hostname || 'Локальный узел' }}</h2>
              <span class="status-pill" :class="{ running: online, error: online === false }"><i />{{ online == null ? 'Проверка' : online ? 'В сети' : 'Недоступен' }}</span>
            </div>
            <dl>
              <div><dt>VPNPanel</dt><dd>{{ node?.version || '—' }}</dd></div>
              <div><dt>Xray</dt><dd>Нет данных</dd></div>
              <div><dt>Время работы панели</dt><dd>{{ uptime }}</dd></div>
              <div><dt>Хранилище панели</dt><dd>{{ node?.disk_total ? ((node.disk_total - node.disk_free) / 1024 ** 3).toFixed(1) + ' / ' + (node.disk_total / 1024 ** 3).toFixed(1) + ' ГиБ' : '—' }}</dd></div>
              <div><dt>Логические ЦП</dt><dd>{{ node?.cores ?? '—' }}</dd></div>
              <div><dt>Активные пользователи</dt><dd>{{ activeUsers.length }} / {{ configUsers.length }}</dd></div>
            </dl>
            <dl v-if="node?.interfaces?.length" class="interface-list" aria-label="IP-адреса интерфейсов">
              <div v-for="iface in node.interfaces" :key="iface.name">
                <dt>{{ iface.name }}</dt>
                <dd v-for="address in iface.addresses" :key="address"><code>{{ address }}</code></dd>
              </div>
            </dl>
          </section>
          <NodeMetrics :samples="samplesHistory" :online="online" :memory-total="node?.memory_total ?? 0" />
        </template>
        <template v-if="tab === 'configs'">
          <AssignmentBar :key="selectedNode" :node="selectedNode" :templates="templates" />
          <div class="workspace" :class="{ 'has-variables': showVariables }">
            <section class="panel editor-panel" :class="{ 'editor-expanded': expanded }" @keydown.esc="expanded = false">
              <div class="template-toolbar"><FileJson :size="18" /><input v-model="name" aria-label="Название шаблона" maxlength="120"><label class="icon-button" title="Импортировать JSON"><Upload :size="16" /><span class="sr-only">Импортировать JSON</span><input class="sr-only" type="file" accept=".json,application/json" :disabled="busy" @change="importConfig"></label><span v-if="dirty" class="unsaved">Не сохранён</span><button class="icon-button" title="Сохранить шаблон" aria-label="Сохранить шаблон" :disabled="busy" @click="save"><Save :size="17" /></button><div class="editor-actions">
                  <button class="icon-button" :title="showVariables ? 'Скрыть боковую панель' : 'Показать боковую панель'" :aria-label="showVariables ? 'Скрыть боковую панель' : 'Показать боковую панель'" :aria-expanded="showVariables" @click="showVariables = !showVariables; expanded = false"><PanelRightClose v-if="showVariables" :size="18" /><PanelRightOpen v-else :size="18" /></button>
                  <button class="icon-button" :title="expanded ? 'Свернуть редактор' : 'Развернуть редактор'" :aria-label="expanded ? 'Свернуть редактор' : 'Развернуть редактор'" :aria-pressed="expanded" @click="expanded = !expanded"><Minimize2 v-if="expanded" :size="18" /><Maximize2 v-else :size="18" /></button>
                </div></div>
              <JsonEditor v-model="body" />
            </section>
            <aside v-show="showVariables" class="panel config-sidebar">
              <div class="panel-header tabs" role="tablist" aria-label="Боковая панель">
                <button id="variables-tab" role="tab" :aria-selected="configSideTab === 'variables'" aria-controls="variables-panel" :class="{ selected: configSideTab === 'variables' }" @click="configSideTab = 'variables'">Переменные</button>
                <button id="configs-tab" role="tab" :aria-selected="configSideTab === 'configs'" aria-controls="configs-panel" :class="{ selected: configSideTab === 'configs' }" @click="configSideTab = 'configs'">Конфиги</button>
              </div>
              <div id="variables-panel" v-show="configSideTab === 'variables'" role="tabpanel" aria-labelledby="variables-tab">
                <PanelVariables :key="selectedNode" :node="selectedNode" :source="body" @saved="output = ''" />
              </div>
              <section id="configs-panel" v-show="configSideTab === 'configs'" class="card" role="tabpanel" aria-labelledby="configs-tab">
                <ConfigLibrary :templates="templates" :selected="templateID" :kind="kind" :busy="busy" @select="load" @create="newTemplate" @reordered="run(() => refreshConfigs(selectedNode))" />
              </section>
            </aside>
            <aside class="inspector">
              <section class="panel card"><h2><CircleUserRound :size="17" />Сборка конфига</h2><label v-if="kind === 'client'">Пользователь<select v-model="userID" :disabled="busy"><option value="" disabled>Выберите пользователя</option><option v-for="u in activeUsers" :key="u.id" :value="u.id">{{ u.name }}</option></select></label><button class="primary" :disabled="busy || (kind === 'client' && !selectedUser)" @click="render"><Activity :size="16" /> Собрать JSON</button></section>
            </aside>
          </div>
          <section v-if="output" class="panel output-panel"><div class="panel-header"><h2><Check :size="16" /> Собранный конфиг</h2><button class="secondary" @click="download"><ArrowDownToLine :size="15" /> Скачать JSON</button></div><pre>{{ output }}</pre></section>
        </template>
        <template v-else-if="tab === 'users'">
          <section class="panel card add-user">
            <form class="user-fields add-user-form" @submit.prevent="addUser">
              <label>Имя<input v-model="userName" maxlength="120" required></label>
              <label>Sub ID<input v-model.trim="newSubID" maxlength="128" placeholder="Автоматически"></label>
              <ExpiryField v-model="newExpiry" />
              <div><button class="primary" :disabled="busy || !userName.trim()"><Plus :size="17" /> Добавить</button></div>
            </form>
          </section>
          <div class="user-filters">
            <label class="user-search"><span class="sr-only">Поиск пользователей</span><input v-model="userSearch" type="search" placeholder="Поиск по имени или Sub ID"></label>
            <label><span class="sr-only">Сортировка</span><select v-model="userSort"><option value="name">По имени</option><option value="days">По остатку дней</option></select></label>
            <button class="secondary" :aria-label="sortDescending ? 'Сортировать по возрастанию' : 'Сортировать по убыванию'" :title="sortDescending ? 'По убыванию' : 'По возрастанию'" @click="sortDescending = !sortDescending">{{ sortDescending ? '↓' : '↑' }}</button>
            <span class="subtle">{{ visibleUsers.length }} / {{ users.length }}</span>
          </div>
          <section class="panel users-panel">
            <div v-if="!visibleUsers.length" class="empty"><Users :size="32" /><h2>{{ users.length ? 'Ничего не найдено' : 'Пока никого нет' }}</h2></div>
            <details v-for="u in visibleUsers" :key="u.id" name="users" class="user-entry" @toggle="userToggle(u.id, $event)">
              <summary class="user-summary" @click="($event.target as HTMLElement).closest('.user-switch') && $event.preventDefault()">
                <ChevronRight :size="16" />
                <span class="user-identity">
                  <strong>{{ u.name }}</strong>
                  <small>{{ u.created_at ? 'Создан ' + userDate.format(u.created_at * 1000) : 'Дата создания неизвестна' }}<template v-if="u.device_count || u.unknown_client_count"> · {{ u.device_count }} HWID<template v-if="u.unknown_client_count"> + {{ u.unknown_client_count }} без HWID</template></template></small>
                </span>
                <span class="subtle user-time-left">{{ timeLeft(u) }}</span>
                <span class="user-traffic">
                  <span v-if="u.limited_used_bytes != null" class="subtle">
                    {{ trafficSize(u.limited_used_bytes) }} расход · {{ u.limited_bytes ? trafficSize(Math.max(0, u.limited_bytes - u.limited_used_bytes)) + ' остаток' : 'Без лимита' }}
                  </span>
                  <span v-else class="subtle">Нет данных · {{ u.limited_bytes ? trafficSize(u.limited_bytes) + ' лимит' : 'Без лимита' }}</span>
                  <span class="traffic-meter" :class="{ 'traffic-meter-unknown': u.limited_used_bytes == null || !u.limited_bytes }"
                    role="meter" aria-label="Расход лимита LIMITED" aria-valuemin="0" :aria-valuemax="u.limited_bytes"
                    :aria-valuenow="u.limited_used_bytes == null ? undefined : Math.min(u.limited_used_bytes, u.limited_bytes)"
                    :aria-valuetext="u.limited_used_bytes == null ? 'Нет данных' : undefined">
                    <span v-if="u.limited_used_bytes != null && u.limited_bytes" :style="{ width: Math.min(100, 100 * u.limited_used_bytes / u.limited_bytes) + '%' }" />
                  </span>
                </span>
                <span class="user-header-actions">
                  <span class="user-state">{{ !u.enabled ? 'Отключён' : isExpired(u) ? 'Срок истёк' : 'Активен' }}</span>
                  <button class="user-switch" type="button" role="switch" :aria-checked="u.enabled"
                    :aria-label="'Доступ пользователя ' + u.name" :disabled="busy"
                    @click.stop.prevent="toggle(u)" @keydown.stop>
                    <span />
                  </button>
                </span>
              </summary>
              <UserFields :name="u.name" :sub-id="u.sub_id" :expires-at="u.expires_at" :limited-bytes="u.limited_bytes" :main="u.uuid" :limited="u.limited_uuid" :busy="busy" @save="saveUser(u, $event)">
                <div class="user-danger-actions">
                  <button class="secondary user-delete" type="button" :disabled="busy" @click="deleteUser(u)">Удалить</button>
                  <button class="secondary user-delete" type="button" :disabled="busy" @click="resetUserUUIDs(u)">Сбросить UUID</button>
                </div>
              </UserFields>
              <UserActivity v-if="openUsers.has(u.id)" :user="u.id" :name="u.name" :limit="u.limited_bytes" />
            </details>
          </section>
        </template>
      </div>
</template>











