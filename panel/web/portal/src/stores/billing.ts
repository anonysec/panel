import { ref, computed } from 'vue'
import { defineStore } from 'pinia'
import { useApi } from '@koris/composables/useApi'
import router from '@/router'
import type { Plan, Payment } from '@koris/types/entities'

/**
 * Payment method from the backend
 */
export interface PaymentMethod {
  id: number
  name: string
  type: string
  instructions: string
  is_active: boolean
  sort_order: number
  created_at: string
}

/**
 * API response types
 */
interface PaymentsResponse {
  ok: boolean
  payments: Payment[]
}

interface PaymentMethodsResponse {
  ok: boolean
  methods: PaymentMethod[]
}

interface PlansResponse {
  ok: boolean
  plans: Plan[]
}

interface SubmitPaymentResponse {
  ok: boolean
  id: number
}

interface RenewResponse {
  ok: boolean
  renewed: boolean
  payment_required: boolean
  required_amount?: number
  payment_id?: number
}

/**
 * Portal billing store (Pinia Composition API style)
 *
 * Manages payment history, payment methods, plans, and
 * payment submission. Uses useApi for all API interactions.
 *
 * Requirements: 3.2, 3.3, 3.4, 23.3
 */
export const useBillingStore = defineStore('portal-billing', () => {
  // ─── State ────────────────────────────────────────────────────────────────
  const payments = ref<Payment[]>([])
  const paymentMethods = ref<PaymentMethod[]>([])
  const plans = ref<Plan[]>([])
  const loading = ref(false)

  // ─── API composable ───────────────────────────────────────────────────────
  const { get, post, error } = useApi({
    onUnauthorized: () => {
      // Clear billing state and redirect to portal login
      payments.value = []
      paymentMethods.value = []
      plans.value = []
      router.push({ name: 'portal-login' })
    },
  })

  // ─── Computed ─────────────────────────────────────────────────────────────
  const pendingPayments = computed(() =>
    payments.value.filter((p) => p.status === 'pending')
  )

  const approvedPayments = computed(() =>
    payments.value.filter((p) => p.status === 'approved')
  )

  const activePlans = computed(() =>
    plans.value.filter((p) => p.is_active)
  )

  // ─── Actions ──────────────────────────────────────────────────────────────

  /**
   * Load all billing data: payments, methods, plans.
   * Called on BillingView mount.
   */
  async function loadBillingData(): Promise<void> {
    loading.value = true
    try {
      const [paymentsRes, methodsRes, plansRes] = await Promise.all([
        get<PaymentsResponse>('/api/portal/payments'),
        get<PaymentMethodsResponse>('/api/portal/payment-methods'),
        get<PlansResponse>('/api/portal/plans'),
      ])
      payments.value = paymentsRes.payments || []
      paymentMethods.value = methodsRes.methods || []
      plans.value = plansRes.plans || []
    } catch {
      // Preserve existing data on error (Requirement 3.4)
    } finally {
      loading.value = false
    }
  }

  /**
   * Submit a payment request (wallet top-up).
   * POST /api/portal/payments → { ok, id }
   */
  async function submitPayment(params: { amount: number; method: string; receipt?: string }): Promise<boolean> {
    loading.value = true
    try {
      await post<SubmitPaymentResponse>('/api/portal/payments', params)
      await loadBillingData()
      return true
    } catch {
      return false
    } finally {
      loading.value = false
    }
  }

  /**
   * Submit a plan renewal request.
   * POST /api/portal/renew → { ok, renewed, payment_required, ... }
   */
  async function renewPlan(planId: number): Promise<RenewResponse | null> {
    loading.value = true
    try {
      const res = await post<RenewResponse>('/api/portal/renew', { plan_id: planId })
      await loadBillingData()
      return res
    } catch {
      return null
    } finally {
      loading.value = false
    }
  }

  // ─── Expose ───────────────────────────────────────────────────────────────
  return {
    // State
    payments,
    paymentMethods,
    plans,
    loading,

    // API state
    error,

    // Computed
    pendingPayments,
    approvedPayments,
    activePlans,

    // Actions
    loadBillingData,
    submitPayment,
    renewPlan,
  }
})
