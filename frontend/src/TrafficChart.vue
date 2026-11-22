<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import uPlot from 'uplot'
import 'uplot/dist/uPlot.min.css'
function formatBytes(value: number) {
  const units = ['Б', 'КиБ', 'МиБ', 'ГиБ', 'ТиБ']
  const index = Math.min(4, Math.max(0, Math.floor(Math.log2(value || 1) / 10)))
  return new Intl.NumberFormat('ru', { maximumFractionDigits: 1 }).format(value / 1024 ** index) + ' ' + units[index]
}

const props = withDefaults(
  defineProps<{
    points:
      | [number[], (number | null)[]]
      | [number[], (number | null)[], (number | null)[]]
    window: [number, number]
    percent?: boolean
    label?: string
    height?: number
  }>(),
  { label: 'Скорость приёма и передачи данных в LAN', height: 200 },
)
const target = ref<HTMLDivElement>()
let chart: uPlot | undefined
let resize: ResizeObserver | undefined
const timeFormat = new Intl.DateTimeFormat('ru', {
  hour: '2-digit',
  minute: '2-digit',
  second: '2-digit',
  hourCycle: 'h23',
})
const minuteFormat = new Intl.DateTimeFormat('ru', {
  hour: '2-digit',
  minute: '2-digit',
  hourCycle: 'h23',
})
const formatTime = (seconds: number) =>
  (props.window[1] - props.window[0] <= 300 ? timeFormat : minuteFormat).format(
    seconds * 1000,
  )
const formatValue = (value: number | null | undefined) =>
  value == null
    ? '—'
    : props.percent
      ? Math.round(value) + '%'
      : formatBytes(Math.round(value)) + '/с'

function updateChart() {
  if (!chart) return
  const plot = chart
  const [min, max] = props.window
  plot.batch(() => {
    plot.setData(props.points, false)
    plot.setScale('x', { min, max })
    let ceiling = 100
    if (!props.percent) {
      let peak = 0
      for (const series of props.points.slice(1)) {
        for (const value of series) peak = Math.max(peak, value ?? 0)
      }
      const unit =
        1024 ** Math.min(4, Math.max(1, Math.floor(Math.log2(peak) / 10)))
      const step = Math.max(unit / 4, 2 ** Math.ceil(Math.log2(peak / 4)))
      ceiling = 4 * step
    }
    plot.setScale('y', { min: 0, max: ceiling })
  })
}

onMounted(() => {
  if (!target.value) return
  const element = target.value
  const theme = getComputedStyle(element)
  const accent = theme.getPropertyValue('--accent').trim()
  const muted = '#a3a3a3'
  const edge = '#262626'
  chart = new uPlot(
    {
      width: Math.max(1, element.clientWidth),
      height: props.height,
      cursor: { drag: { x: false, y: false, setScale: false } },
      legend: { show: true, live: true },
      series: [
        {
          label: 'Время',
          value: (_plot, value) => (value == null ? '—' : formatTime(value)),
        },
        {
          label: props.percent ? 'Загрузка' : 'Приём',
          value: (_plot, value) => formatValue(value),
          stroke: accent,
          fill: accent + '20',
          width: 2,
        },
        ...(props.percent
          ? []
          : [
              {
                label: 'Передача',
                value: (_plot: uPlot, value: number | null) =>
                  formatValue(value),
                stroke: '#83aedb',
                width: 2,
              },
            ]),
      ],
      scales: {
        x: { time: true },
        y: {},
      },
      axes: [
        {
          stroke: muted,
          grid: { stroke: edge },
          ticks: { stroke: edge },
          space: 78,
          values: (_plot, ticks) => ticks.map(formatTime),
        },
        {
          stroke: muted,
          grid: { stroke: edge },
          ticks: { stroke: edge },
          size: props.percent ? 48 : 82,
          splits: (_plot, _axis, _min, max) => [
            0,
            max / 4,
            max / 2,
            max * 0.75,
            max,
          ],
          values: (_plot, ticks) => ticks.map(formatValue),
        },
      ],
    },
    props.points,
    element,
  )
  resize = new ResizeObserver(() =>
    chart?.setSize({
      width: Math.max(1, element.clientWidth),
      height: props.height,
    }),
  )
  resize.observe(element)
  updateChart()
})
watch(() => [props.points, props.window], updateChart)
onBeforeUnmount(() => {
  resize?.disconnect()
  chart?.destroy()
})
</script>

<template>
  <div ref="target" class="chart" role="img" :aria-label="label" />
</template>

