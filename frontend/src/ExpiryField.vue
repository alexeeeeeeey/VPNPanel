<script setup lang="ts">
import { computed, ref, watch } from 'vue'

const value = defineModel<string>({ required: true })
defineProps<{ disabled?: boolean }>()
const time = ref(value.value.slice(11) || '00:00')
const dayValue = computed({
  get: () => value.value.slice(0, 10),
  set: day => { value.value = day ? day + 'T' + time.value : '' },
})
watch(value, next => { time.value = next ? next.slice(11) : '00:00' })
function setTime(event: Event) {
  time.value = (event.target as HTMLInputElement).value
  if (dayValue.value) value.value = dayValue.value + 'T' + time.value
}

function extend(months: number) {
  const current = new Date(value.value).getTime()
  const date = new Date(Math.max(Date.now(), Number.isFinite(current) ? current : 0))
  const day = date.getDate()
  date.setDate(1)
  date.setMonth(date.getMonth() + months)
  const lastDay = new Date(date.getFullYear(), date.getMonth() + 1, 0).getDate()
  date.setDate(Math.min(day, lastDay))
  date.setHours(0, 0, 0, 0)
  const local = new Date(date.getTime() - date.getTimezoneOffset() * 60_000)
  value.value = local.toISOString().slice(0, 16)
}
</script>

<template>
  <fieldset class="expiry-field" :disabled="disabled">
    <div class="expiry-inputs">
      <label>Действует до<input v-model="dayValue" type="date" max="9999-12-31"></label>
      <label>Время<input :value="time" type="text" maxlength="5" placeholder="00:00"
        pattern="([01][0-9]|2[0-3]):[0-5][0-9]" :required="!!dayValue" :disabled="!dayValue" @input="setTime"></label>
      <div class="expiry-actions">
        <button class="secondary" type="button" @click="extend(1)">+ месяц</button>
        <button class="secondary" type="button" @click="extend(3)">+ квартал</button>
      </div>
    </div>
  </fieldset>
</template>
