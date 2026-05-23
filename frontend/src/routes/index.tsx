import { createFileRoute } from '@tanstack/react-router'

import { TradeChartGrid } from '#/components/symm/TradeChart'
import {
  useSymmConnected,
  useSymmFeed,
  useSymmPositionSymbols,
} from '#/lib/symm/use-symm-ui'
import { DashboardHeader } from '#/components/header'
import { DashboardSidebar } from '#/components/sidebar'
import { ChartSection } from '#/components/chart'

const TradingDashboard = () => {
  useSymmFeed()
  const connected = useSymmConnected()
  const positionSymbols = useSymmPositionSymbols()

  return (
    <div className="flex w-full h-full flex-col overflow-hidden bg-(--dash-bg) text-(--dash-text)">
      <DashboardHeader />
      <div className="flex min-h-0 flex-1">
        <ChartSection connected={connected} positionSymbols={positionSymbols} />
        <DashboardSidebar />
      </div>
    </div>
  )
}

export const Route = createFileRoute('/')({
  component: TradingDashboard,
})
