<script setup lang="ts">
import { computed, ref } from 'vue'
import { ArrowDownLeft, ArrowUpRight, Cpu, HardDrive } from 'lucide-vue-next'
import TrafficChart from './TrafficChart.vue'

const props = defineProps<{
  samples: { sampled_at: number; cpu: number | null; memory: number | null; rx: number | null; tx: number | null }[]
  memoryTotal: number
  online?: boolean
}>()
const period = ref(5)
const latest = computed(() => props.online ? props.samples.at(-1) : undefined)
const chartWindow = computed<[number, number]>(() => {
  const end = props.samples.at(-1)?.sampled_at ?? Date.now() / 1000
  return [end - period.value * 60, end]
})
const visible = computed(() => props.samples.filter(sample => sample.sampled_at >= chartWindow.value[0]))
const timestamps = computed(() => visible.value.map(sample => sample.sampled_at))
const cpuPoints = computed<[number[], (number | null)[]]>(() => [timestamps.value, visible.value.map(sample => sample.cpu)])
const memoryPoints = computed<[number[], (number | null)[]]>(() => [timestamps.value, visible.value.map(sample => sample.memory)])
const trafficPoints = computed<[number[], (number | null)[], (number | null)[]]>(() => [timestamps.value, visible.value.map(sample => sample.rx), visible.value.map(sample => sample.tx)])
const percent = (value?: number | null) => value == null ? '—' : Math.round(value) + ' %'
const gib = (bytes: number) => (bytes / 1024 ** 3).toFixed(1) + ' ГиБ'
function speed(bytes?: number | null) {
  if (bytes == null) return '—'
  if (bytes >= 1024 ** 2) return (bytes / 1024 ** 2).toFixed(1) + ' МиБ/с'
  if (bytes >= 1024) return (bytes / 1024).toFixed(1) + ' КиБ/с'
  return Math.round(bytes) + ' Б/с'
}
</script>

<template>
  <label class="metrics-period">Период
    <select v-model="period">
      <option v-for="minutes in [1, 2, 5]" :key="minutes" :value="minutes">{{ minutes }} мин</option>
    </select>
  </label>
  <div class="resource-panels">
    <article class="panel traffic">
      <div class="section-header">
        <h2><Cpu :size="17" /> Процессор</h2>
        <strong>{{ percent(latest?.cpu) }}</strong>
      </div>
      <TrafficChart :points="cpuPoints" :window="chartWindow" percent label="Загрузка процессора" />
    </article>
    <article class="panel traffic">
      <div class="section-header">
        <div>
          <h2><HardDrive :size="17" /> Память</h2>
          <p v-if="latest?.memory != null && memoryTotal">{{ gib(memoryTotal * latest.memory / 100) }} из {{ gib(memoryTotal) }}</p>
        </div>
        <strong>{{ percent(latest?.memory) }}</strong>
      </div>
      <TrafficChart :points="memoryPoints" :window="chartWindow" percent label="Оперативная память" />
    </article>
  </div>
  <section class="panel traffic">
    <div class="section-header traffic-header">
      <div><h2>Трафик интерфейсов</h2></div>
      <div class="traffic-legend">
        <span><ArrowDownLeft :size="16" /> Приём {{ speed(latest?.rx) }}</span>
        <span><ArrowUpRight :size="16" /> Передача {{ speed(latest?.tx) }}</span>
      </div>
    </div>
    <TrafficChart :points="trafficPoints" :window="chartWindow" label="Скорость приёма и передачи по интерфейсам" />
  </section>
</template>
