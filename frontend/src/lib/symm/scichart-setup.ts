import { SciChart3DSurface, SciChartSurface } from 'scichart'

let wasmReady: Promise<void> | null = null

/** Load SciChart 2D/3D wasm from CDN (Vite has no bundled wasm copy). */
export function ensureSciChartWasm(): Promise<void> {
  if (!wasmReady) {
    SciChartSurface.loadWasmFromCDN()
    SciChart3DSurface.loadWasmFromCDN()
    wasmReady = Promise.resolve()
  }
  return wasmReady
}
