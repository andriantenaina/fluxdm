import { setFormatLang } from './format'
import * as core from './i18n/core'
import * as main from './i18n/main'
import * as windows from './i18n/windows'
import * as settings from './i18n/settings'
import * as remote from './i18n/remote'

// One dictionary per area (sibling files), merged here. Every area exports
// `fr` and `en: typeof fr`, so a missing English string is a type error.
const fr = { ...core.fr, ...main.fr, ...windows.fr, ...settings.fr, ...remote.fr }
type Dict = typeof fr
const en: Dict = { ...core.en, ...main.en, ...windows.en, ...settings.en, ...remote.en }

const dicts: Record<string, Dict> = { fr, en }

export type I18nKey = keyof Dict
type StringKey = { [K in I18nKey]: Dict[K] extends string ? K : never }[I18nKey]

export const i18n = $state({ lang: 'fr' })

/** Browser/system language, used before the settings are loaded (remote app). */
export function guessLang(): string {
  const n = (typeof navigator !== 'undefined' && navigator.language) || 'fr'
  return n.toLowerCase().startsWith('fr') ? 'fr' : 'en'
}

export function setLang(l: string) {
  i18n.lang = dicts[l] ? l : 'fr'
  setFormatLang(i18n.lang)
  document.documentElement.lang = i18n.lang
  try { localStorage.setItem('fdm.lang', i18n.lang) } catch { /* private mode */ }
}

export function t<K extends I18nKey>(k: K): Dict[K] {
  return (dicts[i18n.lang] ?? fr)[k]
}

/** Interpolates {name} placeholders: tf('removed', { n: 3 }). */
export function tf(k: StringKey, vars: Record<string, string | number>): string {
  return (t(k) as string).replace(/\{(\w+)\}/g, (m, v) => (v in vars ? String(vars[v]) : m))
}

/** Plural form: looks up `<base>_one` / `<base>_other` and interpolates {n}. */
export function tn(base: string, n: number, vars: Record<string, string | number> = {}): string {
  const d = (dicts[i18n.lang] ?? fr) as Record<string, unknown>
  const one = i18n.lang === 'fr' ? n < 2 : n === 1
  const s = (d[`${base}_${one ? 'one' : 'other'}`] ?? d[`${base}_other`] ?? base) as string
  return s.replace(/\{(\w+)\}/g, (m, v) => (v === 'n' ? String(n) : v in vars ? String(vars[v]) : m))
}

/** Display name of a queue ("main" is the unified main queue name). */
export function queueLabel(name: string): string {
  return name === 'main' || !name ? (t('mainQueue') as string) : name
}
