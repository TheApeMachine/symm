import {
	SciChart3DSurface,
	SciChartPolarSurface,
	SciChartSurface,
} from "scichart";

let wasmReady: Promise<void> | null = null;

/** Base URL for self-hosted SciChart wasm (default `/scichart`). Override with VITE_SCICHART_WASM_BASE. */
export const sciChartWasmBase = (): string => {
	const custom = import.meta.env.VITE_SCICHART_WASM_BASE?.trim();

	if (custom) {
		return custom.replace(/\/$/, "");
	}

	return "/scichart";
};

const sciChartCdnEnabled = (): boolean =>
	import.meta.env.VITE_SCICHART_WASM_CDN === "true";

/** Load SciChart 2D/3D/polar wasm from `/scichart` or CDN when VITE_SCICHART_WASM_CDN=true. */
export const ensureSciChartWasm = (): Promise<void> => {
	if (!wasmReady) {
		if (sciChartCdnEnabled()) {
			SciChartSurface.loadWasmFromCDN();
			SciChart3DSurface.loadWasmFromCDN();
			SciChartPolarSurface.loadWasmFromCDN();
		} else {
			const base = sciChartWasmBase();

			SciChartSurface.configure({
				wasmUrl: `${base}/scichart2d.wasm`,
				wasmNoSimdUrl: `${base}/scichart2d-nosimd.wasm`,
			});
			SciChart3DSurface.configure({
				wasmUrl: `${base}/scichart3d.wasm`,
				wasmNoSimdUrl: `${base}/scichart3d-nosimd.wasm`,
			});
			SciChartSurface.loadWasmLocal();
			SciChart3DSurface.loadWasmLocal();
			SciChartPolarSurface.loadWasmLocal();
		}

		wasmReady = Promise.resolve();
	}

	return wasmReady;
};
