import { describe, expect, it } from "vitest";

import type { EnginePulseEvent } from "#/lib/symm/events";
import {
	confidenceToGaugePercent,
	formatSignalConfidence,
	mergeSignalConfidences,
	peakSignalConfidencesFromPulse,
	type SignalConfidenceSnapshot,
} from "#/lib/symm/signal-confidence";

const emptySnapshot = (): SignalConfidenceSnapshot => ({
	hawkes: 0,
	fluid: 0,
	pumpdump: 0,
	causal: 0,
});

describe("signal-confidence", () => {
	it("should peak confidence per source from the latest engine pulse", () => {
		const pulse = {
			event: "engine_pulse",
			ts: "2026-05-23T12:00:00Z",
			seq: 3,
			phase: "scan",
			measurements: 3,
			candidates: 1,
			open: 0,
			signals: [
				{
					symbol: "PUMP/EUR",
					source: "pumpdump",
					regime: "pump",
					reason: "actual_pump",
					score: 0.726,
					type: "pump",
				},
				{
					symbol: "FLOW/EUR",
					source: "fluid",
					regime: "flow",
					reason: "accumulation",
					score: 0.31,
					type: "flow",
				},
				{
					symbol: "ALT/EUR",
					source: "hawkes",
					regime: "momentum",
					reason: "cluster_buy",
					score: 0.18,
					type: "momentum",
				},
			],
		} satisfies EnginePulseEvent;

		expect(peakSignalConfidencesFromPulse(pulse)).toEqual({
			hawkes: 0.18,
			fluid: 0.31,
			pumpdump: 0.726,
			causal: 0,
		});
	});

	it("should read live source_scores from engine pulse", () => {
		const pulse = {
			event: "engine_pulse",
			ts: "2026-05-23T12:00:00Z",
			seq: 4,
			phase: "scan",
			measurements: 0,
			candidates: 0,
			open: 0,
			signals: [],
			source_scores: {
				hawkes: 0.18,
				fluid: 0.34,
				pumpdump: 0,
				causal: 0.05,
			},
		} satisfies EnginePulseEvent;

		expect(mergeSignalConfidences(emptySnapshot(), pulse)).toEqual({
			hawkes: 0.18,
			fluid: 0.34,
			pumpdump: 0,
			causal: 0.05,
		});
	});

	it("should hold confidence when later pulses omit that source", () => {
		const first = {
			event: "engine_pulse",
			ts: "2026-05-23T12:00:00Z",
			seq: 1,
			phase: "scan",
			measurements: 1,
			candidates: 1,
			open: 0,
			signals: [
				{
					symbol: "PUMP/EUR",
					source: "pumpdump",
					regime: "pump",
					reason: "actual_pump",
					score: 0.726,
					type: "pump",
				},
			],
		} satisfies EnginePulseEvent;

		const empty = {
			event: "engine_pulse",
			ts: "2026-05-23T12:00:01Z",
			seq: 2,
			phase: "scan",
			measurements: 0,
			candidates: 0,
			open: 0,
			signals: [],
		} satisfies EnginePulseEvent;

		const held = mergeSignalConfidences(
			peakSignalConfidencesFromPulse(first),
			empty,
		);

		expect(held).toEqual({
			hawkes: 0,
			fluid: 0,
			pumpdump: 0.726,
			causal: 0,
		});
	});

	it("should map unit confidence directly to gauge percent", () => {
		expect(confidenceToGaugePercent(0)).toBe(0);
		expect(confidenceToGaugePercent(0.726)).toBeCloseTo(72.6);
		expect(confidenceToGaugePercent(1.2)).toBe(100);
	});

	it("should format confidence for compact gauge labels", () => {
		expect(formatSignalConfidence(0)).toBe("0");
		expect(formatSignalConfidence(0.456)).toBe("0.456");
		expect(formatSignalConfidence(12.3)).toBe("12.30");
		expect(formatSignalConfidence(240)).toBe("240");
	});
});
