<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { FileJson, GripVertical, Plus } from 'lucide-vue-next'
import { notify } from './notices'

type Template = { id: string; name: string; kind: 'client' | 'server'; body: string }
const props = defineProps<{ templates: Template[]; selected: string; kind: 'client' | 'server'; busy: boolean }>()
const emit = defineEmits<{ select: [template: Template]; create: [kind: 'client' | 'server']; reordered: [] }>()
const section = ref(props.kind)
watch(() => props.kind, kind => { section.value = kind })
const saving = ref(false)
const dragging = ref('')
const dropTarget = ref('')
const items = computed(() => props.templates.filter(template => template.kind === section.value))

function start(event: DragEvent, id: string) {
  if (props.busy || saving.value) { event.preventDefault(); return }
  dragging.value = id
  if (event.dataTransfer) {
    event.dataTransfer.effectAllowed = 'move'
    event.dataTransfer.setData('text/plain', id)
  }
}
function clearDrag() { dragging.value = ''; dropTarget.value = '' }
async function move(id: string, target: string) {
  if (saving.value || props.busy || id === target) { clearDrag(); return }
  const ids = items.value.map(item => item.id)
  const from = ids.indexOf(id), to = ids.indexOf(target)
  if (from < 0 || to < 0) { clearDrag(); return }
  ids.splice(to, 0, ids.splice(from, 1)[0])
  clearDrag()
  saving.value = true
  try {
    const response = await fetch(new URL('api/template-order', document.baseURI), {
      method: 'PUT', signal: AbortSignal.timeout(15000),
      headers: { 'Content-Type': 'application/json', 'X-Panel-Request': '1' },
      body: JSON.stringify({ kind: section.value, ids }),
    })
    if (!response.ok) throw new Error((await response.json()).error)
    emit('reordered')
  } catch (cause) {
    notify(cause instanceof Error ? cause.message : 'Не удалось сохранить порядок', true)
    emit('reordered')
  } finally { saving.value = false }
}
function keyboardMove(id: string, direction: number) {
  const index = items.value.findIndex(item => item.id === id)
  const target = items.value[index + direction]
  if (target) void move(id, target.id)
}
</script>

<template>
  <div class="section-header">
    <div class="library-sections">
      <button type="button" :class="{ selected: section === 'client' }" :aria-pressed="section === 'client'" :disabled="saving" @click="section = 'client'; clearDrag()">Клиентские</button>
      <button type="button" :class="{ selected: section === 'server' }" :aria-pressed="section === 'server'" :disabled="saving" @click="section = 'server'; clearDrag()">Серверные</button>
    </div>
    <button type="button" class="icon-button" aria-label="Новый конфиг" :disabled="busy || saving" @click="emit('create', section)"><Plus :size="16" /></button>
  </div>
  <p v-if="!items.length" class="hint">Нет конфигов</p>
  <div class="config-list" :aria-busy="saving">
    <button v-for="item in items" :key="item.id" type="button" class="template-item"
      :class="{ 'template-selected': item.id === selected, 'template-dragging': dragging === item.id, 'template-drop-target': dropTarget === item.id && dragging !== item.id }"
      :aria-pressed="item.id === selected" :disabled="busy || saving" :draggable="!busy && !saving"
      title="Перетащите для изменения порядка. С клавиатуры: Alt + ↑ / ↓."
      @click="emit('select', item)"
      @dragstart="start($event, item.id)" @dragend="clearDrag"
      @dragover.prevent="dragging && (dropTarget = item.id)" @drop.prevent="move(dragging, item.id)"
      @keydown.alt.up.prevent="keyboardMove(item.id, -1)" @keydown.alt.down.prevent="keyboardMove(item.id, 1)">
      <span class="template-file-icon"><FileJson :size="18" /></span>
      <span class="template-description">{{ item.name }}</span>
      <GripVertical :size="16" class="template-grip" />
    </button>
  </div>
</template>
