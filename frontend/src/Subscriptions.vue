<script setup lang="ts">
import Notice from './Notice.vue'
import { computed, onMounted, ref } from 'vue'
import { Copy, Save } from 'lucide-vue-next'

const props = defineProps<{
  users: { id: string; name: string; enabled: boolean; sub_id: string }[]
}>()

const form = ref({ enabled: false, ip: '127.0.0.1', port: 9090, path: '/sub', public_url: '', title: '', description: '', support: '', website: '', interval: 0 })
const saved = ref<typeof form.value>()
const running = ref(false)
const busy = ref(false)
const error = ref('')
const message = ref('')
const user = ref('')
const link = computed(() => {
  const settings = saved.value
  if (!settings || !running.value || !props.users.some(item => item.enabled && item.sub_id === user.value)) return ''
  if (settings.public_url) return settings.public_url.replace(/\/+$/, '') + '/' + user.value
  const ip = settings.ip === '0.0.0.0' || settings.ip === '::' ? location.hostname : settings.ip
  const host = ip.includes(':') && !ip.startsWith('[') ? '[' + ip + ']' : ip
  return 'http://' + host + ':' + settings.port + settings.path + '/' + user.value
})

async function sync(save = false) {
  busy.value = true
  error.value = ''
  message.value = ''
  try {
    if (save) {
      const result = await fetch(new URL('api/subscription', document.baseURI), {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json', 'X-Panel-Request': '1' },
        body: JSON.stringify(form.value),
      })
      if (!result.ok) throw new Error((await result.json()).error)
    }
    const result = await fetch(new URL('api/subscription', document.baseURI))
    if (!result.ok) throw new Error('Не удалось загрузить настройки')
    const state = await result.json()
    form.value = { ...state.settings }
    saved.value = state.settings
    running.value = state.running
    error.value = state.error
    if (save) message.value = 'Настройки сохранены'
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : 'Не удалось сохранить настройки'
  } finally {
    busy.value = false
  }
}

async function copy() {
  try {
    await navigator.clipboard.writeText(link.value)
    message.value = 'Ссылка скопирована'
  } catch {
    message.value = 'Выделите и скопируйте ссылку вручную'
  }
}
onMounted(() => sync())
</script>

<template>
  <Notice :text="error" error />
  <Notice :text="message" />
  <form class="subscription-grid" @submit.prevent="sync(true)">
    <section class="panel card subscription-fields">
      <div class="section-header"><h2>Выдача</h2><span class="status-pill" :class="{ running }"><i />{{ running ? 'Работает' : 'Остановлена' }}</span></div>
      <label class="subscription-toggle"><input v-model="form.enabled" type="checkbox"> Включить выдачу</label>
      <label>IP прослушивания<input v-model.trim="form.ip" required placeholder="127.0.0.1"></label>
      <label>Порт<input v-model.number="form.port" type="number" min="1" max="65535" required></label>
      <label>Путь<input v-model.trim="form.path" required placeholder="/sub"></label>
      <label>Публичный URL<input v-model.trim="form.public_url" type="url" placeholder="https://vpn.example.com/sub"></label>
    </section>
    <section class="panel card subscription-fields">
      <h2>Профиль Happ</h2>
      <label>Название<input v-model.trim="form.title" maxlength="25"></label>
      <label>Описание<input v-model.trim="form.description" maxlength="200"></label>
      <label>Ссылка поддержки<input v-model.trim="form.support" type="url" placeholder="https://t.me/…"></label>
      <label>Сайт профиля<input v-model.trim="form.website" type="url"></label>
      <label>Интервал обновления, ч<input v-model.number="form.interval" type="number" min="0" max="8760" required></label>
    </section>
    <div><button class="primary" type="submit" :disabled="busy"><Save :size="16" /> Сохранить</button></div>
  </form>
  <section v-if="running" class="panel card subscription-link">
    <h2>Ссылка пользователя</h2>
    <label>Пользователь
      <select v-model="user">
        <option value="">Выберите пользователя</option>
        <option v-for="item in users.filter(u => u.enabled)" :key="item.id" :value="item.sub_id">{{ item.name }}</option>
      </select>
    </label>
    <div v-if="link" class="subscription-copy">
      <input :value="link" aria-label="Ссылка подписки" readonly>
      <button class="secondary" @click="copy"><Copy :size="16" /> Копировать</button>
    </div>
  </section>
</template>


