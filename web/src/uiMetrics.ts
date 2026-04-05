import { computed } from 'vue'
import { useQuasar } from 'quasar'

// useResponsiveStickyOffset 根据当前屏幕宽度收敛悬浮操作按钮的边距，避免小屏幕贴边过远或过近。
export function useResponsiveStickyOffset() {
  const $q = useQuasar()
  return computed<[number, number]>(() => ($q.screen.lt.md ? [16, 16] : [24, 24]))
}

// useResponsiveDrawerWidth 让导航抽屉在桌面端保持信息密度，在较小设备上避免占用过多内容区域。
export function useResponsiveDrawerWidth() {
  const $q = useQuasar()
  return computed(() => {
    if ($q.screen.lt.sm) {
      return 220
    }
    if ($q.screen.lt.lg) {
      return 240
    }
    return 260
  })
}
