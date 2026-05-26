import "@tanstack/react-start/client-only";

import {
	SciChart3DSurface,
	SciChartDefaults,
	SciChartPolarSurface,
	SciChartSurface,
} from "scichart";

import scichart2dNoSimdWasm from "scichart/_wasm/scichart2d-nosimd.wasm?url";
import scichart2dWasm from "scichart/_wasm/scichart2d.wasm?url";
import scichart3dNoSimdWasm from "scichart/_wasm/scichart3d-nosimd.wasm?url";
import scichart3dWasm from "scichart/_wasm/scichart3d.wasm?url";

let wasmReady: Promise<void> | null = null;

const sciChartCdnEnabled = (): boolean =>
	import.meta.env.VITE_SCICHART_WASM_CDN === "true";

const sciChartWasmBase = (): string | null => {
	const custom = import.meta.env.VITE_SCICHART_WASM_BASE?.trim();

	if (!custom) {
		return null;
	}

	return custom.replace(/\/$/, "");
};

const configureLocalWasm = () => {
	const base = sciChartWasmBase();

	if (base) {
		SciChartSurface.configure({
			wasmUrl: `${base}/scichart2d.wasm`,
			wasmNoSimdUrl: `${base}/scichart2d-nosimd.wasm`,
		});
		SciChart3DSurface.configure({
			wasmUrl: `${base}/scichart3d.wasm`,
			wasmNoSimdUrl: `${base}/scichart3d-nosimd.wasm`,
		});
		return;
	}

	SciChartSurface.configure({
		wasmUrl: scichart2dWasm,
		wasmNoSimdUrl: scichart2dNoSimdWasm,
	});
	SciChart3DSurface.configure({
		wasmUrl: scichart3dWasm,
		wasmNoSimdUrl: scichart3dNoSimdWasm,
	});
};

export const ensureSciChartWasm = (): Promise<void> => {
	if (wasmReady) {
		return wasmReady;
	}

	SciChartSurface.UseCommunityLicense();
	SciChartDefaults.performanceWarnings = false;

	if (sciChartCdnEnabled()) {
		SciChartSurface.loadWasmFromCDN();
		SciChart3DSurface.loadWasmFromCDN();
		SciChartPolarSurface.loadWasmFromCDN();
	} else {
		configureLocalWasm();
	}

	wasmReady = Promise.resolve();
	return wasmReady;
};
