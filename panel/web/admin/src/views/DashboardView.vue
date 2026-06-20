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

// ─── Bandwidth Stats (new period-based) ───
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

const selectedPeriod = ref<BandwidthPeriod>('1d')
const bandwidthData = ref<BandwidthResponse | null>(null)
const bandwidthLoading = ref(false)
const hoveredBar = ref<number | null>(null)
const tooltipPos = ref({ x: 0, y: 0 })

const periodLabels: Record<BandwidthPeriod, string> = {
  '1d': '1D',
  '7d': '1W',
  '30d': '1M',
  'all': 'All',
}

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

const bandwidthChartData = computed(() => {
  if (!bandwidthData.value?.points) return []
  return bandwidthData.value.points.map((p) => ({
    label: p.label,
    value: p.download + p.upload,
    color: 'var(--color-primary)',
  }))
})

// ─── Custom Stacked Area Chart ───
const svgW = 500
const svgH = 180
const pad = { top: 10, right: 10, bottom: 24, left: 10 }

const chartW = computed(() => svgW - pad.left - pad.right)
const chartH = computed(() => svgH - pad.top - pad.bottom)

const maxBandwidth = computed(() => {
  if (!bandwidthData.value?.points?.length) return 1
  return Math.max(...bandwidthData.value.points.map(p => p.download + p.upload), 1)
})

function scaleX(i: number, total: number): number {
  if (total <= 1) return pad.left
  return pad.left + (i / (total - 1)) * chartW.value
}

function scaleY(val: number): number {
  return pad.top + chartH.value - (val / maxBandwidth.value) * chartH.value
}

function buildSmoothPath(points: { x: number; y: number }[]): string {
  if (points.length < 2) return ''
  let d = `M ${points[0].x} ${points[0].y}`
  for (let i = 1; i < points.length; i++) {
    const prev = points[i - 1]
    const curr = points[i]
    const cpx1 = prev.x + (curr.x - prev.x) / 3
    const cpy1 = prev.y
    const cpx2 = curr.x - (curr.x - prev.x) / 3
    const cpy2 = curr.y
    d += ` C ${cpx1} ${cpy1}, ${cpx2} ${cpy2}, ${curr.x} ${curr.y}`
  }
  return d
}

const downloadAreaPath = computed(() => {
  const pts = bandwidthData.value?.points
  if (!pts?.length) return ''
  const n = pts.length
  const line: { x: number; y: number }[] = pts.map((p, i) => ({
    x: scaleX(i, n),
    y: scaleY(p.download),
  }))
  const baseline = pad.top + chartH.value
  const linePath = buildSmoothPath(line)
  return `${linePath} L ${line[n - 1].x} ${baseline} L ${line[0].x} ${baseline} Z`
})

const downloadLinePath = computed(() => {
  const pts = bandwidthData.value?.points
  if (!pts?.length) return ''
  const n = pts.length
  const line = pts.map((p, i) => ({ x: scaleX(i, n), y: scaleY(p.download) }))
  return buildSmoothPath(line)
})

const uploadAreaPath = computed(() => {
  const pts = bandwidthData.value?.points
  if (!pts?.length) return ''
  const n = pts.length
  // Upload stacked on top of download
  const topLine: { x: number; y: number }[] = pts.map((p, i) => ({
    x: scaleX(i, n),
    y: scaleY(p.download + p.upload),
  }))
  const bottomLine: { x: number; y: number }[] = pts.map((p, i) => ({
    x: scaleX(i, n),
    y: scaleY(p.download),
  }))
  const topPath = buildSmoothPath(topLine)
  // Close area: go along bottom line in reverse
  const reversedBottom = [...bottomLine].reverse()
  let closePath = ` L ${reversedBottom[0].x} ${reversedBottom[0].y}`
  for (let i = 1; i < reversedBottom.length; i++) {
    const prev = reversedBottom[i - 1]
    const curr = reversedBottom[i]
    const cpx1 = prev.x + (curr.x - prev.x) / 3
    const cpy1 = prev.y
    const cpx2 = curr.x - (curr.x - prev.x) / 3
    const cpy2 = curr.y
    closePath += ` C ${cpx1} ${cpy1}, ${cpx2} ${cpy2}, ${curr.x} ${curr.y}`
  }
  return `${topPath}${closePath} Z`
})

const uploadLinePath = computed(() => {
  const pts = bandwidthData.value?.points
  if (!pts?.length) return ''
  const n = pts.length
  const line = pts.map((p, i) => ({ x: scaleX(i, n), y: scaleY(p.download + p.upload) }))
  return buildSmoothPath(line)
})

const xLabels = computed(() => {
  const pts = bandwidthData.value?.points
  if (!pts?.length) return []
  const n = pts.length
  if (n <= 6) {
    return pts.map((p, i) => ({ x: scaleX(i, n), text: p.label }))
  }
  // Show ~5 labels evenly spaced
  const step = Math.max(Math.floor(n / 5), 1)
  const labels: { x: number; text: string }[] = []
  for (let i = 0; i < n; i += step) {
    labels.push({ x: scaleX(i, n), text: pts[i].label })
  }
  // Always include last
  if (labels[labels.length - 1]?.text !== pts[n - 1].label) {
    labels.push({ x: scaleX(n - 1, n), text: pts[n - 1].label })
  }
  return labels
})

const hoveredPoint = computed(() => {
  if (hoveredBar.value === null || !bandwidthData.value?.points) return null
  return bandwidthData.value.points[hoveredBar.value] ?? null
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
          <h4 class="panel-title">{{ t('dashboard.data_usage') }}</h4>
          <div class="period-pills">
            <button
              v-for="(label, key) in periodLabels"
              :key="key"
              class="period-pill"
              :class="{ 'period-pill--active': selectedPeriod === key }"
              @click="selectedPeriod = key as BandwidthPeriod"
            >
              {{ label }}
            </button>
          </div>
        </div>
        <div v-if="bandwidthLoading && !bandwidthData" class="traffic-fallback">
          <KSkeleton variant="rect" :width="'100%'" :height="200" />
        </div>
        <div v-else-if="bandwidthChartData.length > 0" class="bandwidth-chart-wrapper" @mouseleave="hoveredBar = null">
          <svg class="stacked-chart" :viewBox="`0 0 ${svgW} ${svgH}`" preserveAspectRatio="none" role="img" aria-label="Stacked area chart showing download and upload bandwidth">
            <defs>
              <linearGradient id="dl-gradient" x1="0" y1="0" x2="0" y2="1">
                <stop offset="0%" stop-color="#3b82f6" stop-opacity="0.5" />
                <stop offset="100%" stop-color="#3b82f6" stop-opacity="0.05" />
              </linearGradient>
              <linearGradient id="ul-gradient" x1="0" y1="0" x2="0" y2="1">
                <stop offset="0%" stop-color="#8b5cf6" stop-opacity="0.5" />
                <stop offset="100%" stop-color="#8b5cf6" stop-opacity="0.05" />
              </linearGradient>
            </defs>
            <!-- Download area (blue, bottom) -->
            <path :d="downloadAreaPath" fill="url(#dl-gradient)" />
            <path :d="downloadLinePath" fill="none" stroke="#3b82f6" stroke-width="2" />
            <!-- Upload area (purple, stacked on top) -->
            <path :d="uploadAreaPath" fill="url(#ul-gradient)" />
            <path :d="uploadLinePath" fill="none" stroke="#8b5cf6" stroke-width="2" />
            <!-- Hover columns (invisible rects for interaction) -->
            <rect
              v-for="(_, i) in bandwidthData?.points ?? []"
              :key="i"
              :x="scaleX(i, bandwidthData!.points.length) - (chartW / (bandwidthData!.points.length) / 2)"
              :y="pad.top"
              :width="chartW / bandwidthData!.points.length"
              :height="chartH"
              fill="transparent"
              @mouseenter="hoveredBar = i"
              @mouseleave="hoveredBar = null"
            />
            <!-- Hover indicator line -->
            <line
              v-if="hoveredBar !== null && bandwidthData?.points"
              :x1="scaleX(hoveredBar, bandwidthData.points.length)"
              :y1="pad.top"
              :x2="scaleX(hoveredBar, bandwidthData.points.length)"
              :y2="pad.top + chartH"
              stroke="var(--color-muted)"
              stroke-width="1"
              stroke-dasharray="3 3"
              opacity="0.5"
            />
            <!-- X-axis labels -->
            <text
              v-for="lbl in xLabels"
              :key="lbl.text"
              :x="lbl.x"
              :y="svgH - 4"
              text-anchor="middle"
            >{{ lbl.text }}</text>
          </svg>
          <!-- Legend -->
          <div class="stacked-chart-legend">
            <span class="stacked-chart-legend__item">
              <span class="stacked-chart-legend__dot stacked-chart-legend__dot--dl"></span>
              Download
            </span>
            <span class="stacked-chart-legend__item">
              <span class="stacked-chart-legend__dot stacked-chart-legend__dot--ul"></span>
              Upload
            </span>
          </div>
          <!-- Custom Tooltip -->
          <div
            v-if="hoveredPoint"
            class="bandwidth-tooltip"
            role="tooltip"
          >
            <span class="bandwidth-tooltip__label">{{ hoveredPoint.label }}</span>
            <div class="bandwidth-tooltip__row">
              <span class="bandwidth-tooltip__dot bandwidth-tooltip__dot--download"></span>
              <span>Download: {{ formatBytes(hoveredPoint.download) }}</span>
            </div>
            <div class="bandwidth-tooltip__row">
              <span class="bandwidth-tooltip__dot bandwidth-tooltip__dot--upload"></span>
              <span>Upload: {{ formatBytes(hoveredPoint.upload) }}</span>
            </div>
            <div class="bandwidth-tooltip__total">
              Total: {{ formatBytes(hoveredPoint.download + hoveredPoint.upload) }}
            </div>
          </div>
        </div>
        <div v-else class="traffic-fallback">
          <p class="traffic-note">{{ t('dashboard.chart_loading') }}</p>
        </div>
        <div class="traffic-summary">
          <div class="traffic-stat">
            <span class="traffic-stat__label">{{ t('dashboard.total_downloaded') }}</span>
            <span class="traffic-stat__value">{{ formatBytes(bandwidthData?.total_download ?? 0) }}</span>
          </div>
          <div class="traffic-stat">
            <span class="traffic-stat__label">{{ t('dashboard.total_uploaded') }}</span>
            <span class="traffic-stat__value">{{ formatBytes(bandwidthData?.total_upload ?? 0) }}</span>
          </div>
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
.chart-panel { padding: var(--space-4); background: var(--color-surface); border: 1px solid var(--color-border); border-radius: var(--radius-lg); }

/* ─── Panel Header with Period Pills ─── */
.panel-header { display: flex; align-items: center; justify-content: space-between; margin-bottom: var(--space-3); }
.period-pills { display: flex; gap: var(--space-1); }
.period-pill {
  padding: var(--space-1) var(--space-2);
  font-size: var(--text-xs);
  font-weight: var(--font-medium);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  background: transparent;
  color: var(--color-muted);
  cursor: pointer;
  transition: all 0.15s ease;
  line-height: 1;
}
.period-pill:hover { color: var(--color-text); border-color: var(--color-primary); }
.period-pill--active {
  background: var(--color-primary);
  color: #fff;
  border-color: var(--color-primary);
}

/* ─── Bandwidth Chart ─── */
.bandwidth-chart-wrapper { position: relative; }

.bandwidth-timeline {
  display: flex;
  justify-content: space-between;
  padding: var(--space-1) var(--space-2) 0;
  font-size: var(--text-xs);
  color: var(--color-muted);
}

.bandwidth-tooltip {
  position: absolute;
  top: var(--space-2);
  right: var(--space-2);
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  padding: var(--space-2) var(--space-3);
  background: var(--color-surface-2, var(--color-surface));
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  box-shadow: var(--shadow-md);
  pointer-events: none;
  z-index: var(--z-tooltip, 50);
  font-size: var(--text-xs);
  min-width: 140px;
}
.bandwidth-tooltip__label { font-weight: var(--font-semibold); color: var(--color-text); margin-bottom: 2px; }
.bandwidth-tooltip__row { display: flex; align-items: center; gap: var(--space-1); color: var(--color-muted); }
.bandwidth-tooltip__dot { width: 8px; height: 8px; border-radius: var(--radius-full); flex-shrink: 0; }
.bandwidth-tooltip__dot--download { background: #3b82f6; }
.bandwidth-tooltip__dot--upload { background: #8b5cf6; }
.bandwidth-tooltip__total { font-weight: var(--font-semibold); color: var(--color-text); border-top: 1px solid var(--color-border); padding-top: var(--space-1); margin-top: var(--space-1); }

/* ─── Traffic Summary ─── */
.traffic-summary { display: flex; gap: var(--space-6); margin-top: var(--space-3); }
.traffic-fallback { display: flex; flex-direction: column; align-items: center; justify-content: center; min-height: 200px; gap: var(--space-4); }
.traffic-stat { display: flex; flex-direction: column; align-items: center; gap: var(--space-1); }
.traffic-stat__label { font-size: var(--text-xs); color: var(--color-muted); text-transform: uppercase; letter-spacing: var(--tracking-wider); }
.traffic-stat__value { font-size: var(--text-xl); font-weight: var(--font-bold); color: var(--color-text); }
.traffic-note { font-size: var(--text-xs); color: var(--color-muted); margin: 0; }

.panel { padding: var(--space-4); background: var(--color-surface); border: 1px solid var(--color-border); border-radius: var(--radius-lg); }
.panel-title { margin: 0; font-size: var(--text-sm); font-weight: var(--font-semibold); color: var(--color-text); }

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
  .traffic-summary { flex-direction: column; align-items: center; }
}

/* ─── Stacked Area Chart ─── */
.stacked-chart {
  width: 100%;
  height: 180px;
  display: block;
}
.stacked-chart text {
  fill: var(--color-muted);
  font-size: 9px;
  font-family: var(--font-family);
}

.stacked-chart-legend {
  display: flex;
  gap: var(--space-4);
  justify-content: center;
  margin-top: var(--space-2);
  font-size: var(--text-xs);
  color: var(--color-muted);
}
.stacked-chart-legend__item {
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
}
.stacked-chart-legend__dot {
  width: 8px;
  height: 8px;
  border-radius: var(--radius-full);
}
.stacked-chart-legend__dot--dl { background: #3b82f6; }
.stacked-chart-legend__dot--ul { background: #8b5cf6; }
</style>
