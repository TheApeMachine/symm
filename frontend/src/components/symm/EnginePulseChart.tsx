import { memo, useCallback } from 'react'
import { SciChartReact } from 'scichart-react'

import {
  initEnginePulseChart,
  type EnginePulseInitResult,
} from '#/lib/symm/engine-pulse-controller'
import { registerFieldStream, unregisterFieldStream } from '#/lib/symm/feed'
import { useSymmConnected, useSymmEnginePulse } from '#/lib/symm/use-symm-ui'
import '#/lib/symm/scichart-setup'

type EnginePulseChartProps = {
  className?: string
}

/** Live aggregate fluid scalars — updates on every field_snapshot, no batching. */
export const EnginePulseChart = memo(function EnginePulseChart({
  className = '',
}: EnginePulseChartProps) {
  const initChart = useCallback(
    (rootElement: string | HTMLDivElement) => initEnginePulseChart(rootElement),
    [],
  )

  const onInit = useCallback((result: EnginePulseInitResult) => {
    registerFieldStream((snapshot) => result.appendField(snapshot))
  }, [])

  const onDelete = useCallback((result?: EnginePulseInitResult) => {
    result?.dispose()
    unregisterFieldStream()
  }, [])

  return (
    <div
      className={`flex min-h-[180px] flex-col overflow-hidden rounded border border-(--dash-border) bg-(--dash-panel) ${className}`}
    >
      <EnginePulseHeader />
      <SciChartReact
        initChart={initChart}
        onInit={onInit}
        onDelete={onDelete}
        className="min-h-0 w-full flex-1"
        innerContainerProps={{ className: 'h-full w-full' }}
      />
      <p className="shrink-0 border-t border-(--dash-border) px-2 py-0.5 text-[9px] text-(--dash-muted)">
        Re · Turb · Div — one point per fluid rescore
      </p>
    </div>
  )
})

const EnginePulseHeader = memo(function EnginePulseHeader() {
  const connected = useSymmConnected()
  const pulse = useSymmEnginePulse()

  return (
    <div className="flex shrink-0 flex-wrap items-center gap-x-3 gap-y-1 border-b border-(--dash-border) px-2 py-1.5">
      <span className="text-xs font-semibold tracking-wide">Engine pulse</span>
      <span className="text-[10px] text-(--dash-muted)">
        {connected ? `tick #${pulse?.seq ?? 0}` : 'Offline'}
      </span>
      {pulse ? (
        <div className="ml-auto flex flex-wrap gap-3 text-[10px] tabular-nums text-(--dash-muted)">
          <PulseMetric label="quotes" value={pulse.ticker_ready} total={pulse.symbols_total} />
          <PulseMetric label="fluid" value={pulse.fluid_sampled} total={pulse.fluid_warming} warm />
          <span>
            sig{' '}
            <span className="font-medium text-(--dash-text)">{pulse.measurements}</span>
          </span>
          <span>
            cand{' '}
            <span className="font-medium text-(--dash-text)">{pulse.candidates}</span>
          </span>
        </div>
      ) : null}
    </div>
  )
})

function PulseMetric({
  label,
  value,
  total,
  warm,
}: {
  label: string
  value?: number
  total?: number
  warm?: boolean
}) {
  if (value === undefined && total === undefined) {
    return null
  }

  return (
    <span>
      {label}{' '}
      <span className="font-medium text-(--dash-text)">{value ?? 0}</span>
      {total !== undefined ? (
        <span>
          {warm ? '+' : '/'}
          {total}
        </span>
      ) : null}
    </span>
  )
})
