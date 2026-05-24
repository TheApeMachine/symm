import {
	SciChart3DSurface,
	SciChartPolarSurface,
	SciChartSurface,
} from "scichart";

let wasmReady: Promise<void> | null = null;

/** Load SciChart 2D/3D/polar wasm from CDN (Vite has no bundled wasm copy). */
export function ensureSciChartWasm(): Promise<void> {
	if (!wasmReady) {
		SciChartSurface.loadWasmFromCDN();
		SciChart3DSurface.loadWasmFromCDN();
		SciChartPolarSurface.loadWasmFromCDN();
		wasmReady = Promise.resolve();
	}
	return wasmReady;
}
