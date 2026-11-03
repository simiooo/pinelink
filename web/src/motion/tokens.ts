export const motion = {
  press: 80, quick: 120, control: 160, surface: 220, page: 280, settle: 360,
  sheen: 1800, spring: { mass: 1, stiffness: 420, damping: 34 },
  enter: 'cubic-bezier(.16,1,.3,1)', state: 'cubic-bezier(.2,0,0,1)',
} as const

export function confirmedPercent(value: number) {
  const bounded = Number.isFinite(value) ? Math.min(1, Math.max(0, value)) : 0
  return bounded === 1 ? 100 : Math.min(99, Math.floor(bounded * 100 + 1e-12))
}
