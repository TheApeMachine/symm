import { describe, expect, it } from "vitest";
import { Frame } from "#/providers/telemetry/telemetry/frame";
import { decodeTelemetryTable } from "#/providers/ws-flatbuffers";

describe("decodeTelemetryTable", () => {
	it("preserves resonance state and prediction vectors", () => {
		const frame = {
			unpack: () => ({
				rows: [
					{
						symbol: "BTC/USD",
						layers: [
							{
								state: [0.25, -0.5],
								prediction: [0.125, -0.25],
							},
						],
					},
				],
			}),
		};

		expect(
			decodeTelemetryTable(Frame.ResonanceFrame, frame).resonance,
		).toEqual([
			{
				symbol: "BTC/USD",
				layers: [
					{
						state: [0.25, -0.5],
						prediction: [0.125, -0.25],
					},
				],
			},
		]);
	});

	it("still expands named state vectors for MCTS frames", () => {
		const frame = {
			unpack: () => ({
				rows: [
					{
						state: [
							{ name: "trend", value: 0.75 },
							{ name: "risk", value: 0.25 },
						],
					},
				],
			}),
		};

		expect(
			decodeTelemetryTable(Frame.BacktestFrame, frame).backtest,
		).toEqual({
			rows: [{ state: { trend: 0.75, risk: 0.25 } }],
		});
	});
	it("retains hindsight counterfactual presence flags", () => {
		const frame = {
			unpack: () => ({
				recommendations: [
					{
						hasCurrent: true,
						hasSuggested: false,
						current: 0.5,
						suggested: 0,
					},
				],
				symbols: [
					{
						opportunities: [
							{
								diagnosis: {
									blockers: [
										{ hasTarget: true, target: 0.5 },
									],
								},
							},
						],
					},
				],
			}),
		};

		const hindsight = decodeTelemetryTable(
			Frame.HindsightFrame,
			frame,
		).hindsight;

		expect(hindsight).toMatchObject({
			recommendations: [
				{ hasCurrent: true, hasSuggested: false },
			],
			symbols: [
				{
					opportunities: [
						{
							diagnosis: {
								blockers: [{ hasTarget: true }],
							},
						},
					],
				},
			],
		});
	});

});
