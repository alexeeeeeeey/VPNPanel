<script setup lang="ts">
import { CircleAlert, CircleCheck, X } from 'lucide-vue-next'
import { activeNotices, dismissNotice } from './notices'
</script>

<template>
  <Teleport to="body">
    <div class="notice-stack" aria-live="polite" aria-relevant="additions">
      <TransitionGroup name="notice">
        <div v-for="notice in activeNotices" :key="notice.id" class="floating-notice" :class="{ 'is-error': notice.error }" :role="notice.error ? 'alert' : 'status'">
          <CircleAlert v-if="notice.error" :size="19" /><CircleCheck v-else :size="19" />
          <span>{{ notice.text }}</span>
          <button type="button" class="icon-button" aria-label="Закрыть уведомление" @click="dismissNotice(notice.id)"><X :size="16" /></button>
        </div>
      </TransitionGroup>
    </div>
  </Teleport>
</template>
