<template>
  <div class="k-table-wrapper">
    <!-- Filter Row -->
    <div v-if="hasFilters" class="k-table__filters" role="search" aria-label="Table filters">
      <div v-for="col in columns.filter(c => c.filterable)" :key="`filter-${col.key}`" class="k-table__filter-cell">
        <label :for="`filter-${col.key}`" class="k-table__filter-label">{{ col.label }}</label>
        <input v-if="!col.filterType || col.filterType === 'text'" :id="`filter-${col.key}`" type="text"
          class="k-table__filter-input" :placeholder="`Filter ${col.label}...`" :value="filters[col.key] ?? ''"
          @input="onFilterInput(col.key, ($event.target as HTMLInputElement).value)" />
        <select v-else-if="col.filterType === 'select'" :id="`filter-${col.key}`" class="k-table__filter-input"
          :value="filters[col.key] ?? ''" @change="onFilterChange(col.key, ($event.target as HTMLSelectElement).value)">
          <option value="">All</option>
          <option v-for="opt in col.filterOptions" :key="opt.value" :value="opt.value">{{ opt.label }}</option>
        </select>
        <div v-else-if="col.filterType === 'date-range'" class="k-table__filter-dates">
          <input :id="`filter-${col.key}-from`" type="date" class="k-table__filter-input k-table__filter-input--date"
            :value="filters[col.key]?.from ?? ''" :aria-label="`${col.label} from`"
            @change="onDateFilter(col.key, 'from', ($event.target as HTMLInputElement).value)" />
          <span class="k-table__filter-sep">–</span>
          <input :id="`filter-${col.key}-to`" type="date" class="k-table__filter-input k-table__filter-input--date"
            :value="filters[col.key]?.to ?? ''" :aria-label="`${col.label} to`"
            @change="onDateFilter(col.key, 'to', ($event.target as HTMLInputElement).value)" />
        </div>
      </div>
    </div>

    <!-- Table -->
    <div class="k-table__scroll-container">
      <table class="k-table" role="table" aria-label="Data table">
        <thead :class="{ 'k-table__head--sticky': stickyHeader }">
          <tr role="row">
            <th v-if="selectable" class="k-table__th k-table__th--check" role="columnheader">
              <span
                role="checkbox"
                tabindex="0"
                class="k-check"
                :class="{ 'k-check--checked': allSelected, 'k-check--indeterminate': someSelected && !allSelected }"
                :aria-checked="allSelected ? 'true' : someSelected ? 'mixed' : 'false'"
                aria-label="Select all rows"
                @click="toggleSelectAll"
                @keydown.space.prevent="toggleSelectAll"
                @keydown.enter.prevent="toggleSelectAll"
              >
                <svg v-if="allSelected" class="k-check__icon" viewBox="0 0 12 12" fill="none"><path d="M2.5 6.5L5 9l4.5-6" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/></svg>
                <svg v-else-if="someSelected && !allSelected" class="k-check__icon" viewBox="0 0 12 12" fill="none"><path d="M3 6h6" stroke="currentColor" stroke-width="2" stroke-linecap="round"/></svg>
              </span>
            </th>
            <th v-for="col in columns" :key="col.key" role="columnheader"
              :class="['k-table__th', col.sortable && 'k-table__th--sortable', sortKey === col.key && 'k-table__th--sorted', `k-table__th--${col.align ?? 'left'}`]"
              :style="col.width ? { width: col.width } : undefined"
              :aria-sort="sortKey === col.key ? (sortDir === 'asc' ? 'ascending' : 'descending') : undefined"
              :tabindex="col.sortable ? 0 : undefined"
              @click="col.sortable ? toggleSort(col.key) : undefined"
              @keydown.enter="col.sortable ? toggleSort(col.key) : undefined"
              @keydown.space.prevent="col.sortable ? toggleSort(col.key) : undefined">
              <span class="k-table__th-content">
                {{ col.label }}
                <span v-if="col.sortable" class="k-table__sort-icon" aria-hidden="true">
                  {{ sortKey === col.key ? (sortDir === 'asc' ? '▲' : '▼') : '⇅' }}
                </span>
              </span>
            </th>
          </tr>
        </thead>
        <tbody>
          <!-- Loading skeleton -->
          <template v-if="loading">
            <tr v-for="i in pageSize" :key="`skeleton-${i}`" class="k-table__row" role="row">
              <td v-if="selectable" class="k-table__td" role="cell"><span class="k-skeleton k-skeleton--sm" /></td>
              <td v-for="col in columns" :key="`skel-${col.key}-${i}`" class="k-table__td" role="cell">
                <span class="k-skeleton" />
              </td>
            </tr>
          </template>
          <!-- Empty state -->
          <tr v-else-if="displayedRows.length === 0" role="row">
            <td :colspan="totalColSpan" class="k-table__empty" role="cell">
              <span v-if="emptyIcon" class="k-table__empty-icon">{{ emptyIcon }}</span>
              <span>{{ emptyText || 'No data available' }}</span>
            </td>
          </tr>
          <!-- Data rows -->
          <template v-else>
            <tr v-for="(row, idx) in displayedRows" :key="getRowKey(row, idx)" role="row" tabindex="0"
              :class="['k-table__row', isSelected(row) && 'k-table__row--selected']"
              @click="emit('row-click', row)" @keydown.enter="emit('row-click', row)"
              @keydown="onRowKeydown($event, idx)">
              <td v-if="selectable" class="k-table__td k-table__td--check" role="cell">
                <span
                  role="checkbox"
                  tabindex="-1"
                  class="k-check"
                  :class="{ 'k-check--checked': isSelected(row) }"
                  :aria-checked="isSelected(row) ? 'true' : 'false'"
                  :aria-label="`Select row ${idx + 1}`"
                  @click.stop="toggleSelect(row)"
                >
                  <svg v-if="isSelected(row)" class="k-check__icon" viewBox="0 0 12 12" fill="none"><path d="M2.5 6.5L5 9l4.5-6" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/></svg>
                </span>
              </td>
              <td v-for="col in columns" :key="`${col.key}-${idx}`" role="cell"
                :class="['k-table__td', `k-table__td--${col.align ?? 'left'}`]">
                <slot :name="`cell-${col.key}`" :row="row" :value="row[col.key]">{{ row[col.key] }}</slot>
              </td>
            </tr>
          </template>
        </tbody>
      </table>
    </div>

    <!-- Pagination footer -->
    <div v-if="showPagination" class="k-table__footer">
      <span class="k-table__page-info">{{ pageStart + 1 }}–{{ pageEnd }} of {{ processedData.length }}</span>
      <nav class="k-table__page-controls" aria-label="Table pagination">
        <button class="k-table__page-btn" :disabled="internalPage <= 1" aria-label="Previous page"
          @click="goToPage(internalPage - 1)">‹</button>
        <button v-for="p in visiblePages" :key="p" :class="['k-table__page-btn', p === internalPage && 'k-table__page-btn--active']"
          :aria-label="`Page ${p}`" :aria-current="p === internalPage ? 'page' : undefined"
          @click="goToPage(p)">{{ p }}</button>
        <button class="k-table__page-btn" :disabled="internalPage >= totalPages" aria-label="Next page"
          @click="goToPage(internalPage + 1)">›</button>
      </nav>
      <div class="k-table__export">
        <button class="k-table__export-btn" @click="emit('export', 'csv')" aria-label="Export CSV">CSV</button>
        <button class="k-table__export-btn" @click="emit('export', 'json')" aria-label="Export JSON">JSON</button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import type { KDataTableColumn, KDataTableProps } from '@koris/types/components'

const props = withDefaults(defineProps<KDataTableProps>(), {
  loading: false,
  selectable: false,
  stickyHeader: false,
  emptyText: 'No data available',
  emptyIcon: '',
  rowHeight: 48,
  pageSize: 10,
  currentPage: 1,
  serverSide: false,
  virtualScroll: false,
})

const emit = defineEmits<{
  (e: 'sort', payload: { key: string; direction: 'asc' | 'desc' }): void
  (e: 'filter', payload: Record<string, any>): void
  (e: 'page-change', page: number): void
  (e: 'row-click', row: any): void
  (e: 'selection-change', selected: any[]): void
  (e: 'export', format: 'csv' | 'json'): void
}>()

const sortKey = ref<string>('')
const sortDir = ref<'asc' | 'desc'>('asc')
const filters = ref<Record<string, any>>({})
const internalPage = ref(props.currentPage)
const selected = ref<any[]>([])

const hasFilters = computed(() => props.columns.some(c => c.filterable))
const totalColSpan = computed(() => props.columns.length + (props.selectable ? 1 : 0))

const processedData = computed(() => {
  if (props.serverSide) return props.data
  let result = [...props.data]

  // Apply filters
  for (const col of props.columns) {
    if (!col.filterable) continue
    const val = filters.value[col.key]
    if (!val) continue
    if (col.filterType === 'date-range') {
      if (val.from || val.to) {
        result = result.filter(row => {
          const d = row[col.key]
          if (!d) return false
          if (val.from && d < val.from) return false
          if (val.to && d > val.to) return false
          return true
        })
      }
    } else if (col.filterType === 'select') {
      result = result.filter(row => String(row[col.key]) === val)
    } else {
      const lower = val.toLowerCase()
      result = result.filter(row => String(row[col.key] ?? '').toLowerCase().includes(lower))
    }
  }

  // Apply sort
  if (sortKey.value) {
    const key = sortKey.value
    const dir = sortDir.value === 'asc' ? 1 : -1
    result.sort((a, b) => {
      const aVal = a[key], bVal = b[key]
      if (aVal == null && bVal == null) return 0
      if (aVal == null) return 1
      if (bVal == null) return -1
      if (typeof aVal === 'number' && typeof bVal === 'number') return (aVal - bVal) * dir
      return String(aVal).localeCompare(String(bVal)) * dir
    })
  }
  return result
})

const totalPages = computed(() => {
  if (props.serverSide) return Math.ceil((props.totalItems ?? props.data.length) / props.pageSize)
  return Math.ceil(processedData.value.length / props.pageSize)
})
const showPagination = computed(() => !props.serverSide && processedData.value.length > props.pageSize)
const pageStart = computed(() => (internalPage.value - 1) * props.pageSize)
const pageEnd = computed(() => Math.min(pageStart.value + props.pageSize, processedData.value.length))
const displayedRows = computed(() => {
  if (props.serverSide) return props.data
  return processedData.value.slice(pageStart.value, pageEnd.value)
})
const visiblePages = computed(() => {
  const pages: number[] = []
  const total = totalPages.value
  const current = internalPage.value
  for (let i = Math.max(1, current - 2); i <= Math.min(total, current + 2); i++) pages.push(i)
  return pages
})

const allSelected = computed(() => displayedRows.value.length > 0 && displayedRows.value.every(r => isSelected(r)))
const someSelected = computed(() => selected.value.length > 0)

function toggleSort(key: string) {
  if (sortKey.value === key) { sortDir.value = sortDir.value === 'asc' ? 'desc' : 'asc' }
  else { sortKey.value = key; sortDir.value = 'asc' }
  emit('sort', { key: sortKey.value, direction: sortDir.value })
}

function onFilterInput(key: string, value: string) {
  filters.value[key] = value || undefined
  internalPage.value = 1
  emit('filter', { ...filters.value })
}
function onFilterChange(key: string, value: string) {
  filters.value[key] = value || undefined
  internalPage.value = 1
  emit('filter', { ...filters.value })
}
function onDateFilter(key: string, bound: 'from' | 'to', value: string) {
  if (!filters.value[key]) filters.value[key] = {}
  filters.value[key][bound] = value || undefined
  internalPage.value = 1
  emit('filter', { ...filters.value })
}
function goToPage(page: number) {
  if (page < 1 || page > totalPages.value) return
  internalPage.value = page
  emit('page-change', page)
}
function getRowKey(row: any, index: number): string | number {
  if (!props.rowKey) return index
  if (typeof props.rowKey === 'function') return props.rowKey(row)
  return row[props.rowKey] ?? index
}
function isSelected(row: any): boolean {
  const key = getRowKey(row, -1)
  return selected.value.some(r => getRowKey(r, -1) === key)
}
function toggleSelect(row: any) {
  if (isSelected(row)) {
    const key = getRowKey(row, -1)
    selected.value = selected.value.filter(r => getRowKey(r, -1) !== key)
  } else { selected.value = [...selected.value, row] }
  emit('selection-change', selected.value)
}
function toggleSelectAll() {
  selected.value = allSelected.value ? [] : [...displayedRows.value]
  emit('selection-change', selected.value)
}
function onRowKeydown(event: KeyboardEvent, rowIdx: number) {
  const rows = (event.currentTarget as HTMLElement)?.parentElement?.querySelectorAll<HTMLElement>('[role="row"][tabindex]')
  if (!rows) return
  if (event.key === 'ArrowDown') { event.preventDefault(); rows[rowIdx + 1]?.focus() }
  else if (event.key === 'ArrowUp') { event.preventDefault(); rows[rowIdx - 1]?.focus() }
}

watch(() => props.currentPage, (val) => { internalPage.value = val })
</script>

<style scoped>
.k-table-wrapper { display: flex; flex-direction: column; border: 1px solid var(--color-border); border-radius: var(--radius-lg); background: var(--color-surface); overflow: hidden; }

/* Filters */
.k-table__filters { display: flex; flex-wrap: wrap; gap: var(--space-3); padding: var(--space-3) var(--space-4); border-bottom: 1px solid var(--color-border); background: var(--color-surface-2); }
.k-table__filter-cell { display: flex; flex-direction: column; gap: var(--space-1); min-width: 140px; }
.k-table__filter-label { font-size: var(--text-xs); color: var(--color-muted); font-weight: var(--font-medium); }
.k-table__filter-input { height: 30px; padding: 0 var(--space-2); background: var(--color-surface); border: 1px solid var(--color-border); border-radius: var(--radius-sm); color: var(--color-text); font-size: var(--text-sm); outline: none; transition: border-color var(--duration-normal); }
.k-table__filter-input:focus { border-color: var(--color-primary); }
.k-table__filter-dates { display: flex; align-items: center; gap: var(--space-1); }
.k-table__filter-input--date { width: 120px; }
.k-table__filter-sep { color: var(--color-muted); font-size: var(--text-sm); }

/* Table core */
.k-table__scroll-container { overflow-x: auto; }
.k-table { width: 100%; border-collapse: collapse; font-size: var(--text-sm); }

/* Header */
.k-table__head--sticky { position: sticky; top: 0; z-index: var(--z-sticky); }
.k-table__th { padding: var(--space-3) var(--space-4); text-align: left; font-weight: var(--font-semibold); font-size: var(--text-xs); color: var(--color-muted); text-transform: uppercase; letter-spacing: var(--tracking-wider); background: var(--color-surface); border-bottom: 1px solid var(--color-border); white-space: nowrap; user-select: none; }
.k-table__th--center { text-align: center; }
.k-table__th--right { text-align: right; }
.k-table__th--check { width: 40px; padding: 0 var(--space-2); text-align: center; line-height: 0; }
.k-table__th--sortable { cursor: pointer; }
.k-table__th--sortable:hover { color: var(--color-text); }
.k-table__th--sortable:focus-visible { outline: 2px solid var(--color-accent); outline-offset: -2px; }
.k-table__th-content { display: inline-flex; align-items: center; gap: var(--space-1); }
.k-table__sort-icon { font-size: 10px; opacity: 0.6; }
.k-table__th--sorted .k-table__sort-icon { opacity: 1; color: var(--color-accent); }

/* Body */
.k-table__row { transition: background var(--duration-fast); }
.k-table__row:hover { background: var(--color-surface-2); }
.k-table__row:focus-visible { outline: 2px solid var(--color-accent); outline-offset: -2px; }
.k-table__row--selected { background: rgba(37, 99, 235, 0.08); }
.k-table__row--selected:hover { background: rgba(37, 99, 235, 0.12); }
.k-table__td { padding: var(--space-3) var(--space-4); color: var(--color-text); border-bottom: 1px solid var(--color-border); vertical-align: middle; }
.k-table__td--center { text-align: center; }
.k-table__td--right { text-align: right; }
.k-table__td--check { width: 40px; padding: 0 var(--space-2); vertical-align: middle; text-align: center; line-height: 0; }

/* Custom checkbox */
.k-check {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 18px;
  height: 18px;
  border: 1.5px solid var(--color-border);
  border-radius: 4px;
  background: var(--color-surface);
  cursor: pointer;
  position: relative;
  transition: all 0.2s cubic-bezier(0.4, 0, 0.2, 1);
  flex-shrink: 0;
  color: transparent;
}
.k-check:hover {
  border-color: var(--color-primary);
  box-shadow: 0 0 0 3px rgba(37, 99, 235, 0.1);
}
.k-check:focus-visible {
  outline: 2px solid var(--color-primary);
  outline-offset: 2px;
}
.k-check--checked {
  background: var(--color-primary);
  border-color: var(--color-primary);
  color: #fff;
  animation: k-check-pop 0.25s cubic-bezier(0.34, 1.56, 0.64, 1);
}
.k-check--indeterminate {
  background: var(--color-primary);
  border-color: var(--color-primary);
  color: #fff;
  animation: k-check-pop 0.25s cubic-bezier(0.34, 1.56, 0.64, 1);
}
.k-check__icon {
  width: 12px;
  height: 12px;
  animation: k-check-draw 0.2s 0.05s cubic-bezier(0.4, 0, 0.2, 1) both;
}
@keyframes k-check-pop {
  0% { transform: scale(1); }
  40% { transform: scale(0.8); }
  100% { transform: scale(1); }
}
@keyframes k-check-draw {
  0% { opacity: 0; transform: scale(0.5); }
  100% { opacity: 1; transform: scale(1); }
}

/* Empty */
.k-table__empty { padding: var(--space-12) var(--space-4); text-align: center; color: var(--color-muted); font-size: var(--text-base); }
.k-table__empty-icon { display: block; font-size: var(--text-3xl); margin-bottom: var(--space-2); opacity: 0.4; }

/* Skeleton */
.k-skeleton { display: block; height: 14px; width: 70%; background: linear-gradient(90deg, var(--color-surface-2) 25%, var(--color-border) 50%, var(--color-surface-2) 75%); background-size: 200% 100%; border-radius: var(--radius-sm); animation: k-shimmer 1.5s infinite; }
.k-skeleton--sm { width: 16px; height: 16px; border-radius: 3px; }
@keyframes k-shimmer { 0% { background-position: 200% 0; } 100% { background-position: -200% 0; } }

/* Footer / Pagination */
.k-table__footer { display: flex; align-items: center; justify-content: space-between; padding: var(--space-3) var(--space-4); border-top: 1px solid var(--color-border); background: var(--color-surface-2); gap: var(--space-4); flex-wrap: wrap; }
.k-table__page-info { font-size: var(--text-xs); color: var(--color-muted); }
.k-table__page-controls { display: flex; align-items: center; gap: var(--space-1); }
.k-table__page-btn { display: inline-flex; align-items: center; justify-content: center; min-width: 28px; height: 28px; padding: 0 var(--space-2); border: 1px solid var(--color-border); border-radius: var(--radius-sm); background: var(--color-surface); color: var(--color-text); font-size: var(--text-sm); cursor: pointer; transition: all var(--duration-fast); }
.k-table__page-btn:hover:not(:disabled) { border-color: var(--color-primary); color: var(--color-primary); }
.k-table__page-btn:disabled { opacity: 0.4; cursor: not-allowed; }
.k-table__page-btn--active { background: var(--color-primary); border-color: var(--color-primary); color: #fff; }
.k-table__export { display: flex; gap: var(--space-2); }
.k-table__export-btn { padding: var(--space-1) var(--space-2); border: 1px solid var(--color-border); border-radius: var(--radius-sm); background: var(--color-surface); color: var(--color-muted); font-size: var(--text-xs); font-weight: var(--font-medium); cursor: pointer; transition: all var(--duration-fast); }
.k-table__export-btn:hover { border-color: var(--color-primary); color: var(--color-primary); }
</style>
