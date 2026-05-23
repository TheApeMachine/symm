import {
  DateTimeNumericAxis,
  EAutoRange,
  ENumericFormat,
  FastLineRenderableSeries,
  MouseWheelZoomModifier,
  NumberRange,
  NumericAxis,
  SciChartSurface,
  XyDataSeries,
  ZoomExtentsModifier,
  ZoomPanModifier,
} from 'scichart'

import type { FieldSnapshotEvent } from '#/lib/symm/events'
import { eventTimeSec } from '#/lib/symm/events'
import { ensureSciChartWasm } from '#/lib/symm/scichart-setup'

const FIFO_CAPACITY = 600

export type EnginePulseInitResult = {
  sciChartSurface: SciChartSurface
  appendField: (snapshot: FieldSnapshotEvent) => void
  dispose: () => void
}

/** Real-time mountain-style lines for aggregate fluid scalars (Re, turb, div). */
class EnginePulseController {
  private surface: SciChartSurface
  private reSeries: XyDataSeries
  private turbSeries: XyDataSeries
  private divSeries: XyDataSeries
  private pointIndex = 0

  private constructor(
    surface: SciChartSurface,
    reSeries: XyDataSeries,
    turbSeries: XyDataSeries,
    divSeries: XyDataSeries,
  ) {
    this.surface = surface
    this.reSeries = reSeries
    this.turbSeries = turbSeries
    this.divSeries = divSeries
  }

  static async create(rootElement: HTMLDivElement): Promise<EnginePulseController> {
    await ensureSciChartWasm()
    const { sciChartSurface, wasmContext } = await SciChartSurface.create(rootElement)
    sciChartSurface.title = ''

    const xAxis = new DateTimeNumericAxis(wasmContext, {
      autoRange: EAutoRange.Always,
      growBy: new NumberRange(0.02, 0.04),
    })
    const yAxis = new NumericAxis(wasmContext, {
      autoRange: EAutoRange.Always,
      growBy: new NumberRange(0.1, 0.1),
      labelFormat: ENumericFormat.Decimal,
      labelPrecision: 3,
    })

    const reSeries = new XyDataSeries(wasmContext, {
      dataSeriesName: 'Re',
      fifoCapacity: FIFO_CAPACITY,
    })
    const turbSeries = new XyDataSeries(wasmContext, {
      dataSeriesName: 'Turb',
      fifoCapacity: FIFO_CAPACITY,
    })
    const divSeries = new XyDataSeries(wasmContext, {
      dataSeriesName: 'Div',
      fifoCapacity: FIFO_CAPACITY,
    })

    sciChartSurface.xAxes.add(xAxis)
    sciChartSurface.yAxes.add(yAxis)
    sciChartSurface.renderableSeries.add(
      new FastLineRenderableSeries(wasmContext, {
        dataSeries: reSeries,
        stroke: '#22C55E',
        strokeThickness: 2,
      }),
      new FastLineRenderableSeries(wasmContext, {
        dataSeries: turbSeries,
        stroke: '#F59E0B',
        strokeThickness: 2,
      }),
      new FastLineRenderableSeries(wasmContext, {
        dataSeries: divSeries,
        stroke: '#3B82F6',
        strokeThickness: 2,
      }),
    )
    sciChartSurface.chartModifiers.add(
      new ZoomPanModifier(),
      new MouseWheelZoomModifier(),
      new ZoomExtentsModifier(),
    )

    return new EnginePulseController(sciChartSurface, reSeries, turbSeries, divSeries)
  }

  get sciChartSurface(): SciChartSurface {
    return this.surface
  }

  appendField(snapshot: FieldSnapshotEvent) {
    const field = snapshot.field
    if (!field) {
      return
    }

    const sec = eventTimeSec(snapshot)
    this.pointIndex += 1
    const x = sec + this.pointIndex * 1e-6

    this.reSeries.append(x, field.re)
    this.turbSeries.append(x, field.turb)
    this.divSeries.append(x, field.div)
    this.surface.invalidateElement()
  }

  dispose() {
    // SciChartReact owns surface teardown.
  }
}

export async function initEnginePulseChart(
  rootElement: string | HTMLDivElement,
): Promise<EnginePulseInitResult> {
  if (typeof rootElement === 'string') {
    throw new Error('initEnginePulseChart requires an HTMLDivElement root')
  }

  const controller = await EnginePulseController.create(rootElement)

  return {
    sciChartSurface: controller.sciChartSurface,
    appendField: (snapshot) => controller.appendField(snapshot),
    dispose: () => controller.dispose(),
  }
}
