<script setup lang="ts">
import { ref, computed, watch, nextTick } from 'vue'
import { useRouter } from 'vue-router'
import { useCommandPalette, type CommandAction } from '@/composables/useCommandPalette'

const router = useRouter()

// Define available actions
const actions = computed<CommandAction[]>(() => [
  { id: 'nav-dashboard', label: 'Go to Dashboard', description: 'Overview and stats', icon: '📊', section: 'Navigation', keywords: ['home', 'overview'], action: () => router.push({ name: 'overview' }) },
  { id: 'nav-customers', label: 'Go to Customers', description: 'Manage user accounts', icon: '👥', section: 'Navigation', keywords: ['users', 'accounts'], action: () => router.push({ name: 'users' }) },
  { id: 'nav-plans', label: 'Go to Plans', description: 'Subscription plans', icon: '💳', section: 'Navigation', keywords: ['pricing', 'subscription'], action: () => router.push({ name: 'plans' }) },
  { id: 'nav-payments', label: 'Go to Payments', description: 'Transaction history', icon: '💰', section: 'Navigation', keywords: ['transactions', 'billing'], action: () => router.push({ name: 'payments' }) },
  { id: 'nav-tickets', label: 'Go to Tickets', description: 'Support tickets', icon: '🎫', section: 'Navigation', keywords: ['support', 'help'], action: () => router.push({ name: 'tickets' }) },
  { id: 'nav-nodes', label: 'Go to Nodes', description: 'Server management', icon: '🖥️', section: 'Navigation', keywords: ['servers', 'vpn', 'services'], action: () => router.push({ name: 'nodes' }) },
  { id: 'nav-settings', label: 'Go to Settings', description: 'Panel configuration', icon: '⚙️', section: 'Navigation', keywords: ['config', 'system'], action: () => router.push({ name: 'settings' }) },
  { id: 'nav-resellers', label: 'Go to Resellers', description: 'Reseller accounts', icon: '🤝', section: 'Navigation', keywords: ['partners'], action: () => router.push({ name: 'users' }) },
])

const { isOpen, query, filteredActions, selectedIndex, close, execute } = useCommandPalette({ actions })

const inputRef = ref<HTMLInputElement | null>(null)
const listRef = ref<HTMLDivElement | null>(null)

// Focus input when opened
watch(isOpen, async (open) => {
  if (open) {
    await nextTick()
    inputRef.value?.focus()
  }
})

// Scroll selected item into view
watch(selectedIndex, async () => {
  await nextTick()
  const active = listRef.value?.querySelector('[data-active="true"]')
  active?.scrollIntoView({ block: 'nearest' })
})

function handleOverlayClick(e: MouseEvent) {
  if (e.target === e.currentTarget) close()
}
</script>

<template>
  <Teleport to="body">
    <Transition name="cmd-palette">
      <div v-if="isOpen" class="cmd-overlay" @click="handleOverlayClick" role="dialog" aria-modal="true" aria-label="Command palette">
        <div class="cmd-palette">
          <div class="cmd-search">
            <svg class="cmd-search-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="11" cy="11" r="7"/><path d="M21 21l-4-4"/></svg>
            <input ref="inputRef" v-model="query" type="text" placeholder="Type a command..." class="cmd-input" aria-label="Search commands" />
            <kbd class="cmd-kbd">ESC</kbd>
          </div>
          <div ref="listRef" class="cmd-list" role="listbox">
            <div v-if="filteredActions.length === 0" class="cmd-empty">No results found</div>
            <button
              v-for="(action, index) in filteredActions"
              :key="action.id"
              role="option"
              :aria-selected="index === selectedIndex"
              :data-active="index === selectedIndex"
              :class="['cmd-item', { 'cmd-item--active': index === selectedIndex }]"
              @click="execute(action)"
              @mouseenter="selectedIndex = index"
            >
              <span v-if="action.icon" class="cmd-item-icon">{{ action.icon }}</span>
              <div class="cmd-item-text">
                <span class="cmd-item-label">{{ action.label }}</span>
                <span v-if="action.description" class="cmd-item-desc">{{ action.description }}</span>
              </div>
              <span v-if="action.shortcut" class="cmd-item-shortcut">{{ action.shortcut }}</span>
            </button>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
.cmd-overlay { position:fixed;inset:0;z-index:var(--z-modal);background:rgba(0,0,0,0.6);backdrop-filter:blur(4px);display:flex;align-items:flex-start;justify-content:center;padding-top:120px; }
.cmd-palette { width:100%;max-width:560px;background:var(--color-surface);border:1px solid var(--color-border);border-radius:var(--radius-xl);box-shadow:var(--shadow-xl);overflow:hidden; }
.cmd-search { display:flex;align-items:center;gap:var(--space-3);padding:var(--space-4);border-bottom:1px solid var(--color-border); }
.cmd-search-icon { width:18px;height:18px;color:var(--color-muted);flex-shrink:0; }
.cmd-input { flex:1;background:none;border:none;outline:none;color:var(--color-text);font-size:var(--text-md);font-family:var(--font-family); }
.cmd-input::placeholder { color:var(--color-muted); }
.cmd-kbd { font-size:10px;padding:2px 6px;border:1px solid var(--color-border);border-radius:var(--radius-sm);color:var(--color-muted);font-family:var(--font-mono); }
.cmd-list { max-height:360px;overflow-y:auto;padding:var(--space-2); }
.cmd-empty { padding:var(--space-8);text-align:center;color:var(--color-muted);font-size:var(--text-sm); }
.cmd-item { display:flex;align-items:center;gap:var(--space-3);width:100%;padding:var(--space-3) var(--space-3);border-radius:var(--radius-md);border:none;background:none;color:var(--color-text);text-align:left;cursor:pointer;transition:background var(--duration-fast); }
.cmd-item:hover,.cmd-item--active { background:var(--color-surface-2); }
.cmd-item-icon { font-size:18px;width:28px;text-align:center;flex-shrink:0; }
.cmd-item-text { flex:1;min-width:0; }
.cmd-item-label { display:block;font-size:var(--text-base);font-weight:var(--font-medium); }
.cmd-item-desc { display:block;font-size:var(--text-xs);color:var(--color-muted);margin-top:2px; }
.cmd-item-shortcut { font-size:var(--text-xs);color:var(--color-muted);font-family:var(--font-mono);padding:2px 6px;border:1px solid var(--color-border);border-radius:var(--radius-sm); }

.cmd-palette-enter-active,.cmd-palette-leave-active { transition:opacity var(--duration-slow),transform var(--duration-slow); }
.cmd-palette-enter-from,.cmd-palette-leave-to { opacity:0; }
.cmd-palette-enter-from .cmd-palette,.cmd-palette-leave-to .cmd-palette { transform:scale(0.96) translateY(-10px); }
</style>
