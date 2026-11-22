<script setup lang="ts">
import { watch } from 'vue'
import { notify } from './notices'

const props = defineProps<{ text: string; error?: boolean }>()
watch(() => props.text, text => { if (text) notify(text, props.error) }, { immediate: true })
</script>

<template></template>
