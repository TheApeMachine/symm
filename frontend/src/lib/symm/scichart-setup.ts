import {
	SciChart3DSurface,
	SciChartDefaults,
	SciChartPolarSurface,
	SciChartSurface,
} from "scichart";

const DEFAULT_WASM_BASE = "/scichart";

let wasmReady: Promise<void> | null = null;
let configured = false;

const sciChartWasmBase = (): string => {
	const custom = import.meta.env.VITE_SCICHART_WASM_BASE?.trim();

	return (custom || DEFAULT_WASM_BASE).replace(/\/$/, "");
};

const configureLocalWasm = () => {
	if (configured) {
		return;
	}

	const base = sciChartWasmBase();

	SciChartSurface.configure({
		wasmUrl: `${base}/scichart2d.wasm`,
		wasmNoSimdUrl: `${base}/scichart2d-nosimd.wasm`,
	});
	SciChart3DSurface.configure({
		wasmUrl: `${base}/scichart3d.wasm`,
		wasmNoSimdUrl: `${base}/scichart3d-nosimd.wasm`,
	});
	configured = true;
};

export const ensureSciChartWasm = async (): Promise<void> => {
	if (typeof window === "undefined") {
		return;
	}

	if (wasmReady) {
		return wasmReady;
	}

	wasmReady = (async () => {
		SciChartSurface.UseCommunityLicense();
		SciChartDefaults.performanceWarnings = false;

		if (import.meta.env.VITE_SCICHART_WASM_CDN === "true") {
			SciChartSurface.loadWasmFromCDN();
			SciChart3DSurface.loadWasmFromCDN();
			SciChartPolarSurface.loadWasmFromCDN();
			return;
		}

		configureLocalWasm();
	})();

	return wasmReady;
};

if (typeof window !== "undefined") {
	void ensureSciChartWasm();
}
