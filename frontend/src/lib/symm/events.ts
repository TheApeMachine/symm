/** Wire protocol v1 — mirrors observ JSON events pushed over ws://host/ws */

export type SymmEventName =
  | 'hello'
  | 'run_start'
  | 'engine_start'
  | 'run_stop'
  | 'status'
  | 'scoreboard'
  | 'decision_trace'
  | 'field_snapshot'
  | 'price_tick'
  | 'chart_seed'
  | 'trade_enter'
  | 'trade_exit'
  | 'stop_ratchet'
  | 'entry_skip'
  | 'feed_warn'
  | 'bootstrap_warn'

export type SymmEvent = {
  event: SymmEventName
  ts: string
  run_id?: string
  [key: string]: unknown
}

export type ScoreboardTarget = {
  symbol: string
  regime: string
  reason: string
  score: number
  effective_score: number
  trail_pct: number
}

export type ScoreboardEvent = SymmEvent & {
  event: 'scoreboard'
  line: number
  median: number
  mad: number
  targets: ScoreboardTarget[]
}

export type DecisionRow = {
  symbol: string
  regime: string
  reason: string
  score: number
  in_play: boolean
  allow: boolean
  why: string
  confidence: number
  effective_score: number
}

export type DecisionTraceEvent = SymmEvent & {
  event: 'decision_trace'
  line: number
  median: number
  mad: number
  scored: number
  in_play: number
  allowed: number
  decisions: DecisionRow[]
}

export type FluidSymbolRow = {
  symbol: string
  change_pct: number
  vol: number
  div: number
  vort: number
  turb: number
  visc: number
  re: number
}

export type FieldFluidState = {
  div: number
  vort: number
  turb: number
  visc: number
  re: number
}

export type FieldSnapshotEvent = SymmEvent & {
  event: 'field_snapshot'
  symbol_count: number
  field: FieldFluidState
  symbols: FluidSymbolRow[]
}

const WHY_LABELS: Record<string, string> = {
  below_line: 'Below cut line',
  field_warming: 'Field warming up',
  pump_unconfirmed: 'Pump not confirmed',
  actual_pump: 'Actual pump',
  stop_cooldown: 'Stop cooldown',
  stop_reentry_weak: 'Re-entry too weak',
  unstable: 'Unstable (needs 2 rescores)',
  necessary_cause: 'Necessary cause missing',
  intervention: 'Backdoor-adjusted intervention',
  counterfactual: 'Counterfactual uplift confirmed',
  ok: 'Passed gate',
}

export function whyLabel(code: string): string {
  return WHY_LABELS[code] ?? code.replaceAll('_', ' ')
}

export type PriceTickEvent = SymmEvent & {
  event: 'price_tick'
  symbol: string
  last: number
  bid: number
  ask: number
  change_pct_24h: number
  at: string
}

export type ChartSeedBar = {
  t: number
  o: number
  h: number
  l: number
  c: number
}

export type ChartSeedEvent = SymmEvent & {
  event: 'chart_seed'
  symbol: string
  bars: ChartSeedBar[]
}

export type TradeEnterEvent = SymmEvent & {
  event: 'trade_enter'
  symbol: string
  regime: string
  reason: string
  score: number
  trail_pct: number
  fill: number
  stop: number
  notional_eur: number
  last?: number
  runner?: boolean
}

export type TradeExitEvent = SymmEvent & {
  event: 'trade_exit'
  symbol: string
  regime: string
  reason: string
  pnl_eur: number
  hold_ms: number
  entry_price: number
  stop_price: number
  peak_price: number
}

export type StopRatchetEvent = SymmEvent & {
  event: 'stop_ratchet'
  symbol: string
  old_stop: number
  new_stop: number
  peak: number
  last: number
}

export type StatusPosition = {
  symbol: string
  regime: string
  entry_price: number
  stop_price: number
  peak_price: number
  last_price?: number
  trail_pct: number
  notional_eur: number
  opened_at?: string
}

export type StatusEvent = SymmEvent & {
  event: 'status'
  equity_eur: number
  cash_eur: number
  closed_pnl_eur: number
  trade_count: number
  win_rate: number
  open_count: number
  positions?: StatusPosition[]
}

export type WatchCommand =
  | { op: 'subscribe'; symbols: string[] }
  | { op: 'unsubscribe'; symbols: string[] }
  | { op: 'watch'; symbol: string }

export const defaultWsUrl =
  typeof window !== 'undefined'
    ? `${window.location.protocol === 'https:' ? 'wss' : 'ws'}://${window.location.hostname}:8765/ws`
    : 'ws://127.0.0.1:8765/ws'

export function eventTimeSec(ev: SymmEvent): number {
  const raw = ev.ts
  const ms = Date.parse(raw)
  return Number.isFinite(ms) ? Math.floor(ms / 1000) : Math.floor(Date.now() / 1000)
}

export function tickTimeSec(ev: PriceTickEvent): number {
  const atMs = Date.parse(ev.at)
  if (Number.isFinite(atMs)) return Math.floor(atMs / 1000)
  return eventTimeSec(ev)
}

export function pickChartSymbol(
  scoreboard: ScoreboardEvent | undefined,
  decisions?: DecisionTraceEvent,
  fallback?: string,
): string | undefined {
  if (scoreboard?.targets?.length) {
    return scoreboard.targets[0].symbol
  }
  const allowed = decisions?.decisions?.find((d) => d.allow)
  if (allowed) {
    return allowed.symbol
  }
  return fallback
}
