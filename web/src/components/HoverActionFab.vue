<template>
  <div class="hover-fab" @mouseenter="showSecondary = true" @mouseleave="showSecondary = false" @touchstart="startLongPress" @touchend="stopLongPress" @touchcancel="stopLongPress">
    <transition name="fab-fade">
      <q-btn v-if="showSecondary" round color="secondary" :icon="secondaryIcon" class="hover-fab-secondary" @click="handleSecondaryClick">
        <q-tooltip>{{ secondaryLabel }}</q-tooltip>
      </q-btn>
    </transition>

    <q-btn round color="primary" :icon="primaryIcon" @click="handlePrimaryClick">
      <q-tooltip>{{ primaryLabel }}</q-tooltip>
    </q-btn>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'

const props = defineProps<{
  primaryIcon: string
  primaryLabel: string
  secondaryIcon: string
  secondaryLabel: string
}>()

const emit = defineEmits<{
  primary: []
  secondary: []
}>()

const showSecondary = ref(false)
let longPressTimer: ReturnType<typeof setTimeout> | null = null

function handlePrimaryClick() {
  emit('primary')
}

function handleSecondaryClick() {
  showSecondary.value = false
  emit('secondary')
}

// startLongPress 让触屏设备也能通过长按打开辅助动作，避免悬浮交互只对桌面可用。
function startLongPress() {
  stopLongPress()
  longPressTimer = setTimeout(() => {
    showSecondary.value = true
  }, 400)
}

function stopLongPress() {
  if (longPressTimer) {
    clearTimeout(longPressTimer)
    longPressTimer = null
  }
}
</script>

<style scoped>
.hover-fab {
  position: relative;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 12px;
}

.hover-fab-secondary {
  box-shadow: 0 8px 24px rgba(15, 23, 42, 0.18);
}

.fab-fade-enter-active,
.fab-fade-leave-active {
  transition: opacity 0.18s ease, transform 0.18s ease;
}

.fab-fade-enter-from,
.fab-fade-leave-to {
  opacity: 0;
  transform: translateY(8px);
}
</style>
