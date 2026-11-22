<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { EditorView, basicSetup } from 'codemirror'
import { json } from '@codemirror/lang-json'

const props = defineProps<{ modelValue: string }>()
const emit = defineEmits<{ 'update:modelValue': [value: string] }>()
const host = ref<HTMLDivElement>()
let view: EditorView | undefined
onMounted(() => {
  view = new EditorView({
    parent: host.value, doc: props.modelValue,
    extensions: [basicSetup, json(), EditorView.lineWrapping, EditorView.contentAttributes.of({ 'aria-label': 'Конфигурация Xray JSON' }), EditorView.theme({
      '&': { height: '100%' },
      '.cm-scroller': { overflow: 'auto' },
    }, { dark: true }), EditorView.updateListener.of(update => {
      if (update.docChanged) emit('update:modelValue', update.state.doc.toString())
    })],
  })
})
watch(() => props.modelValue, value => {
  if (view && value !== view.state.doc.toString()) view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: value } })
})
onBeforeUnmount(() => view?.destroy())
</script>
<template><div ref="host" class="json-editor" /></template>

