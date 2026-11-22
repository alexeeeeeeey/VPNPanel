<script setup lang="ts">
import Notice from './Notice.vue'
import { onBeforeUnmount, onMounted, ref } from 'vue'

const props = defineProps<{ node: string; templates: { id: string; name: string; kind: string }[] }>()
const selected = ref('')
const busy = ref(false)
const error = ref('')
const status = ref('')
let stopped = false
let timer: ReturnType<typeof setTimeout>
async function request<T>(path: string, method = 'GET', data?: object): Promise<T> {
  const prefix = props.node === 'local' ? 'api/' : 'api/nodes/' + encodeURIComponent(props.node) + '/'
  const response = await fetch(new URL(prefix + path, document.baseURI), {
    method, signal: AbortSignal.timeout(15000),
    headers: { 'Content-Type': 'application/json', 'X-Panel-Request': '1' },
    body: data ? JSON.stringify(data) : undefined,
  })
  const result = await response.json()
  if (!response.ok) throw new Error(result.error)
  return result
}
async function refresh() {
  const result = await request<{ template_id: string; missing: string[]; error?: string }>('assignment')
  if (!busy.value) selected.value = result.template_id
  const runtime = await request<{ state: string; error?: string }>('runtime')
  status.value = ({ running: 'Xray работает', waiting: 'Ожидает переменные', unassigned: '', error: 'Ошибка Xray', not_configured: 'Xray не подключён' } as Record<string, string>)[runtime.state] || ''
  error.value = runtime.error || result.error || ''
}
async function save() {
  busy.value = true
  error.value = ''
  try {
    await request('assignment', 'PUT', { template_id: selected.value })
    await refresh()
  } catch (cause) { error.value = cause instanceof Error ? cause.message : 'Не удалось сохранить выбор' }
  finally { busy.value = false }
}
async function poll() {
  try { if (!busy.value) await refresh() }
  catch (cause) { error.value = cause instanceof Error ? cause.message : 'Узел недоступен' }
  if (!stopped) timer = setTimeout(poll, 4000)
}
onMounted(poll)
onBeforeUnmount(() => { stopped = true; clearTimeout(timer) })
</script>

<template>
  <div class="assignment-strip">
    <label>Используемый конфиг
      <select v-model="selected" :disabled="busy" @change="save">
        <option value="">Выберите серверный конфиг</option>
        <option v-for="template in templates.filter(item => item.kind === 'server')" :key="template.id" :value="template.id">{{ template.name }}</option>
      </select>
    </label>
    <span v-if="status" class="subtle">{{ status }}</span>
    <Notice :text="error" error />
  </div>
</template>

