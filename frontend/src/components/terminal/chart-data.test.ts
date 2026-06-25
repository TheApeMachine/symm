import { describe, expect, it } from "vitest";
import {
	appendPredictionFrame,
	emptyPredictionSeries,
	hawkesWireMetrics,
	terminalFluidFieldStats,
	terminalFluidMatrix,
	terminalManifoldMatrix,
	terminalManifoldReading,
	terminalResonanceFrame,
} from "#/components/terminal/chart-data";

const fluidRow = (symbol: string, changePct: number, vol: number) => ({
	symbol,
	change_pct: changePct,
	vol,
	re: Math.abs(changePct) + 0.1,
	div: changePct / 10,
	vort: vol / 100,
	turb: Math.abs(changePct) / 5,
});

describe("terminal chart data", () => {
	it("keeps prediction points sorted by backend x value", () => {
		const series = emptyPredictionSeries();
		const withSecond = appendPredictionFrame(series, {
			kind: "actual",
			x: 20,
			value: 0.4,
		});
		const withFirst = appendPredictionFrame(withSecond, {
			kind: "actual",
			x: 10,
			value: 0.2,
		});

		expect(withFirst.actual.map((point) => point.x)).toEqual([10, 20]);
		expect(withFirst.actual.map((point) => point.value)).toEqual([0.2, 0.4]);
	});

	it("projects fluid symbol frames into the mockup heat field matrix", () => {
		const matrix = terminalFluidMatrix({
			symbols: [
				fluidRow("ALGO/USD", 0.4, 12),
				fluidRow("DOGE/USD", -1.2, 55),
				fluidRow("SOL/USD", 2.1, 90),
			],
		});

		expect(matrix).toHaveLength(38);
		expect(matrix[0]).toHaveLength(64);
		expect(matrix.flat().some((value) => value > 0)).toBe(true);
	});

	it("derives fluid field overlay stats from backend symbol frames", () => {
		const frame = {
			symbols: [
				fluidRow("ALGO/USD", 0.4, 12),
				fluidRow("DOGE/USD", -1.2, 55),
				fluidRow("SOL/USD", 2.1, 90),
			],
		};
		const stats = terminalFluidFieldStats(frame, terminalFluidMatrix(frame));

		expect(stats.gridText).toBe("64 × 38");
		expect(stats.focusSymbol).toBe("ALGO/USD");
		expect(stats.peakText.startsWith("peak ")).toBe(true);
	});

	it("normalizes manifold rho frames locally for terminal canvas charts", () => {
		const matrix = terminalManifoldMatrix({
			rho: [
				[1, 2],
				[3, 5],
			],
		});

		expect(matrix).toEqual([
			[0, 0.25],
			[0.5, 1],
		]);
	});

	it("normalizes resonance universe frames to the focus x-ray", () => {
		const frame = terminalResonanceFrame({
			type: "resonance_universe",
			ts: "2026-06-23T07:00:00Z",
			arch: [4, 3],
			symbol_count: 1,
			focus_symbol: "SOL/USD",
			symbols: [
				{
					symbol: "SOL/USD",
					surprise: 0.2,
					energy: 0.3,
					confidence: 0.4,
					strength: 0.5,
					category: "laminar_resonance",
					latent: [0.1, 0.2, 0.3],
				},
			],
			focus: {
				symbol: "SOL/USD",
				category: "laminar_resonance",
				surprise: 0.2,
				energy: 0.3,
				confidence: 0.4,
				layers: [
					{ state: [0.1, 0.2], prediction: [0.2, 0.3], error_norm: 0.1 },
				],
			},
		});

		expect(frame?.symbol).toBe("SOL/USD");
		expect(frame?.layers).toHaveLength(1);
	});

	it("parses hawkes and manifold backend wire payloads", () => {
		const hawkes = hawkesWireMetrics({
			output: {
				intensity: 1.2,
				branching: 0.72,
				buyIntensity: 0.8,
				sellIntensity: 0.4,
			},
		});

		expect(hawkes?.intensity).toBe(1.2);
		expect(hawkes?.branching).toBe(0.72);
		expect(hawkes?.alpha).toBeCloseTo(0.72 * 0.4);
		expect(hawkes?.beta).toBe(0.4);
		expect(hawkes?.eta).toBeCloseTo(0.72);

		const reading = terminalManifoldReading({
			reading: {
				divergence: 0.012,
				coherence_mag2: 0.84,
				guidance_speed: 0.31,
				viscosity_proxy: 0.55,
				momentum_share: 0.46,
			},
		});

		expect(reading?.divergence).toBe("+0.012");
		expect(reading?.momentumPct).toBe(46);

		const gated = terminalManifoldReading({
			reading: {
				momentum_share: 0.46,
				mode_share: 0.4,
			},
		});

		expect(gated?.momentumGate).toBe("0.40");
		expect(gated?.momentumFg).toBe("var(--up)");
	});

	it("projects fluid rows without volume using symbol order", () => {
		const matrix = terminalFluidMatrix({
			symbols: [
				{ symbol: "ALGO/USD", change_pct: 0.4, re: 0.5, div: 0.1, turb: 0.2 },
				{ symbol: "DOGE/USD", change_pct: -1.2, re: 0.8, div: 0.2, turb: 0.3 },
			],
		});

		expect(matrix.flat().some((value) => value > 0)).toBe(true);
	});
});
