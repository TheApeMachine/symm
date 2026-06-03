import type { ClassValue } from "clsx";
import { clsx } from "clsx";
import { SciChart3DSurface, SciChartDefaults, SciChartSurface } from "scichart";
import { twMerge } from "tailwind-merge";

export const cn = (...inputs: ClassValue[]): string => {
	return twMerge(clsx(inputs));
};

export const formatPnl = (value: number): string => {
	const sign = value >= 0 ? "+" : "";
	return `${sign}€${value.toFixed(4)}`;
};

export const formatEur = (value: number): string => {
	return `€${value.toFixed(2)}`;
};

export const pnlTone = (value: number | undefined): string => {
	if (value === undefined) return "";
	if (value > 0) return "text-(--dash-up)";
	if (value < 0) return "text-(--dash-down)";
	return "text-(--dash-muted)";
};

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
			return;
		}

		configureLocalWasm();
	})();

	return wasmReady;
};
