import { describe, expect, it } from "vitest";
import {
	terminalPhaseScanFromFrame,
	terminalPhaseStatusFromFrame,
	terminalWaveModesFromFrame,
} from "./charts";

describe("terminal phase dial frame", () => {
	it("preserves complex modes and signed scanner responses", () => {
		const frame = {
			phaseReady: true,
			phaseReason: "",
			wave: [
				{ omega: -1, real: 0.5, imaginary: -0.25, linewidth: 0.1 },
				{ omega: 1, real: -0.5, imaginary: 0.25, linewidth: 0.2 },
			],
			phaseScan: [
				{
					angle: 0,
					similarity: 0.8,
					observedAt: "2026-07-20T12:00:00Z",
					outcome: {
						symbol: "BTC/USD",
						direction: "up",
						return: 0.0125,
						horizon: 8,
					},
				},
				{
					angle: Math.PI,
					similarity: -0.8,
					observedAt: "2026-07-20T12:00:00Z",
					outcome: {
						symbol: "BTC/USD",
						direction: "down",
						return: -0.009,
						horizon: 8,
					},
				},
			],
		};

		expect(terminalWaveModesFromFrame(frame)).toEqual(frame.wave);
		expect(terminalPhaseScanFromFrame(frame)).toEqual([
			{
				angle: 0,
				similarity: 0.8,
				observedAt: "2026-07-20T12:00:00Z",
				outcome: {
					symbol: "BTC/USD",
					direction: "up",
					forwardReturn: 0.0125,
					horizon: 8,
				},
			},
			{
				angle: Math.PI,
				similarity: -0.8,
				observedAt: "2026-07-20T12:00:00Z",
				outcome: {
					symbol: "BTC/USD",
					direction: "down",
					forwardReturn: -0.009,
					horizon: 8,
				},
			},
		]);
		expect(terminalPhaseStatusFromFrame(frame)).toEqual({
			ready: true,
			reason: "",
		});
	});

	it("rejects incomplete modes and scan rows", () => {
		expect(
			terminalWaveModesFromFrame({ wave: [{ real: 1, imaginary: 0 }] }),
		).toEqual([]);
		expect(
			terminalPhaseScanFromFrame({
				phaseScan: [
					{
						angle: 0,
						similarity: 1,
						observedAt: "2026-07-20T12:00:00Z",
					},
				],
			}),
		).toEqual([]);
		expect(
			terminalPhaseStatusFromFrame({
				phaseReady: false,
				phaseReason: "awaiting a prior outcome-labeled phase observation",
			}),
		).toEqual({
			ready: false,
			reason: "awaiting a prior outcome-labeled phase observation",
		});
	});
});
