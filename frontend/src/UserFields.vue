<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { Save } from 'lucide-vue-next'
import ExpiryField from './ExpiryField.vue'

const props = defineProps<{ name: string; subId: string; expiresAt: number; limitedBytes: number; main: string; limited: string; busy: boolean }>()
type Changes = { name?: string; sub_id?: string; expires_at?: number; reset_main?: boolean; reset_limited?: boolean; limited_bytes?: number }
const name = ref(props.name)
watch(() => props.name, value => { name.value = value })
const emit = defineEmits<{ save: [changes: Changes] }>()
const bytesPerGiB = 1024 ** 3
const limitGiB = ref<number | string>(props.limitedBytes ? props.limitedBytes / bytesPerGiB : '')
const limitBytes = computed(() => Math.round(Number(limitGiB.value) * bytesPerGiB))
watch(() => props.limitedBytes, value => { limitGiB.value = value ? value / bytesPerGiB : '' })

const savedExpiry = computed(() => {
  if (!props.expiresAt) return ''
  const date = new Date(props.expiresAt * 1000)
  return new Date(date.getTime() - date.getTimezoneOffset() * 60_000).toISOString().slice(0, 16)
})
const subId = ref(props.subId)
const expiry = ref(savedExpiry.value)
watch(() => props.subId, value => { subId.value = value })
watch(savedExpiry, value => { expiry.value = value })

const dirty = computed(() => name.value !== props.name || subId.value !== props.subId || expiry.value !== savedExpiry.value || limitBytes.value !== props.limitedBytes)

function save() {
  if (props.busy || !dirty.value) return
  const changes: Changes = {}
  if (name.value !== props.name) changes.name = name.value
  if (subId.value !== props.subId) changes.sub_id = subId.value
  if (expiry.value !== savedExpiry.value) changes.expires_at = expiry.value ? Math.floor(new Date(expiry.value).getTime() / 1000) : 0
  if (limitBytes.value !== props.limitedBytes) changes.limited_bytes = limitBytes.value
  emit('save', changes)
}
</script>

<template>
  <form class="card user-fields user-edit" @submit.prevent="save">
    <div class="user-identity-fields">
    <label>Имя<input v-model.trim="name" maxlength="120" required :disabled="busy"></label>
    <label>Sub ID<input v-model.trim="subId" maxlength="128" required :disabled="busy"></label>
    <label>Квота в месяц, ГиБ<input v-model.number="limitGiB" type="number" min="0" :max="Number.MAX_SAFE_INTEGER / bytesPerGiB" step="any" placeholder="Без лимита" :disabled="busy"></label>
    </div>
    <div class="uuid-field">
      <label>UUID MAIN<input :value="main" readonly></label>
    </div>
    <div class="uuid-field">
      <label>UUID LIMITED<input :value="limited" readonly></label>
    </div>
    <ExpiryField v-model="expiry" :disabled="busy" />
    <div class="uuid-actions">
      <slot />
      <button v-if="dirty" class="primary" :disabled="busy"><Save :size="16" /> Сохранить</button>
    </div>
  </form>
</template>

