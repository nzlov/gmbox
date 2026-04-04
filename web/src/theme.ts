import { reactive } from 'vue'
import { Dark, setCssVar } from 'quasar'
import type { ThemePreference } from './api'

const STORAGE_KEY = 'gmbox-theme-preference'

export interface ThemePreset {
  name: string
  label: string
  theme_mode: 'light' | 'dark'
  primary_color: string
  secondary_color: string
  accent_color: string
}

export const themePresets: ThemePreset[] = [
  { name: 'classic_blue', label: '经典蓝', theme_mode: 'light', primary_color: '#2563eb', secondary_color: '#7c3aed', accent_color: '#06b6d4' },
  { name: 'forest_green', label: '松林绿', theme_mode: 'light', primary_color: '#166534', secondary_color: '#0f766e', accent_color: '#65a30d' },
  { name: 'sunset_orange', label: '晚霞橙', theme_mode: 'light', primary_color: '#ea580c', secondary_color: '#db2777', accent_color: '#d97706' },
  { name: 'midnight_ink', label: '午夜墨', theme_mode: 'dark', primary_color: '#60a5fa', secondary_color: '#a78bfa', accent_color: '#22d3ee' },
]

export const themeState = reactive<ThemePreference>(loadStoredTheme())

// applyThemePreference 统一把主题偏好映射到 Quasar 品牌色和深色模式，预览时允许跳过本地持久化。
export function applyThemePreference(preference: ThemePreference, options: { persist?: boolean } = {}) {
	const { persist = true } = options
  themeState.theme_name = preference.theme_name
  themeState.theme_mode = preference.theme_mode
  themeState.primary_color = preference.primary_color
  themeState.secondary_color = preference.secondary_color
  themeState.accent_color = preference.accent_color
  Dark.set(preference.theme_mode === 'dark')
  setCssVar('primary', preference.primary_color)
  setCssVar('secondary', preference.secondary_color)
  setCssVar('accent', preference.accent_color)
  document.documentElement.style.setProperty('--gmbox-primary', preference.primary_color)
  document.documentElement.style.setProperty('--gmbox-secondary', preference.secondary_color)
  document.documentElement.style.setProperty('--gmbox-accent', preference.accent_color)
  if (persist) {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(themeState))
  }
}

// loadStoredTheme 优先读取本地缓存，避免登录后接口返回前出现主题闪动。
function loadStoredTheme(): ThemePreference {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (!raw) {
      return defaultThemePreference()
    }
    const parsed = JSON.parse(raw) as Partial<ThemePreference>
    return {
      theme_name: typeof parsed.theme_name === 'string' ? parsed.theme_name : 'classic_blue',
      theme_mode: parsed.theme_mode === 'dark' ? 'dark' : 'light',
      primary_color: typeof parsed.primary_color === 'string' ? parsed.primary_color : '#2563eb',
      secondary_color: typeof parsed.secondary_color === 'string' ? parsed.secondary_color : '#7c3aed',
      accent_color: typeof parsed.accent_color === 'string' ? parsed.accent_color : '#06b6d4',
    }
  } catch {
    return defaultThemePreference()
  }
}

export function defaultThemePreference(): ThemePreference {
  return {
    theme_name: 'classic_blue',
    theme_mode: 'light',
    primary_color: '#2563eb',
    secondary_color: '#7c3aed',
    accent_color: '#06b6d4',
  }
}
