// Motion helpers: Fluent durations, disabled under prefers-reduced-motion
// (Svelte transitions use the Web Animations API, which CSS cannot switch off).
import { prefersReducedMotion } from 'svelte/motion'
import { cubicOut } from 'svelte/easing'

export const DUR_FAST = 167
export const DUR = 250
export const DUR_SLOW = 333

/** Duration in ms, 0 when the user asked for reduced motion. */
export function dur(ms: number): number {
  return prefersReducedMotion.current ? 0 : ms
}

/** Fluent "decelerate" curve cubic-bezier(0,0,0,1) approximation. */
export function decel(t: number): number {
  return 1 - Math.pow(1 - t, 4)
}

export { cubicOut }
