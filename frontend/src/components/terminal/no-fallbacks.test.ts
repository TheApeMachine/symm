import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";
import type { CognitiveReading } from "#/collections/cognitive";
import { activeScopeFor } from "#/routes/cortex";
import {
	dashboardDecisionRows,
	decisionRowsFromFrame,
	kernelFrameForSource,
} from "./rows";
import { resolveScopedFrame } from "./scoped-frame";
import { terminalResonanceLayerMatrixFromFrame } from "./charts";
import { allocationModelFromStores } from "./allocation-side";

const cognitiveReading = (scope: string): CognitiveReading => ({
	scope,
	sequence: "Z7Z4",
	regimePrefix: "trend",
	regimeCohort: 1,
	ambiguous: false,
	sideline: false,
	entropyBits: 1,
	entropyThreshold: 2,
	classConfidence: 0.5,
	contrastEvidence: 0.1,
	lookaheadScore: 0.2,
	lookaheadPaths: 3,
	winnerClass: "hold",
	updatedAt: 1,
});

describe("terminal no-fallback contract", () => {
	it("Concrete symbol focus never uses another symbol's measurement", () => {
		expect(
			kernelFrameForSource(
				{
					fluid: {
						"ETH/USD": {
							scope: "ETH/USD",
							output: { confidence: 0.8 },
						},
					},
				},
				"fluid",
				"BTC/USD",
			),
		).toBeUndefined();
	});

	it("Concrete symbol focus never uses another symbol's resonance frame", () => {
		const source = {
			frame: {
				focus: {
					symbol: "ETH/USD",
					layers: [{ state: [1, 2, 3] }],
				},
			},
		};

		expect(terminalResonanceLayerMatrixFromFrame(source, "BTC/USD")).toEqual(
			[],
		);
	});

	it("Concrete symbol focus never uses another symbol's cognitive frame", () => {
		expect(
			activeScopeFor(
				{ "ETH/USD": cognitiveReading("ETH/USD") },
				null,
				"BTC/USD",
				["ETH/USD"],
			),
		).toBeNull();
	});

	it("Concrete symbol focus never uses another symbol's decision row", () => {
		const model = dashboardDecisionRows(
			{
				fluid: {
					"ETH/USD": {
						scope: "ETH/USD",
						output: { confidence: 0.8 },
					},
				},
			},
			"BTC/USD",
			{
				"ETH/USD": {
					symbol: "ETH/USD",
					steps: [{ path: [0], outcome: "action", reason: "matched" }],
				},
			},
			null,
		);

		expect(model.rows).toHaveLength(0);
	});

	it("Stream mode may preview the first available frame", () => {
		const scoped = resolveScopedFrame(
			{
				"ETH/USD": {
					scope: "ETH/USD",
					output: { confidence: 0.8 },
				},
			},
			"stream",
			"fluid",
		);

		expect(scoped.mode).toBe("stream_preview");
		expect(scoped.frame).toMatchObject({ scope: "ETH/USD" });
	});

	it("Walk traces never create trading decision rows", () => {
		const model = dashboardDecisionRows(
			{},
			"BTC/USD",
			{
				"BTC/USD": {
					symbol: "BTC/USD",
					steps: [{ path: [0], outcome: "action", reason: "matched" }],
				},
			},
			null,
		);

		expect(model.rows).toHaveLength(0);
	});

	it("Raw buy and sell artifacts never imply allow", () => {
		expect(
			decisionRowsFromFrame({ role: "buy", symbol: "BTC/USD", score: 0.9 }),
		).toHaveLength(0);
		expect(
			decisionRowsFromFrame({ role: "sell", symbol: "BTC/USD", score: 0.9 }),
		).toHaveLength(0);
	});

	it("The primary decision table never collapses duplicate symbols", () => {
		const model = dashboardDecisionRows({}, "stream", {}, {
			decisions: [
				{ action_id: "a", symbol: "BTC/USD", verdict: "allow", score: 0.2 },
				{ action_id: "b", symbol: "BTC/USD", verdict: "blocked", score: 0.1 },
			],
		});

		expect(model.rows.map((row) => row.key)).toEqual(["a", "b"]);
	});

	it("The allocation surface never collapses duplicate symbols", () => {
		const alloc = allocationModelFromStores(
			{
				quote: "USD",
				data: [{ asset: "USD", balance: 200, role: "quote" }],
			},
			{
				decisions: [
					{ action_id: "a", symbol: "BTC/USD", verdict: "allow", score: 0.2 },
					{
						action_id: "b",
						symbol: "BTC/USD",
						verdict: "blocked",
						score: 0.1,
					},
				],
			},
		);

		expect(alloc.candidates.map((candidate) => candidate.key)).toEqual([
			"a",
			"b",
		]);
	});

	it("Live decision components do not import frontend decision synthesis", () => {
		const source = readFileSync(new URL("./decision.tsx", import.meta.url), {
			encoding: "utf8",
		});

		for (const forbidden of [
			"combinedScoreFromKernels",
			"decisionRowFromWalk",
			"terminalDecisionsFromWalk",
			"entryLineStats",
			"mergeTerminalDecisionRows",
		]) {
			expect(source).not.toContain(forbidden);
		}
	});

	it("Concrete-focus helpers do not use Object.keys(...)[0] frame selection", () => {
		for (const path of [
			"./scoped-frame.ts",
			"./rows.tsx",
			"./charts.tsx",
			"../../routes/xray.tsx",
			"../../routes/cortex.tsx",
		]) {
			const source = readFileSync(new URL(path, import.meta.url), {
				encoding: "utf8",
			});

			expect(source).not.toMatch(/Object\.keys\([^)]*\)\s*\[\s*0\s*\]/);
		}
	});
});
