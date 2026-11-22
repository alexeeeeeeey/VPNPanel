<script setup lang="ts">
import Notice from './Notice.vue'
import { computed, onMounted, ref } from 'vue'
import { Plus, Save, Trash2 } from 'lucide-vue-next'

const props = defineProps<{ node: string; source: string }>()
const emit = defineEmits<{ saved: [] }>()
type Variable = { name: string; value: string }
const rows = ref<Variable[]>([])
const loaded = ref(false), busy = ref(false), error = ref(''), notice = ref('')
const newName = ref('')
const system = new Set(['UUID', 'UUID_LIMITED', 'USER_UUID', 'USER_UUID_LIMITED', 'CLIENTS_MAIN', 'CLIENTS_LIMITED'])
const required = computed(() => [...new Set([...props.source.matchAll(/[$&]([A-Z][A-Z0-9_]{0,63})/g)].map(match => match[1]).filter(name => !system.has(name)))].sort())
const fields = computed(() => {
  const result = [...rows.value]
  for (const name of required.value) {
    if (!result.some(row => row.name === name)) {
      // Missing fields remain drafts until explicitly saved.
      result.push({ name, value: '' })
    }
  }
  return result.sort((a, b) => a.name.localeCompare(b.name))
})
function edit(row: Variable, value: string) {
  const existing = rows.value.find(item => item.name === row.name)
  if (existing) existing.value = value
  else rows.value.push({ ...row, value })
}
function add() {
  const name = newName.value.replace(/^[$&]/, '').trim().toUpperCase()
  if (!/^[A-Z][A-Z0-9_]{0,63}$/.test(name) || system.has(name)) { error.value = 'Недопустимое имя переменной'; return }
  if (!rows.value.some(row => row.name === name)) rows.value.push({ name, value: '' })
  newName.value = ''
}
async function api<T>(path: string, method = 'GET', data?: object): Promise<T> {
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
async function load() {
  busy.value = true
  try {
    const result = await api<{ body: string }>('variables')
    const values = JSON.parse(result.body) as Record<string, unknown>
    rows.value = Object.entries(values).map(([name, value]) => ({
      name,
      value: value == null ? '' : typeof value === 'object' ? JSON.stringify(value) : String(value),
    }))
    loaded.value = true
  } catch (cause) { error.value = cause instanceof Error ? cause.message : 'Узел недоступен' }
  finally { busy.value = false }
}
async function save() {
  if (busy.value) return
  busy.value = true; error.value = ''; notice.value = ''
  try {
    const values: Record<string, unknown> = {}
    for (const row of rows.value) {
      values[row.name] = row.value
    }
    await api('variables', 'PUT', { body: JSON.stringify(values) })
    notice.value = 'Переменные сохранены'
    emit('saved')
  } catch (cause) { error.value = cause instanceof Error ? cause.message : 'Не удалось сохранить настройки' }
  finally { busy.value = false }
}
onMounted(load)
</script>

<template>
  <section class="panel card config-settings">
    
    <Notice :text="error" error />
    <Notice :text="notice" />
    <form v-if="loaded" @submit.prevent="save">
      <div v-for="row in fields" :key="row.name" class="variable-row">
        <div class="variable-row-header">
          <label :for="'variable-' + row.name"><code>{{ '$' + row.name }}</code><span v-if="required.includes(row.name)" class="subtle"> *</span></label>
          <button v-if="!required.includes(row.name)" type="button" class="icon-button" :disabled="busy" :aria-label="'Удалить ' + row.name" @click="rows = rows.filter(item => item.name !== row.name)"><Trash2 :size="14" /></button>
        </div>
        <div class="variable-value">
          <input :id="'variable-' + row.name" :value="row.value" :disabled="busy" placeholder="Не задано" @input="edit(row, ($event.target as HTMLInputElement).value)">
        </div>
      </div>
      <div class="variable-add">
        <input v-model="newName" aria-label="Имя новой переменной" placeholder="CUSTOM" :disabled="busy" @keydown.enter.prevent="add">
        <button type="button" class="secondary" :disabled="busy || !newName.trim()" aria-label="Добавить переменную" @click="add"><Plus :size="16" /></button>
      </div>
      <button class="primary" :disabled="busy"><Save :size="16" /> Сохранить переменные</button>
    </form>
    <button v-else class="secondary" :disabled="busy" @click="load">Загрузить</button>
  </section>
</template>



