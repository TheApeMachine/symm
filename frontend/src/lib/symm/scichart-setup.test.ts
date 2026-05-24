import { describe, expect, it, vi } from "vitest";

vi.mock("scichart", () => ({
	SciChartSurface: {
		configure: vi.fn(),
		loadWasmFromCDN: vi.fn(),
		loadWasmLocal: vi.fn(),
	},
	SciChart3DSurface: {
		configure: vi.fn(),
		loadWasmFromCDN: vi.fn(),
		loadWasmLocal: vi.fn(),
	},
	SciChartPolarSurface: {
		loadWasmFromCDN: vi.fn(),
		loadWasmLocal: vi.fn(),
	},
}));

describe("sciChartWasmBase", () => {
	it("defaults to /scichart", async () => {
		const { sciChartWasmBase } = await import("#/lib/symm/scichart-setup");
		expect(sciChartWasmBase()).toBe("/scichart");
	});
});

describe("ensureSciChartWasm", () => {
	it("configures local wasm paths by default", async () => {
		vi.resetModules();
		const scichart = await import("scichart");
		const { ensureSciChartWasm } = await import("#/lib/symm/scichart-setup");

		await ensureSciChartWasm();

		expect(scichart.SciChartSurface.configure).toHaveBeenCalledWith({
			wasmUrl: "/scichart/scichart2d.wasm",
			wasmNoSimdUrl: "/scichart/scichart2d-nosimd.wasm",
		});
		expect(scichart.SciChartSurface.loadWasmLocal).toHaveBeenCalled();
	});
});
