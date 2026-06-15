/**
 * Shared component prop and emit interfaces for KorisPanel
 * Used by the shared component library (@koris/ui)
 */

import type { VNode } from 'vue'

// --- KButton ---

export interface KButtonProps {
  variant?: 'primary' | 'ghost' | 'danger' | 'text'
  size?: 'sm' | 'md' | 'lg'
  loading?: boolean
  disabled?: boolean
  icon?: string
  iconPosition?: 'left' | 'right'
  fullWidth?: boolean
}

// --- KDataTable ---

export interface KDataTableColumn<T = any> {
  key: string
  label: string
  sortable?: boolean
  filterable?: boolean
  width?: string
  align?: 'left' | 'center' | 'right'
  render?: (value: any, row: T) => VNode
  filterType?: 'text' | 'select' | 'date-range'
  filterOptions?: { label: string; value: string }[]
}

export interface KDataTableProps<T = any> {
  columns: KDataTableColumn<T>[]
  data: T[]
  loading?: boolean
  selectable?: boolean
  stickyHeader?: boolean
  emptyText?: string
  emptyIcon?: string
  rowKey?: string | ((row: T) => string | number)
  virtualScroll?: boolean
  rowHeight?: number
  pageSize?: number
  currentPage?: number
  totalItems?: number
  serverSide?: boolean
}

// --- KDrawer ---

export interface KDrawerProps {
  open: boolean
  side?: 'right' | 'left'
  width?: string
  title?: string
  closable?: boolean
  overlay?: boolean
}

// --- KConfirmDialog ---

export interface ConfirmOptions {
  title: string
  message: string
  confirmText?: string
  cancelText?: string
  variant?: 'danger' | 'warning' | 'info'
  icon?: string
}

// --- KChart ---

export interface ChartDataPoint {
  label: string
  value: number
  color?: string
}

export interface ChartOptions {
  showTooltip?: boolean
  showGrid?: boolean
  showLegend?: boolean
  yAxisFormat?: (value: number) => string
  xAxisFormat?: (label: string) => string
  gradientFill?: boolean
  smoothCurve?: boolean
}

export interface KChartProps {
  type: 'line' | 'area' | 'bar' | 'donut'
  data: ChartDataPoint[]
  options?: ChartOptions
  animate?: boolean
  interactive?: boolean
  height?: number
}

// --- KFormField ---

export interface ValidationRule {
  type: 'required' | 'minLength' | 'maxLength' | 'pattern' | 'custom'
  value?: any
  message: string
  validator?: (value: any) => boolean
}

export interface KFormFieldProps {
  label: string
  name?: string
  rules?: ValidationRule[]
  error?: string
  hint?: string
  required?: boolean
}

// --- Navigation ---

export interface NavItem {
  key: string
  label: string
  icon: string
  badge?: number | string
  children?: NavItem[]
}

export interface Breadcrumb {
  label: string
  to?: string
}
