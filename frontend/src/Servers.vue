<script setup lang="ts">
import Notice from './Notice.vue'
import { computed, reactive, ref, watch } from 'vue'
import { ChevronRight, Plus, Save } from 'lucide-vue-next'

type Server = { id: string; name: string; url: string; managed: boolean }
const props = defineProps<{
  servers: Server[]
  self: { id: string; name: string; url: string }
  states: Record<string, { online: boolean; status?: { hostname: string; version: string; node_name: string; node_id: string } }>
}>()
const emit = defineEmits<{ changed: [] }>()
const id = ref('')
const url = ref('')
const key = ref('')
const code = ref('')
const manual = ref(false)
const error = ref('')
const busy = ref(false)
const query = ref('')
const drafts = reactive<Record<string, { name: string; url: string; key: string; code: string }>>({})
watch(() => props.servers, (servers, previous) => {
  for (const server of servers) {
    if (previous.find(item => item.id === server.id)?.url !== server.url || previous.find(item => item.id === server.id)?.name !== server.name) {
      delete drafts[server.id]
    }
  }
})
const visible = computed(() => props.servers.filter(server => (server.name + ' ' + server.url).toLowerCase().includes(query.value.trim().toLowerCase())))
const own = computed<Server>(() => ({ ...props.self, id: 'local', managed: true }))
watch(() => props.self, (current, previous) => {
  if (current.name !== previous.name || current.url !== previous.url) delete drafts.local
})

function draft(server: Server) {
  return drafts[server.id] ??= { name: server.name, url: server.url, key: '', code: '' }
}
function dirty(server: Server) {
  const value = draft(server)
  return value.name !== server.name || value.url !== server.url || !!value.key || !!value.code
}
function status(id: string) {
  return props.states[id]?.online == null ? 'Проверка' : props.states[id]?.online ? 'В сети' : 'Нет связи'
}
async function submit(method: string, target: string, data?: object) {
  if (busy.value) return
  busy.value = true
  error.value = ''
  try {
    const path = 'api/nodes' + (target ? '/' + encodeURIComponent(target) : '')
    const response = await fetch(new URL(path, document.baseURI), {
      method,
      signal: AbortSignal.timeout(15000),
      headers: { 'Content-Type': 'application/json', 'X-Panel-Request': '1' },
      body: data ? JSON.stringify(data) : undefined,
    })
    if (!response.ok) throw new Error((await response.json()).error)
    if (method === 'POST') {
      id.value = ''; url.value = ''; key.value = ''; code.value = ''
    } else {
      if (method === 'PUT') {
        const value = drafts[target]
        if (value) { value.key = ''; value.code = '' }
      } else delete drafts[target]
    }
    emit('changed')
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : 'Ошибка связи с панелью'
  } finally {
    busy.value = false
  }
}
function remove(server: Server) {
  if (!busy.value && window.confirm('Удалить подключение «' + server.name + '»?')) void submit('DELETE', server.id)
}
</script>

<template>
  <Notice :text="error" error />
  <section class="panel card add-server">
    <form class="user-fields" :class="{ 'server-manual': manual }" @submit.prevent="submit('POST', '', manual ? { name: id, url, key } : { code })">
      <template v-if="manual">
        <label>Имя узла<input v-model.trim="id" maxlength="120" required :disabled="busy"></label>
        <label>URL панели<input v-model.trim="url" type="url" required :disabled="busy" placeholder="https://node.example.com/panel"></label>
        <label>API-ключ<input v-model="key" type="password" autocomplete="off" :disabled="busy"></label>
      </template>
      <label v-else class="server-code">Строка подключения<input v-model.trim="code" type="password" autocomplete="off" required :disabled="busy"></label>
      <div class="server-add-actions">
        <button class="secondary" type="button" :disabled="busy" @click="manual = !manual">{{ manual ? 'По строке' : 'Вручную' }}</button>
        <button class="primary" :disabled="busy"><Plus :size="16" /> Добавить</button>
      </div>
    </form>
  </section>
  <div class="user-filters">
    <label class="user-search"><span class="sr-only">Поиск серверов</span><input v-model="query" type="search" placeholder="Поиск по имени или адресу"></label>
  </div>
  <section class="panel users-panel">
    <details v-if="!query || (self.name + ' ' + self.url).toLowerCase().includes(query.trim().toLowerCase())" name="servers" class="user-entry">
      <summary class="server-summary">
        <ChevronRight :size="16" />
        <span class="user-identity"><strong>{{ self.name }}</strong><small>Этот узел</small></span>
        <span class="status-pill" :class="{ running: states.local?.online }">{{ status('local') }}</span>
      </summary>
      <form class="card user-fields user-edit" @submit.prevent="submit('PUT', 'local', { name: draft(own).name, url: draft(own).url })">
        <label>Имя узла<input v-model.trim="draft(own).name" maxlength="120" required :disabled="busy"></label>
        <label>URL панели<input v-model.trim="draft(own).url" type="url" :disabled="busy"></label>
        <label>UUID узла<input :value="self.id" readonly></label>
        <div class="uuid-actions">
          <button v-if="dirty(own)" class="primary" :disabled="busy"><Save :size="16" /> Сохранить</button>
        </div>
      </form>
    </details>
    <details v-for="server in visible" :key="server.id" name="servers" class="user-entry">
      <summary class="server-summary">
        <ChevronRight :size="16" />
        <span class="user-identity"><strong>{{ server.name }}</strong><small>{{ server.url }}</small></span>
        <span class="status-pill" :class="{ running: states[server.id]?.online }">{{ status(server.id) }}</span>
      </summary>
      <form class="card user-fields user-edit" @submit.prevent="submit('PUT', server.id, draft(server))">
        <label>Имя узла<input v-model.trim="draft(server).name" maxlength="120" required :disabled="busy || !server.managed"></label>
        <label>URL панели<input v-model.trim="draft(server).url" type="url" required :disabled="busy || !server.managed"></label>
        <label>API-ключ<input v-model="draft(server).key" type="password" autocomplete="off" placeholder="Сохранённый ключ" :disabled="busy || !server.managed"></label>
        <label>Новая строка подключения<input v-model.trim="draft(server).code" type="password" autocomplete="off" :disabled="busy || !server.managed"></label>
        <div v-if="server.managed" class="uuid-actions">
          <button class="secondary user-delete" type="button" :disabled="busy" @click="remove(server)">Удалить</button>
          <button v-if="dirty(server)" class="primary" :disabled="busy"><Save :size="16" /> Сохранить</button>
        </div>
      </form>
    </details>
  </section>
</template>



