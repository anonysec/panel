<script setup lang="ts">
import { computed, ref, onMounted, onUnmounted, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useRealtimeStore } from '@/stores/realtime'
import { useCustomersStore } from '@/stores/customers'
import { useNodesStore } from '@/stores/nodes'
import { useI18n } from '@koris/composables/useI18n'
import { useApi } from '@koris/composables/useApi'
import { formatDate } from '@koris/composables/useFormatDate'
import KChart from '@koris/ui/KChart.vue'
import KStatusPill from '@koris/ui/KStatusPill.vue'
import KSkeleton from '@koris/ui/KSkeleton.vue'

const { t } = useI18n()
const router = useRouter()
const realtime = useRealtimeStore()
const customers = useCustomersStore()
const nodes = useNodesStore()
const api = useApi()

customers.loadCustomers()
nodes.loadNodes()

const statCards = computed(() => [
  { label: t('stat.revenue'), value: `$${realtime.stats.approved_payments}`, icon: '💰', route: 'payments' },
  { label: t('stat.active_users'), value: realtime.stats.active_customers, icon: '👥', route: 'customers' },
  { label: t('stat.nodes_online'), value: realtime.stats.nodes, icon: '🖥️', route: 'nodes' },
  { label: t('stat.open_tickets'), value: realtime.stats.open_tickets, icon: '🎫', route: 'tickets' },
])

function handleStatClick(routeName: string) {
  router.push({ name: routeName })
}

// ─── Bandwidth Stats (Bar Chart) ───
type BandwidthPeriod = '1d' | '7d' | '30d' | 'all'
interface BandwidthPoint {
  label: string
  download: number
  upload: number
}
interface BandwidthResponse {
  ok: boolean
  total_download: number
  total_upload: number
  points: BandwidthPoint[]
}

const selectedPeriod = ref<BandwidthPeriod>('7d')
const bandwidthData = ref<BandwidthResponse | null>(null)
const bandwidthLoading = ref(false)
const hoveredBar = ref<number | null>(null)

async function fetchBandwidthStats() {
  bandwidthLoading.value = true
  try {
    const data = await api.get<BandwidthResponse>(`/api/admin/bandwidth-stats?period=${selectedPeriod.value}`)
    if (data && data.ok) {
      bandwidthData.value = data
    }
  } catch {
    // silently fail, chart just shows empty
  } finally {
    bandwidthLoading.value = false
  }
}

// ─── Bar Chart SVG ───
const svgW = 600
const svgH = 300
const pad = { top: 16, right: 16, bottom: 30, left: 48 }

const chartW = computed(() => svgW - pad.left - pad.right)
const chartH = computed(() => svgH - pad.top - pad.bottom)

function formatBytesShort(bytes: number): string {
  if (bytes === 0) return '0 B'
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1048576) return `${(bytes / 1024).toFixed(0)} KB`
  if (bytes < 1073741824) return `${(bytes / 1048576).toFixed(0)} MB`
  return `${(bytes / 1073741824).toFixed(0)} GB`
}

const barChartData = computed(() => {
  const pts = bandwidthData.value?.points
  if (!pts?.length) return { bars: [], yTicks: [], xLabels: [] }

  const values = pts.map(p => p.download + p.upload)
  const max = Math.max(...values, 1)
  const n = pts.length
  const gap = 4
  const barW = (chartW.value - (n - 1) * gap) / n

  const bars = pts.map((p, i) => {
    const val = p.download + p.upload
    const h = (val / max) * chartH.value
    return {
      x: pad.left + i * (barW + gap),
      y: pad.top + chartH.value - h,
      width: barW,
      height: h,
      label: p.label,
      download: p.download,
      upload: p.upload,
      total: val,
    }
  })

  // Y-axis ticks (5 levels: 0 to max)
  const yTicks = Array.from({ length: 5 }, (_, i) => {
    const val = (max / 4) * i
    return {
      y: pad.top + chartH.value - (val / max) * chartH.value,
      label: formatBytesShort(val),
    }
  })

  // X-axis labels (format as MM/DD for dates, HH:00 for hours)
  const xLabels = pts.map((p, i) => ({
    x: pad.left + i * (barW + gap) + barW / 2,
    text: p.label.includes('-') ? p.label.slice(5).replace('-', '/') : p.label,
  }))

  return { bars, yTicks, xLabels }
})

const totalUsage = computed(() => {
  return (bandwidthData.value?.total_download ?? 0) + (bandwidthData.value?.total_upload ?? 0)
})

const hoveredPoint = computed(() => {
  if (hoveredBar.value === null || !barChartData.value.bars.length) return null
  return barChartData.value.bars[hoveredBar.value] ?? null
})

let refreshInterval: ReturnType<typeof setInterval> | null = null

onMounted(() => {
  fetchBandwidthStats()
  refreshInterval = setInterval(fetchBandwidthStats, 60000)
})

onUnmounted(() => {
  if (refreshInterval) clearInterval(refreshInterval)
})

watch(selectedPeriod, () => {
  fetchBandwidthStats()
})

// ─── End Bandwidth Stats ───

const userStatusData = computed(() => {
  const active = customers.list.filter(c => c.status === 'active').length
  const limited = customers.list.filter(c => c.status === 'limited').length
  const disabled = customers.list.filter(c => c.status === 'disabled').length
  const expired = customers.list.filter(c => c.status === 'expired').length
  return [
    { label: `${t('status.active')} (${active})`, value: active },
    { label: `${t('status.limited')} (${limited})`, value: limited },
    { label: `${t('status.disabled')} (${disabled})`, value: disabled },
    { label: `${t('status.expired')} (${expired})`, value: expired },
  ]
})

const donutColors = [
  'var(--color-primary)',
  'var(--color-accent)',
  'var(--color-brand-2)',
  'var(--color-success)',
]

const recentUsers = computed(() => customers.list.slice(0, 6))

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1048576) return `${(bytes / 1024).toFixed(1)} KB`
  if (bytes < 1073741824) return `${(bytes / 1048576).toFixed(1)} MB`
  return `${(bytes / 1073741824).toFixed(2)} GB`
}

function formatDuration(seconds: number): string {
  const h = Math.floor(seconds / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  return h > 0 ? `${h}h ${m}m` : `${m}m`
}
</script>

<template>
  <div class="page dashboard">
    <!-- Stats Grid -->
    <section class="stats-grid" aria-label="Overview statistics">
      <div v-for="stat in statCards" :key="stat.label" class="stat-card stat-card--clickable" @click="handleStatClick(stat.route)">
        <span class="stat-card__icon">{{ stat.icon }}</span>
        <div class="stat-card__body">
          <span class="stat-card__value">{{ stat.value }}</span>
          <span class="stat-card__label">{{ stat.label }}</span>
        </div>
      </div>
    </section>

    <!-- Charts Row -->
    <section class="charts-row">
      <div class="chart-panel chart-panel--traffic">
        <div class="panel-header">
          <div>
            <div class="panel-title">Usage</div>
            <div class="panel-subtitle">Monitor admin traffic usage over time</div>
          </div>
          <select v-model="selectedPeriod" class="period-select" aria-label="Select time period">
            <option value="1d">24 hours</option>
            <option value="7d">7 days</option>
            <option value="30d">30 days</option>
            <option value="all">All time</option>
          </select>
        </div>
        <div v-if="bandwidthLoading && !bandwidthData" class="traffic-fallback">
          <KSkeleton variant="rect" :width="'100%'" :height="300" />
        </div>
        <div v-else-if="barChartData.bars.length > 0" class="bandwidth-chart-wrapper" @mouseleave="hoveredBar = null">
          <svg class="bar-chart" :viewBox="`0 0 ${svgW} ${svgH}`" preserveAspectRatio="xMidYMid meet" role="img" aria-label="Bar chart showing bandwidth usage">
            <!-- Dashed horizontal grid lines -->
            <line
              v-for="tick in barChartData.yTicks"
              :key="'grid-' + tick.label"
              :x1="pad.left"
              :y1="tick.y"
              :x2="svgW - pad.right"
              :y2="tick.y"
              stroke="var(--color-border)"
              stroke-width="1"
              stroke-dasharray="4 3"
              opacity="0.5"
            />
            <!-- Y-axis labels -->
            <text
              v-for="tick in barChartData.yTicks"
              :key="'y-' + tick.label"
              :x="pad.left - 8"
              :y="tick.y + 4"
              text-anchor="end"
              class="chart-axis-label"
            >{{ tick.label }}</text>
            <!-- Bars with rounded top -->
            <rect
              v-for="(bar, i) in barChartData.bars"
              :key="'bar-' + i"
              class="chart-bar"
              :class="{ 'chart-bar--hovered': hoveredBar === i }"
              :x="bar.x"
              :y="bar.y"
              :width="bar.width"
              :height="bar.height"
              :rx="6"
              :ry="6"
              fill="var(--color-primary)"
              @mouseenter="hoveredBar = i"
            />
            <!-- Flat bottom rectangles to mask the bottom rounded corners -->
            <rect
              v-for="(bar, i) in barChartData.bars"
              :key="'bar-base-' + i"
              :x="bar.x"
              :y="pad.top + chartH - 6"
              :width="bar.width"
              :height="6"
              fill="var(--color-primary)"
              :opacity="hoveredBar === i ? 0.8 : 1"
              class="chart-bar-base"
              @mouseenter="hoveredBar = i"
            />
            <!-- X-axis labels -->
            <text
              v-for="(lbl, i) in barChartData.xLabels"
              :key="'x-' + i"
              :x="lbl.x"
              :y="svgH - 8"
              text-anchor="middle"
              class="chart-axis-label"
            >{{ lbl.text }}</text>
          </svg>
          <!-- Hover tooltip -->
          <div
            v-if="hoveredPoint !== null && hoveredBar !== null"
            class="bar-tooltip"
            :style="{ left: `${((hoveredPoint.x + hoveredPoint.width / 2) / svgW) * 100}%`, top: `${(hoveredPoint.y / svgH) * 100}%` }"
            role="tooltip"
          >
            <span class="bar-tooltip__label">{{ hoveredPoint.label }}</span>
            <span class="bar-tooltip__value">{{ formatBytes(hoveredPoint.total) }}</span>
            <span class="bar-tooltip__caret"></span>
          </div>
        </div>
        <div v-else class="traffic-fallback">
          <p class="traffic-note">{{ t('dashboard.chart_loading') }}</p>
        </div>
        <!-- Footer -->
        <div class="chart-footer">
          <div class="chart-footer__total">
            Usage During Period: <span class="font-mono">{{ formatBytes(totalUsage) }}</span>
          </div>
          <div class="chart-footer__desc">Total traffic usage across all servers</div>
        </div>
      </div>
      <div class="chart-panel chart-panel--donut">
        <h4 class="panel-title">{{ t('dashboard.user_status') }}</h4>
        <KChart
          v-if="customers.list.length > 0"
          type="donut"
          :data="userStatusData"
          :height="200"
          :animate="true"
          :interactive="true"
        />
        <KSkeleton v-else variant="rect" :width="'100%'" :height="200" />
        <!-- Status Legend -->
        <div v-if="customers.list.length > 0" class="donut-legend">
          <div
            v-for="(item, i) in userStatusData"
            :key="item.label"
            class="donut-legend__item"
          >
            <span class="donut-legend__dot" :style="{ background: donutColors[i] }" />
            <span class="donut-legend__label">{{ item.label }}</span>
            <span class="donut-legend__value">{{ item.value }}</span>
          </div>
        </div>
      </div>
    </section>

    <!-- Recent Users -->
    <section class="panel">
      <h4 class="panel-title">{{ t('dashboard.recent_users') }}</h4>
      <div class="recent-table">
        <table class="mini-table" role="table">
          <thead>
            <tr>
              <th>{{ t('user.username') }}</th>
              <th>{{ t('user.display_name') }}</th>
              <th>{{ t('user.status') }}</th>
              <th>{{ t('user.plan') }}</th>
              <th>{{ t('user.created') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="user in recentUsers" :key="user.id">
              <td class="text-primary">{{ user.username }}</td>
              <td>{{ user.display_name }}</td>
              <td><KStatusPill :status="user.status" size="sm" /></td>
              <td>{{ user.plan }}</td>
              <td class="text-muted">{{ formatDate(user.created_at) }}</td>
            </tr>
            <tr v-if="recentUsers.length === 0">
              <td colspan="5" class="text-muted text-center">{{ t('empty.no_users') }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>

    <!-- Node Health + Live Sessions -->
    <section class="bottom-row">
      <div class="panel panel--nodes">
        <h4 class="panel-title">{{ t('dashboard.node_health') }}</h4>
        <div class="node-cards">
          <div v-for="node in nodes.list" :key="node.id" class="node-health-card">
            <div class="node-health-card__header">
              <span class="node-health-card__name">{{ node.name }}</span>
              <KStatusPill :status="node.status" size="sm" />
            </div>
            <div class="node-health-card__metrics">
              <div class="metric-bar">
                <span class="metric-bar__label">CPU</span>
                <div class="metric-bar__track">
                  <div class="metric-bar__fill" :style="{ width: `${node.status_metrics?.cpu_percent ?? 0}%` }" />
                </div>
                <span class="metric-bar__value">{{ node.status_metrics?.cpu_percent ?? 0 }}%</span>
              </div>
              <div class="metric-bar">
                <span class="metric-bar__label">RAM</span>
                <div class="metric-bar__track">
                  <div class="metric-bar__fill metric-bar__fill--accent" :style="{ width: `${node.status_metrics?.ram_percent ?? 0}%` }" />
                </div>
                <span class="metric-bar__value">{{ node.status_metrics?.ram_percent ?? 0 }}%</span>
              </div>
            </div>
          </div>
          <p v-if="nodes.list.length === 0" class="text-muted">{{ t('empty.no_nodes') }}</p>
        </div>
      </div>

      <div class="panel panel--sessions">
        <h4 class="panel-title">{{ t('dashboard.live_sessions') }}</h4>
        <div class="sessions-list">
          <div v-for="session in realtime.liveSessions.slice(0, 8)" :key="session.id" class="session-row">
            <span class="session-row__user">{{ session.username }}</span>
            <span class="session-row__ip text-muted">{{ session.framed_ip }}</span>
            <span class="session-row__node text-muted">{{ session.node_name }}</span>
            <span class="session-row__traffic">{{ formatBytes(session.input_bytes + session.output_bytes) }}</span>
            <span class="session-row__duration text-muted">{{ formatDuration(session.session_seconds) }}</span>
          </div>
          <p v-if="realtime.liveSessions.length === 0" class="text-muted">{{ t('empty.no_sessions') }}</p>
        </div>
      </div>
    </section>
  </div>
</template>

<style scoped>
.dashboard { display: flex; flex-direction: column; gap: var(--space-6); }

.stats-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: var(--space-4); }
.stat-card { display: flex; align-items: center; gap: var(--space-3); padding: var(--space-4); background: var(--color-surface); border: 1px solid var(--color-border); border-radius: var(--radius-lg); }
.stat-card--clickable { cursor: pointer; transition: transform 0.15s, border-color 0.15s; }
.stat-card--clickable:hover { transform: translateY(-2px); border-color: rgba(37, 99, 235, 0.3); }
.stat-card__icon { font-size: 1.5rem; }
.stat-card__body { display: flex; flex-direction: column; }
.stat-card__value { font-size: var(--text-xl); font-weight: var(--font-bold); color: var(--color-text); }
.stat-card__label { font-size: var(--text-xs); color: var(--color-muted); text-transform: uppercase; letter-spacing: var(--tracking-wider); }

.charts-row { display: grid; grid-template-columns: 2fr 1fr; gap: var(--space-4); }
.chart-panel { padding: var(--space-3); background: var(--color-surface); border: 1px solid var(--color-border); border-radius: var(--radius-lg); }

/* ─── Panel Header ─── */
.panel-header { display: flex; align-items: flex-start; justify-content: space-between; margin-bottom: var(--space-3); }
.panel-title { margin: 0; font-size: var(--text-sm); font-weight: var(--font-semibold); color: var(--color-text); }
.panel-subtitle { font-size: var(--text-xs); color: var(--color-muted); margin-top: 4px; }

/* ─── Period Select Dropdown ─── */
.period-select {
  height: 32px;
  width: 128px;
  padding: 0 12px;
  font-size: 12px;
  border-radius: var(--radius-md);
  border: 1px solid var(--color-border);
  background: transparent;
  color: var(--color-text);
  appearance: none;
  -webkit-appearance: none;
  cursor: pointer;
  background-image: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='12' height='12' viewBox='0 0 24 24' fill='none' stroke='%239ca3af' stroke-width='2' stroke-linecap='round' stroke-linejoin='round'%3E%3Cpolyline points='6 9 12 15 18 9'%3E%3C/polyline%3E%3C/svg%3E");
  background-repeat: no-repeat;
  background-position: right 10px center;
  padding-right: 28px;
}
.period-select:focus {
  outline: none;
  border-color: var(--color-primary);
  box-shadow: 0 0 0 2px rgba(59, 130, 246, 0.15);
}
.period-select option {
  background: var(--color-surface);
  color: var(--color-text);
}

/* ─── Bar Chart ─── */
.bandwidth-chart-wrapper { position: relative; }

.bar-chart {
  width: 100%;
  height: 300px;
  display: block;
}
.bar-chart text {
  fill: var(--color-muted);
  font-size: 10px;
  font-family: var(--font-family);
}
.chart-axis-label {
  font-size: 10px;
  font-weight: 500;
  fill: var(--color-muted);
}
.chart-bar {
  transition: opacity 0.15s ease;
  cursor: pointer;
}
.chart-bar--hovered,
.chart-bar:hover {
  opacity: 0.8;
}
.chart-bar-base {
  pointer-events: none;
  transition: opacity 0.15s ease;
}

/* ─── Bar Tooltip ─── */
.bar-tooltip {
  position: absolute;
  transform: translateX(-50%) translateY(-100%);
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 2px;
  padding: 8px 14px;
  background: rgba(15, 15, 20, 0.92);
  backdrop-filter: blur(8px);
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: var(--radius-md);
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.4);
  pointer-events: none;
  z-index: var(--z-tooltip, 50);
  font-size: 11px;
  min-width: 90px;
  margin-top: -8px;
}
.bar-tooltip__label {
  font-weight: var(--font-medium);
  color: var(--color-muted);
  font-size: 10px;
}
.bar-tooltip__value {
  font-weight: var(--font-bold);
  color: var(--color-text);
  font-size: 13px;
  font-family: var(--font-mono, monospace);
}
.bar-tooltip__caret {
  position: absolute;
  bottom: -5px;
  left: 50%;
  transform: translateX(-50%);
  width: 0;
  height: 0;
  border-left: 5px solid transparent;
  border-right: 5px solid transparent;
  border-top: 5px solid rgba(15, 15, 20, 0.92);
}

/* ─── Chart Footer ─── */
.chart-footer {
  padding-top: var(--space-2);
  border-top: 1px solid var(--color-border);
  margin-top: var(--space-3);
}
.chart-footer__total {
  font-size: var(--text-sm);
  color: var(--color-muted);
}
.chart-footer__total .font-mono {
  font-family: var(--font-mono, monospace);
  font-weight: 600;
  color: var(--color-text);
}
.chart-footer__desc {
  font-size: var(--text-xs);
  color: var(--color-muted);
  margin-top: 2px;
}

.traffic-fallback { display: flex; flex-direction: column; align-items: center; justify-content: center; min-height: 300px; gap: var(--space-4); }
.traffic-note { font-size: var(--text-xs); color: var(--color-muted); margin: 0; }

.panel { padding: var(--space-4); background: var(--color-surface); border: 1px solid var(--color-border); border-radius: var(--radius-lg); }

/* ─── Donut Legend ─── */
.donut-legend { display: flex; flex-direction: column; gap: var(--space-2); margin-top: var(--space-3); padding-top: var(--space-3); border-top: 1px solid var(--color-border); }
.donut-legend__item { display: flex; align-items: center; gap: var(--space-2); font-size: var(--text-sm); }
.donut-legend__dot { width: 10px; height: 10px; border-radius: var(--radius-full); flex-shrink: 0; }
.donut-legend__label { flex: 1; color: var(--color-muted); }
.donut-legend__value { font-weight: var(--font-semibold); color: var(--color-text); }

.mini-table { width: 100%; border-collapse: collapse; font-size: var(--text-sm); }
.mini-table th { text-align: left; padding: var(--space-2) var(--space-3); color: var(--color-muted); font-size: var(--text-xs); text-transform: uppercase; letter-spacing: var(--tracking-wider); border-bottom: 1px solid var(--color-border); }
.mini-table td { padding: var(--space-2) var(--space-3); border-bottom: 1px solid var(--color-border); color: var(--color-text); }

.bottom-row { display: grid; grid-template-columns: 1fr 1fr; gap: var(--space-4); }
.node-cards { display: flex; flex-direction: column; gap: var(--space-3); }
.node-health-card { padding: var(--space-3); background: var(--color-surface-2); border-radius: var(--radius-md); }
.node-health-card__header { display: flex; justify-content: space-between; align-items: center; margin-bottom: var(--space-2); }
.node-health-card__name { font-weight: var(--font-medium); font-size: var(--text-sm); }
.node-health-card__metrics { display: flex; flex-direction: column; gap: var(--space-2); }

.metric-bar { display: flex; align-items: center; gap: var(--space-2); }
.metric-bar__label { font-size: var(--text-xs); color: var(--color-muted); width: 32px; }
.metric-bar__track { flex: 1; height: 6px; background: var(--color-border); border-radius: 3px; overflow: hidden; }
.metric-bar__fill { height: 100%; background: var(--color-primary); border-radius: 3px; transition: width 0.3s ease; }
.metric-bar__fill--accent { background: var(--color-accent); }
.metric-bar__value { font-size: var(--text-xs); color: var(--color-muted); width: 32px; text-align: right; }

.sessions-list { display: flex; flex-direction: column; gap: var(--space-2); }
.session-row { display: grid; grid-template-columns: 1.5fr 1fr 1fr 1fr 0.8fr; gap: var(--space-2); padding: var(--space-2) 0; border-bottom: 1px solid var(--color-border); font-size: var(--text-sm); align-items: center; }
.session-row__user { font-weight: var(--font-medium); color: var(--color-primary); }

.text-primary { color: var(--color-primary); }
.text-muted { color: var(--color-muted); }
.text-center { text-align: center; }

@media (max-width: 768px) {
  .charts-row, .bottom-row { grid-template-columns: 1fr; }
}
</style>
