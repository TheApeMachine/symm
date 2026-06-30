import { describe, expect, it } from "vitest";
import {
	appendKernelSparkHistory,
	auditRowsFromDecisionFrame,
	dashboardDecisionRows,
	decisionRowsFromFrame,
	kernelFrameForSource,
	kernelHealthSummary,
	positionRowsFromFrames,
} from "./rows";

describe("terminal dashboard rows", () => {
	it("projects trader decision frames without fabricating candidates", () => {
		const rows = decisionRowsFromFrame({
			role: "decision",
			symbol: "OP/EUR",
			type: "market",
			verdict: "allow",
			why: "admitted",
			confidence: 0.72,
		});

		expect(rows).toHaveLength(1);
		expect(rows[0]).toMatchObject({
			symbol: "OP/EUR",
			verdict: "allow",
			scoreText: "0.720",
			why: "admitted",
			edgePositive: true,
		});
	});

	it("uses trader score ahead of entry confidence when both are present", () => {
		const rows = decisionRowsFromFrame({
			role: "decision",
			symbol: "OP/EUR",
			type: "market",
			verdict: "blocked",
			why: "field_risk",
			confidence: 1,
			score: 0.31,
		});

		expect(rows[0]).toMatchObject({
			symbol: "OP/EUR",
			scoreText: "0.310",
			scoreValue: 0.31,
			why: "field risk",
		});
	});

	it("honors a blocked verdict stamped on a buy artifact", () => {
		const rows = decisionRowsFromFrame({
			role: "buy",
			symbol: "OP/EUR",
			verdict: "blocked",
			why: "below_edge",
			score: 0.12,
		});

		expect(rows[0]).toMatchObject({
			symbol: "OP/EUR",
			verdict: "blocked",
			why: "below edge",
			scoreValue: 0.12,
		});
	});

	it("uses trader score in audit rows ahead of entry confidence", () => {
		const rows = auditRowsFromDecisionFrame({
			seq: 1,
			decisions: [
				{
					symbol: "OP/EUR",
					verdict: "allow",
					confidence: 1,
					score: 0.31,
				},
			],
		});

		expect(rows[0]?.reason).toBe("candidate scored 0.310");
	});

	it("does not fabricate dashboard decision rows from walk traces", () => {
		const model = dashboardDecisionRows(
			{
				fluid: {
					"BTC/EUR": {
						output: {
							category: "laminar",
							confidence: 0.6,
							surprise: 0.2,
							status: "measured",
						},
					},
				},
			},
			"BTC/EUR",
			{
				"BTC/EUR": {
					symbol: "BTC/EUR",
					steps: [{ path: [0], outcome: "rejected", reason: "below_entry" }],
				},
			},
			null,
		);

		expect(model.rows).toHaveLength(0);
		expect(model.line).toBe(0);
	});

	it("reports kernel health from focused live readings", () => {
		const summary = kernelHealthSummary(
			{
				fluid: {
					"BTC/EUR": {
						output: {
							category: "laminar",
							confidence: 0.72,
							surprise: 0.4,
							status: "measured",
						},
					},
				},
				causal: {
					"BTC/EUR": {
						output: {
							category: "alpha",
							confidence: 0.58,
							surprise: 0.3,
							status: "calibrating",
						},
					},
				},
			},
			"BTC/EUR",
			["fluid", "causal", "resonance"],
		);

		expect(summary).toEqual({ measured: 1, total: 3, label: "1/3 measured" });
	});

	it("falls back from stream focus to live symbol readings", () => {
		const frame = kernelFrameForSource(
			{
				hawkes: {
					"SRM/USD": {
						output: { category: "frenzy", confidence: 0.64 },
					},
				},
			},
			"hawkes",
			"stream",
		);

		expect(frame).toMatchObject({
			output: { category: "frenzy", confidence: 0.64 },
		});
	});

	it("keeps a bounded kernel spark history per focused scope", () => {
		const empty = { scope: "", stamp: "", values: [] };
		const first = appendKernelSparkHistory(empty, "fluid:BTC/EUR", "1", 0.1, 2);
		const second = appendKernelSparkHistory(
			first,
			"fluid:BTC/EUR",
			"2",
			0.2,
			2,
		);
		const duplicate = appendKernelSparkHistory(
			second,
			"fluid:BTC/EUR",
			"2",
			0.9,
			2,
		);
		const third = appendKernelSparkHistory(
			duplicate,
			"fluid:BTC/EUR",
			"3",
			0.3,
			2,
		);
		const reset = appendKernelSparkHistory(third, "fluid:ETH/EUR", "1", 0.4, 2);

		expect(second.values).toEqual([0.1, 0.2]);
		expect(duplicate).toBe(second);
		expect(third.values).toEqual([0.2, 0.3]);
		expect(reset.values).toEqual([0.4]);
	});

	it("summarizes open positions from backend position frames", () => {
		const summary = positionRowsFromFrames({
			quote: "EUR",
			positions: [
				{
					symbol: "SOL/EUR",
					entry: 20,
					mark: 22,
					unrealizedPnl: 4,
					changePct: 10,
					stop: 19,
					peak: 23,
				},
			],
		});

		expect(summary.netText).toBe("net +€4.00");
		expect(summary.rows).toHaveLength(1);
		expect(summary.rows[0]).toMatchObject({
			symbol: "SOL/EUR",
			plText: "P/L +€4.0000",
			pctText: "+10.00%",
		});
	});

	it("formats audit rows with trader sequence and observed time", () => {
		const rows = auditRowsFromDecisionFrame({
			seq: 3789,
			observed_at: new Date(2026, 5, 26, 10, 55, 37).getTime(),
			decisions: [
				{
					symbol: "OP/EUR",
					source: "cognitive",
					verdict: "allow",
					confidence: 0.387,
				},
				{
					symbol: "TON/EUR",
					type: "predict",
					verdict: "blocked",
					why: "below_edge",
					confidence: 0.347,
				},
			],
		});

		expect(rows).toHaveLength(2);
		expect(rows[0]).toMatchObject({
			reason: "candidate scored 0.387",
			meta: "score · OP/EUR · cognitive",
			time: "#3789 · 10:55:37",
		});
		expect(rows[1]).toMatchObject({
			reason: "below edge",
			meta: "reject · TON/EUR · predict",
			time: "#3789 · 10:55:37",
		});
	});
});
