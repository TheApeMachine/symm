import {
  EDrawMeshAs,
  EAxisPlaneDrawLabelsMode,
  GradientColorPalette,
  MouseWheelZoomModifier3D,
  NumberRange,
  NumericAxis3D,
  OrbitModifier3D,
  ResetCamera3DModifier,
  SciChart3DSurface,
  SurfaceMeshRenderableSeries3D,
  UniformGridDataSeries3D,
  Vector3,
  zeroArray2D,
} from 'scichart'

import type { FieldSnapshotEvent } from '#/lib/symm/events'
import {
  FLUID_GRID_SIZE,
  gridFromSnapshot,
  type FluidGrid,
} from '#/lib/symm/fluid-grid'
import { ensureSciChartWasm } from '#/lib/symm/scichart-setup'

const PALETTE = {
  dark: '#0f172a',
  indigo: '#6366f1',
  sky: '#38bdf8',
  green: '#4ade80',
  orange: '#fb923c',
  pink: '#f472b6',
  stroke: '#38bdf8',
}

const UNLABELED_AXIS = {
  drawLabels: false,
  drawMajorTickLines: false,
  drawMinorTickLines: false,
} as const

const HIDDEN_PLANE_LABELS = {
  drawLabelsMode: EAxisPlaneDrawLabelsMode.Hidden,
  drawTitlesMode: EAxisPlaneDrawLabelsMode.Hidden,
} as const

function gridCenter(yMid = 0.5): Vector3 {
  const half = FLUID_GRID_SIZE / 2
  return new Vector3(half, yMid, half)
}

function setWorldHeight(surface: SciChart3DSurface, ySpan: number): void {
  const worldY = Math.max(ySpan * 3, 8)
  surface.worldDimensions = new Vector3(FLUID_GRID_SIZE, worldY, FLUID_GRID_SIZE)
}

function frameCameraInitial(surface: SciChart3DSurface, yMid: number, ySpan: number): void {
  setWorldHeight(surface, ySpan)

  const half = FLUID_GRID_SIZE / 2
  const orbit = FLUID_GRID_SIZE * 1.2
  const camera = surface.camera
  camera.target = gridCenter(yMid)
  camera.position = new Vector3(
    half - orbit * 0.82,
    yMid + orbit * 0.62,
    half + orbit * 0.82,
  )
}

function retargetCamera(surface: SciChart3DSurface, yMid: number): void {
  surface.camera.target = gridCenter(yMid)
}

export type FluidSurfaceInitResult = {
  sciChartSurface: SciChart3DSurface
  update: (snapshot: FieldSnapshotEvent) => void
  dispose: () => void
}

/** Realtime 3D terrain of market Reynolds (change% × vol bins). */
class FluidSurfaceController {
  private surface: SciChart3DSurface
  private dataSeries: UniformGridDataSeries3D
  private meshSeries: SurfaceMeshRenderableSeries3D
  private heightmap = zeroArray2D([FLUID_GRID_SIZE, FLUID_GRID_SIZE])
  private followView = true
  private readonly interactionAbort = new AbortController()
  private resizeObserver: ResizeObserver | null = null
  private latestFrame = { yMin: 0, yMax: 1, ySpan: 1 }

  private constructor(
    surface: SciChart3DSurface,
    dataSeries: UniformGridDataSeries3D,
    meshSeries: SurfaceMeshRenderableSeries3D,
  ) {
    this.surface = surface
    this.dataSeries = dataSeries
    this.meshSeries = meshSeries
  }

  static async create(rootElement: HTMLDivElement): Promise<FluidSurfaceController> {
    await ensureSciChartWasm()
    const { sciChart3DSurface, wasmContext } = await SciChart3DSurface.create(rootElement, {
      worldDimensions: new Vector3(FLUID_GRID_SIZE, 12, FLUID_GRID_SIZE),
      disableAspect: true,
      background: PALETTE.dark,
      xyAxisPlane: HIDDEN_PLANE_LABELS,
      zyAxisPlane: HIDDEN_PLANE_LABELS,
      zxAxisPlane: HIDDEN_PLANE_LABELS,
    })

    frameCameraInitial(sciChart3DSurface, 0.5, 1)

    const gridRange = new NumberRange(0, FLUID_GRID_SIZE - 1)
    sciChart3DSurface.xAxis = new NumericAxis3D(wasmContext, {
      ...UNLABELED_AXIS,
      visibleRange: gridRange,
    })
    sciChart3DSurface.yAxis = new NumericAxis3D(wasmContext, UNLABELED_AXIS)
    sciChart3DSurface.zAxis = new NumericAxis3D(wasmContext, {
      ...UNLABELED_AXIS,
      visibleRange: gridRange,
    })

    const heightmap = zeroArray2D([FLUID_GRID_SIZE, FLUID_GRID_SIZE])
    const dataSeries = new UniformGridDataSeries3D(wasmContext, {
      yValues: heightmap,
      xStep: 1,
      zStep: 1,
      dataSeriesName: 'Market fluid',
    })

    const colorMap = new GradientColorPalette(wasmContext, {
      gradientStops: [
        { offset: 1, color: PALETTE.pink },
        { offset: 0.75, color: PALETTE.orange },
        { offset: 0.5, color: PALETTE.green },
        { offset: 0.3, color: PALETTE.sky },
        { offset: 0.1, color: PALETTE.indigo },
        { offset: 0, color: PALETTE.dark },
      ],
    })

    const meshSeries = new SurfaceMeshRenderableSeries3D(wasmContext, {
      dataSeries,
      minimum: 0,
      maximum: 1,
      opacity: 0.92,
      cellHardnessFactor: 0.85,
      shininess: 0.1,
      lightingFactor: 0.35,
      highlight: 0.8,
      stroke: PALETTE.stroke,
      strokeThickness: 0.5,
      drawSkirt: false,
      drawMeshAs: EDrawMeshAs.SOLID_WITH_CONTOURS,
      meshColorPalette: colorMap,
    })

    sciChart3DSurface.renderableSeries.add(meshSeries)
    sciChart3DSurface.chartModifiers.add(
      new MouseWheelZoomModifier3D(),
      new OrbitModifier3D(),
      new ResetCamera3DModifier(),
    )

    const controller = new FluidSurfaceController(sciChart3DSurface, dataSeries, meshSeries)
    controller.bindUserNavigation(rootElement)
    controller.bindResize(rootElement)
    return controller
  }

  get sciChartSurface(): SciChart3DSurface {
    return this.surface
  }

  update(snapshot: FieldSnapshotEvent) {
    this.applyGrid(gridFromSnapshot(snapshot))
  }

  dispose() {
    this.interactionAbort.abort()
    this.resizeObserver?.disconnect()
    this.resizeObserver = null
  }

  private bindResize(rootElement: HTMLDivElement) {
    this.resizeObserver = new ResizeObserver(() => {
      if (rootElement.clientWidth <= 0 || rootElement.clientHeight <= 0) {
        return
      }
      this.surface.invalidateElement()
    })
    this.resizeObserver.observe(rootElement)
  }

  private bindUserNavigation(rootElement: HTMLDivElement) {
    const signal = this.interactionAbort.signal
    const leaveFollowMode = () => {
      this.followView = false
    }
    rootElement.addEventListener('wheel', leaveFollowMode, { passive: true, signal })
    rootElement.addEventListener('pointerdown', leaveFollowMode, { passive: true, signal })
    rootElement.addEventListener(
      'dblclick',
      () => {
        this.followView = true
        this.applyViewFrame()
        this.surface.invalidateElement()
      },
      { passive: true, signal },
    )
  }

  private applyGrid(grid: FluidGrid) {
    const pad = Math.max((grid.max - grid.min) * 0.08, 0.05)
    const yMin = grid.min - pad
    const yMax = grid.max + pad
    const ySpan = yMax - yMin

    for (let z = 0; z < FLUID_GRID_SIZE; z++) {
      for (let x = 0; x < FLUID_GRID_SIZE; x++) {
        this.heightmap[z][x] = grid.heights[z][x]
      }
    }

    this.dataSeries.setYValues(this.heightmap)
    this.meshSeries.minimum = yMin
    this.meshSeries.maximum = yMax
    this.latestFrame = { yMin, yMax, ySpan }

    if (this.followView) {
      this.applyViewFrame()
    }
    this.surface.invalidateElement()
  }

  private applyViewFrame() {
    const { yMin, yMax, ySpan } = this.latestFrame
    const yAxis = this.surface.yAxis
    if (yAxis) {
      yAxis.visibleRange = new NumberRange(yMin, yMax)
    }
    setWorldHeight(this.surface, ySpan)
    retargetCamera(this.surface, (yMin + yMax) / 2)
  }
}

export async function initFluidSurface(
  rootElement: string | HTMLDivElement,
): Promise<FluidSurfaceInitResult> {
  if (typeof rootElement === 'string') {
    throw new Error('initFluidSurface requires an HTMLDivElement root')
  }
  const controller = await FluidSurfaceController.create(rootElement)
  return {
    sciChartSurface: controller.sciChartSurface,
    update: (snapshot) => controller.update(snapshot),
    dispose: () => controller.dispose(),
  }
}
